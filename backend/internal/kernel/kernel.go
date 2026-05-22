package kernel

import (
	"context"
	"fmt"
	"log/slog"
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

	// 跟踪系统
	tracer  Tracer
	traceMu sync.Mutex

	// 检查点系统
	checkpointer Checkpointer

	// 事件系统
	eventHandlers []EventHandler
	eventMu       sync.RWMutex

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

// SetSystemPrompt 热更新系统提示词（无需重启内核）
func (k *AgentKernel) SetSystemPrompt(prompt string) {
	k.systemPrompt = prompt
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

func (k *AgentKernel) buildMessages(session *Session, query *Query) []Message {
	messages := make([]Message, 0)

	// 系统提示词
	systemPrompt := k.systemPrompt
	if k.skillManager != nil {
		systemPrompt = k.skillManager.InjectPrompt(query.Content, systemPrompt)
	}
	if systemPrompt != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// 注入上轮反思结果（提升后续对话质量）
	if session.Metadata != nil {
		if ref, ok := session.Metadata["reflection"]; ok {
			if r, ok := ref.(*ReflectionResult); ok && r != nil {
				hint := fmt.Sprintf("[上轮反思] 质量评分: %d/10", r.Quality)
				if len(r.Issues) > 0 {
					hint += fmt.Sprintf(" | 问题: %s", strings.Join(r.Issues, "; "))
				}
				if len(r.Suggestions) > 0 {
					hint += fmt.Sprintf(" | 建议: %s", strings.Join(r.Suggestions, "; "))
				}
				messages = append(messages, Message{
					Role:    "system",
					Content: hint,
				})
			}
			// 清除已注入的反思，避免重复使用
			delete(session.Metadata, "reflection")
		}
	}

	// 加载历史记忆
	if k.memory != nil && len(session.Messages) > 0 {
		history, err := k.memory.Load(context.Background(), session.ID, 20)
		if err == nil && len(history) > 0 {
			messages = append(messages, history...)
		}
	}

	// 注入跨会话学习洞察
	if k.learner != nil {
		insights, err := k.learner.GetInsights(context.Background(), query.Content)
		if err == nil && len(insights) > 0 {
			messages = append(messages, Message{
				Role:    "system",
				Content: "[历史学习] " + strings.Join(insights, " | "),
			})
		}
	}

	// 注入相关知识库上下文
	if k.knowledgeCollector != nil {
		ctx := context.Background()
		kbCtx, docIDs, err := k.knowledgeCollector.InjectContext(ctx, query.Content, 500)
		if err == nil && kbCtx != "" {
			messages = append(messages, Message{
				Role:    "system",
				Content: kbCtx,
			})
			// 记录被注入的知识文档ID到会话，后续用于反馈
			if len(docIDs) > 0 {
				if session.Metadata == nil {
					session.Metadata = make(map[string]interface{})
				}
				session.Metadata["knowledge_doc_ids"] = docIDs
			}
		}
	}

	// 用户当前查询
	messages = append(messages, Message{
		Role:    "user",
		Content: query.Content,
	})

	return messages
}

func (k *AgentKernel) buildOptions(opts QueryOptions) map[string]interface{} {
	options := make(map[string]interface{})
	if opts.Temperature > 0 {
		options["temperature"] = opts.Temperature
	} else {
		options["temperature"] = 0.7
	}
	if opts.MaxTokens > 0 {
		options["max_tokens"] = opts.MaxTokens
	}
	if opts.ResponseFormat != nil {
		options["response_format"] = opts.ResponseFormat
	}
	return options
}

func (k *AgentKernel) executeTool(ctx context.Context, tc ToolCall, sessionID string) *ToolResult {
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

func (k *AgentKernel) publishEvent(event Event) {
	k.eventMu.RLock()
	handlers := make([]EventHandler, len(k.eventHandlers))
	copy(handlers, k.eventHandlers)
	k.eventMu.RUnlock()

	for _, h := range handlers {
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
		return // 已有标题
	}

	for _, msg := range session.Messages {
		if msg.Role == "user" && msg.Content != "" {
			rs := []rune(strings.TrimSpace(msg.Content))
			if len(rs) == 0 {
				return
			}
			title := string(rs[:min(len(rs), 25)])
			if len(rs) > 25 {
				title += "…"
			}
			session.Metadata["title"] = title
			return
		}
	}
}
