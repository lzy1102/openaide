package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// trackedHandler wraps an EventHandler with a unique ID for safe unsubscribe.
type trackedHandler struct {
	id      uint64
	handler EventHandler
}

// AgentKernel Agent 内核实现
// 作为所有 AI 智能的唯一收敛点，实现 ReAct 循环
type AgentKernel struct {
	// 核心依赖（通过接口解耦）
	llmProvider  LLMProvider
	toolExecutor ToolExecutor
	memory       Memory
	sessionStore SessionStore
	compressor   ContextCompressor
	permission   PermissionChecker

	// 增强能力（可选）
	reflection     Reflection
	skillActor     *SkillActor // CSP actor, zero-lock
	approver       Approver
	adaptiveRounds *AdaptiveRounds
	planner        *Planner // 复杂任务分解(可选,仅 complexity >= 阈值时触发)

	// 跟踪系统
	tracer Tracer

	// 检查点系统
	checkpointer Checkpointer

	// 指标系统
	metrics *MetricsStore

	// 代码索引(prompt 阶段注入相关代码,可选)
	codeIndexer CodeIndexer

	// 事件系统
	handlerSeq    atomic.Uint64 // monotonic ID counter for tracked handlers
	eventHandlers atomic.Value  // []trackedHandler — lock-free reads

	// 无锁状态（atomic.Value）
	systemPrompt atomic.Value // string — read-heavy, written only on config change
	state        atomic.Value // KernelState — write-once, read-often

	// 配置
	maxRounds    int
	maxTokens    int
	stallTimeout time.Duration
}

// Config 内核配置
type Config struct {
	MaxRounds    int
	MaxTokens    int
	SystemPrompt string
	// LLMStallTimeout LLM 流式无输出停滞阈值。0=默认 120s。
	LLMStallTimeout time.Duration
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxRounds:    10,
		MaxTokens:    4000,
		SystemPrompt: "",
	}
}

// stallTimeoutValue 返回停滞阈值(0 时回退默认 120s)。
func (k *AgentKernel) stallTimeoutValue() time.Duration {
	if k.stallTimeout > 0 {
		return k.stallTimeout
	}
	return 120 * time.Second
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
		llmProvider:  llm,
		toolExecutor: tools,
		memory:       memory,
		sessionStore: sessions,
		maxRounds:    config.MaxRounds,
		maxTokens:    config.MaxTokens,
		stallTimeout: config.LLMStallTimeout,
	}

	// 压缩器由调用方注入(infra 层用 LLMCompressor);默认 nil 表示不压缩
	k.systemPrompt.Store(config.SystemPrompt)
	k.state.Store(StateIdle)
	k.eventHandlers.Store([]trackedHandler{})

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

func (k *AgentKernel) SetApprover(a Approver)               { k.approver = a }
func (k *AgentKernel) SetAdaptiveRounds(ar *AdaptiveRounds) { k.adaptiveRounds = ar }
func (k *AgentKernel) SetMetrics(ms *MetricsStore)          { k.metrics = ms }

// SetPlanner 设置任务规划器(可选)。
// 设置后,复杂查询(complexity >= 15)会在 ReAct 循环前生成子任务计划,
// 作为系统消息注入,引导 agent 按步骤执行。
func (k *AgentKernel) SetPlanner(p *Planner) { k.planner = p }

// SetCodeIndexer 设置代码索引器(可选)。
// 设置后,coding/debugging 任务会在 prompt 中注入与 query 语义相关的代码 chunk。
func (k *AgentKernel) SetCodeIndexer(ci CodeIndexer) { k.codeIndexer = ci }
func (k *AgentKernel) SetMaxRounds(n int) {
	if n > 0 {
		k.maxRounds = n
		slog.Info("Kernel max_rounds updated", "value", n)
	}
}

func (k *AgentKernel) SetMaxTokens(n int) {
	if n > 0 {
		k.maxTokens = n
		slog.Info("Kernel max_tokens updated", "value", n)
	}
}

// ApplyConfig hot-reloads mutable kernel settings from config.
// Only updates values that are safe to change mid-session.
func (k *AgentKernel) ApplyConfig(maxRounds, maxTokens, minRounds, maxRoundsCap int) {
	if maxRounds > 0 {
		k.maxRounds = maxRounds
	}
	if maxTokens > 0 {
		k.maxTokens = maxTokens
	}
	slog.Info("Kernel config applied", "max_rounds", k.maxRounds, "max_tokens", k.maxTokens)
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

func (k *AgentKernel) SetSkillActor(sa *SkillActor) { k.skillActor = sa }

// SetUserVerdict stores user feedback for the current session.
// Verdict should be "good", "bad", or "" (clear).
// This is the only ground truth signal in the learning pipeline.
func (k *AgentKernel) SetUserVerdict(ctx context.Context, sessionID, verdict string) {
	if k.sessionStore == nil {
		return
	}
	session, err := k.sessionStore.Get(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	if verdict == "" {
		delete(session.Metadata, "user_verdict")
	} else {
		session.Metadata["user_verdict"] = verdict
	}
	k.sessionStore.Update(ctx, session)
	slog.Info("User verdict recorded", "session", sessionID[:min(8, len(sessionID))], "verdict", verdict)
}
func (k *AgentKernel) GetSkillActor() *SkillActor { return k.skillActor }

// SetSystemPrompt 热更新系统提示词（线程安全）
func (k *AgentKernel) SetSystemPrompt(prompt string) {
	k.systemPrompt.Store(prompt)
}

// GetState 获取当前状态
func (k *AgentKernel) GetState() KernelState {
	return k.state.Load().(KernelState)
}

// Subscribe 订阅事件，返回 handler ID 用于后续取消订阅
func (k *AgentKernel) Subscribe(handler EventHandler) uint64 {
	id := k.handlerSeq.Add(1)
	old := k.eventHandlers.Load().([]trackedHandler)
	newHandlers := make([]trackedHandler, len(old)+1)
	copy(newHandlers, old)
	newHandlers[len(old)] = trackedHandler{id: id, handler: handler}
	k.eventHandlers.Store(newHandlers)
	return id
}

// Unsubscribe 通过 ID 取消订阅
func (k *AgentKernel) Unsubscribe(id uint64) {
	old := k.eventHandlers.Load().([]trackedHandler)
	newHandlers := make([]trackedHandler, 0, len(old))
	for _, th := range old {
		if th.id != id {
			newHandlers = append(newHandlers, th)
		}
	}
	k.eventHandlers.Store(newHandlers)
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

func (k *AgentKernel) buildMessages(ctx context.Context, session *Session, query *Query, analysis *QueryAnalysis) []Message {
	// ── Stable Prefix: System Message (Layers 0-2,5; prompt cache friendly) ──
	systemMsg := k.buildSystemLayer(ctx, query)
	messages := make([]Message, 0)
	if systemMsg != "" {
		messages = append(messages, Message{Role: "system", Content: systemMsg})
	}

	// History (adjacent to system, stable)
	// 按 token 预算截断:历史约占上下文的 1/4,超过预算的旧消息丢弃。
	// 无 MaxTokens 配置时回退到 20 条(原行为)。
	if k.memory != nil && len(session.Messages) > 0 {
		limit := 20
		if k.maxTokens > 0 {
			limit = 200
		}
		history, _ := k.memory.Load(ctx, session.ID, limit)
		if k.maxTokens > 0 && len(history) > 2 {
			history = trimHistoryToBudget(history, k.maxTokens/4)
		}
		messages = append(messages, history...)
	}
	// User query
	messages = append(messages, Message{Role: "user", Content: query.Content})

	// ── Dynamic Tail: Layers 3-6 (after user query, varies per query) ──

	// L3: Task Adapter (coding/review/think/debugging) — per-query, not cached
	// 复用统一查询分析的 TaskType，避免重复 LLM 分类调用
	taskType := ""
	if analysis != nil {
		taskType = analysis.TaskType
	}
	if l3 := k.promptL3(ctx, query.Content, taskType); l3 != "" {
		messages = append(messages, Message{Role: "system", Content: l3})
	}

	// L5: Previous-round reflection (dynamic tail)
	if session.Metadata != nil {
		if ref, ok := session.Metadata["reflection"]; ok {
			if r, ok := ref.(*ReflectionResult); ok && r != nil {
				if l5 := promptL5(r); l5 != "" {
					messages = append(messages, Message{Role: "system", Content: l5})
				}
			}
			delete(session.Metadata, "reflection")
		}
	}

	// L4: Cross-session learner insights + ProjectMind conventions (dynamic tail)
	if query.Options.ProjectContext != "" {
		messages = append(messages, Message{Role: "system", Content: "[ProjectKnowledge]\n" + query.Options.ProjectContext})
	}

	slog.Info("buildMessages complete", "msgs", len(messages))
	return messages
}

// ── Layer Builders ──────────────────────────────────────────
// Each layer has a clear activation condition and injection point.

// buildSystemLayer assembles the prompt. L0 safety rules are always included;
// custom prompts (config kernel.system_prompt / opencode instructions) are
// appended as an additional layer — they never replace the built-in layers.
func (k *AgentKernel) buildSystemLayer(ctx context.Context, query *Query) string {
	// Layered build: L0 core rules + user prompts + L1 project context
	sp := k.buildSystemPrompt(query)

	// External instructions (SetSystemPrompt / opencode) append, never override
	if custom := k.systemPrompt.Load().(string); custom != "" {
		sp += "\n\n" + custom
	}

	if k.skillActor != nil {
		sp = k.skillActor.InjectPrompt(ctx, query.Content, sp)
	}

	return sp
}

// needsStrategyAdvice checks if the query warrants strategy injection (L3).
// Activated by: long queries, build/implement/refactor keywords.
func runGitCmd(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// snipOldToolOutputs 对旧工具输出做头尾保留裁剪（Claude Code 风格）
// 最近 keepFull 条完整保留，更早的保留前 500 字符 + 后 500 字符，中间替换为 snipped 标记
//
// 已被 snipOldToolOutputsDynamic 替代(支持按上下文压力动态调整)。
// 保留此函数供测试和向后兼容。
func snipOldToolOutputs(messages []Message) {
	snipOldToolOutputsDynamic(messages, 0) // 0 = 宽松档
}

// contextPressure 表示当前上下文压力等级。
// 0 = 宽松(<70%), 1 = 中等(70-85%), 2 = 激进(85%+)
type contextPressure int

const (
	pressureLow    contextPressure = 0 // <70%
	pressureMedium contextPressure = 1 // 70-85%
	pressureHigh   contextPressure = 2 // 85%+
)

// snipOldToolOutputsDynamic 根据上下文压力动态调整裁剪强度。
// 压力越高,完整保留的条数越少、头尾保留长度越短。
func snipOldToolOutputsDynamic(messages []Message, pressure contextPressure) {
	var keepFull, headLen, tailLen int
	switch pressure {
	case pressureMedium:
		keepFull, headLen, tailLen = 2, 300, 300
	case pressureHigh:
		keepFull, headLen, tailLen = 1, 200, 200
	default: // pressureLow
		keepFull, headLen, tailLen = 4, 500, 500
	}

	toolIdx := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			if toolIdx >= keepFull {
				content := messages[i].Content
				if len(content) > headLen+tailLen+100 {
					head := safeSliceHead(content, headLen)
					tail := safeSliceTail(content, tailLen)
					snipped := len(content) - len(head) - len(tail)
					messages[i].Content = fmt.Sprintf("%s\n... [%d chars snipped] ...\n%s", head, snipped, tail)
				}
			}
			toolIdx++
		}
	}
}

// 工具结果单条字符上限(约 5000 tokens)。
// 超过此长度的工具输出在入 messages 前会被头尾截断,
// 防止单条 read_file/list_directory 把整个上下文吃爆。
const maxToolResultChars = 20000

// truncateToolResult 对单条工具结果做头尾保留截断。
// 超过 maxToolResultChars 时保留头部 + 尾部,中间替换为截断标记。
// 这样 LLM 仍能看到文件开头(包声明/import/函数签名)和结尾(返回值/错误),
// 只是中间部分被省略 —— 比 snipOldToolOutputs 的下一轮裁剪更早介入,
// 避免当前轮就因单条过大而超出上下文限制。
func truncateToolResult(content string) string {
	if len(content) <= maxToolResultChars {
		return content
	}
	headLen := maxToolResultChars * 2 / 5 // 40% 给头部(签名/声明)
	tailLen := maxToolResultChars * 2 / 5 // 40% 给尾部(返回值/错误)
	head := safeSliceHead(content, headLen)
	tail := safeSliceTail(content, tailLen)
	snipped := len(content) - len(head) - len(tail)
	return fmt.Sprintf("%s\n\n... [%d chars truncated — use read_file with offset/limit to view this section] ...\n\n%s", head, snipped, tail)
}

// estimateContextPressure 根据当前 token 使用量估算上下文压力等级。
// promptTokens 是最近一次 LLM 调用返回的 prompt_tokens(更准确)。
func (k *AgentKernel) estimateContextPressure(promptTokens int) contextPressure {
	used := promptTokens
	if k.maxTokens <= 0 {
		return pressureLow
	}
	ratio := used * 100 / k.maxTokens
	switch {
	case ratio >= 85:
		return pressureHigh
	case ratio >= 70:
		return pressureMedium
	default:
		return pressureLow
	}
}

// safeSliceHead returns prefix of s up to maxBytes without breaking UTF-8 characters.
func safeSliceHead(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	pos := maxBytes
	for pos > 0 && s[pos]&0xC0 == 0x80 {
		pos--
	}
	return s[:pos]
}

// safeSliceTail returns suffix of s approximately tailLen bytes, aligned to UTF-8 boundary.
func safeSliceTail(s string, tailLen int) string {
	if len(s) <= tailLen {
		return s
	}
	start := len(s) - tailLen
	for start < len(s) && s[start]&0xC0 == 0x80 {
		start++
	}
	return s[start:]
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
	"read_file":      true,
	"list_directory": true,
	"search_files":   true,
	"search_symbols": true,
	"web_search":     true,
	"web_fetch":      true,
	"read_image":     true,
	"git_status":     true,
	"git_diff":       true,
	"git_log":        true,
	"git_blame":      true,
}

// isParallelSafe 判断工具是否可以和其他工具并行执行
// DangerousTools 是需要交互审批(OnApproval 回调)的工具。
// 当前为空 —— 审批在"方案"层面(plan 执行前)进行,不在每条命令/工具层面。
// 安全靠以下机制保障:
//   - execute_command: handler 层有危险命令黑名单(tools_filesystem.go)
//   - 写操作: 有 Undo 机制(undo_edit)+ 原子写 + 文件锁
//   - approval.go: AutoApprover 仍有 LLM 风险评估,作为非交互路径的兜底
var DangerousTools = map[string]string{}

func isParallelSafe(name string) bool {
	return parallelSafeTools[name]
}

// extractToolPath 从工具参数中提取文件路径,用于审批提示。
// 支持单文件工具(write_file/diff_edit)和多文件工具(edit_files)。
func extractToolPath(toolName, arguments string) string {
	if toolName == "edit_files" {
		var args struct {
			Edits []struct {
				Path string `json:"path"`
			} `json:"edits"`
		}
		if json.Unmarshal([]byte(arguments), &args) == nil && len(args.Edits) > 0 {
			if len(args.Edits) == 1 {
				return args.Edits[0].Path
			}
			return fmt.Sprintf("%s (+%d more)", args.Edits[0].Path, len(args.Edits)-1)
		}
		return ""
	}
	var args struct {
		Path string `json:"path"`
	}
	json.Unmarshal([]byte(arguments), &args)
	return args.Path
}

func (k *AgentKernel) executeTool(ctx context.Context, tc ToolCall, sessionID string, opts *QueryOptions) *ToolResult {
	// 交互审批（REPL pterm 回调）
	if opts != nil && opts.OnApproval != nil {
		if _, dangerous := DangerousTools[tc.Function.Name]; dangerous {
			path := extractToolPath(tc.Function.Name, tc.Function.Arguments)
			if !opts.OnApproval(tc.Function.Name, path, tc.Function.Arguments) {
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
			"tool_name":  tc.Function.Name,
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
	k.state.Store(state)
}

// GetSlashCommands 获取所有技能对应的斜杠命令
func (k *AgentKernel) GetSlashCommands() map[string]string {
	if k.skillActor == nil {
		return nil
	}
	// Return empty — slash commands are managed by the REPL
	return map[string]string{}
}

func (k *AgentKernel) publishEvent(event Event) {
	handlers := k.eventHandlers.Load().([]trackedHandler)

	for _, th := range handlers {
		th := th
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			done := make(chan struct{}, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Warn("Event handler panicked", "event", event.Type, "handler_id", th.id, "panic", r)
					}
				}()
				th.handler.HandleEvent(event)
				done <- struct{}{}
			}()

			select {
			case <-done:
			case <-ctx.Done():
				slog.Warn("Event handler timed out", "event", event.Type, "handler_id", th.id)
			}
		}()
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
func (k *AgentKernel) generateSessionTitle(sessionID, firstQuery string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := k.llmProvider.Chat(ctx, []Message{
		{Role: "user", Content: fmt.Sprintf("Generate a SHORT session title (3-5 words) for this query. Reply with ONLY the title, no explanation.\nQuery: %s", firstQuery)},
	}, nil, map[string]interface{}{"max_tokens": 30, "temperature": 0, "route": "execution", "no_thinking": true})
	if err != nil {
		return
	}
	title := strings.TrimSpace(resp.Content)
	if title != "" && len([]rune(title)) <= 50 {
		k.setSessionTitle(ctx, sessionID, title)
	}
}

// setSessionTitle atomically sets a session title from the title goroutine.
func (k *AgentKernel) setSessionTitle(ctx context.Context, sessionID, title string) {
	session, err := k.sessionStore.Get(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata["title"] = title
	k.sessionStore.Update(ctx, session)
}

// loadSessionMessages loads recent session messages for process supervision.
func (k *AgentKernel) loadSessionMessages(ctx context.Context, sessionID string) []Message {
	if k.sessionStore == nil {
		return nil
	}
	session, err := k.sessionStore.Get(ctx, sessionID)
	if err != nil || session == nil || len(session.Messages) == 0 {
		return nil
	}
	msgs := session.Messages
	if len(msgs) > 30 {
		msgs = msgs[len(msgs)-30:]
	}
	return msgs
}
