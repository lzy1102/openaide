package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
	"openaide/backend/src/services/orchestration"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrchestrationService 智能编排服务 - 统一入口
// 整合 TaskAnalyzer -> TeamPlanner -> ConfirmationFlow -> TeamOrchestrator 的完整流程
type OrchestrationService struct {
	db           *gorm.DB
	llmClient    llm.LLMClient
	coordinator  *TeamCoordinatorService

	// 编排组件
	taskAnalyzer   *orchestration.TaskAnalyzer
	teamPlanner    *orchestration.TeamPlanner
	confirmFlow    *orchestration.ConfirmationFlow
	teamOrchestrator *orchestration.TeamOrchestrator

	// 结构化规划引擎
	structuredPlanner *StructuredPlanner

	// 计划回顾与动态调整
	planReview      *PlanReviewService
	replanningEngine *ReplanningEngine

	// 执行引擎（真正执行子任务）
	agentExecutor    *AgentExecutor
	toolCallingSvc   *ToolCallingService
	dialogueSvc      *DialogueService

	// 运行中的任务
	sessions      map[string]*OrchestrationSession
	sessionMutex  sync.RWMutex

	// 配置
	config        OrchestrationConfig
}

// OrchestrationConfig 编排配置
type OrchestrationConfig struct {
	DefaultLLMModel      string        `json:"default_llm_model"`
	MaxConcurrentTasks   int           `json:"max_concurrent_tasks"`
	SessionTimeout       time.Duration `json:"session_timeout"`
	AutoApproveThreshold float64       `json:"auto_approve_threshold"` // 置信度高于此值自动批准
	EnableCache          bool          `json:"enable_cache"`
}

// OrchestrationSession 编排会话
type OrchestrationSession struct {
	ID            string                      `json:"id"`
	UserID        string                      `json:"user_id"`
	UserMessage   string                      `json:"user_message"`
	Status        string                      `json:"status"` // analyzing, planning, confirming, executing, completed, failed
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`

	// 各阶段数据（旧编排流程）
	Analysis      *orchestration.TaskAnalysis  `json:"analysis,omitempty"`
	Plan          *orchestration.TeamPlan      `json:"plan,omitempty"`
	Confirmation  *orchestration.ConfirmationRequest `json:"confirmation,omitempty"`
	Execution     *orchestration.ExecutionResult `json:"execution,omitempty"`

	// 结构化规划数据（新编排流程）
	StructuredPlan   *StructuredPlan           `json:"structured_plan,omitempty"`
	Checkpoints      []ExecutionCheckpoint     `json:"checkpoints,omitempty"`
	CurrentSubtask   string                    `json:"current_subtask,omitempty"`

	// 错误信息
	Error         string                      `json:"error,omitempty"`

	// 上下文
	Context       context.Context
	Cancel        context.CancelFunc
}

// OrchestrationRequest 编排请求
type OrchestrationRequest struct {
	UserMessage   string                 `json:"user_message" binding:"required"`
	UserID        string                 `json:"user_id" binding:"required"`
	Context       map[string]interface{} `json:"context,omitempty"`
	Options       map[string]interface{} `json:"options,omitempty"`
}

// OrchestrationResponse 编排响应
type OrchestrationResponse struct {
	SessionID     string                      `json:"session_id"`
	Status        string                      `json:"status"`
	Stage         string                      `json:"stage"`
	Data          interface{}                 `json:"data,omitempty"`
	RequireAction bool                        `json:"require_action,omitempty"` // 是否需要用户操作
	ActionType    string                      `json:"action_type,omitempty"`    // approve, adjust, input
	ActionPrompt  string                      `json:"action_prompt,omitempty"`
	CreatedAt     time.Time                   `json:"created_at"`
}

// NewOrchestrationService 创建智能编排服务
func NewOrchestrationService(db *gorm.DB, llmClient llm.LLMClient, coordinator *TeamCoordinatorService, executor *AgentExecutor) *OrchestrationService {
	// 创建 LLM 适配器
	llmAdapter := &llmServiceAdapter{client: llmClient}

	// 创建编排组件
	taskAnalyzer := orchestration.NewTaskAnalyzer(llmClient, "gpt-4")
	teamPlanner := orchestration.NewTeamPlanner(llmAdapter)
	confirmFlow := orchestration.NewConfirmationFlow()

	// 创建执行器适配器
	var executorAdapter orchestration.AgentExecutorInterface
	if executor != nil {
		executorAdapter = &agentExecutorAdapter{executor: executor}
	}

	// 创建协调器适配器
	coordinatorAdapter := &coordinatorServiceAdapter{coordinator: coordinator}
	teamOrchestrator := orchestration.NewTeamOrchestrator(db, coordinatorAdapter, executorAdapter)

	return &OrchestrationService{
		db:              db,
		llmClient:       llmClient,
		coordinator:     coordinator,
		taskAnalyzer:    taskAnalyzer,
		teamPlanner:     teamPlanner,
		confirmFlow:     confirmFlow,
		teamOrchestrator: teamOrchestrator,
		agentExecutor:   executor,
		sessions:        make(map[string]*OrchestrationSession),
		config: OrchestrationConfig{
			DefaultLLMModel:      "gpt-4",
			MaxConcurrentTasks:   5,
			SessionTimeout:       30 * time.Minute,
			AutoApproveThreshold: 0.85,
			EnableCache:          true,
		},
	}
}

// SetToolCallingService 设置工具调用服务（用于子任务执行）
func (s *OrchestrationService) SetToolCallingService(toolCallingSvc *ToolCallingService) {
	s.toolCallingSvc = toolCallingSvc
}

// SetDialogueService 设置对话服务（用于创建临时对话执行子任务）
func (s *OrchestrationService) SetDialogueService(dialogueSvc *DialogueService) {
	s.dialogueSvc = dialogueSvc
}

// SetStructuredPlanner 设置结构化规划引擎
func (s *OrchestrationService) SetStructuredPlanner(planner *StructuredPlanner) {
	s.structuredPlanner = planner

	// 同时初始化回顾和调整引擎
	if planner.llmClient != nil {
		s.planReview = NewPlanReviewService(planner.llmClient, "")
		s.replanningEngine = NewReplanningEngine(planner.llmClient, "", planner)
	}
}

// SetPlanReview 设置计划回顾服务
func (s *OrchestrationService) SetPlanReview(planReview *PlanReviewService) {
	s.planReview = planReview
}

// SetReplanningEngine 设置动态重规划引擎
func (s *OrchestrationService) SetReplanningEngine(replanningEngine *ReplanningEngine) {
	s.replanningEngine = replanningEngine
}

// ReviewAndAdapt 执行中回顾和动态调整计划
// 在每个子任务完成后调用，检查是否需要调整计划
func (s *OrchestrationService) ReviewAndAdapt(ctx context.Context, plan *StructuredPlan, checkpoints []ExecutionCheckpoint, currentSubtask string, userMessage string) (*PlanReviewResult, *ReplanResult, error) {
	if s.planReview == nil {
		return nil, nil, fmt.Errorf("plan review service not initialized")
	}

	// 步骤 1: 回顾执行状态
	review, err := s.planReview.ReviewExecution(ctx, plan, checkpoints, currentSubtask)
	if err != nil {
		log.Printf("[Orchestration] Plan review failed: %v", err)
		return nil, nil, err
	}

	log.Printf("[Orchestration] Review result: status=%s, recommendation=%s, replan=%v",
		review.OverallStatus, review.Recommendation, review.ReplanRequired)

	// 步骤 2: 如果需要调整，调用调整引擎
	var replanResult *ReplanResult
	if review.ReplanRequired && s.replanningEngine != nil {
		replanLevel := review.Recommendation
		if replanLevel == "abort" {
			replanLevel = "fallback"
		}

		replanReq := &ReplanRequest{
			OriginalPlan:   plan,
			Checkpoints:    checkpoints,
			Issues:         review.Issues,
			UserMessage:    userMessage,
			CurrentSubtask: currentSubtask,
			ReplanLevel:    replanLevel,
		}

		replanResult, err = s.replanningEngine.Replan(ctx, replanReq)
		if err != nil {
			log.Printf("[Orchestration] Replanning failed: %v", err)
		} else {
			log.Printf("[Orchestration] Replan result: level=%s, changes=%d", replanResult.Level, len(replanResult.Changes))
		}
	}

	return review, replanResult, nil
}

// CreateCheckpoint 创建执行检查点
func (s *OrchestrationService) CreateCheckpoint(planID string, phaseIndex int, subtaskID string, status string, output, errMsg string) ExecutionCheckpoint {
	if s.planReview == nil {
		return ExecutionCheckpoint{
			ID:         fmt.Sprintf("cp_%s_%d_%s", planID, phaseIndex, subtaskID),
			Status:     status,
			Output:     output,
			Error:      errMsg,
		}
	}
	return s.planReview.CreateCheckpoint(planID, phaseIndex, subtaskID, status, output, errMsg)
}

// ProcessUserMessage 处理用户消息 - 主入口
// 优先使用结构化规划器（如果可用），否则回退到旧编排流程
func (s *OrchestrationService) ProcessUserMessage(ctx context.Context, req *OrchestrationRequest) (*OrchestrationResponse, error) {
	// 创建会话
	session := &OrchestrationSession{
		ID:          uuid.New().String(),
		UserID:      req.UserID,
		UserMessage: req.UserMessage,
		Status:      "analyzing",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	session.Context, session.Cancel = context.WithTimeout(ctx, s.config.SessionTimeout)

	s.sessionMutex.Lock()
	s.sessions[session.ID] = session
	s.sessionMutex.Unlock()

	// 如果结构化规划器可用，使用新的5步规划流程
	if s.structuredPlanner != nil {
		return s.processWithStructuredPlanner(ctx, session, req)
	}

	// 回退到旧编排流程
	return s.processWithLegacyFlow(ctx, session, req)
}

// processWithStructuredPlanner 使用结构化规划器处理（5步流程）
func (s *OrchestrationService) processWithStructuredPlanner(ctx context.Context, session *OrchestrationSession, req *OrchestrationRequest) (*OrchestrationResponse, error) {
	log.Printf("[Orchestration] Using structured planner for session %s", session.ID)

	// 阶段 1-5: 深度理解 + 结构化规划 + 依赖分析 + 工具规划 + 风险评估
	structuredPlan, err := s.structuredPlanner.Plan(ctx, req.UserMessage, req.UserID, nil)
	if err != nil {
		log.Printf("[Orchestration] Structured planning failed, falling back to legacy: %v", err)
		return s.processWithLegacyFlow(ctx, session, req)
	}

	session.StructuredPlan = structuredPlan
	session.Status = "planning"
	session.UpdatedAt = time.Now()

	log.Printf("[Orchestration] Structured plan created: %d phases, %d dependencies, %d tools assigned",
		len(structuredPlan.Phases), len(structuredPlan.Dependencies), len(structuredPlan.ToolPlan))

	// 构建确认提案
	proposal := s.buildStructuredPlanProposal(session, structuredPlan)

	// 检查置信度，高置信度自动执行
	if structuredPlan.Understanding != nil && structuredPlan.Understanding.Confidence >= s.config.AutoApproveThreshold {
		return s.autoExecuteStructuredPlan(session, structuredPlan)
	}

	// 需要用户确认
	session.Status = "confirming"

	return &OrchestrationResponse{
		SessionID:     session.ID,
		Status:        "awaiting_confirmation",
		Stage:         "confirmation",
		Data: map[string]interface{}{
			"structured_plan": structuredPlan,
			"proposal":        proposal,
		},
		RequireAction: true,
		ActionType:    "approve",
		ActionPrompt:  proposal + "\n\n回复 'yes' 确认执行，或 'no' 取消任务。",
		CreatedAt:     time.Now(),
	}, nil
}

// processWithLegacyFlow 使用旧编排流程
func (s *OrchestrationService) processWithLegacyFlow(ctx context.Context, session *OrchestrationSession, req *OrchestrationRequest) (*OrchestrationResponse, error) {
	// 阶段 1: 任务分析
	analysis, err := s.analyzeTask(ctx, session, req)
	if err != nil {
		return s.errorResponse(session, fmt.Sprintf("任务分析失败: %v", err)), nil
	}

	// 阶段 2: 团队规划
	plan, err := s.planTeam(ctx, session, analysis)
	if err != nil {
		return s.errorResponse(session, fmt.Sprintf("团队规划失败: %v", err)), nil
	}

	// 阶段 3: 确认流程
	confirmation, err := s.createConfirmation(ctx, session, plan)
	if err != nil {
		return s.errorResponse(session, fmt.Sprintf("创建确认流程失败: %v", err)), nil
	}

	// 检查是否可以自动批准
	confidence := s.taskAnalyzer.EstimateConfidence(analysis)
	if confidence >= s.config.AutoApproveThreshold {
		// 自动批准，直接执行
		return s.autoExecute(session, confirmation)
	}

	// 需要用户确认
	session.Status = "confirming"
	session.Confirmation = confirmation

	proposalText := s.confirmFlow.FormatProposal(confirmation.Proposal)

	return &OrchestrationResponse{
		SessionID:     session.ID,
		Status:        "awaiting_confirmation",
		Stage:         "confirmation",
		Data:          confirmation,
		RequireAction: true,
		ActionType:    "approve",
		ActionPrompt:  proposalText + "\n\n回复 'yes' 确认执行，或 'no' 取消任务。",
		CreatedAt:     time.Now(),
	}, nil
}

// buildStructuredPlanProposal 构建结构化计划的确认提案
func (s *OrchestrationService) buildStructuredPlanProposal(session *OrchestrationSession, plan *StructuredPlan) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 任务: %s\n\n", session.UserMessage))

	if plan.Understanding != nil {
		sb.WriteString(fmt.Sprintf("🎯 意图理解: %s\n", plan.Understanding.UserIntent))
		if len(plan.Understanding.ImplicitNeeds) > 0 {
			sb.WriteString(fmt.Sprintf("💡 隐含需求: %v\n", plan.Understanding.ImplicitNeeds))
		}
		sb.WriteString(fmt.Sprintf("📊 置信度: %.0f%%\n\n", plan.Understanding.Confidence*100))
	}

	sb.WriteString(fmt.Sprintf("📑 执行计划 (%d 个阶段):\n", len(plan.Phases)))
	for i, phase := range plan.Phases {
		sb.WriteString(fmt.Sprintf("\n阶段 %d: %s\n", i+1, phase.Name))
		if phase.Description != "" {
			sb.WriteString(fmt.Sprintf("  描述: %s\n", phase.Description))
		}
		for j, task := range phase.Subtasks {
			sb.WriteString(fmt.Sprintf("  ⬜ %d.%d %s (%d分钟)\n", i+1, j+1, task.Title, task.Estimated))
		}
	}

	if len(plan.Dependencies) > 0 {
		sb.WriteString(fmt.Sprintf("\n🔗 依赖关系: %d 个\n", len(plan.Dependencies)))
	}

	if plan.RiskAssessment != nil && len(plan.RiskAssessment.TopRisks) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ 风险提醒 (%d 个):\n", len(plan.RiskAssessment.TopRisks)))
		for _, risk := range plan.RiskAssessment.TopRisks {
			sb.WriteString(fmt.Sprintf("  - %s [概率:%s 影响:%s]: %s\n", risk.Description, risk.Probability, risk.Impact, risk.Mitigation))
		}
	}

	if len(plan.FallbackPlans) > 0 {
		sb.WriteString(fmt.Sprintf("\n🛡️ 回退方案: %d 个已准备\n", len(plan.FallbackPlans)))
	}

	return sb.String()
}

// autoExecuteStructuredPlan 自动执行结构化计划
func (s *OrchestrationService) autoExecuteStructuredPlan(session *OrchestrationSession, plan *StructuredPlan) (*OrchestrationResponse, error) {
	log.Printf("[Orchestration] Auto-executing structured plan for session %s", session.ID)

	// 初始化检查点
	session.Checkpoints = make([]ExecutionCheckpoint, 0)
	session.Status = "executing"
	session.UpdatedAt = time.Now()

	// 启动异步执行
	go s.executeStructuredPlan(session, plan)

	return &OrchestrationResponse{
		SessionID: session.ID,
		Status:    "executing",
		Stage:     "execution",
		Data: map[string]interface{}{
			"structured_plan": plan,
			"message":         "结构化计划已自动批准并开始执行",
		},
		CreatedAt: time.Now(),
	}, nil
}

// executeStructuredPlan 执行结构化计划（带回顾和重规划）
// 真正调用 AgentExecutor 或 ToolCallingService 执行每个子任务
func (s *OrchestrationService) executeStructuredPlan(session *OrchestrationSession, plan *StructuredPlan) {
	log.Printf("[Orchestration] Starting structured plan execution for session %s", session.ID)

	// 保存初始编排记录
	if err := s.saveOrchestrationRecord(session, plan); err != nil {
		log.Printf("[Orchestration] Failed to save initial record: %v", err)
	}

	// 收集前置子任务的输出，作为后续子任务的上下文
	subtaskOutputs := make(map[string]string)
	var orchestrationID string
	// 从已保存的记录中获取 orchestrationID
	if s.db != nil {
		var record models.OrchestrationRecord
		if err := s.db.Where("session_id = ?", session.ID).Order("created_at DESC").First(&record).Error; err == nil {
			orchestrationID = record.ID
		}
	}

	for phaseIdx, phase := range plan.Phases {
		log.Printf("[Orchestration] Executing phase %d: %s", phaseIdx+1, phase.Name)

		// 判断阶段内子任务是否可以并行执行
		parallelTasks := make([]*Subtask, 0)
		sequentialTasks := make([]*Subtask, 0)

		for i := range phase.Subtasks {
			st := &phase.Subtasks[i]
			if st.CanParallel && len(st.Skills) > 0 {
				parallelTasks = append(parallelTasks, st)
			} else {
				sequentialTasks = append(sequentialTasks, st)
			}
		}

		// 先执行并行任务
		if len(parallelTasks) > 0 {
			log.Printf("[Orchestration] Phase %d: executing %d parallel subtasks", phaseIdx+1, len(parallelTasks))
			var wg sync.WaitGroup
			var mu sync.Mutex

			for _, st := range parallelTasks {
				wg.Add(1)
				go func(subtask *Subtask) {
					defer wg.Done()
					output, success, errMsg, duration := s.executeSubtaskWithMetrics(session, plan, phaseIdx, subtask, subtaskOutputs)
					mu.Lock()
					if success {
						subtaskOutputs[subtask.ID] = output
					}
					mu.Unlock()
					// 持久化子任务记录
					if orchestrationID != "" {
						go s.saveSubtaskRecord(session.ID, orchestrationID, phaseIdx, subtask, output, errMsg, success, duration)
					}
				}(st)
			}
			wg.Wait()
		}

		// 再执行串行任务
		for _, subtask := range sequentialTasks {
			if session.Context.Err() != nil {
				log.Printf("[Orchestration] Session %s cancelled", session.ID)
				session.Status = "cancelled"
				s.updateOrchestrationRecord(session)
				return
			}

			output, success, errMsg, duration := s.executeSubtaskWithMetrics(session, plan, phaseIdx, subtask, subtaskOutputs)
			if success {
				subtaskOutputs[subtask.ID] = output
			}
			// 持久化子任务记录
			if orchestrationID != "" {
				go s.saveSubtaskRecord(session.ID, orchestrationID, phaseIdx, subtask, output, errMsg, success, duration)
			}
		}

		// 每个阶段结束后更新编排记录
		s.updateOrchestrationRecord(session)
	}

	session.Status = "completed"
	session.UpdatedAt = time.Now()
	s.updateOrchestrationRecord(session)
	log.Printf("[Orchestration] Structured plan execution completed for session %s", session.ID)
}

// executeSubtaskWithMetrics 执行单个子任务，返回 (输出, 是否成功, 错误信息, 耗时)
func (s *OrchestrationService) executeSubtaskWithMetrics(
	session *OrchestrationSession,
	plan *StructuredPlan,
	phaseIdx int,
	subtask *Subtask,
	previousOutputs map[string]string,
) (string, bool, string, time.Duration) {
	session.CurrentSubtask = subtask.ID

	// 创建检查点 - 开始执行
	checkpoint := s.CreateCheckpoint(plan.ID, phaseIdx, subtask.ID, "in_progress", "", "")
	session.Checkpoints = append(session.Checkpoints, checkpoint)

	log.Printf("[Orchestration] Executing subtask: %s - %s (type=%s)", subtask.ID, subtask.Title, subtask.Type)

	// 构建子任务的执行上下文（包含前置任务输出）
	ctx := session.Context
	startTime := time.Now()

	// 构建任务描述，包含前置依赖的输出
	taskDescription := s.buildSubtaskDescription(subtask, plan, previousOutputs)

	// 选择执行方式：优先使用 AgentExecutor，回退到 ToolCallingService
	var output string
	var success bool
	var errMsg string

	if s.agentExecutor != nil {
		// 使用 AgentExecutor 执行（完整的 ReAct 循环）
		output, success, errMsg = s.executeWithAgentExecutor(ctx, session, subtask, taskDescription)
	} else if s.toolCallingSvc != nil && s.dialogueSvc != nil {
		// 使用 ToolCallingService 执行（创建临时对话）
		output, success, errMsg = s.executeWithToolCallingService(ctx, session, subtask, taskDescription)
	} else {
		// 降级：直接调用 LLM，无工具
		output, success, errMsg = s.executeWithLLMOnly(ctx, session, subtask, taskDescription)
	}

	duration := time.Since(startTime)

	// 更新检查点
	checkpoint.CompletedAt = time.Now()
	if success {
		checkpoint.Status = "completed"
		checkpoint.Output = output
		log.Printf("[Orchestration] Subtask %s completed in %v (output=%d chars)", subtask.ID, duration, len(output))
	} else {
		checkpoint.Status = "failed"
		checkpoint.Output = output
		checkpoint.Error = errMsg
		log.Printf("[Orchestration] Subtask %s failed after %v: %s", subtask.ID, duration, errMsg)

		// 执行回顾和重规划
		if s.planReview != nil && s.replanningEngine != nil {
			log.Printf("[Orchestration] Reviewing after failure of subtask %s", subtask.ID)
			review, replan, err := s.ReviewAndAdapt(ctx, plan, session.Checkpoints, subtask.ID, session.UserMessage)
			if err == nil && review != nil {
				log.Printf("[Orchestration] Review: %s, replan: %v", review.Recommendation, replan != nil)
				if replan != nil {
					log.Printf("[Orchestration] Applying replan: %s with %d changes", replan.Level, len(replan.Changes))
					// TODO: 应用重规划变更到 plan（需要线程安全）
				}
			}
		}
	}

	return output, success, errMsg, duration
}

// buildSubtaskDescription 构建子任务描述，包含上下文
func (s *OrchestrationService) buildSubtaskDescription(subtask *Subtask, plan *StructuredPlan, previousOutputs map[string]string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("任务: %s\n", subtask.Title))
	sb.WriteString(fmt.Sprintf("描述: %s\n", subtask.Description))
	if len(subtask.Skills) > 0 {
		sb.WriteString(fmt.Sprintf("所需技能: %v\n", subtask.Skills))
	}

	// 添加总体目标上下文
	if plan.Understanding != nil {
		sb.WriteString(fmt.Sprintf("\n总体目标: %s\n", plan.Understanding.UserIntent))
	}

	// 添加前置任务的输出作为上下文
	if len(previousOutputs) > 0 {
		sb.WriteString("\n前置任务输出（供参考）:\n")
		for id, output := range previousOutputs {
			// 截断过长的输出
			displayOutput := output
			if len(displayOutput) > 500 {
				displayOutput = displayOutput[:500] + "..."
			}
			sb.WriteString(fmt.Sprintf("- [%s]: %s\n", id, displayOutput))
		}
	}

	return sb.String()
}

// executeWithAgentExecutor 使用 AgentExecutor 执行子任务（完整 ReAct 循环）
func (s *OrchestrationService) executeWithAgentExecutor(
	ctx context.Context,
	session *OrchestrationSession,
	subtask *Subtask,
	description string,
) (string, bool, string) {
	req := &TaskExecRequest{
		TaskID:          subtask.ID,
		TaskTitle:       subtask.Title,
		TaskDescription: description,
		AgentName:       fmt.Sprintf("Agent-%s", subtask.Type),
		AgentRole:       subtask.Type,
		AgentPrompt:     fmt.Sprintf("你是一个专业的 %s 专家。请高效完成分配给你的任务。", subtask.Type),
		ModelID:         "", // 使用默认模型
		Context: map[string]interface{}{
			"subtask_type": subtask.Type,
			"session_id":   session.ID,
			"phase":        session.CurrentSubtask,
		},
	}

	result, err := s.agentExecutor.Execute(ctx, req)
	if err != nil {
		return "", false, fmt.Sprintf("AgentExecutor 执行失败: %v", err)
	}

	return result.Output, result.Success, ""
}

// executeWithToolCallingService 使用 ToolCallingService 执行子任务
func (s *OrchestrationService) executeWithToolCallingService(
	ctx context.Context,
	session *OrchestrationSession,
	subtask *Subtask,
	description string,
) (string, bool, string) {
	// 创建临时对话来执行子任务
	var dialogueID string
	if s.dialogueSvc != nil {
		dialogue := s.dialogueSvc.CreateDialogue(session.UserID, fmt.Sprintf("subtask-%s", subtask.ID))
		dialogueID = dialogue.ID
	} else {
		dialogueID = fmt.Sprintf("orch-%s-%s", session.ID, subtask.ID)
	}

	// 使用 ToolCallingService 执行（带完整工具调用循环）
	msg, err := s.toolCallingSvc.SendMessageWithTools(
		ctx,
		dialogueID,
		session.UserID,
		description,
		"", // 使用默认模型
		map[string]interface{}{},
	)
	if err != nil {
		return "", false, fmt.Sprintf("ToolCallingService 执行失败: %v", err)
	}

	if msg == nil {
		return "", false, "ToolCallingService 返回空消息"
	}

	return msg.Content, true, ""
}

// executeWithLLMOnly 仅使用 LLM 执行（无工具，降级方案）
func (s *OrchestrationService) executeWithLLMOnly(
	ctx context.Context,
	session *OrchestrationSession,
	subtask *Subtask,
	description string,
) (string, bool, string) {
	messages := []llm.Message{
		{Role: "system", Content: fmt.Sprintf("你是一个专业的 %s 专家。请完成以下任务。", subtask.Type)},
		{Role: "user", Content: description},
	}

	req := &llm.ChatRequest{
		Messages:    messages,
		Model:       s.config.DefaultLLMModel,
		Temperature: 0.7,
		MaxTokens:   4000,
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return "", false, fmt.Sprintf("LLM 调用失败: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", false, "LLM 返回空响应"
	}

	return resp.Choices[0].Message.Content, true, ""
}

// HandleUserAction 处理用户操作
func (s *OrchestrationService) HandleUserAction(ctx context.Context, sessionID, action, comment string) (*OrchestrationResponse, error) {
	s.sessionMutex.RLock()
	session, exists := s.sessions[sessionID]
	s.sessionMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// 处理确认操作
	switch action {
	case "approve", "yes", "y":
		// 如果有结构化计划，使用结构化执行
		if session.StructuredPlan != nil {
			return s.startStructuredExecution(session)
		}
		return s.executePlan(ctx, session)

	case "reject", "no", "n":
		session.Status = "cancelled"
		session.UpdatedAt = time.Now()
		return &OrchestrationResponse{
			SessionID: sessionID,
			Status:    "cancelled",
			Stage:     "confirmation",
			Data:      "任务已取消",
			CreatedAt: time.Now(),
		}, nil

		case "adjust":
			return s.adjustPlan(ctx, session, comment)

	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// startStructuredExecution 启动结构化计划执行
func (s *OrchestrationService) startStructuredExecution(session *OrchestrationSession) (*OrchestrationResponse, error) {
	if session.StructuredPlan == nil {
		return nil, fmt.Errorf("no structured plan available")
	}

	log.Printf("[Orchestration] Starting structured execution for session %s", session.ID)

	// 初始化检查点
	session.Checkpoints = make([]ExecutionCheckpoint, 0)
	session.Status = "executing"
	session.UpdatedAt = time.Now()

	// 启动异步执行（带回顾和重规划）
	go s.executeStructuredPlan(session, session.StructuredPlan)

	return &OrchestrationResponse{
		SessionID: session.ID,
		Status:    "executing",
		Stage:     "execution",
		Data: map[string]interface{}{
			"message":      "结构化计划开始执行",
			"plan_id":      session.StructuredPlan.ID,
			"total_phases": len(session.StructuredPlan.Phases),
		},
		CreatedAt: time.Now(),
	}, nil
}

// GetSessionStatus 获取会话状态
func (s *OrchestrationService) GetSessionStatus(sessionID string) (*OrchestrationSession, error) {
	s.sessionMutex.RLock()
	defer s.sessionMutex.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session, nil
}

// GetExecutionProgress 获取执行进度
func (s *OrchestrationService) GetExecutionProgress(sessionID string) (*OrchestrationResponse, error) {
	session, err := s.GetSessionStatus(sessionID)
	if err != nil {
		return nil, err
	}

	if session.Status != "executing" {
		return &OrchestrationResponse{
			SessionID: sessionID,
			Status:    session.Status,
			Stage:     session.Status,
			CreatedAt: time.Now(),
		}, nil
	}

	// 获取实际执行进度
	progress := s.getExecutionProgress(session)

	return &OrchestrationResponse{
		SessionID: sessionID,
		Status:    "executing",
		Stage:     "execution",
		Data:      progress,
		CreatedAt: time.Now(),
	}, nil
}

// CancelSession 取消会话
func (s *OrchestrationService) CancelSession(sessionID string) error {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.Cancel != nil {
		session.Cancel()
	}

	session.Status = "cancelled"
	session.UpdatedAt = time.Now()

	return nil
}

// CleanupSessions 清理过期会话
func (s *OrchestrationService) CleanupSessions() {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if now.Sub(session.UpdatedAt) > s.config.SessionTimeout {
			if session.Cancel != nil {
				session.Cancel()
			}
			delete(s.sessions, id)
		}
	}
}

// ==================== 公开查询方法 ====================

// ListSessions 列出用户的所有会话
func (s *OrchestrationService) ListSessions(userID string) []*OrchestrationSession {
	s.sessionMutex.RLock()
	defer s.sessionMutex.RUnlock()

	var result []*OrchestrationSession
	for _, session := range s.sessions {
		if userID == "" || session.UserID == userID {
			result = append(result, session)
		}
	}
	return result
}

// AnalyzeTask 仅分析任务（不执行）
func (s *OrchestrationService) AnalyzeTask(ctx context.Context, req *OrchestrationRequest) (*orchestration.TaskAnalysis, error) {
	session := &OrchestrationSession{
		ID:          uuid.New().String(),
		UserID:      req.UserID,
		UserMessage: req.UserMessage,
		Status:      "analyzing",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	analysis, err := s.analyzeTask(ctx, session, req)
	if err != nil {
		return nil, err
	}
	// 分析完成后清理临时会话
	return analysis, nil
}

// PlanTeam 仅生成团队方案（不执行）
func (s *OrchestrationService) PlanTeam(ctx context.Context, req *OrchestrationRequest) (*orchestration.TeamPlan, error) {
	session := &OrchestrationSession{
		ID:          uuid.New().String(),
		UserID:      req.UserID,
		UserMessage: req.UserMessage,
		Status:      "planning",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	analysis, err := s.analyzeTask(ctx, session, req)
	if err != nil {
		return nil, err
	}
	plan, err := s.planTeam(ctx, session, analysis)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// GetTeamTemplates 列出所有可用团队模板
func (s *OrchestrationService) GetTeamTemplates() []string {
	return s.teamPlanner.ListTemplates()
}

// GetTeamTemplate 获取指定团队模板
func (s *OrchestrationService) GetTeamTemplate(name string) (*orchestration.TeamTemplate, error) {
	return s.teamPlanner.GetTemplate(name)
}

// ==================== 私有方法 ====================

// analyzeTask 任务分析阶段
func (s *OrchestrationService) analyzeTask(ctx context.Context, session *OrchestrationSession, req *OrchestrationRequest) (*orchestration.TaskAnalysis, error) {
	session.Status = "analyzing"
	session.UpdatedAt = time.Now()

	analysisReq := &orchestration.AnalysisRequest{
		UserMessage: req.UserMessage,
		Context:     req.Context,
	}

	analysis, err := s.taskAnalyzer.Analyze(ctx, analysisReq)
	if err != nil {
		return nil, err
	}

	session.Analysis = analysis
	return analysis, nil
}

// planTeam 团队规划阶段
func (s *OrchestrationService) planTeam(ctx context.Context, session *OrchestrationSession, analysis *orchestration.TaskAnalysis) (*orchestration.TeamPlan, error) {
	session.Status = "planning"
	session.UpdatedAt = time.Now()

	// 转换为 TeamPlanner 分析格式
	plannerAnalysis := &orchestration.TeamPlannerAnalysis{
		Goal:         analysis.Description,
		Description:  analysis.Description,
		Type:         analysis.TaskType,
		Complexity:   analysis.Complexity,
		Requirements: analysis.Dependencies,
		Deliverables: analysis.Skills,
	}

	plan, err := s.teamPlanner.PlanTeam(ctx, plannerAnalysis)
	if err != nil {
		return nil, err
	}

	session.Plan = plan
	return plan, nil
}

// createConfirmation 创建确认流程
func (s *OrchestrationService) createConfirmation(ctx context.Context, session *OrchestrationSession, plan *orchestration.TeamPlan) (*orchestration.ConfirmationRequest, error) {
	session.Status = "confirming"
	session.UpdatedAt = time.Now()

	// 创建执行方案
	proposal := s.buildProposal(session, plan)

	confirmation, err := s.confirmFlow.CreateConfirmationRequest(ctx, session.ID, session.UserID, proposal)
	if err != nil {
		return nil, err
	}

	return confirmation, nil
}

// buildProposal 构建执行方案
func (s *OrchestrationService) buildProposal(session *OrchestrationSession, plan *orchestration.TeamPlan) *orchestration.ExecutionProposal {
	// 转换任务为执行步骤
	steps := make([]*orchestration.ExecutionStep, len(plan.Tasks))
	for i, task := range plan.Tasks {
		steps[i] = &orchestration.ExecutionStep{
			ID:          task.ID,
			Order:       i,
			Name:        task.Title,
			Description: task.Description,
			AssignedTo:  task.AssignedTo,
			Duration:    task.Estimated,
			Dependencies: task.Dependencies,
		}
	}

	// 转换角色为团队成员
	members := make([]*orchestration.TeamMember, len(plan.Roles))
	for i, role := range plan.Roles {
		members[i] = &orchestration.TeamMember{
			Role:   role.Role,
			Skills: role.Skills,
			Model:  role.LLMModel,
		}
	}

	return &orchestration.ExecutionProposal{
		TaskSummary:       session.UserMessage,
		TaskType:          session.Analysis.TaskType,
		Priority:          session.Analysis.Priority,
		EstimatedDuration: plan.EstimatedTime,
		TeamConfig:        &orchestration.TeamConfiguration{Members: members},
		ExecutionPlan:     steps,
	}
}

// autoExecute 自动执行
func (s *OrchestrationService) autoExecute(session *OrchestrationSession, confirmation *orchestration.ConfirmationRequest) (*OrchestrationResponse, error) {
	// 标记为已批准
	confirmation.State = orchestration.StateApproved
	confirmation.Response = &orchestration.ConfirmationResponse{
		Action:     orchestration.ActionApprove,
		Comment:    "Auto-approved by high confidence",
		RespondedAt: time.Now(),
	}

	return s.executePlan(session.Context, session)
}

// executePlan 执行计划
func (s *OrchestrationService) executePlan(ctx context.Context, session *OrchestrationSession) (*OrchestrationResponse, error) {
	session.Status = "executing"
	session.UpdatedAt = time.Now()

	// 转换为团队配置
	teamConfig := s.teamPlanner.ConvertToTeamConfig(session.Plan)

	// 创建团队
	team, err := s.teamOrchestrator.CreateTeamFromPlan(&orchestration.OrchestratorTeamPlan{
		ID:          uuid.New().String(),
		Name:        teamConfig.Team.Name,
		Description: teamConfig.Team.Description,
		Goal:        session.Analysis.Description,
		Strategy:    "sequential",
		Members:     convertMembersToPlan(teamConfig.Members),
		Tasks:       convertTasksToPlan(teamConfig.Tasks),
		Config:      teamConfig.Team.Config,
		CreatedAt:   time.Now(),
	})
	if err != nil {
		return s.errorResponse(session, fmt.Sprintf("创建团队失败: %v", err)), nil
	}

	// 启动执行
	if err := s.teamOrchestrator.StartExecution(team.ID); err != nil {
		return s.errorResponse(session, fmt.Sprintf("启动执行失败: %v", err)), nil
	}

	// 异步监控执行
	go s.monitorExecution(session, team.ID)

	return &OrchestrationResponse{
		SessionID: session.ID,
		Status:    "executing",
		Stage:     "execution",
		Data: map[string]interface{}{
			"team_id":   team.ID,
			"team_name": team.Name,
			"message":   "团队已创建并开始执行",
		},
		CreatedAt: time.Now(),
	}, nil
}

// monitorExecution 监控执行
func (s *OrchestrationService) monitorExecution(session *OrchestrationSession, teamID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-session.Context.Done():
			return
		case <-ticker.C:
			// 检查执行状态
			report, err := s.teamOrchestrator.MonitorProgress(teamID)
			if err != nil {
				session.Error = err.Error()
				session.Status = "failed"
				session.UpdatedAt = time.Now()
				return
			}

			// 检查是否完成
			if report.TaskStatus.Completed == report.TaskStatus.Total {
				result, _ := s.teamOrchestrator.AggregateResults(teamID)
				session.Execution = result
				session.Status = "completed"
				session.UpdatedAt = time.Now()
				return
			}
		}
	}
}

// errorResponse 错误响应
func (s *OrchestrationService) errorResponse(session *OrchestrationSession, errMsg string) *OrchestrationResponse {
	session.Status = "failed"
	session.Error = errMsg
	session.UpdatedAt = time.Now()

	return &OrchestrationResponse{
		SessionID: session.ID,
		Status:    "error",
		Stage:     session.Status,
		Data:      errMsg,
		CreatedAt: time.Now(),
	}
}

// ==================== 辅助类型 ====================

// llmServiceAdapter LLM 服务适配器
type llmServiceAdapter struct {
	client llm.LLMClient
}

// Chat 实现 LLMClient 接口
func (a *llmServiceAdapter) Chat(ctx context.Context, messages []map[string]string, options map[string]interface{}) (map[string]interface{}, error) {
	// 转换消息格式
	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{
			Role:    msg["role"],
			Content: msg["content"],
		}
	}

	req := &llm.ChatRequest{
		Messages:    llmMessages,
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   4000,
	}

	if temp, ok := options["temperature"].(float64); ok {
		req.Temperature = temp
	}

	resp, err := a.client.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	// 转换响应格式
	return map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"content": resp.Choices[0].Message.Content,
				},
			},
		},
		"usage": resp.Usage,
	}, nil
}

// convertMembersToPlan 转换成员为计划格式
func convertMembersToPlan(members []models.TeamMember) []orchestration.TeamMemberPlan {
	result := make([]orchestration.TeamMemberPlan, len(members))
	for i, m := range members {
		skills := make([]string, len(m.Capabilities))
		for j, c := range m.Capabilities {
			skills[j] = c.Name
		}
		result[i] = orchestration.TeamMemberPlan{
			ID:           m.ID,
			Name:         m.Name,
			Role:         m.Role,
			Capabilities: skills,
			LLMModel:     "gpt-4", // 默认模型
		}
	}
	return result
}

// convertTasksToPlan 转换任务为计划格式
func convertTasksToPlan(tasks []models.Task) []orchestration.TaskPlanItem {
	result := make([]orchestration.TaskPlanItem, len(tasks))
	for i, t := range tasks {
		depIDs := make([]string, len(t.Dependencies))
		for j, d := range t.Dependencies {
			depIDs[j] = d.DependsOn
		}
		result[i] = orchestration.TaskPlanItem{
			ID:           t.ID,
			Title:        t.Title,
			Description:  t.Description,
			Type:         t.Type,
			Priority:     t.Priority,
			AssignedTo:   t.AssignedTo,
			Dependencies: depIDs,
			Estimated:    time.Duration(t.Estimated) * time.Minute,
		}
	}
	return result
}

// coordinatorServiceAdapter 协调器服务适配器
type coordinatorServiceAdapter struct {
	coordinator *TeamCoordinatorService
}

// agentExecutorAdapter Agent 执行器适配器
type agentExecutorAdapter struct {
	executor *AgentExecutor
}

// Execute 实现 AgentExecutorInterface
func (a *agentExecutorAdapter) Execute(ctx context.Context, req *orchestration.AgentExecRequest) (*orchestration.AgentExecResult, error) {
	innerReq := &TaskExecRequest{
		TaskID:          req.TaskID,
		TaskTitle:       req.TaskTitle,
		TaskDescription: req.TaskDescription,
		AgentName:       req.AgentName,
		AgentRole:       req.AgentRole,
		AgentPrompt:     req.AgentPrompt,
		ModelID:         req.ModelID,
		Context:         req.Context,
		TeamGoal:        req.TeamGoal,
	}

	result, err := a.executor.Execute(ctx, innerReq)
	if err != nil {
		return nil, err
	}

	return &orchestration.AgentExecResult{
		Success:    result.Success,
		Output:     result.Output,
		Summary:    result.Summary,
		ToolCalls:  result.ToolCalls,
		TokensUsed: result.TokensUsed,
	}, nil
}

// AssignTask 分配任务
func (a *coordinatorServiceAdapter) AssignTask(teamID, taskID, agentID string) error {
	return a.coordinator.AssignTask(taskID, agentID)
}

// CompleteTask 完成任务
func (a *coordinatorServiceAdapter) CompleteTask(teamID, taskID string, result map[string]interface{}) error {
	// TeamCoordinatorService 没有 CompleteTask 方法，我们使用 UpdateTaskStatus
	return a.coordinator.UpdateTaskStatus(taskID, "completed", "")
}

// FailTask 任务失败
func (a *coordinatorServiceAdapter) FailTask(teamID, taskID string, errMsg string) error {
	return a.coordinator.UpdateTaskStatus(taskID, "failed", errMsg)
}

// adjustPlan 方案调整
func (s *OrchestrationService) adjustPlan(ctx context.Context, session *OrchestrationSession, comment string) (*OrchestrationResponse, error) {
	if comment == "" {
		return nil, fmt.Errorf("adjustment comment is required")
	}

	session.Status = "adjusting"
	session.UpdatedAt = time.Now()

	// 用 LLM 根据用户反馈调整计划
	adjustPrompt := fmt.Sprintf(
		"用户对以下执行方案提出了调整意见，请根据反馈修改方案。\n\n原始任务: %s\n\n当前计划:\n",
		session.UserMessage,
	)

	if session.Plan != nil {
		for _, task := range session.Plan.Tasks {
			adjustPrompt += fmt.Sprintf("- [%s] %s (分配给: %s)\n", task.ID, task.Title, task.AssignedTo)
		}
	}

	adjustPrompt += fmt.Sprintf("\n用户调整意见: %s\n\n请分析用户意见，返回 JSON 格式的调整建议（包含 adjusted_tasks, removed_tasks, added_tasks 字段）。", comment)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个任务规划助手，根据用户反馈调整执行计划。"},
			{Role: "user", Content: adjustPrompt},
		},
		Model:       s.config.DefaultLLMModel,
		Temperature: 0.3,
		MaxTokens:   4000,
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return s.errorResponse(session, fmt.Sprintf("调整方案失败: %v", err)), nil
	}

	if len(resp.Choices) == 0 {
		return s.errorResponse(session, "调整方案失败: LLM 无响应"), nil
	}

	adjustedContent := resp.Choices[0].Message.Content

	return &OrchestrationResponse{
		SessionID:     session.ID,
		Status:        "adjusted",
		Stage:         "confirmation",
		Data:          adjustedContent,
		RequireAction: true,
		ActionType:    "approve",
		ActionPrompt:  adjustedContent + "\n\n调整后的方案如上，回复 'yes' 确认执行，或继续提出修改意见。",
		CreatedAt:     time.Now(),
	}, nil
}

// getExecutionProgress 获取实际执行进度
func (s *OrchestrationService) getExecutionProgress(session *OrchestrationSession) map[string]interface{} {
	progress := map[string]interface{}{
		"progress":    0,
		"total_tasks":  0,
		"completed":    0,
		"failed":       0,
		"running":      0,
		"pending":      0,
		"message":      "等待执行...",
	}

	if session.Plan == nil {
		return progress
	}

	totalTasks := len(session.Plan.Tasks)
	progress["total_tasks"] = totalTasks

	if session.Execution != nil {
		summary := session.Execution.TaskSummary
		progress["completed"] = summary.Completed
		progress["failed"] = summary.Failed
		progress["running"] = 0
		progress["pending"] = totalTasks - summary.Completed - summary.Failed

		if totalTasks > 0 {
			pct := float64(summary.Completed) / float64(totalTasks) * 100
			progress["progress"] = pct
		}

		if summary.Completed+summary.Failed >= totalTasks {
			progress["message"] = "执行完成"
		} else {
			progress["message"] = fmt.Sprintf("正在执行中... (%d/%d)", summary.Completed+summary.Failed, totalTasks)
		}
	} else if session.Status == "executing" {
		// 执行中但还没有结果
		progress["progress"] = 10
		progress["pending"] = totalTasks
		progress["running"] = 1
		progress["message"] = "团队创建中，准备执行..."
	}

	return progress
}

// ==================== 持久化方法 ====================

// saveOrchestrationRecord 保存编排执行记录到数据库
func (s *OrchestrationService) saveOrchestrationRecord(session *OrchestrationSession, plan *StructuredPlan) error {
	if s.db == nil {
		return nil
	}

	record := &models.OrchestrationRecord{
		ID:          uuid.New().String(),
		SessionID:   session.ID,
		UserID:      session.UserID,
		UserMessage: session.UserMessage,
		Status:      session.Status,
		StartedAt:   session.CreatedAt,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if session.Analysis != nil {
		record.TaskType = session.Analysis.TaskType
		record.Complexity = session.Analysis.Complexity
	}

	if plan != nil {
		planJSON, _ := json.Marshal(plan)
		record.PlanJSON = string(planJSON)
	}

	if len(session.Checkpoints) > 0 {
		cpJSON, _ := json.Marshal(session.Checkpoints)
		record.CheckpointsJSON = string(cpJSON)
	}

	if session.Status == "completed" || session.Status == "failed" || session.Status == "cancelled" {
		now := time.Now()
		record.CompletedAt = &now
		record.DurationMs = time.Since(session.CreatedAt).Milliseconds()
	}

	if session.Error != "" {
		record.Error = session.Error
	}

	return s.db.Create(record).Error
}

// saveSubtaskRecord 保存子任务执行记录
func (s *OrchestrationService) saveSubtaskRecord(sessionID, orchestrationID string, phaseIdx int, subtask *Subtask, output, errMsg string, success bool, duration time.Duration) error {
	if s.db == nil {
		return nil
	}

	status := "completed"
	if !success {
		status = "failed"
	}

	record := &models.SubtaskExecutionRecord{
		ID:              uuid.New().String(),
		OrchestrationID: orchestrationID,
		SessionID:       sessionID,
		SubtaskID:       subtask.ID,
		PhaseIndex:      phaseIdx,
		Title:           subtask.Title,
		Description:     subtask.Description,
		Type:            subtask.Type,
		Status:          status,
		Output:          output,
		Error:           errMsg,
		StartedAt:       time.Now().Add(-duration),
		DurationMs:      duration.Milliseconds(),
		CreatedAt:       time.Now(),
	}

	if success {
		now := time.Now()
		record.CompletedAt = &now
	}

	return s.db.Create(record).Error
}

// updateOrchestrationRecord 更新编排记录状态
func (s *OrchestrationService) updateOrchestrationRecord(session *OrchestrationSession) error {
	if s.db == nil {
		return nil
	}

	var record models.OrchestrationRecord
	if err := s.db.Where("session_id = ?", session.ID).Order("created_at DESC").First(&record).Error; err != nil {
		return err
	}

	record.Status = session.Status
	record.UpdatedAt = time.Now()

	if len(session.Checkpoints) > 0 {
		cpJSON, _ := json.Marshal(session.Checkpoints)
		record.CheckpointsJSON = string(cpJSON)
	}

	if session.Status == "completed" || session.Status == "failed" || session.Status == "cancelled" {
		now := time.Now()
		record.CompletedAt = &now
		record.DurationMs = time.Since(session.CreatedAt).Milliseconds()
	}

	if session.Error != "" {
		record.Error = session.Error
	}

	return s.db.Save(&record).Error
}

// GetOrchestrationHistory 获取用户的编排执行历史
func (s *OrchestrationService) GetOrchestrationHistory(userID string, limit int) ([]models.OrchestrationRecord, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	var records []models.OrchestrationRecord
	query := s.db.Where("user_id = ?", userID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// GetSubtaskRecords 获取子任务执行记录
func (s *OrchestrationService) GetSubtaskRecords(sessionID string) ([]models.SubtaskExecutionRecord, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	var records []models.SubtaskExecutionRecord
	if err := s.db.Where("session_id = ?", sessionID).Order("phase_index, created_at").Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}
