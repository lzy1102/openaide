package graph

import (
	"context"
	"fmt"
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
