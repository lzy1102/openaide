package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/projectmind"
)

// PlanApprover 规划审批回调：返回 true 表示批准执行
type PlanApprover func(plan *Plan) bool

// Orchestrator 编排器 - 协调内核与外部系统
type Orchestrator struct {
	kernel     kernel.Kernel
	llmGateway kernel.LLMProvider
	toolExec   kernel.ToolExecutor
	memory     kernel.Memory
	sessions   kernel.SessionStore
	compressor kernel.ContextCompressor
	permission kernel.PermissionChecker
	approver   PlanApprover // 规划审批回调（nil = 自动批准）
	team       *Team        // 多 Agent 团队（可选）

	// 可配置参数
	PreviewTimeout  time.Duration
	DeepTimeout     time.Duration
	subAgentTimeout time.Duration // 子代理超时;0 = 默认 60s

	// 项目持久记忆（跨会话积累）
	mind *projectmind.ProjectMind

	// 模型路由配置
	ModelRouting ModelRouting

	promptsDir string // 系统提示词目录（用于自适应更新）
	lang       string

	// 项目知识缓存（跨子Agent共享，避免重复读取）
	projectFacts string

	// OnProgress 进度回调（用于 TUI 报告子 Agent 执行状态）
	OnProgress func(phase, detail string)
}

// ModelRouting 按能力分配模型
type ModelRouting struct {
	Reasoning string `json:"reasoning" yaml:"reasoning"` // analyst/coder/reviewer
	Execution string `json:"execution" yaml:"execution"` // executor/classifier
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
		kernel:         k,
		llmGateway:     llm,
		toolExec:       tools,
		memory:         mem,
		sessions:       sessions,
		PreviewTimeout: 15 * time.Second,
		DeepTimeout:    120 * time.Second,
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

// SetPlanApprover 设置规划审批回调（用于交互式确认）
func (o *Orchestrator) SetPlanApprover(approver PlanApprover) {
	o.approver = approver
}

// SetTeam 设置多 Agent 团队
func (o *Orchestrator) SetTeam(t *Team) {
	o.team = t
}

// SetSubAgentTimeout 覆盖子代理超时(测试或特殊场景)。
func (o *Orchestrator) SetSubAgentTimeout(d time.Duration) { o.subAgentTimeout = d }

// SetProjectMind 设置项目持久记忆
func (o *Orchestrator) SetProjectMind(pm *projectmind.ProjectMind) {
	o.mind = pm
	pm.ExpireOldFacts()
}

// GetProjectMind 获取项目记忆（供外部读取）
func (o *Orchestrator) GetProjectMind() *projectmind.ProjectMind {
	return o.mind
}

// GetToolExecutor 返回工具执行器（供外部规划器使用）
func (o *Orchestrator) GetToolExecutor() kernel.ToolExecutor {
	return o.toolExec
}

// GetLLMProvider 返回 LLM 提供商（供外部规划器使用）
func (o *Orchestrator) GetLLMProvider() kernel.LLMProvider {
	return o.llmGateway
}

// CleanupOldSessions 清理过期会话（7天 TTL），防止子Agent会话堆积
func (o *Orchestrator) CleanupOldSessions(ctx context.Context) {
	if o.sessions == nil {
		return
	}
	deleted, err := o.sessions.CleanupOldSessions(ctx, 7*24*time.Hour)
	if err != nil {
		slog.Warn("Session cleanup failed", "error", err)
	} else if deleted > 0 {
		slog.Info("Cleaned up old sessions", "count", deleted)
	}
}

// PreviewPlan 仅规划不执行，返回拆分后的计划（用于交互式确认）
func (o *Orchestrator) PreviewPlan(ctx context.Context, content string) (*Plan, error) {
	slog.Info("PreviewPlan start", "query", content[:min(80, len(content))])
	planner := NewPlanner(o.llmGateway)
	planner.SetToolExecutor(o.toolExec)
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
	planner.SetToolExecutor(o.toolExec)

	// 注入已有项目知识
	if o.mind != nil {
		facts := o.mind.FactsForPrompt()
		risks := o.mind.RisksForPlanning()
		if facts != "" {
			content = facts + "\n\n## 当前任务\n" + content
		}
		if risks != "" {
			content += "\n\n## 已知风险（请在计划中考虑）\n" + risks
		}
	}

	// Phase 1: Research
	research, err := planner.Research(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("research phase failed: %w", err)
	}
	// 缓存研究发现 + 写入 ProjectMind
	o.projectFacts = fmt.Sprintf("模块: %s\n风险: %s\n发现: %s",
		research.Modules, research.Risks, research.Findings)
	if o.mind != nil {
		o.mind.AddCodeFact(research.Findings, "research discovered", nil, 0.7, "research")
	}

	// Phase 2: Propose alternatives (注入历史方案效果)
	if o.mind != nil {
		advice := o.mind.StrategyAdvice()
		if advice != "" {
			content += "\n\n" + advice
		}
	}
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

// ExecuteWithPlan 使用已有计划直接执行（跳过重新规划）
func (o *Orchestrator) ProcessQuery(ctx context.Context, userID, projectID, content string, opts kernel.QueryOptions) (*kernel.Response, error) {
	slog.Debug("ProcessQuery start", "user", userID, "project", projectID, "content_len", len(content))
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
	session, err := o.getOrCreateSession(ctx, projectID, userID, "")
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

	// 注入 ProjectMind 项目知识
	if o.mind != nil {
		query.Options.ProjectContext = o.mind.FactsForPrompt() + "\n" + o.mind.GenerateLearnedRules()
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

// ProcessQueryStream 流式处理用户查询。
// sessionID 非空时在指定会话中续聊（恢复历史上下文）；为空时使用该项目的最近会话。
func (o *Orchestrator) ProcessQueryStream(ctx context.Context, sessionID, userID, projectID, content string, opts kernel.QueryOptions) (<-chan kernel.StreamChunk, error) {
	slog.Info("ProcessQueryStream start", "user", userID, "session", sessionID, "content_len", len(content))
	// 获取或创建会话（指定会话优先，否则取项目最近会话）
	session, err := o.getOrCreateSession(ctx, projectID, userID, sessionID)
	if err != nil {
		return nil, err
	}

	// 注入 ProjectMind 项目知识
	if o.mind != nil {
		opts.ProjectContext = o.mind.FactsForPrompt() + "\n" + o.mind.GenerateLearnedRules()
	}

	query := &kernel.Query{
		SessionID: session.ID,
		Content:   content,
		UserID:    userID,
		ProjectID: projectID,
		Options:   opts,
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

	compressed, saved, err := o.compressor.Compress(ctx, messages, 4000)
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

// UpdateSession 保存会话修改（ESC undo 等场景）
func (o *Orchestrator) UpdateSession(ctx context.Context, session *kernel.Session) error {
	if o.sessions == nil {
		return fmt.Errorf("session store not configured")
	}
	return o.sessions.Update(ctx, session)
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

func (o *Orchestrator) getOrCreateSession(ctx context.Context, projectID, userID, sessionID string) (*kernel.Session, error) {
	// 指定会话优先：续聊场景恢复该会话的完整历史
	if sessionID != "" && o.sessions != nil {
		session, err := o.sessions.Get(ctx, sessionID)
		if err == nil && session != nil {
			return session, nil
		}
	}

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
