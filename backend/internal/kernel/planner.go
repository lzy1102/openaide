package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// SubTask represents a single step in a decomposed task plan.
type SubTask struct {
	Goal           string `json:"goal"`            // 本步要达成什么(一句话)
	Approach       string `json:"approach"`        // 如何达成(一句话,可提工具)
	ExpectedResult string `json:"expected_result"` // 成功长什么样(一句话,可验证)
}

// TaskPlan 是复杂查询的分解计划。
// 在 ReAct 循环开始前生成一次,作为系统消息注入,
// 引导 agent 按步骤执行并在每步后自我验证。
type TaskPlan struct {
	Query    string    `json:"query"`
	SubTasks []SubTask `json:"sub_tasks"`
}

// Planner 用 LLM 把复杂查询分解为子任务。
// 简单查询不分解(返回 nil),零开销。
type Planner struct {
	llm LLMProvider
}

// NewPlanner 创建一个基于 LLM 的规划器。
func NewPlanner(llm LLMProvider) *Planner {
	return &Planner{llm: llm}
}

// planComplexityThreshold 触发规划的复杂度阈值。
// analyzeQuery 返回的 Complexity >= 此值时才调用 Planner,
// 避免对简单查询浪费一次 LLM 调用。
const planComplexityThreshold = 15

const planPrompt = `You are a task planner for an AI coding agent. Decompose this query into 2-5 concrete subtasks.

## Query
%s

## Output Format
Return a JSON object with a "sub_tasks" array. Each subtask has:
- goal: what this step achieves (one sentence)
- approach: how to achieve it (one sentence, mention specific tools if relevant)
- expected_result: what success looks like (one sentence, must be verifiable)

## Rules
- Only decompose if the task genuinely needs multiple sequential steps
- For simple single-step tasks, return an empty sub_tasks array
- Each subtask should be independently verifiable (can you tell if it succeeded?)
- Order subtasks by dependency (earlier steps enable later ones)
- Keep descriptions concise — the agent will see the full plan

Reply with ONLY the JSON object, no other text.`

// Plan 为给定查询生成任务计划。
// 失败或查询太简单时返回 nil(调用方应优雅降级,继续无计划执行)。
// 只对复杂查询调用(complexity >= planComplexityThreshold)。
func (p *Planner) Plan(ctx context.Context, query string) *TaskPlan {
	if p == nil || p.llm == nil {
		return nil
	}
	start := time.Now()

	planCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(planPrompt, truncStr(query, 800))
	resp, err := p.llm.Chat(planCtx, []Message{
		{Role: "user", Content: prompt},
	}, nil, map[string]interface{}{
		"max_tokens":  600,
		"temperature": 0.2,
		"route":       "execution",
		"no_thinking": true,
	})
	if err != nil {
		slog.Debug("Planner LLM failed, skipping plan", "error", err)
		return nil
	}

	body := strings.TrimSpace(resp.Content)
	body = strings.TrimPrefix(body, "```json")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)

	var plan TaskPlan
	if err := json.Unmarshal([]byte(body), &plan); err != nil {
		slog.Debug("Planner parse failed, skipping plan", "error", err, "body", truncStr(body, 80))
		return nil
	}
	if len(plan.SubTasks) == 0 {
		return nil
	}
	// 防御:LLM 可能返回过多子任务,截断到 5 个
	if len(plan.SubTasks) > 5 {
		plan.SubTasks = plan.SubTasks[:5]
	}

	plan.Query = query
	slog.Info("Task plan generated",
		"subtasks", len(plan.SubTasks),
		"duration", time.Since(start))
	return &plan
}

// ProgressMessage 生成当前执行进度的回写消息。
// 每轮注入一次,让 agent 明确已完成的步骤与下一步,
// 避免在长工具序列中丢失计划方向。返回空串表示无需更新。
func (p *TaskPlan) ProgressMessage(doneStep, totalSteps int) Message {
	if p == nil || len(p.SubTasks) == 0 || totalSteps <= 0 {
		return Message{}
	}
	if doneStep >= totalSteps {
		return Message{Role: "system", Content: "[Plan Progress] All steps completed. Provide the final summary."}
	}
	var sb strings.Builder
	sb.WriteString("[Plan Progress]\n")
	sb.WriteString("Completed " + fmt.Sprintf("%d/%d", doneStep, totalSteps) + " steps.\n")
	// 已完成步骤不做逐条展示,只提示下一步,减少上下文噪音
	if next := p.SubTasks[doneStep]; doneStep < totalSteps {
		sb.WriteString("Next step: ")
		sb.WriteString(fmt.Sprintf("Step %d — %s\n  Approach: %s\n  Expected: %s\n",
			doneStep+1, next.Goal, next.Approach, next.ExpectedResult))
	}
	return Message{Role: "system", Content: sb.String()}
}

// ToSystemMessage 把计划转为系统消息注入 ReAct 循环。
// 消息放在 user query 之后,作为执行指引。
func (p *TaskPlan) ToSystemMessage() Message {
	if p == nil || len(p.SubTasks) == 0 {
		return Message{}
	}
	var sb strings.Builder
	sb.WriteString("[Task Plan]\n")
	sb.WriteString("Follow this plan step by step. After each step, verify the result matches the expected outcome before proceeding to the next.\n\n")
	for i, st := range p.SubTasks {
		sb.WriteString(fmt.Sprintf("Step %d: %s\n", i+1, st.Goal))
		sb.WriteString(fmt.Sprintf("  Approach: %s\n", st.Approach))
		sb.WriteString(fmt.Sprintf("  Expected: %s\n", st.ExpectedResult))
		if i < len(p.SubTasks)-1 {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n\nIf a step fails twice, reflect on why and try a different approach before retrying.")
	return Message{Role: "system", Content: sb.String()}
}

// shouldPlan 判断是否需要为本次查询生成计划。
// 基于 analyzeQuery 的复杂度评估。
func shouldPlan(analysis *QueryAnalysis) bool {
	return analysis != nil && analysis.Complexity >= planComplexityThreshold
}
