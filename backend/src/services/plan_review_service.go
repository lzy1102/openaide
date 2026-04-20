package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"openaide/backend/src/services/llm"
)

// PlanReviewService 执行中计划回顾服务
// 核心功能：
// 1. 检查当前执行是否在计划内
// 2. 评估子任务执行结果
// 3. 识别偏差和阻塞
// 4. 决定是否需要重新规划
type PlanReviewService struct {
	llmClient llm.LLMClient
	model     string
}

// ExecutionCheckpoint 执行检查点
type ExecutionCheckpoint struct {
	ID            string    `json:"id"`
	PlanID        string    `json:"plan_id"`
	PhaseIndex    int       `json:"phase_index"`
	SubtaskID     string    `json:"subtask_id"`
	Timestamp     time.Time `json:"timestamp"`

	// 状态
	Status        string `json:"status"` // in_progress, completed, failed, blocked, skipped

	// 执行结果
	Output        string `json:"output,omitempty"`
	Error         string `json:"error,omitempty"`

	// 计划对比
	InPlan        bool   `json:"in_plan"`         // 当前执行是否在计划内
	ExpectedVsActual string `json:"expected_vs_actual,omitempty"` // 期望与实际的对比

	// 偏差信息
	DeviationType string   `json:"deviation_type,omitempty"` // none, minor, major, blocker
	DeviationDesc string   `json:"deviation_desc,omitempty"`

	// 是否需要重新规划
	NeedsReplan   bool   `json:"needs_replan"`
	ReplanReason  string `json:"replan_reason,omitempty"`
}

// PlanReviewResult 计划回顾结果
type PlanReviewResult struct {
	// OverallStatus 整体执行状态
	OverallStatus string `json:"overall_status"` // on_track, minor_deviation, major_deviation, blocked, needs_replan

	// CompletedSubtasks 已完成的子任务
	CompletedSubtasks []string `json:"completed_subtasks"`

	// CurrentSubtask 当前子任务
	CurrentSubtask string `json:"current_subtask"`

	// RemainingSubtasks 剩余子任务
	RemainingSubtasks []string `json:"remaining_subtasks"`

	// BlockedSubtasks 阻塞的子任务
	BlockedSubtasks []string `json:"blocked_subtasks"`

	// Issues 发现的问题
	Issues []PlanIssue `json:"issues"`

	// Recommendation 建议操作
	Recommendation string `json:"recommendation"` // continue, adjust, replan, abort

	// ReplanRequired 是否需要重新规划
	ReplanRequired bool `json:"replan_required"`

	// Confidence 回顾结果的可信度
	Confidence float64 `json:"confidence"`
}

// PlanIssue 计划执行中发现的问题
type PlanIssue struct {
	Severity    string `json:"severity"`    // low, medium, high, critical
	Type        string `json:"type"`        // deviation, blocker, resource, dependency, quality
	SubtaskID   string `json:"subtask_id"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

// NewPlanReviewService 创建计划回顾服务
func NewPlanReviewService(llmClient llm.LLMClient, model string) *PlanReviewService {
	if model == "" {
		model = "gpt-4"
	}
	return &PlanReviewService{
		llmClient: llmClient,
		model:     model,
	}
}

// ReviewExecution 回顾执行状态
func (prs *PlanReviewService) ReviewExecution(ctx context.Context, plan *StructuredPlan, checkpoints []ExecutionCheckpoint, currentSubtask string) (*PlanReviewResult, error) {
	log.Printf("[PlanReview] Reviewing execution: plan has %d phases, %d checkpoints", len(plan.Phases), len(checkpoints))

	// 步骤 1: 分析当前执行状态
	analysis := prs.analyzeProgress(plan, checkpoints, currentSubtask)

	// 步骤 2: 识别问题和偏差
	issues := prs.identifyIssues(plan, checkpoints, currentSubtask)

	// 步骤 3: 使用 LLM 进行深度回顾
	if len(issues) > 0 || analysis.DeviationLevel > 0 {
		llmResult, err := prs.llmDeepReview(ctx, plan, checkpoints, issues, currentSubtask)
		if err == nil && llmResult != nil {
			return llmResult, nil
		}
		// LLM 回顾失败，使用启发式分析
		log.Printf("[PlanReview] LLM deep review failed, falling back to heuristic analysis: %v", err)
	}

	// 步骤 4: 生成回顾结果
	return prs.buildReviewResult(plan, checkpoints, currentSubtask, analysis, issues)
}

// CreateCheckpoint 创建执行检查点
func (prs *PlanReviewService) CreateCheckpoint(planID string, phaseIndex int, subtaskID string, status string, output, errMsg string) ExecutionCheckpoint {
	cp := ExecutionCheckpoint{
		ID:         fmt.Sprintf("cp_%s_%d_%s", planID, phaseIndex, subtaskID),
		PlanID:     planID,
		PhaseIndex: phaseIndex,
		SubtaskID:  subtaskID,
		Timestamp:  time.Now(),
		Status:     status,
		Output:     output,
		Error:      errMsg,
		InPlan:     true, // 默认在计划内
	}

	// 判断偏差
	if errMsg != "" {
		cp.DeviationType = "major"
		cp.DeviationDesc = errMsg
	}

	return cp
}

// analyzeProgress 分析执行进度
func (prs *PlanReviewService) analyzeProgress(plan *StructuredPlan, checkpoints []ExecutionCheckpoint, currentSubtask string) *ProgressAnalysis {
	var completed, failed, blocked []string
	var maxDeviation int // 0=none, 1=minor, 2=major, 3=blocker

	for _, cp := range checkpoints {
		switch cp.Status {
		case "completed":
			completed = append(completed, cp.SubtaskID)
		case "failed":
			failed = append(failed, cp.SubtaskID)
			if cp.DeviationType == "blocker" {
				maxDeviation = max(maxDeviation, 3)
			} else {
				maxDeviation = max(maxDeviation, 2)
			}
		case "blocked":
			blocked = append(blocked, cp.SubtaskID)
			maxDeviation = max(maxDeviation, 3)
		}
	}

	return &ProgressAnalysis{
		Completed:     completed,
		Failed:        failed,
		Blocked:       blocked,
		DeviationLevel: maxDeviation,
		Progress:      float64(len(completed)) / float64(prs.countTotalSubtasks(plan)),
	}
}

// ProgressAnalysis 进度分析
type ProgressAnalysis struct {
	Completed      []string `json:"completed"`
	Failed         []string `json:"failed"`
	Blocked        []string `json:"blocked"`
	DeviationLevel int      `json:"deviation_level"`
	Progress       float64  `json:"progress"`
}

// identifyIssues 识别执行问题
func (prs *PlanReviewService) identifyIssues(plan *StructuredPlan, checkpoints []ExecutionCheckpoint, currentSubtask string) []PlanIssue {
	var issues []PlanIssue

	// 检查失败的子任务
	for _, cp := range checkpoints {
		if cp.Status == "failed" && cp.Error != "" {
			issues = append(issues, PlanIssue{
				Severity:    "high",
				Type:        "blocker",
				SubtaskID:   cp.SubtaskID,
				Description: fmt.Sprintf("子任务 '%s' 执行失败: %s", cp.SubtaskID, cp.Error),
				Suggestion:  "尝试回退策略或重新规划",
			})
		}
		if cp.Status == "blocked" {
			issues = append(issues, PlanIssue{
				Severity:    "high",
				Type:        "dependency",
				SubtaskID:   cp.SubtaskID,
				Description: fmt.Sprintf("子任务 '%s' 被阻塞", cp.SubtaskID),
				Suggestion:  "检查依赖关系，可能需要调整执行顺序",
			})
		}
	}

	// 检查依赖是否满足
	for _, dep := range plan.Dependencies {
		depCompleted := prs.isSubtaskCompleted(dep.To, checkpoints)
		toBlocked := prs.isSubtaskBlocked(dep.From, checkpoints)
		if !depCompleted && toBlocked {
			issues = append(issues, PlanIssue{
				Severity:    "medium",
				Type:        "dependency",
				SubtaskID:   dep.From,
				Description: fmt.Sprintf("子任务 '%s' 依赖 '%s'，但依赖未满足", dep.From, dep.To),
				Suggestion:  "先完成依赖任务或调整依赖关系",
			})
		}
	}

	return issues
}

// llmDeepReview 使用 LLM 进行深度回顾
func (prs *PlanReviewService) llmDeepReview(ctx context.Context, plan *StructuredPlan, checkpoints []ExecutionCheckpoint, issues []PlanIssue, currentSubtask string) (*PlanReviewResult, error) {
	// 构建检查点摘要
	var checkpointSummary strings.Builder
	for _, cp := range checkpoints {
		checkpointSummary.WriteString(fmt.Sprintf("- 子任务 '%s': 状态=%s", cp.SubtaskID, cp.Status))
		if cp.Output != "" {
			checkpointSummary.WriteString(fmt.Sprintf(", 输出: %s", truncatePlanStr(cp.Output, 100)))
		}
		if cp.Error != "" {
			checkpointSummary.WriteString(fmt.Sprintf(", 错误: %s", truncatePlanStr(cp.Error, 100)))
		}
		checkpointSummary.WriteString("\n")
	}

	// 构建问题列表
	var issueList strings.Builder
	for _, issue := range issues {
		issueList.WriteString(fmt.Sprintf("- [%s] 子任务 '%s': %s\n", issue.Severity, issue.SubtaskID, issue.Description))
	}

	// 构建计划摘要
	var planSummary strings.Builder
	for _, phase := range plan.Phases {
		planSummary.WriteString(fmt.Sprintf("阶段 %d: %s\n", phase.Order, phase.Name))
		for _, st := range phase.Subtasks {
			planSummary.WriteString(fmt.Sprintf("  - %s: %s\n", st.ID, st.Title))
		}
	}

	prompt := fmt.Sprintf(`你是一个执行回顾专家。请分析以下计划的执行情况，判断是否需要调整计划。

## 原始计划
%s

## 执行检查点
%s

## 发现的问题
%s

## 当前子任务
%s

## 回顾要求

1. **整体评估**: 当前执行是否在轨道上？偏差程度如何？
2. **问题分析**: 每个问题的严重性和影响范围是什么？
3. **可行性判断**: 原计划是否仍然可行？是否需要调整？
4. **建议操作**: 是继续执行、局部调整、还是重新规划？

请以 JSON 格式输出：
{
  "overall_status": "on_track|minor_deviation|major_deviation|blocked|needs_replan",
  "completed_subtasks": ["已完成的子任务ID"],
  "current_subtask": "当前子任务ID",
  "remaining_subtasks": ["剩余子任务ID"],
  "blocked_subtasks": ["阻塞的子任务ID"],
  "issues": [
    {"severity": "low|medium|high|critical", "type": "deviation|blocker|resource|dependency|quality", "subtask_id": "ID", "description": "描述", "suggestion": "建议"}
  ],
  "recommendation": "continue|adjust|replan|abort",
  "replan_required": true/false,
  "confidence": 0.0-1.0
}`, planSummary.String(), checkpointSummary.String(), issueList.String(), currentSubtask)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "你是一个执行回顾专家。判断执行是否偏离计划，并给出调整建议。只返回 JSON。"},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       prs.model,
		Temperature: 0.2,
		MaxTokens:   2000,
	}

	resp, err := prs.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM deep review failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = extractJSON(content)

	var result PlanReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse review result: %w", err)
	}

	return &result, nil
}

// buildReviewResult 构建回顾结果（启发式分析）
func (prs *PlanReviewService) buildReviewResult(plan *StructuredPlan, checkpoints []ExecutionCheckpoint, currentSubtask string, analysis *ProgressAnalysis, issues []PlanIssue) (*PlanReviewResult, error) {
	var completed, remaining, blocked []string

	for _, phase := range plan.Phases {
		for _, st := range phase.Subtasks {
			isCompleted := prs.isSubtaskCompleted(st.ID, checkpoints)
			isBlocked := prs.isSubtaskBlocked(st.ID, checkpoints)

			if isCompleted {
				completed = append(completed, st.ID)
			} else if isBlocked {
				blocked = append(blocked, st.ID)
			} else {
				remaining = append(remaining, st.ID)
			}
		}
	}

	// 判断整体状态
	overallStatus := "on_track"
	recommendation := "continue"
	replanRequired := false
	confidence := 0.8

	if len(issues) > 0 {
		maxSeverity := 0
		for _, issue := range issues {
			switch issue.Severity {
			case "critical":
				maxSeverity = max(maxSeverity, 4)
			case "high":
				maxSeverity = max(maxSeverity, 3)
			case "medium":
				maxSeverity = max(maxSeverity, 2)
			case "low":
				maxSeverity = max(maxSeverity, 1)
			}
		}

		switch maxSeverity {
		case 4:
			overallStatus = "blocked"
			recommendation = "replan"
			replanRequired = true
			confidence = 0.7
		case 3:
			overallStatus = "major_deviation"
			recommendation = "adjust"
			replanRequired = true
			confidence = 0.75
		case 2:
			overallStatus = "minor_deviation"
			recommendation = "adjust"
			confidence = 0.85
		}
	}

	if analysis.DeviationLevel >= 3 {
		overallStatus = "blocked"
		recommendation = "replan"
		replanRequired = true
	} else if analysis.DeviationLevel >= 2 {
		if overallStatus == "on_track" {
			overallStatus = "major_deviation"
			recommendation = "adjust"
		}
	}

	return &PlanReviewResult{
		OverallStatus:      overallStatus,
		CompletedSubtasks:  completed,
		CurrentSubtask:     currentSubtask,
		RemainingSubtasks:  remaining,
		BlockedSubtasks:    blocked,
		Issues:             issues,
		Recommendation:     recommendation,
		ReplanRequired:     replanRequired,
		Confidence:         confidence,
	}, nil
}

// isSubtaskCompleted 检查子任务是否已完成
func (prs *PlanReviewService) isSubtaskCompleted(subtaskID string, checkpoints []ExecutionCheckpoint) bool {
	for _, cp := range checkpoints {
		if cp.SubtaskID == subtaskID && cp.Status == "completed" {
			return true
		}
	}
	return false
}

// isSubtaskBlocked 检查子任务是否被阻塞
func (prs *PlanReviewService) isSubtaskBlocked(subtaskID string, checkpoints []ExecutionCheckpoint) bool {
	for _, cp := range checkpoints {
		if cp.SubtaskID == subtaskID && cp.Status == "blocked" {
			return true
		}
	}
	return false
}

// countTotalSubtasks 统计总子任务数
func (prs *PlanReviewService) countTotalSubtasks(plan *StructuredPlan) int {
	count := 0
	for _, phase := range plan.Phases {
		count += len(phase.Subtasks)
	}
	return count
}

// max 返回最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
