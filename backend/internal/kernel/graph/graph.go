package graph

import (
	"context"
	"fmt"

	"openaide/backend/internal/kernel"
)

// Node 图中的节点 — 代表一个 ReAct Agent 执行步骤
type Node struct {
	Name         string
	SystemPrompt string
	Tools        []string
	Handler      NodeHandler // optional: 自定义处理逻辑，nil 则使用默认 ReAct
}

// NodeResult 节点执行结果
type NodeResult struct {
	Name      string
	Content   string
	ToolCalls int
	Error     error
}

// NodeHandler 节点处理函数签名
type NodeHandler func(ctx context.Context, input string, results map[string]*NodeResult) (string, error)

// Edge 图中的有向边
type Edge struct {
	From string
	To   string
	Cond EdgeCondition // 条件，nil 表示无条件执行
	Meta string        // optional: 边描述
}

// EdgeCondition 边条件 — 根据已有结果判断是否走这条边
type EdgeCondition func(results map[string]*NodeResult) bool

// Graph 有向图（DAG）
type Graph struct {
	Nodes map[string]*Node
	Edges []Edge
}

// NewGraph 创建空图
func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
		Edges: make([]Edge, 0),
	}
}

// AddNode 添加节点
func (g *Graph) AddNode(node *Node) {
	g.Nodes[node.Name] = node
}

// AddEdge 添加边
func (g *Graph) AddEdge(edge Edge) {
	g.Edges = append(g.Edges, edge)
}

// TopoSort 拓扑排序 — 返回节点的执行顺序
func (g *Graph) TopoSort() ([]string, error) {
	inDegree := make(map[string]int, len(g.Nodes))
	for name := range g.Nodes {
		inDegree[name] = 0
	}
	// 计算入度
	adj := make(map[string][]string, len(g.Nodes))
	for _, e := range g.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		inDegree[e.To]++
	}

	// Kahn 算法
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		order = append(order, u)
		for _, v := range adj[u] {
			inDegree[v]--
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	if len(order) != len(g.Nodes) {
		return nil, fmt.Errorf("graph has cycle or unreachable nodes: order=%d, nodes=%d", len(order), len(g.Nodes))
	}
	return order, nil
}

// Dependencies 返回指定节点的直接前置依赖节点
func (g *Graph) Dependencies(name string) []string {
	var deps []string
	for _, e := range g.Edges {
		if e.To == name {
			deps = append(deps, e.From)
		}
	}
	return deps
}

// Executor 图执行器
type Executor struct {
	kernel *kernel.AgentKernel
}

// NewExecutor 创建图执行器
func NewExecutor(k *kernel.AgentKernel) *Executor {
	return &Executor{kernel: k}
}

// ExecResult 图执行结果
type ExecResult struct {
	NodeResults map[string]*NodeResult
	FinalOutput string
}

// Execute 执行整个图（按拓扑排序顺序）
func (ex *Executor) Execute(ctx context.Context, graph *Graph, query string) (*ExecResult, error) {
	order, err := graph.TopoSort()
	if err != nil {
		return nil, fmt.Errorf("topo sort: %w", err)
	}

	results := make(map[string]*NodeResult)

	for _, name := range order {
		node := graph.Nodes[name]
		if node.Handler != nil {
			// 自定义处理器
			content, handlerErr := node.Handler(ctx, query, results)
			results[name] = &NodeResult{
				Name:    name,
				Content: content,
				Error:   handlerErr,
			}
			continue
		}

		// 默认 ReAct 处理器
		result := ex.executeNode(ctx, node, query, results, graph)
		results[name] = result
		if result.Error != nil {
			// 检查是否有条件边跳转到异常处理
			routed := false
			for _, e := range graph.Edges {
				if e.From == name && e.Cond != nil && e.Cond(results) {
					routed = true
					// 条件边是隐式的 — 后续的拓扑序会覆盖
					break
				}
			}
			if !routed {
				// 没有异常路由 → 继续
			}
		}
	}

	// 找到终节点（没有出边）
	finalName := ""
	for name := range graph.Nodes {
		hasOut := false
		for _, e := range graph.Edges {
			if e.From == name {
				hasOut = true
				break
			}
		}
		if !hasOut {
			finalName = name
			break
		}
	}

	finalOutput := ""
	if finalName != "" && results[finalName] != nil {
		finalOutput = results[finalName].Content
	}

	return &ExecResult{
		NodeResults: results,
		FinalOutput: finalOutput,
	}, nil
}

func (ex *Executor) executeNode(ctx context.Context, node *Node, query string, results map[string]*NodeResult, graph *Graph) *NodeResult {
	// 收集依赖结果作为上下文
	var depContents []string
	for _, dep := range graph.Dependencies(node.Name) {
		if r, ok := results[dep]; ok && r != nil {
			depContents = append(depContents, fmt.Sprintf("=== %s ===\n%s", dep, r.Content))
		}
	}

	input := query
	if len(depContents) > 0 {
		prefix := ""
		for _, c := range depContents {
			prefix += c + "\n\n"
		}
		input = prefix + "当前用户请求: " + query
	}

	// 构建角色 prompt
	systemPrompt := node.SystemPrompt

	opts := kernel.QueryOptions{}
	if len(node.Tools) > 0 {
		opts.ToolFilter = node.Tools
	}

	// 通过 kernel 执行
	q := &kernel.Query{
		Content:   input,
		Options:   opts,
		SessionID: "",
	}

	// 如果没有 session（临时执行），注入 system prompt 到内容前
	if systemPrompt != "" {
		q.Content = systemPrompt + "\n\n" + input
	}

	resp, err := ex.kernel.Process(ctx, q)
	if err != nil {
		return &NodeResult{Name: node.Name, Error: err}
	}

	return &NodeResult{
		Name:      node.Name,
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	}
}
