package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/projectmind"
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

	// 可配置参数
	PreviewTimeout time.Duration
	DeepTimeout    time.Duration

	// 项目持久记忆（跨会话积累）
	mind *projectmind.ProjectMind

	// 模型路由配置
	ModelRouting ModelRouting

	// 项目知识缓存（跨子Agent共享，避免重复读取）
	projectFacts string
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
	if store, ok := o.sessions.(*kernel.FileSessionStore); ok {
		deleted, err := store.CleanupOldSessions(ctx, 7*24*time.Hour)
		if err != nil {
			slog.Warn("Session cleanup failed", "error", err)
		} else if deleted > 0 {
			slog.Info("Cleaned up old sessions", "count", deleted)
		}
	}
}

// RunSubAgent 在隔离的临时会话中运行指定角色，只回传结果摘要
// 主 Agent 上下文不会被子 Agent 的工具调用污染
func (o *Orchestrator) RunSubAgent(ctx context.Context, userID, projectID, roleName, task string, previousResults []string) (string, error) {
	if o.team == nil {
		return "", fmt.Errorf("team not configured")
	}
	role := o.team.GetRole(roleName)
	if role == nil {
		return "", fmt.Errorf("role not found: %s", roleName)
	}

	// 构建子 Agent 的输入：角色定义 + 前置结果 + 当前任务
	var input strings.Builder
	input.WriteString(fmt.Sprintf("## 你的角色: %s\n%s\n\n## 可用工具: %s\n\n",
		role.Name, role.Prompt, strings.Join(role.Tools, ", ")))
	if len(previousResults) > 0 {
		input.WriteString("## 前置步骤结果:\n")
		for _, r := range previousResults {
			input.WriteString(r + "\n")
		}
		input.WriteString("\n")
	}
	if o.projectFacts != "" {
		input.WriteString("## 项目已知信息（无需重复读取）\n")
		input.WriteString(o.projectFacts)
		input.WriteString("\n\n")
	}
	if o.mind != nil {
		conventions := o.mind.ConventionsForPrompt()
		if conventions != "" { input.WriteString(conventions + "\n\n") }
		failures := o.mind.RecentFailures()
		if failures != "" { input.WriteString(failures + "\n\n") }
	}
	input.WriteString(fmt.Sprintf("## 当前任务\n%s\n\n请完成此任务，输出你的工作结果。", task))

	// 使用唯一 userID 创建真正隔离的临时会话
	sessionID := fmt.Sprintf("%s-%s-%d", userID, roleName, time.Now().UnixNano())
	opts := kernel.QueryOptions{}
	if len(role.Tools) > 0 {
		opts.ToolFilter = role.Tools
	}
	if model := o.pickModel(roleName); model != "" {
		opts.ModelID = model
	}
	resp, err := o.processSingle(ctx, sessionID, projectID, input.String(), opts)
	if err != nil {
		return "", fmt.Errorf("sub-agent %s failed: %w", roleName, err)
	}
	return resp.Content, nil
}

// PreviewPlan 仅规划不执行，返回拆分后的计划（用于交互式确认）
func (o *Orchestrator) PreviewPlan(ctx context.Context, content string) (*Plan, error) {
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
		o.extractFactsFromResearch(research)
	}

	// Phase 2: Propose alternatives (注入历史方案效果)
	if o.mind != nil {
		advice := o.mind.StrategyAdvice()
		if advice != "" { content += "\n\n" + advice }
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
func (o *Orchestrator) ExecuteWithPlan(ctx context.Context, userID, projectID, content string, plan *Plan, opts kernel.QueryOptions) (*kernel.Response, error) {
	return o.executePlan(ctx, userID, projectID, content, plan, opts)
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

// executePlan 智能路由多 Agent 执行：LLM 根据任务类型自动选择角色组合
func (o *Orchestrator) executePlan(ctx context.Context, userID, projectID, content string, plan *Plan, opts kernel.QueryOptions) (*kernel.Response, error) {
	// 智能路由：LLM 根据任务类型和子任务内容选择最佳角色管线
	pipeline := o.routePipeline(ctx, plan)
	var results []string
	totalTools := 0

	// Phase 1: 执行 — 按依赖分组，组内并行、组间串行
	groups := groupByDependency(plan.Subtasks)
	results = make([]string, len(plan.Subtasks))
	for _, group := range groups {
		var wg sync.WaitGroup
		for _, st := range group {
			wg.Add(1)
			go func(st SubTask) {
				defer wg.Done()
				roleName := o.assignRole(ctx, pipeline, st)
				// 只传递已完成的依赖结果
				var deps []string
				for _, depID := range st.DependsOn {
					if results[depID-1] != "" {
						deps = append(deps, results[depID-1])
					}
				}
				task := fmt.Sprintf("总体目标: %s\n当前步骤: %s\n具体要求: %s\n\nTDD 原则：涉及代码修改时请先编写测试用例再实现。",
					plan.Goal, st.Title, st.Description)
				content, err := o.RunSubAgent(ctx, userID, projectID, roleName, task, deps)
				if err != nil {
					results[st.ID-1] = fmt.Sprintf("❌ 步骤%d(%s)失败: %v", st.ID, st.Title, err)
					return
				}
				results[st.ID-1] = fmt.Sprintf("### 步骤%d: %s [%s]\n%s", st.ID, st.Title, roleName, content)
			}(st)
		}
		wg.Wait()
		totalTools += len(group)
	}

	execSummary := strings.Join(results, "\n\n---\n\n")

	// Phase 2: 根据管线决定是否需要测试和审查
	var testReport, finalReport string

	if pipelineHas(pipeline, "executor") {
		verifyTask := fmt.Sprintf("总体目标: %s\n\n已完成的工作:\n%s\n\n验证完整性：编译/构建/测试/逻辑检查。发现问题直接修复。",
			plan.Goal, execSummary)
		testReport, _ = o.RunSubAgent(ctx, userID, projectID, "executor", verifyTask, results)
		totalTools++
	}
	if testReport == "" {
		testReport = "（跳过验证阶段）"
	}

	// 自反思闭环：最多重试 2 次
	const maxRetries = 2
	for retry := 0; retry <= maxRetries; retry++ {
		if pipelineHas(pipeline, "reviewer") {
			reviewTask := fmt.Sprintf("总体目标: %s\n\n执行结果:\n%s\n\n验证结果:\n%s\n\n生成验收报告：完成内容、修改文件、验证状态、遗留问题。\n如果存在未解决问题，在报告末尾标注 [需要返工] 并列出需要修复的具体问题。\nMarkdown 格式。",
				plan.Goal, execSummary, testReport)
			reviewContent, err := o.RunSubAgent(ctx, userID, projectID, "reviewer", reviewTask, nil)
			if err == nil {
				finalReport = fmt.Sprintf("## %s\n\n### 验证结果\n%s\n\n---\n\n### 验收报告\n%s",
					plan.Goal, testReport, reviewContent)
				totalTools++

				// 自反思：检查是否需要返工
				if retry < maxRetries && strings.Contains(reviewContent, "[需要返工]") {
					fixTask := fmt.Sprintf("验收发现问题，需要修复：\n\n%s\n\n请修复以上问题。", reviewContent)
					fixContent, fixErr := o.RunSubAgent(ctx, userID, projectID, "coder", fixTask, results)
					if fixErr == nil {
						results = append(results, fmt.Sprintf("### 返工修复 (第%d次)\n%s", retry+1, fixContent))
						execSummary = strings.Join(results, "\n\n---\n\n")
						totalTools++
						continue // 重新进入审查
					}
				}
			}
		}
		break // 审查通过或达到最大重试
	}

	if finalReport == "" {
		finalReport = fmt.Sprintf("## %s\n\n### 执行结果\n%s\n\n### 验证结果\n%s", plan.Goal, execSummary, testReport)
	}

	// 记录执行历史 + 自动学习项目约定
	if o.mind != nil {
		o.mind.RecordExecution(plan.Goal, strings.Join(pipeline, "+"), true,
			nil, nil, nil, 0, o.pickModel("coder"))
		o.mind.UpdateStrategy(strings.Join(pipeline, "+"), true, plan.Goal)
		// 从测试输出中学习
		if testReport != "" { o.mind.AnalyzeBuildError(testReport) }
		o.mind.SessionCount++
		o.mind.Save()
		// 同步 ProjectMind 事实到知识库（语义搜索可用）
		if o.knowledge != nil {
			o.mind.SyncToKnowledgeBase(o.knowledge)
		}
	}

	return &kernel.Response{
		Content:    finalReport,
		ToolCalls:  totalTools,
		TokensUsed: 0,
		Duration:   0,
	}, nil
}

// routePipeline 让 LLM 根据任务类型选择需要的角色管线
func (o *Orchestrator) routePipeline(ctx context.Context, plan *Plan) []string {
	if o.team == nil {
		return []string{"coder", "executor", "reviewer"}
	}

	subtaskDescs := make([]string, len(plan.Subtasks))
	for i, st := range plan.Subtasks {
		subtaskDescs[i] = fmt.Sprintf("%d. %s: %s", st.ID, st.Title, st.Description)
	}

	prompt := fmt.Sprintf(`分析以下任务，选择需要的角色（可多选）。

角色:
- analyst: 分析代码、研究问题（只读）
- coder: 编写/修改代码、实现功能
- executor: 运行测试、执行命令、验证
- reviewer: 审查代码质量、安全性、生成报告

任务目标: %s
子任务:
%s

规则:
- 纯分析/研究类任务 → analyst
- 涉及代码修改 → coder + executor
- 重要功能/安全相关 → coder + executor + reviewer
- 简单查询 → 单角色

只回复角色名称，逗号分隔。如: analyst,coder,executor,reviewer`, plan.Goal, strings.Join(subtaskDescs, "\n"))

	messages := []kernel.Message{
		{Role: "system", Content: "你是任务路由器。输出需要的角色名（逗号分隔）。"},
		{Role: "user", Content: prompt},
	}
	resp, err := o.llmGateway.Chat(ctx, messages, nil, map[string]interface{}{"max_tokens": 50, "temperature": 0})
	if err != nil {
		return []string{"coder", "executor", "reviewer"}
	}

	var roles []string
	for _, r := range strings.Split(strings.TrimSpace(resp.Content), ",") {
		r = strings.TrimSpace(r)
		if o.team.GetRole(r) != nil {
			roles = append(roles, r)
		}
	}
	if len(roles) == 0 {
		return []string{"analyst"}
	}
	return roles
}

// assignRole 让 LLM 从可用管线中选择最适合子任务的角色
func (o *Orchestrator) assignRole(ctx context.Context, pipeline []string, st SubTask) string {
	if len(pipeline) == 1 {
		return pipeline[0]
	}
	if o.team == nil {
		return pipeline[0]
	}

	var roleDescs []string
	for _, rn := range pipeline {
		if role := o.team.GetRole(rn); role != nil {
			roleDescs = append(roleDescs, fmt.Sprintf("- %s: %s", rn, role.Description))
		}
	}
	if len(roleDescs) == 0 {
		return pipeline[0]
	}

	prompt := fmt.Sprintf(`从以下角色中选择最合适的一个来完成子任务。

可用角色:
%s

子任务:
标题: %s
描述: %s
建议工具: %s

只回复角色名称（如 analyst/coder/executor/reviewer），不要解释。`, strings.Join(roleDescs, "\n"), st.Title, st.Description, st.ToolHints)

	messages := []kernel.Message{
		{Role: "user", Content: prompt},
	}
	resp, err := o.llmGateway.Chat(ctx, messages, nil, map[string]interface{}{"max_tokens": 20, "temperature": 0})
	if err != nil {
		return pipeline[0]
	}

	choice := strings.TrimSpace(resp.Content)
	if o.team.GetRole(choice) != nil {
		return choice
	}
	return pipeline[0]
}

func pipelineHas(pipeline []string, role string) bool {
	for _, r := range pipeline {
		if r == role {
			return true
		}
	}
	return false
}

// pickModel 根据角色选择对应能力的模型
func (o *Orchestrator) pickModel(role string) string {
	if o.ModelRouting.Reasoning == "" { return "" }
	switch role {
	case "analyst", "coder", "reviewer":
		return o.ModelRouting.Reasoning
	case "executor", "classifier":
		if o.ModelRouting.Execution != "" {
			return o.ModelRouting.Execution
		}
		return o.ModelRouting.Reasoning
	}
	return ""
}

// extractFactsFromResearch 从研究报告提取关键事实写入 ProjectMind
func (o *Orchestrator) extractFactsFromResearch(r *ResearchReport) {
	modules := strings.Split(r.Modules, ",")
	for _, m := range modules {
		m = strings.TrimSpace(m)
		if m != "" {
			o.mind.AddCodeFact(m, "research discovered", nil, 0.7, "research")
		}
	}
	if r.Risks != "" {
		for _, risk := range strings.Split(r.Risks, ",") {
			risk = strings.TrimSpace(risk)
			if risk != "" {
				o.mind.AddLearning("pitfall", risk)
			}
		}
	}
	o.mind.Save()
}

// handleDiscoveries 处理执行中发现的新信息
func (o *Orchestrator) handleDiscoveries(content string) (replanNeeded bool) {
	if o.mind == nil { return false }

	// 检测 DISCOVERY 信号
	discoveryPattern := "[DISCOVERY:"
	if idx := strings.Index(content, discoveryPattern); idx >= 0 {
		end := strings.Index(content[idx:], "]")
		if end > 0 {
			discovery := strings.TrimSpace(content[idx+len(discoveryPattern) : idx+end])
			o.mind.AddLearning("pattern", discovery)
			o.mind.Save()
		}
	}

	// 检测 REPLAN 信号
	if strings.Contains(content, "[REPLAN:") {
		return true
	}

	// 检测关键架构信息 → 自动记录
	if strings.Contains(content, "PostgreSQL") || strings.Contains(content, "MySQL") || strings.Contains(content, "SQLite") {
		o.mind.SetArchitecture("", o.mind.Architecture.Framework,
			extractDBType(content), o.mind.Architecture.KeyModules)
	}
	if strings.Contains(content, "framework") || strings.Contains(content, "框架") {
		if strings.Contains(content, "Gin") { o.mind.SetArchitecture("", "Gin", o.mind.Architecture.Database, nil) }
		if strings.Contains(content, "net/http") { o.mind.SetArchitecture("", "net/http", o.mind.Architecture.Database, nil) }
	}

	return false
}

func extractDBType(content string) string {
	for _, db := range []string{"PostgreSQL", "MySQL", "SQLite", "MongoDB", "Redis"} {
		if strings.Contains(content, db) { return db }
	}
	return ""
}

// groupByDependency 按依赖关系分组：同一组内的子任务可以并行执行
func groupByDependency(subtasks []SubTask) [][]SubTask {
	completed := make(map[int]bool)
	var groups [][]SubTask
	remaining := make([]SubTask, len(subtasks))
	copy(remaining, subtasks)

	for len(remaining) > 0 {
		var group []SubTask
		var nextRound []SubTask
		for _, st := range remaining {
			ready := true
			for _, depID := range st.DependsOn {
				if !completed[depID] {
					ready = false
					break
				}
			}
			if ready {
				group = append(group, st)
			} else {
				nextRound = append(nextRound, st)
			}
		}
		if len(group) == 0 {
			// 防止死循环：有循环依赖时全部作为一组
			group = remaining
			remaining = nil
		} else {
			for _, st := range group {
				completed[st.ID] = true
			}
		}
		groups = append(groups, group)
		remaining = nextRound
	}
	return groups
}

