package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/tools"
)

// PlanApprover 规划审批回调：返回 true 表示批准执行
type PlanApprover func(plan *Plan) bool

// Orchestrator 编排器 - 协调内核与外部系统
type Orchestrator struct {
	kernel      kernel.Kernel
	llmGateway  kernel.LLMProvider
	toolExec    kernel.ToolExecutor
	memory      kernel.Memory
	sessions    kernel.SessionStore
	compressor  kernel.ContextCompressor
	permission  kernel.PermissionChecker
	knowledge   kernel.KnowledgeCollector
	approver    PlanApprover // 规划审批回调（nil = 自动批准）
	team        *Team        // 多 Agent 团队（可选）
}

// NewOrchestrator 创建编排器
func NewOrchestrator(
	k kernel.Kernel,
	llm kernel.LLMProvider,
	tools kernel.ToolExecutor,
	mem kernel.Memory,
	sessions kernel.SessionStore,
) *Orchestrator {
	return &Orchestrator{
		kernel:     k,
		llmGateway: llm,
		toolExec:   tools,
		memory:     mem,
		sessions:   sessions,
	}
}

// SetContextCompressor 设置上下文压缩器
func (o *Orchestrator) SetContextCompressor(c kernel.ContextCompressor) {
	o.compressor = c
}

// SetPermissionChecker 设置权限检查器
func (o *Orchestrator) SetPermissionChecker(p kernel.PermissionChecker) {
	o.permission = p
}

// SetKnowledgeCollector 设置知识收集器
func (o *Orchestrator) SetKnowledgeCollector(kc kernel.KnowledgeCollector) {
	o.knowledge = kc
}

// SetPlanApprover 设置规划审批回调（用于交互式确认）
func (o *Orchestrator) SetPlanApprover(approver PlanApprover) {
	o.approver = approver
}

// SetTeam 设置多 Agent 团队
func (o *Orchestrator) SetTeam(t *Team) {
	o.team = t
}

// PreviewPlan 仅规划不执行，返回拆分后的计划（用于交互式确认）
func (o *Orchestrator) PreviewPlan(ctx context.Context, content string) (*Plan, error) {
	planner := NewPlanner(o.llmGateway)
	return planner.Plan(ctx, content)
}

// DeepPlanResult 深度规划完整结果
type DeepPlanResult struct {
	Research  *ResearchReport
	Proposals *Proposals
	Plan      *Plan
	Chosen    *Proposal
}

// DeepPlan 深度规划：研究 → 方案分析 → 生成计划（不含选择，调用方负责选择方案）
func (o *Orchestrator) DeepPlan(ctx context.Context, content string) (*DeepPlanResult, error) {
	planner := NewPlanner(o.llmGateway)

	// Phase 1: Research
	research, err := planner.Research(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("research phase failed: %w", err)
	}

	// Phase 2: Propose alternatives
	proposals, err := planner.Propose(ctx, content, research)
	if err != nil {
		return nil, fmt.Errorf("propose phase failed: %w", err)
	}

	return &DeepPlanResult{
		Research:  research,
		Proposals: proposals,
	}, nil
}

// DeepPlanFinalize 用户选择方案后，生成详细计划
func (o *Orchestrator) DeepPlanFinalize(ctx context.Context, content string, result *DeepPlanResult, choiceIndex int) (*Plan, error) {
	if choiceIndex < 0 || choiceIndex >= len(result.Proposals.Options) {
		return nil, fmt.Errorf("invalid choice: %d", choiceIndex)
	}
	chosen := &result.Proposals.Options[choiceIndex]
	result.Chosen = chosen

	planner := NewPlanner(o.llmGateway)
	plan, err := planner.PlanWithApproach(ctx, content, result.Research, chosen)
	if err != nil {
		return nil, fmt.Errorf("plan phase failed: %w", err)
	}

	result.Plan = plan
	return plan, nil
}

// ProcessQuery 处理用户查询 — LLM 自动判断是否需要拆分任务
func (o *Orchestrator) ProcessQuery(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (*kernel.Response, error) {
	planner := NewPlanner(o.llmGateway)
	plan, err := planner.Plan(ctx, content)
	if err == nil && len(plan.Subtasks) > 1 {
		// 审批门：需要用户确认规划
		if o.approver != nil && !o.approver(plan) {
			return &kernel.Response{
				Content:   "规划已取消。",
				ToolCalls: 0,
			}, nil
		}
		return o.executePlan(ctx, userID, projectID, content, plan, opts)
	}

	// LLM 判断不需要拆分（返回1个子任务）→ 直接执行
	return o.processSingle(ctx, userID, projectID, content, opts)
}

// processSingle 单步执行（原有逻辑）
func (o *Orchestrator) processSingle(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (*kernel.Response, error) {
	start := time.Now()

	// 1. 获取或创建会话
	session, err := o.getOrCreateSession(ctx, projectID, userID)
	if err != nil {
		return nil, fmt.Errorf("session error: %w", err)
	}

	// 2. 构建查询
	query := &kernel.Query{
		SessionID: session.ID,
		Content:   content,
		UserID:    userID,
		ProjectID: projectID,
		Options:   opts,
	}

	// 3. 权限检查（前置）
	if o.permission != nil {
		allowed, reason := o.permission.Check(ctx, "query.process", "kernel", map[string]interface{}{
			"user_id":    userID,
			"project_id": projectID,
			"content":    content,
		})
		if !allowed {
			return &kernel.Response{
				Content:  fmt.Sprintf("请求被拒绝: %s", reason),
				Duration: time.Since(start),
			}, nil
		}
	}

	// 注入知识库到 context（供工具 handler 使用）
	if o.knowledge != nil {
		ctx = tools.WithKnowledge(ctx, o.knowledge)
	}

	// 4. 调用内核处理
	resp, err := o.kernel.Process(ctx, query)
	if err != nil {
		slog.Error("Kernel process failed", "error", err, "session", session.ID)
		return nil, err
	}

	// 5. 保存到记忆
	if o.memory != nil {
		go o.saveInteraction(ctx, session.ID, content, resp.Content)
	}

	slog.Info("Query processed", "session", session.ID, "duration", time.Since(start), "tokens", resp.TokensUsed)
	return resp, nil
}

// ProcessQueryStream 流式处理用户查询
func (o *Orchestrator) ProcessQueryStream(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (<-chan kernel.StreamChunk, error) {
	// 获取或创建会话
	session, err := o.getOrCreateSession(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}

	query := &kernel.Query{
		SessionID: session.ID,
		Content:   content,
		UserID:    userID,
		ProjectID: projectID,
		Options:   opts,
	}

	// 注入知识库到 context
	if o.knowledge != nil {
		ctx = tools.WithKnowledge(ctx, o.knowledge)
	}

	return o.kernel.ProcessStream(ctx, query)
}

// CreateSession 创建会话
func (o *Orchestrator) CreateSession(ctx context.Context, projectID, userID string) (*kernel.Session, error) {
	if o.sessions == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	return o.sessions.Create(ctx, projectID, userID)
}

// DeleteSession 删除会话
func (o *Orchestrator) DeleteSession(ctx context.Context, sessionID string) error {
	if o.sessions == nil {
		return fmt.Errorf("session store not configured")
	}
	return o.sessions.Delete(ctx, sessionID)
}

// GetSessionHistory 获取会话历史
func (o *Orchestrator) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]kernel.Message, error) {
	if o.sessions == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	session, err := o.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(session.Messages) > limit {
		return session.Messages[len(session.Messages)-limit:], nil
	}
	return session.Messages, nil
}

// ListSessions 列出会话
func (o *Orchestrator) ListSessions(ctx context.Context, projectID, userID string, limit, offset int) ([]*kernel.Session, error) {
	if o.sessions == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	return o.sessions.List(ctx, projectID, userID, limit, offset)
}

// SearchMemory 搜索记忆
func (o *Orchestrator) SearchMemory(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	if o.memory == nil {
		return nil, 0, fmt.Errorf("memory not configured")
	}
	return o.memory.Search(ctx, query, limit)
}

// CompressSession 压缩会话上下文
func (o *Orchestrator) CompressSession(ctx context.Context, sessionID string) error {
	if o.compressor == nil {
		return fmt.Errorf("compressor not configured")
	}

	messages, err := o.memory.Load(ctx, sessionID, 100)
	if err != nil {
		return err
	}

	compressed, saved, err := o.compressor.Compress(messages, 4000)
	if err != nil {
		return err
	}

	slog.Info("Session compressed", "session", sessionID, "saved_tokens", saved, "before", len(messages), "after", len(compressed))
	return nil
}

// GetToolDefinitions 获取工具定义列表
func (o *Orchestrator) GetToolDefinitions() []kernel.ToolDefinition {
	if o.toolExec == nil {
		return nil
	}
	return o.toolExec.GetDefinitions()
}

// GetSession 获取单个会话
func (o *Orchestrator) GetSession(ctx context.Context, sessionID string) (*kernel.Session, error) {
	if o.sessions == nil {
		return nil, fmt.Errorf("session store not configured")
	}
	return o.sessions.Get(ctx, sessionID)
}

// GetStats 获取系统统计
func (o *Orchestrator) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"kernel_state": o.kernel.GetState().String(),
	}

	if o.toolExec != nil {
		// 工具数量
		if registry, ok := o.toolExec.(interface{ Count() int }); ok {
			stats["tool_count"] = registry.Count()
		}
	}

	return stats
}

// ============ 内部方法 ============

func (o *Orchestrator) getOrCreateSession(ctx context.Context, projectID, userID string) (*kernel.Session, error) {
	// 列出已有会话
	if o.sessions != nil {
		sessions, err := o.sessions.List(ctx, projectID, userID, 1, 0)
		if err == nil && len(sessions) > 0 {
			return sessions[0], nil
		}
	}

	// 创建新会话
	if o.sessions != nil {
		return o.sessions.Create(ctx, projectID, userID)
	}

	return nil, fmt.Errorf("session store not configured")
}

func (o *Orchestrator) saveInteraction(ctx context.Context, sessionID, userContent, assistantContent string) {
	messages := []kernel.Message{
		{Role: "user", Content: userContent},
		{Role: "assistant", Content: assistantContent},
	}
	if err := o.memory.Save(ctx, sessionID, messages); err != nil {
		slog.Warn("Failed to save interaction", "error", err)
	}
}

// ============ 增强能力集成 ============

// EnhancedOrchestrator 增强编排器（带反思、学习、模式检测）
type EnhancedOrchestrator struct {
	*Orchestrator
	reflection      kernel.Reflection
	learner         kernel.Learner
	patternDetector kernel.PatternDetector
}

// NewEnhancedOrchestrator 创建增强编排器
func NewEnhancedOrchestrator(base *Orchestrator) *EnhancedOrchestrator {
	return &EnhancedOrchestrator{Orchestrator: base}
}

// SetReflection 设置反思能力
func (e *EnhancedOrchestrator) SetReflection(r kernel.Reflection) {
	e.reflection = r
}

// SetLearner 设置学习能力
func (e *EnhancedOrchestrator) SetLearner(l kernel.Learner) {
	e.learner = l
}

// SetPatternDetector 设置模式检测器
func (e *EnhancedOrchestrator) SetPatternDetector(pd kernel.PatternDetector) {
	e.patternDetector = pd
}

// ProcessQuery 处理查询（带增强能力）
func (e *EnhancedOrchestrator) ProcessQuery(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (*kernel.Response, error) {
	// 调用基础编排
	resp, err := e.Orchestrator.ProcessQuery(ctx, userID, projectID, content, opts)
	if err != nil {
		return nil, err
	}

	// 异步执行增强能力
	go e.enhance(ctx, userID, projectID, content, resp)

	return resp, nil
}

func (e *EnhancedOrchestrator) enhance(ctx context.Context, userID, projectID, query string, resp *kernel.Response) {
	// 获取会话历史用于模式检测
	if e.patternDetector != nil && e.memory != nil {
		sessions, _ := e.sessions.List(ctx, projectID, userID, 1, 0)
		if len(sessions) > 0 {
			messages, _ := e.memory.Load(ctx, sessions[0].ID, 50)
			if len(messages) > 0 {
				patterns, _ := e.patternDetector.Detect(ctx, sessions[0].ID, messages)
				if len(patterns) > 0 {
					slog.Debug("Patterns detected", "count", len(patterns))
				}
			}
		}
	}

	// 学习
	if e.learner != nil {
		record := kernel.ExecutionRecord{
			Query:     query,
			Response:  resp.Content,
			Success:   resp.Error == "",
			Duration:  int64(resp.Duration),
		}
		if err := e.learner.Learn(ctx, record); err != nil {
			slog.Warn("Learning failed", "error", err)
		}
	}
}

// executePlan 完整任务生命周期：执行(TDD) → 测试 → 验收（多 Agent 角色分工）
func (o *Orchestrator) executePlan(ctx context.Context, userID, projectID, content string, plan *Plan, opts kernel.QueryOptions) (*kernel.Response, error) {
	var results []string
	totalTools := 0

	// 角色提示词注入
	coderPrompt := o.rolePrompt("coder")
	reviewerPrompt := o.rolePrompt("reviewer")
	executorPrompt := o.rolePrompt("executor")

	// Phase 1: 执行 — 逐个完成子任务（程序员角色 + TDD）
	for i, st := range plan.Subtasks {
		tddInstruction := "\n\n## TDD 原则：请先编写测试用例，确认测试失败后再实现功能。实现后运行测试确认通过。"
		subQuery := fmt.Sprintf("%s## 总体目标: %s\n## 当前步骤 (%d/%d): %s\n## 具体要求: %s\n%s\n\n请完成此步骤的任务。",
			coderPrompt, plan.Goal, i+1, len(plan.Subtasks), st.Title, st.Description, tddInstruction)

		if i > 0 {
			subQuery += fmt.Sprintf("\n\n## 已完成的步骤结果:\n%s", strings.Join(results, "\n"))
		}

		resp, err := o.processSingle(ctx, userID, projectID, subQuery, opts)
		if err != nil {
			results = append(results, fmt.Sprintf("❌ 步骤%d(%s)失败: %v", st.ID, st.Title, err))
			continue
		}
		results = append(results, fmt.Sprintf("### 步骤%d: %s\n%s", st.ID, st.Title, resp.Content))
		totalTools += resp.ToolCalls
	}

	execSummary := strings.Join(results, "\n\n---\n\n")

	// Phase 2: 测试验证（执行者角色）
	testQuery := fmt.Sprintf(`%s## 总体目标: %s

## 已完成的工作:
%s

## 你的任务：验证以上工作是否完整正确
1. 运行编译/构建命令，确认无报错
2. 运行测试，确认全部通过
3. 检查逻辑正确性和需求覆盖率
4. 发现问题请直接修复

请开始验证。`, executorPrompt, plan.Goal, execSummary)

	testResp, testErr := o.processSingle(ctx, userID, projectID, testQuery, opts)
	var testReport string
	if testErr != nil {
		testReport = fmt.Sprintf("⚠ 验证阶段出错: %v", testErr)
	} else {
		testReport = testResp.Content
		totalTools += testResp.ToolCalls
	}

	// Phase 3: 验收报告（审查者角色）
	reviewQuery := fmt.Sprintf(`%s## 总体目标: %s

## 执行结果:
%s

## 验证结果:
%s

## 你的任务：生成最终验收报告
用简洁的语言总结：
1. **完成了什么** — 1-2句话
2. **修改了哪些文件** — 列出文件路径
3. **验证状态** — 是否通过测试
4. **遗留问题** — 如果没有就写"无"

输出格式：Markdown，不要用代码块包裹。`, reviewerPrompt, plan.Goal, execSummary, testReport)

	reviewResp, reviewErr := o.processSingle(ctx, userID, projectID, reviewQuery, opts)
	var finalReport string
	if reviewErr != nil {
		finalReport = fmt.Sprintf("## %s\n\n### 执行结果\n%s\n\n### 验证结果\n%s\n\n### 验收报告\n生成失败: %v",
			plan.Goal, execSummary, testReport, reviewErr)
	} else {
		finalReport = fmt.Sprintf("## %s\n\n### 验证结果\n%s\n\n---\n\n### 验收报告\n%s",
			plan.Goal, testReport, reviewResp.Content)
		totalTools += reviewResp.ToolCalls
	}

	return &kernel.Response{
		Content:    finalReport,
		ToolCalls:  totalTools,
		TokensUsed: 0,
		Duration:   0,
	}, nil
}

// rolePrompt 获取指定角色的系统提示词（用于多 Agent 分工）
func (o *Orchestrator) rolePrompt(roleName string) string {
	if o.team == nil {
		return ""
	}
	role := o.team.GetRole(roleName)
	if role == nil {
		return ""
	}
	return fmt.Sprintf("## 你的角色: %s\n%s\n\n## 可用工具: %s\n",
		role.Name, role.Prompt, strings.Join(role.Tools, ", "))
}
