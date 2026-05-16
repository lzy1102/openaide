package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"openaide/backend/internal/kernel"
)

// DAGNode 工作流节点
type DAGNode struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Prompt      string   `json:"prompt"`
	DependsOn   []string `json:"depends_on"`
	Tools       []string `json:"tools"`
	Status      string   `json:"status"` // pending, running, done, failed
	Result      string   `json:"result"`
}

// DAG 有向无环图工作流
type DAG struct {
	Nodes []*DAGNode `json:"nodes"`
}

// WorkflowEngine DAG执行引擎
type WorkflowEngine struct {
	orchestrator *Orchestrator
}

// NewWorkflowEngine 创建工作流引擎
func NewWorkflowEngine(o *Orchestrator) *WorkflowEngine {
	return &WorkflowEngine{orchestrator: o}
}

// Parse 从JSON解析DAG
func (e *WorkflowEngine) Parse(data string) (*DAG, error) {
	var dag DAG
	if err := json.Unmarshal([]byte(data), &dag); err != nil {
		return nil, err
	}
	return &dag, nil
}

// PlanDAG 从用户请求自动生成DAG
func (e *WorkflowEngine) PlanDAG(ctx context.Context, query string) (*DAG, error) {
	prompt := fmt.Sprintf(`将以下任务拆分为DAG工作流节点，输出JSON:
{
  "nodes": [
    {"id":"1","title":"步骤1","prompt":"具体做什么","depends_on":[],"tools":["read_file"]},
    {"id":"2","title":"步骤2","prompt":"...","depends_on":["1"],"tools":["execute_command"]}
  ]
}
规则: 最多5个节点，每个节点有明确的id/title/prompt/tools。depends_on是依赖的节点id列表。
任务: %s
只输出JSON。`, query)

	resp, err := e.orchestrator.ProcessQuery(ctx, "dag-planner", "default", prompt, kernel.QueryOptions{MaxTokens: 1000})
	if err != nil {
		return nil, err
	}
	return e.Parse(extractJSON(resp.Content))
}

// Execute 执行DAG — 并行执行无依赖的节点
func (e *WorkflowEngine) Execute(ctx context.Context, userID, projectID string, dag *DAG, opts kernel.QueryOptions) (*kernel.Response, error) {
	if dag == nil || len(dag.Nodes) == 0 {
		return nil, fmt.Errorf("empty DAG")
	}

	completed := make(map[string]string) // nodeID → result
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, len(dag.Nodes))

	for len(completed) < len(dag.Nodes) {
		ready := e.readyNodes(dag.Nodes, completed)

		for _, node := range ready {
			node.Status = "running"
			wg.Add(1)
			go func(n *DAGNode) {
				defer wg.Done()
				// 构建包含依赖结果的上下文
				contextParts := []string{n.Prompt}
				for _, depID := range n.DependsOn {
					if result, ok := completed[depID]; ok {
						contextParts = append(contextParts, fmt.Sprintf("[上一步结果] %s", result))
					}
				}

				rOpts := opts
				if len(n.Tools) > 0 {
					rOpts.ToolFilter = n.Tools
				}

				resp, err := e.orchestrator.ProcessQuery(ctx, userID, projectID, strings.Join(contextParts, "\n"), rOpts)
				mu.Lock()
				if err != nil {
					n.Status = "failed"
					n.Result = err.Error()
					errCh <- err
				} else {
					n.Status = "done"
					n.Result = resp.Content
					completed[n.ID] = resp.Content
				}
				mu.Unlock()
			}(node)
		}
		wg.Wait()

		if len(ready) == 0 && len(completed) < len(dag.Nodes) {
			// 死锁
			break
		}
	}

	// 汇总
	var parts []string
	for _, node := range dag.Nodes {
		status := "✅"
		if node.Status == "failed" {
			status = "❌"
		}
		parts = append(parts, fmt.Sprintf("### %s %s\n%s", status, node.Title, node.Result))
	}

	return &kernel.Response{
		Content: strings.Join(parts, "\n\n"),
	}, nil
}

func (e *WorkflowEngine) readyNodes(nodes []*DAGNode, completed map[string]string) []*DAGNode {
	var ready []*DAGNode
	for _, node := range nodes {
		if node.Status != "pending" {
			continue
		}
		allDone := true
		for _, depID := range node.DependsOn {
			if _, ok := completed[depID]; !ok {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, node)
		}
	}
	return ready
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end <= start {
		return "{}"
	}
	return s[start : end+1]
}
