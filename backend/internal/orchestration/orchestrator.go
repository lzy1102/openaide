package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

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

	// Agent 间共享工作区
	workspace *Workspace

	// 项目持久记忆（跨会话积累）
	mind *projectmind.ProjectMind

	// 模型路由配置
	ModelRouting ModelRouting

	promptsDir string // 系统提示词目录（用于自适应更新）
	lang      string

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
	slog.Info("SubAgent executing", "role", roleName, "task", task[:min(80, len(task))], "prev_results", len(previousResults))
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
	if o.workspace != nil {
		if ws := o.workspace.Summary(); ws != "" {
			input.WriteString(ws + "\n\n")
		}
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
		slog.Warn("SubAgent failed", "role", roleName, "error", err)
		return "", fmt.Errorf("sub-agent %s failed: %w", roleName, err)
	}
	if o.workspace != nil {
		o.workspace.Put(roleName+"_result", "result", resp.Content, roleName)
	}
	slog.Info("SubAgent completed", "role", roleName, "output_len", len(resp.Content), "tool_calls", resp.ToolCalls, "tokens", resp.TokensUsed)
	return resp.Content, nil
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
	slog.Info("ProcessQueryStream start", "user", userID, "content_len", len(content))
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


// Branch 执行分支——从主线分叉出来处理发现的问题，完成后收敛回主线
type Branch struct {
	Trigger  string   // 触发分支的原因 (DISCOVERY/ERROR/REVIEW_ISSUE)
	Task     string   // 分支任务描述
	Role     string   // 执行角色
	Result   string   // 分支执行结果
	Learnings []string // 从分支学到的经验
}

// executePlan 主线→分支→收敛 执行模型
func (o *Orchestrator) executePlan(ctx context.Context, userID, projectID, content string, plan *Plan, opts kernel.QueryOptions) (*kernel.Response, error) {
	roleMap := o.routePipeline(ctx, plan)
	var results []string
	var branches []Branch
	totalTools := 0
	hasExecutor := false
	hasReviewer := false
	for _, role := range roleMap {
		if role == "executor" { hasExecutor = true }
		if role == "reviewer" { hasReviewer = true }
	}

	// Phase 1: 执行 — 按依赖分组，组内并行、组间串行
	groups := groupByDependency(plan.Subtasks)
	results = make([]string, len(plan.Subtasks))
	for _, group := range groups {
		g, gCtx := errgroup.WithContext(ctx)
		for _, st := range group {
			g.Go(func() error {
				roleName := roleMap[st.ID-1]
				if roleName == "" { roleName = "coder" }
				// 只传递已完成的依赖结果
				var deps []string
				for _, depID := range st.DependsOn {
					if results[depID-1] != "" {
						deps = append(deps, results[depID-1])
					}
				}
				task := fmt.Sprintf("总体目标: %s\n当前步骤: %s\n具体要求: %s\n\nTDD 原则：涉及代码修改时请先编写测试用例再实现。",
					plan.Goal, st.Title, st.Description)
				if o.OnProgress != nil {
					o.OnProgress("execute", fmt.Sprintf("步骤%d/%d: %s [%s]", st.ID, len(plan.Subtasks), st.Title, roleName))
				}
				content, err := o.RunSubAgent(gCtx, userID, projectID, roleName, task, deps)
				if err != nil {
					results[st.ID-1] = fmt.Sprintf("❌ 步骤%d(%s)失败: %v", st.ID, st.Title, err)
					return err
				}
				results[st.ID-1] = fmt.Sprintf("### 步骤%d: %s [%s]\n%s", st.ID, st.Title, roleName, content)

				// 检测是否需要开启分支
				if needsBranch, branchTask := detectBranchSignal(content); needsBranch {
					slog.Info("Branch created", "trigger", branchTask[:min(50, len(branchTask))])
					branch := o.executeBranch(gCtx, userID, projectID, branchTask, results, &branches)
					branches = append(branches, branch)
					// 分支结果注入到当前步骤结果
					results[st.ID-1] += fmt.Sprintf("\n\n### 🔀 分支: %s\n%s", branch.Trigger, branch.Result)
					totalTools++
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			slog.Warn("Subtask group had errors", "error", err)
		}
		totalTools += len(group)
	}

	execSummary := strings.Join(results, "\n\n---\n\n")

	// Phase 2: 根据管线决定是否需要测试和审查
	// 自修正执行循环：测试→审查→修复→再测试→再审查，直到通过
	var testReport, finalReport string
	maxIterations := 5
	noProgressCount := 0
	previousIssueCount := -1

	for iteration := 0; iteration < maxIterations; iteration++ {
		// 测试验证
		if hasExecutor {
			if o.OnProgress != nil {
				o.OnProgress("verify", fmt.Sprintf("验证轮次 %d/%d", iteration+1, maxIterations))
			}
			verifyTask := fmt.Sprintf(`总体目标: %s

已完成的工作:
%s

## 验证规则（严格遵守）

你必须**实际执行命令**来验证，不能凭推理判断。以下行为**禁止**：
- ❌ "代码看起来正确" — 没跑过测试就不能说正确
- ❌ "逻辑上没问题" — 必须实际编译运行
- ❌ "应该能通过" — 没有"应该"，只有"实际通过了"或"失败了"
- ❌ 跳过测试直接报告"验证通过"

### 必须执行的步骤：
1. 用 execute_command 运行编译/构建命令
2. 用 execute_command 运行测试套件
3. **将命令的实际输出粘贴到报告中**（stdout + stderr）
4. 如果有失败，分析根因并尝试修复
5. 只有所有测试**实际通过**后才能报告"验证通过"

如果无法修复，输出具体的错误信息和失败命令。`, plan.Goal, execSummary)
			var verifyErr error
		testReport, verifyErr = o.RunSubAgent(ctx, userID, projectID, "executor", verifyTask, results)
		totalTools++
		if verifyErr != nil {
			testReport = fmt.Sprintf("验证执行失败: %v", verifyErr)
		}
	}
	if testReport == "" {
		testReport = "（跳过验证阶段）"
	}

		// 审查验收
		if !hasReviewer {
			finalReport = fmt.Sprintf("## %s\n\n### 执行结果\n%s\n\n### 验证结果\n%s", plan.Goal, execSummary, testReport)
			break
		}

		if o.OnProgress != nil {
			o.OnProgress("review", fmt.Sprintf("审查轮次 %d/%d", iteration+1, maxIterations))
		}
		reviewTask := fmt.Sprintf(`总体目标: %s

执行结果:
%s

验证结果:
%s

生成验收报告。格式:
## 验收报告
**状态**: [通过/需要返工]
**完成内容**: 1-2句话
**修改文件**: 列表
**遗留问题**: 如果没有写"无"

如果存在未解决问题，在报告开头写"**状态**: 需要返工"并列出具体问题。
如果全部通过，写"**状态**: 通过"。`, plan.Goal, execSummary, testReport)

		reviewContent, err := o.RunSubAgent(ctx, userID, projectID, "reviewer", reviewTask, nil)
		if err != nil {
			finalReport = fmt.Sprintf("## %s\n\n### 执行结果\n%s\n\n### 验证结果\n%s\n\n### 验收报告\n生成失败: %v", plan.Goal, execSummary, testReport, err)
			break
		}
		totalTools++

		// 检查是否需要返工
		if !strings.Contains(reviewContent, "需要返工") && !strings.Contains(reviewContent, "未通过") {
			finalReport = fmt.Sprintf("## %s\n\n### 验证结果\n%s\n\n---\n\n%s", plan.Goal, testReport, reviewContent)
			break // 通过了!
		}

		// 检测是否在进步
		currentIssueCount := strings.Count(reviewContent, "⚠") + strings.Count(reviewContent, "❌")
		if currentIssueCount >= previousIssueCount && previousIssueCount >= 0 {
			noProgressCount++
		} else {
			noProgressCount = 0
		}
		previousIssueCount = currentIssueCount

		if noProgressCount >= 2 {
			finalReport = fmt.Sprintf("## %s\n\n### 验证结果\n%s\n\n---\n\n%s\n\n⚠ 连续 %d 次迭代未减少问题，停止尝试。请人工介入。", plan.Goal, testReport, reviewContent, noProgressCount)
			break
		}

		// 提取问题 → 分析根因 → 生成修复方案 → 重新执行
		analyzeTask := fmt.Sprintf("验收发现以下问题，请分析根因并给出修复方案：\n\n%s\n\n之前的尝试如果失败过，请换一种方法。输出：1.根因分析 2.修复步骤(编号列表)", reviewContent)
		analysisContent, analyzeErr := o.RunSubAgent(ctx, userID, projectID, "analyst", analyzeTask, results)
		if analyzeErr != nil {
			continue
		}

		fixTask := fmt.Sprintf("根据以下分析修复问题：\n\n%s\n\n请逐一修复。如果某个问题无法修复，标注 [BLOCKED: 原因]。", analysisContent)
		fixContent, fixErr := o.RunSubAgent(ctx, userID, projectID, "coder", fixTask, results)
		if fixErr != nil {
			continue
		}

		// Lint/Repair: 自动运行 linter，错误反馈给 LLM 修复（Aider 风格）
		fixContent = o.lintRepairLoop(ctx, userID, projectID, fixContent, results)

		// Test Generation: 为修改生成测试 → 运行 → 失败则修复
		fixContent = o.testGenLoop(ctx, userID, projectID, fixContent, results)

		// 检测死胡同
		if strings.Contains(fixContent, "[BLOCKED:") {
			finalReport = fmt.Sprintf("## %s\n\n### 验证结果\n%s\n\n---\n\n%s\n\n### 阻塞\n%s\n\n任务遇到无法自动解决的问题，需要人工介入。", plan.Goal, testReport, reviewContent, fixContent)
			break
		}

		results = append(results, fmt.Sprintf("### 修复迭代 %d\n根因分析:\n%s\n\n修复:\n%s", iteration+1, analysisContent, fixContent))
		execSummary = strings.Join(results, "\n\n---\n\n")
		totalTools++

		// 学习: 记录修复经验
		if o.mind != nil {
			o.mind.AddLearning("pattern", fmt.Sprintf("修复经验: %s → %s", truncateForLearning(reviewContent), truncateForLearning(fixContent)))
			o.mind.Save()
		}
	}

	if finalReport == "" {
		finalReport = fmt.Sprintf("## %s\n\n### 执行结果\n%s\n\n### 验证结果\n%s\n\n⚠ 达到最大迭代次数(%d)，请人工检查。", plan.Goal, execSummary, testReport, maxIterations)
	}

	// 记录执行历史 + 自动学习
	if o.mind != nil {
		success := !strings.Contains(finalReport, "需要返工") && !strings.Contains(finalReport, "人工介入") && !strings.Contains(finalReport, "最大迭代")
		o.mind.RecordExecution(plan.Goal, fmt.Sprintf("%v", roleMap), success,
			nil, nil, nil, 0, o.pickModel("coder"))
		o.mind.UpdateStrategy(fmt.Sprintf("%v", roleMap), success, plan.Goal)
		if testReport != "" { o.mind.AnalyzeBuildError(testReport) }
		o.mind.SessionCount++
		o.mind.Save()
		if o.knowledge != nil {
			o.mind.SyncToKnowledgeBase(o.knowledge)
			if o.mind.HasHighConfidenceConventions() {
				o.mind.SyncToSystemPrompt(o.promptsDir, o.lang)
				// Auto-create skills from high-confidence conventions
				if ak, ok := o.kernel.(*kernel.AgentKernel); ok {
					o.mind.SyncConventionsToSkills(ak.GetSkillManager())
				}
			}
		}
	}

	return &kernel.Response{
		Content:    finalReport,
		ToolCalls:  totalTools,
		TokensUsed: 0,
		Duration:   0,
	}, nil
}

func truncateForLearning(s string) string {
	if len([]rune(s)) > 100 {
		return string([]rune(s)[:100]) + "..."
	}
	return s
}

// detectBranchSignal 检测子Agent输出中是否需要开启分支
func detectBranchSignal(content string) (bool, string) {
	for _, signal := range []string{"[DISCOVERY:", "[ISSUE:", "[BRANCH:", "[BLOCKED:"} {
		if idx := strings.Index(content, signal); idx >= 0 {
			end := strings.Index(content[idx:], "]")
			if end > 0 {
				task := strings.TrimSpace(content[idx+len(signal) : idx+end])
				return true, task
			}
		}
	}
	return false, ""
}

// executeBranch 执行一个分支：分析问题 → 生成方案 → 实施修复
func (o *Orchestrator) executeBranch(ctx context.Context, userID, projectID, trigger string, mainResults []string, branches *[]Branch) Branch {
	b := Branch{Trigger: trigger}

	// 1. 分析师分析问题
	analyzeTask := fmt.Sprintf("分析以下问题，找出根因：%s\n\n已完成的工作:\n%s\n\n输出: 1.根因 2.影响范围 3.建议修复方案", trigger, strings.Join(mainResults, "\n"))
	analysis, err := o.RunSubAgent(ctx, userID, projectID, "analyst", analyzeTask, nil)
	if err != nil {
		b.Result = fmt.Sprintf("分析失败: %v", err)
		return b
	}

	// 2. 程序员实施修复
	fixTask := fmt.Sprintf("根据分析修复问题：\n\n问题: %s\n分析: %s\n\n请实施修复。如果无法修复，标注 [BLOCKED: 原因]", trigger, analysis)
	fix, err := o.RunSubAgent(ctx, userID, projectID, "coder", fixTask, nil)
	if err != nil {
		b.Result = fmt.Sprintf("修复失败: %v", err)
		return b
	}

	// 3. 收敛：提取经验，回写主线
	b.Result = fmt.Sprintf("根因: %s\n修复: %s", analysis, fix)
	b.Learnings = []string{trigger + " → " + truncateForLearning(analysis)}

	// 记录到 ProjectMind
	if o.mind != nil {
		o.mind.AddLearning("pattern", b.Learnings[0])
		o.mind.Save()
	}

	return b
}

// routePipeline 让 LLM 根据任务类型选择需要的角色管线
// routePipeline 一次性为所有子任务分配角色（一次 LLM 调用替代 N+1 次）
func (o *Orchestrator) routePipeline(ctx context.Context, plan *Plan) map[int]string {
	result := make(map[int]string)
	if o.team == nil {
		for i := range plan.Subtasks { result[i] = "coder" }
		return result
	}

	// 构建子任务描述
	var descs strings.Builder
	for _, st := range plan.Subtasks {
		fmt.Fprintf(&descs, "%d. %s: %s [tool_hints: %s]\n", st.ID, st.Title, st.Description, st.ToolHints)
	}

	prompt := fmt.Sprintf(`为每个子任务分配最合适的角色。每个子任务恰好一个角色。

角色: analyst(分析研究), coder(编写代码), executor(测试验证), reviewer(审查报告)

任务目标: %s
子任务:
%s

回复格式: 子任务ID=角色, 如 "1=analyst, 2=coder, 3=executor"`, plan.Goal, descs.String())

	messages := []kernel.Message{
		{Role: "system", Content: "你是任务路由器。为每个子任务选择最合适的角色。回复格式: ID=角色,用逗号分隔。"},
		{Role: "user", Content: prompt},
	}
	resp, err := o.llmGateway.Chat(ctx, messages, nil, map[string]interface{}{"max_tokens": 100, "temperature": 0, "route": "execution", "no_thinking": true})
	if err != nil {
		for i := range plan.Subtasks { result[i] = "coder" }
		return result
	}

	// 解析多种格式: "1=analyst, 2=coder" 或 "1:analyst\n2:coder"
	resp.Content = strings.ReplaceAll(resp.Content, "\n", ",")
	resp.Content = strings.ReplaceAll(resp.Content, ":", "=")
	for _, part := range strings.Split(resp.Content, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 { continue }
		id := 0
		fmt.Sscanf(strings.TrimSpace(kv[0]), "%d", &id)
		role := strings.TrimSpace(kv[1])
		if o.team.GetRole(role) != nil && id > 0 {
			result[id-1] = role
		}
	}

	// 兜底：未分配的子任务用 coder
	for i := range plan.Subtasks {
		if _, ok := result[i]; !ok {
			result[i] = "coder"
		}
	}
	return result
}


// pickModel 根据角色选择对应能力的模型
func (o *Orchestrator) pickModel(role string) string {
	if o.ModelRouting.Reasoning == "" { return "" }
	switch role {
	// Architect: 分析/审查 → 推理模型（深度思考）
	case "analyst", "reviewer":
		return o.ModelRouting.Reasoning
	// Editor: 编码/执行 → 执行模型（快速、无 thinking）
	case "coder", "executor", "classifier":
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

// extractErrorSummary returns a short summary from lint errors for pattern tracking.
func extractErrorSummary(errors map[string]bool) string {
	count := 0
	var sample string
	for e := range errors {
		if count >= 3 { break }
		e = strings.TrimSpace(e)
		if len(e) > 60 { e = e[:57] + "..." }
		if sample != "" { sample += "; " }
		sample += e
		count++
	}
	return sample
}

// lintRepairLoop 自动运行 linter，错误反馈给 LLM 修复（Aider 风格）
// 最多重试 3 次，每次只反馈新增的 lint 错误
func (o *Orchestrator) lintRepairLoop(ctx context.Context, userID, projectID, fixContent string, results []string) string {
	const maxRetries = 3
	prevErrors := make(map[string]bool)

	for retry := 0; retry < maxRetries; retry++ {
		lintOutput := runLint()
		if lintOutput == "" {
			if o.mind != nil && retry > 0 { o.mind.AddLearning("pattern", "lint-fix: "+extractErrorSummary(prevErrors)) }
		return fixContent
		}

		// 只反馈新增的错误（去重）
		var newErrors []string
		for _, line := range strings.Split(lintOutput, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !prevErrors[line] {
				newErrors = append(newErrors, line)
				prevErrors[line] = true
			}
		}
		if len(newErrors) == 0 {
			return fixContent // 没有新错误
		}

		slog.Debug("Lint/Repair: fixing errors", "retry", retry+1, "new_errors", len(newErrors))
		lintFixTask := fmt.Sprintf("代码检查发现以下问题，请逐一修复：\n\n%s\n\n只修复 lint 问题，不要改变代码逻辑。输出修复后的代码变更。",
			strings.Join(newErrors, "\n"))
		lintFix, err := o.RunSubAgent(ctx, userID, projectID, "coder", lintFixTask, results)
		if err != nil {
			slog.Warn("Lint/Repair: coder failed", "error", err)
			if o.mind != nil { o.mind.AddLearning("convention", "lint-fail: "+extractErrorSummary(prevErrors)) }
			return fixContent
		}
		if strings.Contains(lintFix, "[BLOCKED:") {
			return fixContent
		}
		results = append(results, fmt.Sprintf("### Lint 修复 %d\n%s", retry+1, lintFix))
	}
	return fixContent
}

// runLint 检测项目语言并运行对应 linter
// testGenLoop 自动生成测试 → 运行 → 失败则修复（最多 2 轮）
func (o *Orchestrator) testGenLoop(ctx context.Context, userID, projectID, fixContent string, results []string) string {
	const maxRetries = 2
	for retry := 0; retry < maxRetries; retry++ {
		genTask := fmt.Sprintf("为刚才的代码修改生成单元测试。只输出测试代码，不要修改业务逻辑。\n\n修改内容:\n%s", fixContent[:min(3000, len(fixContent))])
		testCode, err := o.RunSubAgent(ctx, userID, projectID, "coder", genTask, results)
		if err != nil || testCode == "" { return fixContent }

		// 运行测试
		testOutput := runTests()
		if testOutput == "" {
			return fixContent // 无测试框架或测试通过
		}
		if !strings.Contains(testOutput, "FAIL") {
			return fixContent // 测试通过
		}

		slog.Debug("TestGen: tests failed, fixing", "retry", retry+1)
		fixTask := fmt.Sprintf("测试失败，请修复代码或测试：\n\n失败信息:\n%s", testOutput[:min(1000, len(testOutput))])
		fixCode, err := o.RunSubAgent(ctx, userID, projectID, "coder", fixTask, results)
		if err != nil { return fixContent }
		results = append(results, fmt.Sprintf("### 测试修复 %d\n%s", retry+1, fixCode))
	}
	return fixContent
}

// runTests 检测项目语言并运行测试
func runTests() string {
	if _, err := os.Stat("go.mod"); err == nil {
		cmd := exec.Command("go", "test", "./...", "-count=1", "-timeout=30s")
		cmd.Stderr = nil
		out, _ := cmd.Output()
		return strings.TrimSpace(string(out))
	}
	for _, testCmd := range [][]string{
		{"npm", "test", "--", "--passWithNoTests"},
		{"pytest", "-x", "-q"},
		{"cargo", "test"},
	} {
		if _, err := exec.LookPath(testCmd[0]); err == nil {
			cmd := exec.Command(testCmd[0], testCmd[1:]...)
			cmd.Stderr = nil
			out, _ := cmd.Output()
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func runLint() string {
	// Go 项目
	if _, err := os.Stat("go.mod"); err == nil {
		cmd := exec.Command("golangci-lint", "run", "--out-format=line-number")
		cmd.Stderr = nil
		out, _ := cmd.Output()
		return strings.TrimSpace(string(out))
	}
	// 通用：检查是否有可用的 linter
	for _, linter := range []string{"eslint", "ruff", "pylint"} {
		if _, err := exec.LookPath(linter); err == nil {
			cmd := exec.Command(linter, ".")
			cmd.Stderr = nil
			out, _ := cmd.Output()
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

