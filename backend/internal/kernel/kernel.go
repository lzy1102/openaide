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
	patternDetector  PatternDetector
	knowledgeCollector KnowledgeCollector
	qualityGate      QualityGate
	skillActor *SkillActor // CSP actor, zero-lock
	approver         Approver
	adaptiveRounds   *AdaptiveRounds
	queryOptions     *QueryOptions // 当前查询的选项（含交互回调）

	// 跟踪系统
	tracer  Tracer

	// 检查点系统
	checkpointer Checkpointer

	// 事件系统
	eventHandlers atomic.Value // []EventHandler — lock-free reads

	// 无锁状态（atomic.Value）
	systemPrompt atomic.Value // string — read-heavy, written only on config change
	state        atomic.Value // KernelState — write-once, read-often

	// 配置
	maxRounds int
	maxTokens int
}

// Config 内核配置
type Config struct {
	MaxRounds    int
	MaxTokens    int
	SystemPrompt string

	PatternMinClusterSize      int     // queries to trigger distillation (default 8)
	PatternSimilarityThreshold float64 // cosine threshold for clustering (default 0.80)
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxRounds:                  10,
		MaxTokens:                  4000,
		SystemPrompt:               defaultSystemPrompt(),
		PatternMinClusterSize:      8,
		PatternSimilarityThreshold: 0.80,
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
	}

	// 默认使用简单压缩器
	k.compressor = &SimpleCompressor{}
	k.systemPrompt.Store(config.SystemPrompt)
	k.state.Store(StateIdle)
	k.eventHandlers.Store([]EventHandler{})

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

func (k *AgentKernel) SetSkillActor(sa *SkillActor) { k.skillActor = sa }
func (k *AgentKernel) GetSkillActor() *SkillActor      { return k.skillActor }
func (k *AgentKernel) SetQualityGate(gate QualityGate) {
	k.qualityGate = gate
}

// SetSystemPrompt 热更新系统提示词（线程安全）
func (k *AgentKernel) SetSystemPrompt(prompt string) {
	k.systemPrompt.Store(prompt)
}

// GetState 获取当前状态
func (k *AgentKernel) GetState() KernelState {
	return k.state.Load().(KernelState)
}

// Subscribe 订阅事件
func (k *AgentKernel) Subscribe(handler EventHandler) {
	old := k.eventHandlers.Load().([]EventHandler)
	newHandlers := make([]EventHandler, len(old)+1)
	copy(newHandlers, old)
	newHandlers[len(old)] = handler
	k.eventHandlers.Store(newHandlers)
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
	systemMsg := k.buildSystemLayer(ctx, query)
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

		// L3: Task Adapter (coding/review/teaching/research) — per-query, not cached
		if l3 := promptL3(query.Content); l3 != "" {
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

	// L6: Knowledge base RAG
	if k.knowledgeCollector != nil {
		kbCtx, docIDs, _ := k.knowledgeCollector.InjectContext(ctx, query.Content, 500)
		if kbCtx != "" {
			messages = append(messages, Message{Role: "system", Content: "[Knowledge] " + kbCtx})
			// Save doc IDs for quality feedback in doReflection
			if len(docIDs) > 0 {
				if session.Metadata == nil {
					session.Metadata = make(map[string]interface{})
				}
				session.Metadata["knowledge_doc_ids"] = docIDs
			}
		}
	}

	slog.Info("buildMessages complete", "msgs", len(messages))
	return messages
}

// ── Layer Builders ──────────────────────────────────────────
// Each layer has a clear activation condition and injection point.

// buildSystemLayer assembles the prompt. Priority:
// 1. Custom prompt from ~/.openaide/data/prompts/system.{lang}.md
// 2. Layered prompts (L0+L1+L3) with file overrides per layer
func (k *AgentKernel) buildSystemLayer(ctx context.Context, query *Query) string {
	// If user has a custom monolithic system prompt, use it
	if sp := k.systemPrompt.Load().(string); sp != "" {
		if k.skillActor != nil {
			sp = k.skillActor.InjectPrompt(ctx, query.Content, sp)
		}
		return sp
	}

	// Build layered prompt (L0+L1+L3) — each layer file-overridable
	sp := k.buildSystemPrompt(query)

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
			if !k.queryOptions.OnApproval(tc.Function.Name, path, tc.Function.Arguments) {
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
	handlers := k.eventHandlers.Load().([]EventHandler)

	for _, h := range handlers {
		h := h
		go func(handler EventHandler) {
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
		session.Metadata["title"] = title
		k.sessionStore.Update(ctx, session)
	}
}

