package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AgentKernel Agent 内核实现
// 作为所有 AI 智能的唯一收敛点，实现 ReAct 循环
type AgentKernel struct {
	// 核心依赖（通过接口解耦）
	llmProvider    LLMProvider
	toolExecutor   ToolExecutor
	memory         Memory
	sessionStore   SessionStore
	compressor     ContextCompressor
	permission     PermissionChecker

	// 增强能力（可选）
	reflection       Reflection
	learner          Learner
	patternDetector  PatternDetector
	knowledgeCollector KnowledgeCollector
	qualityGate      QualityGate
	skillManager     *SkillManager
	skillEvolution   *SkillEvolution
	approver         Approver
	adaptiveRounds   *AdaptiveRounds
	queryOptions     *QueryOptions // 当前查询的选项（含交互回调）

	// 跟踪系统
	tracer  Tracer
	traceMu sync.Mutex

	promptMu sync.RWMutex // protects systemPrompt

	// 检查点系统
	checkpointer Checkpointer

	// 事件系统
	eventHandlers []EventHandler
	eventMu       sync.RWMutex
	eventSem      chan struct{} // 限制并发 handler goroutine 数量

	// 状态管理
	state     KernelState
	stateMu   sync.RWMutex

	// 配置
	maxRounds     int
	maxTokens     int
	systemPrompt  string
}

// Config 内核配置
type Config struct {
	MaxRounds    int
	MaxTokens    int
	SystemPrompt string
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxRounds: 10,
		MaxTokens: 4000,
		SystemPrompt: defaultSystemPrompt(),
	}
}

// NewAgentKernel 创建 Agent 内核
func NewAgentKernel(
	llm LLMProvider,
	tools ToolExecutor,
	memory Memory,
	sessions SessionStore,
	config *Config,
) *AgentKernel {
	if config == nil {
		config = DefaultConfig()
	}

	k := &AgentKernel{
		llmProvider:   llm,
		toolExecutor:  tools,
		memory:        memory,
		sessionStore:  sessions,
		maxRounds:     config.MaxRounds,
		maxTokens:     config.MaxTokens,
		systemPrompt:  config.SystemPrompt,
		state:         StateIdle,
		eventHandlers: make([]EventHandler, 0),
			eventSem:      make(chan struct{}, 16),
	}

	// 默认使用简单压缩器
	k.compressor = &SimpleCompressor{}

	return k
}

// SetPermissionChecker 设置权限检查器
func (k *AgentKernel) SetPermissionChecker(pc PermissionChecker) {
	k.permission = pc
}

// SetContextCompressor 设置上下文压缩器
func (k *AgentKernel) SetContextCompressor(c ContextCompressor) {
	k.compressor = c
}

// SetReflection 设置反思能力
func (k *AgentKernel) SetReflection(r Reflection) {
	k.reflection = r
}

// SetLearner 设置学习能力
func (k *AgentKernel) SetLearner(l Learner) {
	k.learner = l
}

// SetPatternDetector 设置模式检测器
func (k *AgentKernel) SetPatternDetector(pd PatternDetector) {
	k.patternDetector = pd
}

// SetKnowledgeCollector 设置知识收集器
func (k *AgentKernel) SetKnowledgeCollector(kc KnowledgeCollector) {
	k.knowledgeCollector = kc
}

// SetQualityGate 设置质量门控
// QualityGate 质量门控接口
type QualityGate interface {
	Pass(query, response string, toolSuccesses, toolFailures int, reflection *ReflectionResult) bool
}

func (k *AgentKernel) SetApprover(a Approver) { k.approver = a }
func (k *AgentKernel) SetAdaptiveRounds(ar *AdaptiveRounds) { k.adaptiveRounds = ar }
func (k *AgentKernel) SetMaxRounds(n int) {
	if n > 0 { k.maxRounds = n; slog.Info("Kernel max_rounds updated", "value", n) }
}
func (k *AgentKernel) SetMaxTokens(n int) {
	if n > 0 { k.maxTokens = n; slog.Info("Kernel max_tokens updated", "value", n) }
}

func (k *AgentKernel) SetTracer(t Tracer) {
	k.tracer = t
}

func (k *AgentKernel) SetCheckpointer(cp Checkpointer) {
	k.checkpointer = cp
}

// ResumeSession 从最新的检查点恢复会话
// 返回恢复后的消息列表、已完成的轮数、以及是否找到有效检查点
func (k *AgentKernel) ResumeSession(ctx context.Context, sessionID string) (messages []Message, completedRounds int, found bool, err error) {
	if k.checkpointer == nil {
		return nil, 0, false, nil
	}
	cp, err := k.checkpointer.LoadLatest(ctx, sessionID)
	if err != nil {
		return nil, 0, false, fmt.Errorf("load checkpoint: %w", err)
	}
	if cp == nil || len(cp.Messages) == 0 {
		return nil, 0, false, nil
	}

	slog.Info("Resumed session from checkpoint",
		"session_id", sessionID,
		"round", cp.Round,
		"messages", len(cp.Messages),
	)

	if k.tracer != nil {
		k.tracer.Record(ctx, &TraceEvent{
			Type: TraceCheckpoint, Name: "checkpoint_restore", SessionID: sessionID,
			Input:  map[string]interface{}{"round": cp.Round, "messages": len(cp.Messages)},
			Status: TraceStatusOK,
		})
	}

	return cp.Messages, cp.Round, true, nil
}

func (k *AgentKernel) SetSkillManager(sm *SkillManager) {
	k.skillManager = sm
}

func (k *AgentKernel) SetSkillEvolution(se *SkillEvolution) {
	k.skillEvolution = se
}

func (k *AgentKernel) SetQualityGate(gate QualityGate) {
	k.qualityGate = gate
}

// SetSystemPrompt 热更新系统提示词（线程安全）
func (k *AgentKernel) SetSystemPrompt(prompt string) {
	k.promptMu.Lock()
	k.systemPrompt = prompt
	k.promptMu.Unlock()
}

// GetState 获取当前状态
func (k *AgentKernel) GetState() KernelState {
	k.stateMu.RLock()
	defer k.stateMu.RUnlock()
	return k.state
}

// Subscribe 订阅事件
func (k *AgentKernel) Subscribe(handler EventHandler) {
	k.eventMu.Lock()
	defer k.eventMu.Unlock()
	k.eventHandlers = append(k.eventHandlers, handler)
}

// Unsubscribe 取消订阅
// 注意: 由于 EventHandlerFunc 是函数类型无法直接比较，调用者应保存 Subscribe 时的返回值
func (k *AgentKernel) Unsubscribe(handler EventHandler) {
	// EventHandler 可能是不可比较的函数类型，跳过
	// 实际使用中 handler 的生命周期跟随应用，通常不需要 Unsubscribe
}

// ============ 内部方法 ============

func (k *AgentKernel) getOrCreateSession(ctx context.Context, query *Query) (*Session, error) {
	if query.SessionID != "" {
		session, err := k.sessionStore.Get(ctx, query.SessionID)
		if err == nil && session != nil {
			return session, nil
		}
	}

	// 创建新会话
	session, err := k.sessionStore.Create(ctx, query.ProjectID, query.UserID)
	if err != nil {
		return nil, err
	}

	k.publishEvent(Event{
		Type:      EventSessionCreated,
		Source:    "kernel",
		Data:      map[string]interface{}{"session_id": session.ID},
		Timestamp: time.Now(),
	})

	return session, nil
}

func (k *AgentKernel) buildMessages(ctx context.Context, session *Session, query *Query) []Message {
	// ── Stable Prefix: System Message (Layers 0-2,5; prompt cache friendly) ──
	systemMsg := k.buildSystemLayer(query)
	messages := make([]Message, 0)
	if systemMsg != "" {
		messages = append(messages, Message{Role: "system", Content: systemMsg})
	}

	// History (adjacent to system, stable)
	if k.memory != nil && len(session.Messages) > 0 {
		history, _ := k.memory.Load(ctx, session.ID, 20)
		messages = append(messages, history...)
	}

	// User query
	messages = append(messages, Message{Role: "user", Content: query.Content})

	// ── Dynamic Tail: Layers 3-6 (after user query, varies per query) ──

	// L3: Strategy hints (complex tasks only)
	if needsStrategyAdvice(query) {
		if hint := k.buildStrategyLayer(query); hint != "" {
			messages = append(messages, Message{Role: "system", Content: hint})
		}
	}

	// L4: Previous-round reflection
	if session.Metadata != nil {
		if ref, ok := session.Metadata["reflection"]; ok {
			if r, ok := ref.(*ReflectionResult); ok && r != nil {
				hint := fmt.Sprintf("[Reflection] Quality:%d/10", r.Quality)
				if len(r.Issues) > 0 { hint += fmt.Sprintf(" Issues:%s", strings.Join(r.Issues, ";")) }
				if len(r.Suggestions) > 0 { hint += fmt.Sprintf(" Tips:%s", strings.Join(r.Suggestions, ";")) }
				messages = append(messages, Message{Role: "system", Content: hint})
			}
			delete(session.Metadata, "reflection")
		}
	}

	// L5b: Cross-session learner insights
	if k.learner != nil {
		insights, _ := k.learner.GetInsights(ctx, query.Content)
		if len(insights) > 0 {
			messages = append(messages, Message{Role: "system", Content: "[Learned] " + strings.Join(insights, " | ")})
		}
	}

	// L6: Knowledge base RAG
	if k.knowledgeCollector != nil {
		kbCtx, _, _ := k.knowledgeCollector.InjectContext(ctx, query.Content, 500)
		if kbCtx != "" {
			messages = append(messages, Message{Role: "system", Content: "[Knowledge] " + kbCtx})
		}
	}

	return messages
}

// ── Layer Builders ──────────────────────────────────────────
// Each layer has a clear activation condition and injection point.

// buildSystemLayer (L0+L1+L2+L5): Identity + Rules + Project Context + Skill
func (k *AgentKernel) buildSystemLayer(query *Query) string {
	k.promptMu.RLock()
	sp := k.systemPrompt
	k.promptMu.RUnlock()

	// L5: Skill prompt injection (activated by keyword match on query)
	if k.skillManager != nil {
		sp = k.skillManager.InjectPrompt(query.Content, sp)
	}
	if sp == "" { return "" }

	// L2: Project context (always injected, short)
	cwd, _ := os.Getwd()
	if k.queryOptions != nil && k.queryOptions.WorkingDir != "" {
		cwd = k.queryOptions.WorkingDir
	}
	gitNote := ""
	if out, err := runGitCmd("rev-parse", "--abbrev-ref", "HEAD"); err == nil && out != "" {
		gitNote = fmt.Sprintf(" (branch: %s)", out)
	}
	sp += fmt.Sprintf("\n\n[WorkingDir] %s%s", cwd, gitNote)

	// L2b: Project rules (OPENAIDE.md, CLAUDE.md, etc.)
	ruleFiles := []string{"OPENAIDE.md", "CLAUDE.md", "CODEBUDDY.md", "CONVENTIONS.md", ".github/copilot-instructions.md"}
	if entries, _ := os.ReadDir(filepath.Join(cwd, ".cursor/rules")); entries != nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				ruleFiles = append(ruleFiles, ".cursor/rules/"+e.Name())
			}
		}
	}
	for _, name := range ruleFiles {
		data, err := os.ReadFile(filepath.Join(cwd, name))
		if err != nil || len(data) == 0 { continue }
		sp += fmt.Sprintf("\n\n[Rules:%s]\n%s", name, string(data))
	}

	// L2c: RepoMap project symbol map (5-min TTL cache)
	if repoMap := GenerateRepoMap(cwd); repoMap != "" {
		sp += "\n\n" + repoMap
	}

	return sp
}

// needsStrategyAdvice checks if the query warrants strategy injection (L3).
// Activated by: long queries, build/implement/refactor keywords.
func needsStrategyAdvice(query *Query) bool {
	return len(query.Content) > 200 ||
		strings.Contains(query.Content, "build") || strings.Contains(query.Content, "implement") ||
		strings.Contains(query.Content, "refactor") || strings.Contains(query.Content, "design") ||
		strings.Contains(query.Content, "构建") || strings.Contains(query.Content, "实现") ||
		strings.Contains(query.Content, "重构") || strings.Contains(query.Content, "设计")
}

// buildStrategyLayer (L3): Experience-based strategy hints for complex tasks.
// Activated only by needsStrategyAdvice. Injects learner insights.
func (k *AgentKernel) buildStrategyLayer(query *Query) string {
	if k.learner == nil { return "" }
	insights, _ := k.learner.GetInsights(context.Background(), query.Content)
	if len(insights) == 0 { return "" }
	return "[Strategy] " + strings.Join(insights, "; ")
}


func runGitCmd(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil { return "", err }
	return strings.TrimSpace(string(out)), nil
}

// snipOldToolOutputs 对旧工具输出做头尾保留裁剪（Claude Code 风格）
// 最近 keepFull 条完整保留，更早的保留前 500 字符 + 后 500 字符，中间替换为 snipped 标记
func snipOldToolOutputs(messages []Message) {
	const keepFull = 4       // 最近 N 条完整保留
	const headLen = 500      // 保留头部长度
	const tailLen = 500      // 保留尾部长度

	// 从后往前数 tool 消息
	toolIdx := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			if toolIdx >= keepFull {
				content := messages[i].Content
				if len(content) > headLen+tailLen+100 {
					head := content[:headLen]
					tail := content[len(content)-tailLen:]
					snipped := len(content) - headLen - tailLen
					messages[i].Content = fmt.Sprintf("%s\n... [%d chars snipped] ...\n%s", head, snipped, tail)
				}
			}
			toolIdx++
		}
	}
}

func (k *AgentKernel) buildOptions(opts QueryOptions) map[string]interface{} {
	options := make(map[string]interface{})
	if opts.Temperature > 0 {
		options["temperature"] = opts.Temperature
		// 思考模式下 temperature 不生效（deepseek/openai reasoning models），
		// 但保留显式传入的值；由 provider 层根据 thinking 状态决定是否透传
	}
	if opts.MaxTokens > 0 {
		options["max_tokens"] = opts.MaxTokens
	}
	if opts.ResponseFormat != nil {
		options["response_format"] = opts.ResponseFormat
	}
	return options
}

// parallelSafeTools 可在同一批次内并行执行的工具（只读/无副作用）
var parallelSafeTools = map[string]bool{
	"read_file":       true,
	"list_directory":  true,
	"search_files":    true,
	"search_symbols":  true,
	"web_search":      true,
	"web_fetch":       true,
	"read_image":      true,
	"search_knowledge": true,
	"git_status":      true,
	"git_diff":        true,
	"git_log":         true,
	"git_blame":       true,
}

// isParallelSafe 判断工具是否可以和其他工具并行执行
func isParallelSafe(name string) bool {
	return parallelSafeTools[name]
}

func (k *AgentKernel) executeTool(ctx context.Context, tc ToolCall, sessionID string) *ToolResult {
	// 交互审批（REPL pterm 回调）
	if k.queryOptions != nil && k.queryOptions.OnApproval != nil {
		if _, dangerous := DangerousTools[tc.Function.Name]; dangerous {
			var path string
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
				path = args.Path
			}
			if !k.queryOptions.OnApproval(tc.Function.Name, path) {
				return &ToolResult{Error: "user denied"}
			}
		}
	}
	// 审批检查（高危工具需要用户确认）
	if k.approver != nil {
		if reason, dangerous := DangerousTools[tc.Function.Name]; dangerous {
			result := k.approver.RequestApproval(ctx, &ApprovalRequest{
				ID:     tc.ID,
				Tool:   tc.Function.Name,
				Args:   tc.Function.Arguments,
				Reason: reason,
				Risk:   "high",
			})
			if !result.Approved {
				return &ToolResult{Error: fmt.Sprintf("Approval denied: %s", result.Reason)}
			}
		}
	}

	// 权限检查
	if k.permission != nil {
		allowed, reason := k.permission.Check(ctx, "tool.execute", tc.Function.Name, map[string]interface{}{
			"tool_name": tc.Function.Name,
			"session_id": sessionID,
		})
		if !allowed {
			return &ToolResult{
				Error: fmt.Sprintf("Permission denied: %s", reason),
			}
		}
	}

	result, err := k.toolExecutor.Execute(ctx, tc, sessionID)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	// 统一截断工具输出，避免超大数据塞进 LLM 上下文
	const maxOutput = 50 * 1024
	if s, ok := result.Content.(string); ok && len(s) > maxOutput {
		result.Content = s[:maxOutput] + "\n... (truncated)"
	}
	return result
}

func (k *AgentKernel) saveToMemory(ctx context.Context, sessionID string, messages []Message) {
	if k.memory == nil {
		return
	}
	if err := k.memory.Save(ctx, sessionID, messages); err != nil {
		slog.Warn("Failed to save memory", "error", err)
	}
}

func (k *AgentKernel) setState(state KernelState) {
	k.stateMu.Lock()
	defer k.stateMu.Unlock()
	k.state = state
}

// GetSlashCommands 获取所有技能对应的斜杠命令
func (k *AgentKernel) GetSlashCommands() map[string]string {
	if k.skillManager == nil {
		return nil
	}
	return k.skillManager.GetSlashCommands()
}

func (k *AgentKernel) publishEvent(event Event) {
	k.eventMu.RLock()
	handlers := make([]EventHandler, len(k.eventHandlers))
	copy(handlers, k.eventHandlers)
	k.eventMu.RUnlock()

	for _, h := range handlers {
		h := h
		k.eventSem <- struct{}{}
		go func(handler EventHandler) {
			defer func() { <-k.eventSem }()
			done := make(chan struct{}, 1)
			go func() {
				handler.HandleEvent(event)
				done <- struct{}{}
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				slog.Warn("Event handler timed out", "event", event.Type)
			}
		}(h)
	}
}

// ensureSessionTitle 从第一条 user 消息提取会话标题
func ensureSessionTitle(session *Session) {
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	if _, ok := session.Metadata["title"]; ok {
		return
	}

	for _, msg := range session.Messages {
		if msg.Role == "user" && msg.Content != "" {
			rs := []rune(strings.TrimSpace(msg.Content))
			if len(rs) == 0 {
				return
			}
			// 先用截断标题兜底，LLM 标题异步替换
			title := string(rs[:min(len(rs), 25)])
			if len(rs) > 25 {
				title += "…"
			}
			session.Metadata["title"] = title
			return
		}
	}
}

// generateSessionTitle 用 LLM 生成有意义的会话标题（异步，不阻塞）
func (k *AgentKernel) generateSessionTitle(session *Session, firstQuery string) {
	resp, err := k.llmProvider.Chat(context.Background(), []Message{
		{Role: "user", Content: fmt.Sprintf("Generate a SHORT session title (3-5 words) for this query. Reply with ONLY the title, no explanation.\nQuery: %s", firstQuery)},
	}, nil, map[string]interface{}{"max_tokens": 30, "temperature": 0})
	if err != nil {
		return
	}
	title := strings.TrimSpace(resp.Content)
	if title != "" && len([]rune(title)) <= 50 {
		session.Metadata["title"] = title
		k.sessionStore.Update(context.Background(), session)
	}
}
