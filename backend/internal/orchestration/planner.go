package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"openaide/backend/internal/kernel"
)

// SubTask 子任务
type SubTask struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ToolHints   string `json:"tool_hints,omitempty"` // 建议使用的工具
	DependsOn   []int  `json:"depends_on,omitempty"` // 依赖的子任务ID
}

// Plan 任务规划结果
type Plan struct {
	Goal     string    `json:"goal"`
	Subtasks []SubTask `json:"subtasks"`
}

// Planner 任务规划器 — 将复杂任务拆分为子任务
type Planner struct {
	llm kernel.LLMProvider
}

// NewPlanner 创建规划器
func NewPlanner(llm kernel.LLMProvider) *Planner {
	return &Planner{llm: llm}
}

var planningPrompt = `你是一个任务规划专家。将用户的复杂请求拆分为可执行的子任务。

## 规则
1. 每个子任务应该是独立的、可执行的一步操作
2. 子任务数不超过5个
3. 如果请求很简单（单步可完成），返回1个子任务即可
4. 每个子任务描述要具体，包含"做什么"和"怎么做"
5. 标注建议使用的工具：read_file/write_file/execute_command/list_directory/search_files/git_status/search_knowledge/add_knowledge

## 输出格式（严格JSON）
{
  "goal": "一句话概括目标",
  "subtasks": [
    {"id": 1, "title": "子任务标题", "description": "具体做什么", "tool_hints": "建议工具1, 建议工具2"},
    {"id": 2, "title": "...", "description": "...", "depends_on": [1]}
  ]
}

## 用户请求
%s

## 你的规划（JSON）`

// Plan 分析请求并生成任务规划
func (p *Planner) Plan(ctx context.Context, query string) (*Plan, error) {
	if !p.needsPlanning(query) {
		// 简单请求不需要规划
		return &Plan{
			Goal: query,
			Subtasks: []SubTask{{
				ID: 1, Title: query, Description: query,
			}},
		}, nil
	}

	prompt := fmt.Sprintf(planningPrompt, query)
	messages := []kernel.Message{
		{Role: "system", Content: "You are a task planner. Output ONLY valid JSON, no other text."},
		{Role: "user", Content: prompt},
	}

	resp, err := p.llm.Chat(ctx, messages, nil, map[string]interface{}{
		"temperature": 0.3,
		"max_tokens":  1000,
	})
	if err != nil {
		// 规划失败，退化为单步执行
		return &Plan{
			Goal: query,
			Subtasks: []SubTask{{
				ID: 1, Title: query, Description: query,
			}},
		}, nil
	}

	plan, err := p.parsePlan(resp.Content)
	if err != nil {
		return &Plan{
			Goal: query,
			Subtasks: []SubTask{{
				ID: 1, Title: query, Description: query,
			}},
		}, nil
	}

	return plan, nil
}

// needsPlanning 判断是否需要任务规划（简单问题跳过）
func (p *Planner) needsPlanning(query string) bool {
	// 短问题不需要规划
	if len([]rune(query)) < 30 {
		return false
	}

	// 包含多步骤关键词
	multiStep := []string{
		"然后", "再", "并且", "同时", "分别",
		"先", "接着", "最后", "之后", "全部",
		"所有", "每个", "逐个", "依次",
		"first", "then", "and", "also", "all",
		"each", "every", "both",
	}
	for _, kw := range multiStep {
		if strings.Contains(strings.ToLower(query), kw) {
			return true
		}
	}

	return len([]rune(query)) > 100 // 超长问题也需要规划
}

func (p *Planner) parsePlan(content string) (*Plan, error) {
	// 提取 JSON 部分
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var plan Plan
	if err := json.Unmarshal([]byte(content[start:end+1]), &plan); err != nil {
		return nil, err
	}

	if len(plan.Subtasks) == 0 {
		return nil, fmt.Errorf("empty plan")
	}

	return &plan, nil
}

// ExecutePlan 执行规划 — 逐个执行子任务，结果汇总
func (o *Orchestrator) ExecutePlan(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (*kernel.Response, error) {
	planner := NewPlanner(o.llmGateway)

	plan, err := planner.Plan(ctx, content)
	if err != nil {
		// 无法规划，退化为直接执行
		return o.ProcessQuery(ctx, userID, projectID, content, opts)
	}

	// 简单任务不需要分步
	if len(plan.Subtasks) <= 1 {
		return o.ProcessQuery(ctx, userID, projectID, content, opts)
	}

	// 执行规划
	var results []string
	for i, st := range plan.Subtasks {
		subQuery := fmt.Sprintf("## 任务目标: %s\n## 当前步骤 (%d/%d): %s\n## 步骤描述: %s\n\n请完成此步骤。",
			plan.Goal, i+1, len(plan.Subtasks), st.Title, st.Description)

		if i > 0 {
			// 注入前面步骤的结果
			subQuery += fmt.Sprintf("\n\n## 前面步骤的结果:\n%s", strings.Join(results, "\n"))
		}

		resp, err := o.ProcessQuery(ctx, userID, projectID, subQuery, opts)
		if err != nil {
			results = append(results, fmt.Sprintf("步骤%d失败: %v", st.ID, err))
			continue
		}
		results = append(results, fmt.Sprintf("[步骤%d: %s]\n%s", st.ID, st.Title, resp.Content))
	}

	// 汇总：用 LLM 总结所有子任务的结果
	summary := fmt.Sprintf("任务完成。共%d个子任务：\n\n%s", len(plan.Subtasks), strings.Join(results, "\n\n"))
	return &kernel.Response{
		Content:    summary,
		ToolCalls:  len(plan.Subtasks),
		TokensUsed: 0,
		Duration:   0,
	}, nil
}
