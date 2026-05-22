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

var planningPrompt = `你是一个任务规划专家。分析用户请求，自行判断是否需要拆分为子任务。

## 何时拆分
- 请求涉及多个独立步骤 → 拆分
- 请求需要先分析再行动 → 拆分
- 请求包含多个文件/模块 → 拆分
- 请求是单一明确操作 → **不拆分**，返回1个子任务
- 请求是简单问答 → **不拆分**，返回1个子任务

## 规则
1. 每个子任务应该是独立可执行的一步
2. 子任务数不超过5个
3. 简单请求返回1个子任务（title和description都用原始请求）
4. 标注建议工具：read_file/write_file/execute_command/list_directory/search_files/git_status/search_knowledge/add_knowledge

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

var planTool = kernel.ToolDefinition{
	Type: "function",
	Function: kernel.FunctionDef{
		Name:        "create_plan",
		Description: "Create a structured plan with subtasks for a complex user request",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"goal": map[string]interface{}{
					"type":        "string",
					"description": "One-sentence summary of the overall goal",
				},
				"subtasks": map[string]interface{}{
					"type":        "array",
					"description": "List of executable subtasks",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id":          map[string]interface{}{"type": "integer", "description": "Subtask ID (1-based)"},
							"title":       map[string]interface{}{"type": "string", "description": "Short title"},
							"description": map[string]interface{}{"type": "string", "description": "What to do and how"},
							"tool_hints":  map[string]interface{}{"type": "string", "description": "Suggested tools, comma separated"},
							"depends_on": map[string]interface{}{
								"type":        "array",
								"items":       map[string]interface{}{"type": "integer"},
								"description": "IDs of subtasks this depends on",
							},
						},
						"required": []string{"id", "title", "description"},
					},
				},
			},
			"required": []string{"goal", "subtasks"},
		},
	},
}

// Plan 通过 LLM 分析请求并生成任务规划。
// LLM 自主判断是否需要拆分：简单请求返回 1 个子任务，复杂请求返回多个。
func (p *Planner) Plan(ctx context.Context, query string) (*Plan, error) {
	defaultPlan := &Plan{
		Goal: query,
		Subtasks: []SubTask{{
			ID: 1, Title: query, Description: query,
		}},
	}

	// 1. 尝试 function calling 规划
	plan, err := p.planWithFunctionCall(ctx, query)
	if err == nil && plan != nil {
		return plan, nil
	}

	// 2. 回退到文本 JSON 解析
	plan, err = p.planWithTextPrompt(ctx, query)
	if err == nil && plan != nil {
		return plan, nil
	}

	return defaultPlan, nil
}

// planWithFunctionCall 使用 function calling 获取结构化规划
func (p *Planner) planWithFunctionCall(ctx context.Context, query string) (*Plan, error) {
	messages := []kernel.Message{
		{Role: "system", Content: "You are a task planner. Analyze the user's request. For simple single-step requests, return exactly 1 subtask. Only split into multiple subtasks when the request genuinely requires multiple steps."},
		{Role: "user", Content: query},
	}

	resp, err := p.llm.Chat(ctx, messages, []kernel.ToolDefinition{planTool}, map[string]interface{}{
		"temperature": 0.3,
		"max_tokens":  500,
	})
	if err != nil {
		return nil, fmt.Errorf("function calling planning failed: %w", err)
	}

	if len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("no tool calls in response")
	}

	return parsePlanFromFC(resp.ToolCalls[0].Function.Arguments)
}

// planWithTextPrompt 回退方案：通过文本提示 + JSON 提取
func (p *Planner) planWithTextPrompt(ctx context.Context, query string) (*Plan, error) {
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
		return nil, err
	}

	return parsePlan(resp.Content)
}


func parsePlanFromFC(arguments string) (*Plan, error) {
	var plan Plan
	if err := json.Unmarshal([]byte(arguments), &plan); err != nil {
		return nil, fmt.Errorf("unmarshal function call plan: %w", err)
	}
	if len(plan.Subtasks) == 0 {
		return nil, fmt.Errorf("empty plan from function call")
	}
	return &plan, nil
}

func parsePlan(content string) (*Plan, error) {
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
