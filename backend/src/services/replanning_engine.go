package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"openaide/backend/src/services/llm"
)

// ReplanningEngine 动态计划调整引擎
// 核心功能：
// 1. 基于执行结果调整计划
// 2. 局部调整（微调执行顺序、替换工具）
// 3. 全局重新规划（原计划完全不可行）
// 4. 降级方案（简化目标，保底完成）
type ReplanningEngine struct {
	llmClient       llm.LLMClient
	model           string
	structuredPlanner *StructuredPlanner
}

// ReplanRequest 重新规划请求
type ReplanRequest struct {
	OriginalPlan  *StructuredPlan       `json:"original_plan"`
	Checkpoints   []ExecutionCheckpoint `json:"checkpoints"`
	Issues        []PlanIssue           `json:"issues"`
	UserMessage   string                `json:"user_message"`
	CurrentSubtask string               `json:"current_subtask"`
	ReplanLevel   string                `json:"replan_level"` // adjust, replan, fallback
}

// ReplanResult 重新规划结果
type ReplanResult struct {
	Level         string `json:"level"`           // adjust, replan, fallback
	Plan          *StructuredPlan `json:"plan,omitempty"`
	AdjustedPlan  *StructuredPlan `json:"adjusted_plan,omitempty"`
	Summary       string `json:"summary"`
	Changes       []PlanChange `json:"changes"`
	UserNotify    bool   `json:"user_notify"`
	UserMessage   string `json:"user_message,omitempty"`
}

// PlanChange 计划变更
type PlanChange struct {
	Type        string `json:"type"`         // reorder, replace, add, remove, simplify
	Target      string `json:"target"`       // 子任务ID或阶段名
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

// NewReplanningEngine 创建动态计划调整引擎
func NewReplanningEngine(llmClient llm.LLMClient, model string, structuredPlanner *StructuredPlanner) *ReplanningEngine {
	if model == "" {
		model = "gpt-4"
	}
	return &ReplanningEngine{
		llmClient:       llmClient,
		model:           model,
		structuredPlanner: structuredPlanner,
	}
}

// Replan 根据执行情况调整计划
func (re *ReplanningEngine) Replan(ctx context.Context, req *ReplanRequest) (*ReplanResult, error) {
	log.Printf("[ReplanningEngine] Starting replan: level=%s, issues=%d", req.ReplanLevel, len(req.Issues))

	switch req.ReplanLevel {
	case "adjust":
		return re.partialAdjust(ctx, req)
	case "replan":
		return re.fullReplan(ctx, req)
	case "fallback":
		return re.fallbackPlan(ctx, req)
	default:
		return re.partialAdjust(ctx, req)
	}
}

// partialAdjust 局部调整：调整顺序、替换工具、简化步骤
func (re *ReplanningEngine) partialAdjust(ctx context.Context, req *ReplanRequest) (*ReplanResult, error) {
	// 克隆原计划
	adjusted := re.clonePlan(req.OriginalPlan)

	var changes []PlanChange

	// 1. 标记失败的子任务为需要跳过或重试
	for _, issue := range req.Issues {
		if issue.Type == "blocker" || issue.Type == "dependency" {
			// 尝试找到替代方案
			change, err := re.findAlternative(ctx, adjusted, issue)
			if err == nil && change != nil {
				changes = append(changes, *change)
			}
		}
	}

	// 2. 使用 LLM 优化剩余计划
	if len(changes) > 0 {
		optimized, newChanges, err := re.optimizeWithLLM(ctx, adjusted, req)
		if err == nil {
			adjusted = optimized
			changes = append(changes, newChanges...)
		}
	}

	if len(changes) == 0 {
		changes = append(changes, PlanChange{
			Type:        "info",
			Target:      "overall",
			Description: "局部调整：继续执行原计划，跳过已失败的子任务",
			Reason:      "失败子任务不影响整体目标",
		})
	}

	return &ReplanResult{
		Level:        "adjust",
		AdjustedPlan: adjusted,
		Summary:      fmt.Sprintf("计划已局部调整，%d 处变更", len(changes)),
		Changes:      changes,
		UserNotify:   false,
	}, nil
}

// fullReplan 全局重新规划
func (re *ReplanningEngine) fullReplan(ctx context.Context, req *ReplanRequest) (*ReplanResult, error) {
	log.Printf("[ReplanningEngine] Full replanning needed")

	// 获取已完成的工作
	completedTasks := re.getCompletedTasks(req.Checkpoints)

	// 构建新的用户消息（包含已完成的工作）
	newMessage := req.UserMessage
	if len(completedTasks) > 0 {
		var completedStr strings.Builder
		completedStr.WriteString("以下是已经完成的工作：\n")
		for _, ct := range completedTasks {
			completedStr.WriteString(fmt.Sprintf("- %s: %s\n", ct.ID, truncatePlanStr(ct.Output, 200)))
		}
		newMessage = req.UserMessage + "\n\n" + completedStr.String() + "\n\n请基于以上已完成的工作，重新规划剩余任务。"
	}

	// 调用结构化规划引擎重新规划
	newPlan, err := re.structuredPlanner.Plan(ctx, newMessage, "", nil)
	if err != nil {
		// 重新规划失败，使用降级方案
		return re.fallbackPlan(ctx, req)
	}

	return &ReplanResult{
		Level:      "replan",
		Plan:       newPlan,
		Summary:    "计划已重新生成",
		Changes:    []PlanChange{{Type: "replan", Target: "overall", Description: "原计划不可行，已重新规划", Reason: "多个关键子任务失败"}},
		UserNotify: true,
		UserMessage: "原执行计划遇到问题，系统已为您重新制定了新的计划。",
	}, nil
}

// fallbackPlan 降级方案：简化目标，保底完成
func (re *ReplanningEngine) fallbackPlan(ctx context.Context, req *ReplanRequest) (*ReplanResult, error) {
	log.Printf("[ReplanningEngine] Creating fallback/degraded plan")

	// 获取已完成的工作
	completedTasks := re.getCompletedTasks(req.Checkpoints)

	// 创建最简化的计划
	var completedSummary strings.Builder
	for _, ct := range completedTasks {
		completedSummary.WriteString(fmt.Sprintf("- %s: %s\n", ct.ID, truncatePlanStr(ct.Output, 100)))
	}

	prompt := fmt.Sprintf(`原任务执行过程中遇到了严重问题，无法完成原计划。
请创建一个简化的降级方案，只保留最核心的部分。

## 原始任务
%s

## 已完成的工作
%s

## 问题
请忽略上述失败的部分，基于已完成的工作，创建一个最简化的完成方案。

请以 JSON 格式输出简化的计划：
{
  "phases": [
    {
      "name": "阶段名称",
      "description": "阶段描述",
      "order": 1,
      "subtasks": [
        {
          "id": "子任务ID",
          "title": "标题",
          "description": "描述",
          "type": "research",
          "skills": [],
          "order": 1,
          "estimated": 10,
          "required": true,
          "can_parallel": false
        }
      ]
    }
  ],
  "dependencies": [],
  "tool_plan": [],
  "risk_assessment": {"overall_risk": "low", "top_risks": [], "mitigations": []},
  "fallback_plans": [],
  "estimated_time": 总时间,
  "plan_summary": "一段话描述降级方案"
}`, req.UserMessage, completedSummary.String())

	req2 := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "创建一个最简化的降级方案。只保留核心目标。只返回 JSON。"},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       re.model,
		Temperature: 0.2,
		MaxTokens:   3000,
	}

	resp, err := re.llmClient.Chat(ctx, req2)
	if err != nil {
		// 最后的兜底：返回空计划
		return &ReplanResult{
			Level:      "fallback",
			Plan:       nil,
			Summary:    "无法生成降级方案，请用户手动处理",
			Changes:    []PlanChange{{Type: "error", Target: "overall", Description: "所有自动规划方案均失败", Reason: "任务过于复杂或环境限制"}},
			UserNotify: true,
			UserMessage: "抱歉，系统遇到了无法自动解决的问题。请检查任务复杂度或环境配置后重试。",
		}, nil
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = extractJSON(content)

	var plan StructuredPlan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return &ReplanResult{
			Level:      "fallback",
			Summary:    "降级方案生成失败",
			Changes:    []PlanChange{{Type: "error", Target: "overall", Description: "降级方案解析失败", Reason: "LLM 输出格式错误"}},
			UserNotify: true,
			UserMessage: "系统无法自动解决当前问题，请手动干预。",
		}, nil
	}

	return &ReplanResult{
		Level:      "fallback",
		Plan:       &plan,
		Summary:    "已创建降级方案（简化版）",
		Changes:    []PlanChange{{Type: "simplify", Target: "overall", Description: "原计划无法执行，已创建简化方案", Reason: "关键路径受阻"}},
		UserNotify: true,
		UserMessage: "原执行计划遇到严重问题，系统已为您创建了简化的降级方案。",
	}, nil
}

// findAlternative 为失败的子任务寻找替代方案
func (re *ReplanningEngine) findAlternative(ctx context.Context, plan *StructuredPlan, issue PlanIssue) (*PlanChange, error) {
	prompt := fmt.Sprintf(`子任务 '%s' 执行失败：%s
请提供一个替代方案。

可选策略：
1. 用不同的工具/方法完成相同的目标
2. 简化目标，用更简单的方式达到类似效果
3. 调整执行顺序，绕过这个障碍

请以 JSON 格式输出：
{"type": "replace|simplify|reorder", "target": "子任务ID", "description": "替代方案描述", "reason": "原因"}`,
		issue.SubtaskID, issue.Description)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "为失败的子任务找到替代方案。只返回 JSON。"},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       re.model,
		Temperature: 0.2,
		MaxTokens:   500,
	}

	resp, err := re.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	content := extractJSON(resp.Choices[0].Message.Content)
	var change PlanChange
	if err := json.Unmarshal([]byte(content), &change); err != nil {
		return nil, err
	}

	return &change, nil
}

// optimizeWithLLM 使用 LLM 优化调整后的计划
func (re *ReplanningEngine) optimizeWithLLM(ctx context.Context, plan *StructuredPlan, req *ReplanRequest) (*StructuredPlan, []PlanChange, error) {
	var issuesStr strings.Builder
	for _, issue := range req.Issues {
		issuesStr.WriteString(fmt.Sprintf("- [%s] %s: %s\n", issue.Severity, issue.SubtaskID, issue.Description))
	}

	prompt := fmt.Sprintf(`以下计划在执行中遇到了问题。请优化剩余的计划部分。

## 原计划
（已在上下文中）

## 遇到的问题
%s

## 要求
1. 保持已完成的部分不变
2. 优化剩余子任务的执行顺序
3. 考虑失败的子任务的影响

请返回优化后的完整计划 JSON。`, issuesStr.String())

	req2 := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "优化任务计划，考虑执行中遇到的问题。只返回 JSON。"},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       re.model,
		Temperature: 0.3,
		MaxTokens:   4000,
	}

	resp, err := re.llmClient.Chat(ctx, req2)
	if err != nil {
		return plan, nil, err
	}

	content := extractJSON(resp.Choices[0].Message.Content)
	var newPlan StructuredPlan
	if err := json.Unmarshal([]byte(content), &newPlan); err != nil {
		return plan, nil, err
	}

	// 生成变更列表
	var changes []PlanChange
	if len(newPlan.Phases) != len(plan.Phases) {
		changes = append(changes, PlanChange{Type: "reorder", Target: "phases", Description: "阶段已重新组织", Reason: "执行问题"})
	}

	return &newPlan, changes, nil
}

// getCompletedTasks 获取已完成的任务
type completedTask struct {
	ID     string `json:"id"`
	Output string `json:"output"`
}

func (re *ReplanningEngine) getCompletedTasks(checkpoints []ExecutionCheckpoint) []completedTask {
	var completed []completedTask
	for _, cp := range checkpoints {
		if cp.Status == "completed" {
			completed = append(completed, completedTask{
				ID:     cp.SubtaskID,
				Output: cp.Output,
			})
		}
	}
	return completed
}

// clonePlan 克隆计划
func (re *ReplanningEngine) clonePlan(plan *StructuredPlan) *StructuredPlan {
	// 简单的深拷贝
	cloned := *plan
	return &cloned
}
