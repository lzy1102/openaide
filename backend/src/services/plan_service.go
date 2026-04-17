package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"openaide/backend/src/services/llm"
)

// PlanResult 规划结果
type PlanResult struct {
	NeedsPlanning bool   `json:"needs_planning"`
	SessionID     string `json:"session_id"`
	PlanSummary   string `json:"plan_summary"`
	TaskType      string `json:"task_type"`
	Complexity    string `json:"complexity"`
	SubtaskCount  int    `json:"subtask_count"`
	Status        string `json:"status"` // planned, executing, completed, failed
	Result        string `json:"result"`
}

// PlanService 规划服务 - 桥接对话层和编排层
type PlanService struct {
	db           *gorm.DB
	orchestrator *OrchestrationService
	modelSvc     *ModelService
	router       *ModelRouter
	logger       *LoggerService
}

// NewPlanService 创建规划服务
func NewPlanService(
	db *gorm.DB,
	orchestrator *OrchestrationService,
	modelSvc *ModelService,
	router *ModelRouter,
	logger *LoggerService,
) *PlanService {
	return &PlanService{
		db:           db,
		orchestrator: orchestrator,
		modelSvc:     modelSvc,
		router:       router,
		logger:       logger,
	}
}

// NeedsPlanning 快速判断是否需要规划
func (s *PlanService) NeedsPlanning(ctx context.Context, content string) bool {
	if len(content) < 10 {
		return false
	}

	// 用路由器分类任务类型
	taskType := s.router.classifyTask(content)

	// code 类型且内容较长 → 可能需要规划
	if taskType == "code" && len(content) > 50 {
		return true
	}

	// reasoning 类型且内容较长
	if taskType == "reasoning" && len(content) > 80 {
		return true
	}

	// 包含规划触发关键词
	planningKeywords := []string{
		"帮我完成", "帮我实现", "帮我开发", "帮我设计",
		"项目", "方案", "多步骤", "分步",
		"完整", "系统", "平台", "模块",
		"帮我搭建", "帮我构建", "帮我创建",
		"plan", "project", "step by step", "multi-step",
	}
	contentLower := strings.ToLower(content)
	for _, kw := range planningKeywords {
		if strings.Contains(contentLower, strings.ToLower(kw)) {
			return true
		}
	}

	return false
}

// PlanAndExecute 分析任务并生成计划（不自动执行，等待用户确认）
func (s *PlanService) PlanAndExecute(ctx context.Context, content, modelID, userID string) (*PlanResult, error) {
	if s.orchestrator == nil {
		return nil, fmt.Errorf("orchestration service not available")
	}

	// 调用编排服务分析并生成计划
	req := &OrchestrationRequest{
		UserMessage: content,
		UserID:      userID,
	}

	resp, err := s.orchestrator.ProcessUserMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("plan generation failed: %w", err)
	}

	result := &PlanResult{
		SessionID: resp.SessionID,
		Status:    resp.Status,
	}

	// 提取任务分析信息
	session, err := s.orchestrator.GetSessionStatus(resp.SessionID)
	if err == nil && session != nil {
		if session.Analysis != nil {
			result.TaskType = session.Analysis.TaskType
			result.Complexity = session.Analysis.Complexity
			result.SubtaskCount = len(session.Analysis.Subtasks)
		}
	}

	// 根据状态处理
	switch resp.Status {
	case "awaiting_confirmation":
		result.NeedsPlanning = true
		result.PlanSummary = s.formatPlanSummary(session, resp)
		result.Status = "planned"

	case "executing":
		// 高置信度自动批准
		result.NeedsPlanning = true
		result.PlanSummary = "任务已自动批准并开始执行（置信度 ≥ 0.85）\n"
		if session != nil && session.Plan != nil {
			result.PlanSummary += s.formatTeamPlan(session.Plan)
		}
		result.Status = "executing"

	case "completed":
		// 直接完成（简单任务）
		result.NeedsPlanning = false
		result.Status = "completed"
		if session != nil && session.Execution != nil {
			result.Result = fmt.Sprintf("%v", session.Execution)
		}

	case "error":
		result.Status = "failed"
		result.Result = fmt.Sprintf("规划失败: %v", resp.Data)

	default:
		result.NeedsPlanning = true
		result.PlanSummary = fmt.Sprintf("状态: %s", resp.Status)
	}

	return result, nil
}

// ExecutePlan 用户确认后执行计划
func (s *PlanService) ExecutePlan(ctx context.Context, sessionID string) (*PlanResult, error) {
	if s.orchestrator == nil {
		return nil, fmt.Errorf("orchestration service not available")
	}

	// 批准执行
	resp, err := s.orchestrator.HandleUserAction(ctx, sessionID, "approve", "")
	if err != nil {
		return nil, fmt.Errorf("execute plan failed: %w", err)
	}

	result := &PlanResult{
		SessionID: sessionID,
	}

	switch resp.Status {
	case "executing":
		result.Status = "executing"
		result.PlanSummary = "计划已批准，开始执行..."

		// 等待执行完成（最多 5 分钟）
		execResult := s.waitForCompletion(ctx, sessionID, 5*time.Minute)
		if execResult != nil {
			result.Status = execResult.Status
			result.Result = execResult.Result
		}

	case "cancelled":
		result.Status = "cancelled"
		result.PlanSummary = "任务已取消"

	default:
		result.Status = resp.Status
	}

	return result, nil
}

// CancelPlan 取消计划
func (s *PlanService) CancelPlan(ctx context.Context, sessionID string) error {
	if s.orchestrator == nil {
		return fmt.Errorf("orchestration service not available")
	}
	_, err := s.orchestrator.HandleUserAction(ctx, sessionID, "reject", "")
	return err
}

// GetPlanStatus 查询计划状态
func (s *PlanService) GetPlanStatus(ctx context.Context, sessionID string) (*PlanResult, error) {
	if s.orchestrator == nil {
		return nil, fmt.Errorf("orchestration service not available")
	}

	session, err := s.orchestrator.GetSessionStatus(sessionID)
	if err != nil {
		return nil, err
	}

	result := &PlanResult{
		SessionID: sessionID,
		Status:    session.Status,
	}

	if session.Analysis != nil {
		result.TaskType = session.Analysis.TaskType
		result.Complexity = session.Analysis.Complexity
		result.SubtaskCount = len(session.Analysis.Subtasks)
	}

	if session.Error != "" {
		result.Result = session.Error
		result.Status = "failed"
	}

	return result, nil
}

// ChatWithPlan 带规划的聊天入口
// 自动判断是否需要规划，不需要时走普通对话
func (s *PlanService) ChatWithPlan(ctx context.Context, content, modelID, userID, dialogueID string, options map[string]interface{}) (*PlanResult, error) {
	// 判断是否需要规划
	if !s.NeedsPlanning(ctx, content) {
		return &PlanResult{
			NeedsPlanning: false,
			Status:        "skip",
		}, nil
	}

	// 需要规划，生成计划
	return s.PlanAndExecute(ctx, content, modelID, userID)
}

// ==================== 私有方法 ====================

// waitForCompletion 等待执行完成
func (s *PlanService) waitForCompletion(ctx context.Context, sessionID string, timeout time.Duration) *PlanResult {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return &PlanResult{Status: "cancelled"}
		case <-ticker.C:
			session, err := s.orchestrator.GetSessionStatus(sessionID)
			if err != nil {
				continue
			}

			switch session.Status {
			case "completed":
				result := &PlanResult{Status: "completed"}
				if session.Execution != nil {
					result.Result = formatExecutionResult(session.Execution)
				}
				return result

			case "failed":
				return &PlanResult{
					Status: "failed",
					Result: session.Error,
				}

			case "cancelled":
				return &PlanResult{Status: "cancelled"}
			}
		}
	}

	return &PlanResult{
		Status: "timeout",
		Result: fmt.Sprintf("执行超时（%v），请稍后查询状态", timeout),
	}
}

// formatPlanSummary 格式化计划摘要
func (s *PlanService) formatPlanSummary(session *OrchestrationSession, resp *OrchestrationResponse) string {
	var parts []string

	parts = append(parts, "📋 **任务规划方案**\n")

	// 任务分析
	if session != nil && session.Analysis != nil {
		analysis := session.Analysis
		parts = append(parts, fmt.Sprintf("**任务类型**: %s", analysis.TaskType))
		parts = append(parts, fmt.Sprintf("**复杂度**: %s", analysis.Complexity))
		parts = append(parts, fmt.Sprintf("**优先级**: %s", analysis.Priority))
		if len(analysis.Subtasks) > 0 {
			parts = append(parts, fmt.Sprintf("**子任务数**: %d", len(analysis.Subtasks)))
		}
	}

	parts = append(parts, "")

	// 执行计划
	if session != nil && session.Plan != nil {
		parts = append(parts, s.formatTeamPlan(session.Plan))
	}

	// 确认提示
	if resp.RequireAction {
		parts = append(parts, "")
		parts = append(parts, "---")
		parts = append(parts, "回复 **确认** 或 **执行** 开始执行，回复 **取消** 放弃任务。")
	}

	return strings.Join(parts, "\n")
}

// formatTeamPlan 格式化团队计划
func (s *PlanService) formatTeamPlan(plan interface{}) string {
	// 使用 fmt 安全处理 nil plan
	if plan == nil {
		return ""
	}
	return fmt.Sprintf("**执行计划**: %v", plan)
}

// formatExecutionResult 格式化执行结果
func formatExecutionResult(execution interface{}) string {
	if execution == nil {
		return "执行完成（无详细结果）"
	}
	result := fmt.Sprintf("%v", execution)
	// 截断过长的结果
	if len(result) > 2000 {
		result = result[:2000] + "..."
	}
	return result
}

// NewOrchestrationServiceWithModel 创建带模型选择的编排服务
func NewOrchestrationServiceWithModel(db *gorm.DB, modelSvc *ModelService, coordinator *TeamCoordinatorService, executor *AgentExecutor) *OrchestrationService {
	llmClient := modelSvc.GetLLMClient()
	if llmClient == nil {
		log.Println("[PlanService] WARNING: no LLM client available for orchestration, using nil client")
		// OrchestrationService 会处理 nil client 的情况
		llmClient = &noopLLMClient{}
	}
	return NewOrchestrationService(db, llmClient, coordinator, executor)
}

// noopLLMClient 空操作 LLM 客户端（当没有可用模型时的降级方案）
type noopLLMClient struct{}

func (c *noopLLMClient) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    "assistant",
					Content: "当前没有可用的 LLM 模型来处理此请求。请先配置并启用至少一个 LLM 模型。",
				},
			},
		},
	}, nil
}

func (c *noopLLMClient) ChatStream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.ChatStreamChunk, error) {
	ch := make(chan llm.ChatStreamChunk, 1)
	ch <- llm.ChatStreamChunk{
		Choices: []llm.StreamChoice{
			{
				Delta: llm.MessageDelta{
					Role:    "assistant",
					Content: "当前没有可用的 LLM 模型来处理此请求。",
				},
				FinishReason: "stop",
			},
		},
	}
	close(ch)
	return ch, nil
}
