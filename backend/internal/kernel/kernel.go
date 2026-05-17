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
	approver         Approver
	adaptiveRounds   *AdaptiveRounds

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

func (k *AgentKernel) SetSkillManager(sm *SkillManager) {
	k.skillManager = sm
}

func (k *AgentKernel) SetQualityGate(gate QualityGate) {
	k.qualityGate = gate
}

// Process 处理用户查询（同步）
func (k *AgentKernel) Process(ctx context.Context, query *Query) (*Response, error) {
	start := time.Now()

	// 1. 发布查询接收事件
	k.publishEvent(Event{
		Type:      EventQueryReceived,
		Source:    "kernel",
		Data:      map[string]interface{}{"session_id": query.SessionID, "content": query.Content},
		Timestamp: time.Now(),
	})

	// 2. 获取或创建会话
	session, err := k.getOrCreateSession(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("session error: %w", err)
	}

	// 3. 构建消息列表
	messages := k.buildMessages(session, query)

	// 4. 获取工具定义
	tools := k.toolExecutor.GetDefinitions()
	if len(query.Options.ToolFilter) > 0 {
		tools = k.toolExecutor.GetDefinitionsByNames(query.Options.ToolFilter)
	}

	// 5. ReAct 循环
	k.setState(StateThinking)
	totalToolCalls := 0
	totalTokens := 0

	maxRounds := k.maxRounds
	if k.adaptiveRounds != nil {
		maxRounds = k.adaptiveRounds.Calculate(query.Content, len(session.Messages))
	}
	for round := 0; round < maxRounds; round++ {
		// 检查上下文长度，必要时压缩
		if k.compressor != nil {
			tokenCount := k.compressor.EstimateTokens(messages)
			if tokenCount > k.maxTokens {
				compressed, saved, err := k.compressor.Compress(messages, k.maxTokens)
				if err == nil {
					messages = compressed
					slog.Debug("Context compressed", "saved_tokens", saved)
				}
			}
		}

		// 调用 LLM
		llmResp, err := k.llmProvider.Chat(ctx, messages, tools, k.buildOptions(query.Options))
		if err != nil {
			k.setState(StateError)
			return nil, fmt.Errorf("llm error: %w", err)
		}

		if llmResp.Usage != nil {
			totalTokens += llmResp.Usage.TotalTokens
		}

		// 添加 assistant 消息（包含 reasoning_content）
		messages = append(messages, Message{
			Role:             "assistant",
			Content:          llmResp.Content,
			ReasoningContent: llmResp.ReasoningContent,
			ToolCalls:        llmResp.ToolCalls,
		})

		// 无工具调用 -> 返回结果
		if len(llmResp.ToolCalls) == 0 {
			k.setState(StateResponding)

			// 保存到记忆
			k.saveToMemory(ctx, session.ID, messages)

			// 更新会话
			session.Messages = messages
			session.UpdatedAt = time.Now()
			k.sessionStore.Update(ctx, session)

			// 触发反思（如果启用）
			if k.reflection != nil {
				go k.doReflection(ctx, session.ID, query.Content, llmResp.Content, totalToolCalls)
			}

			k.setState(StateIdle)
			k.publishEvent(Event{
				Type:      EventResponseEnded,
				Source:    "kernel",
				Data:      map[string]interface{}{"session_id": session.ID, "tokens": totalTokens},
				Timestamp: time.Now(),
			})

			return &Response{
				Content:    llmResp.Content,
				ToolCalls:  totalToolCalls,
				TokensUsed: totalTokens,
				Duration:   time.Since(start),
				Model:      llmResp.Model,
			}, nil
		}

		// 并行执行工具调用
		k.setState(StateToolCalling)
		type toolResult struct {
			id      string
			name    string
			content string
			err     string
		}
		results := make([]toolResult, len(llmResp.ToolCalls))
		var wg sync.WaitGroup

		for i, tc := range llmResp.ToolCalls {
			k.publishEvent(Event{
				Type:      EventToolCallStarted,
				Source:    "kernel",
				Data:      map[string]interface{}{"tool": tc.Function.Name, "session_id": session.ID},
				Timestamp: time.Now(),
			})

			wg.Add(1)
			go func(idx int, call ToolCall) {
				defer wg.Done()
				r := k.executeTool(ctx, call, session.ID)
				content := fmt.Sprintf("%v", r.Content)
				errStr := ""
				if r.Error != "" {
					errStr = r.Error
					content = fmt.Sprintf("Error: %s", r.Error)
				}
				results[idx] = toolResult{
					id:      call.ID,
					name:    call.Function.Name,
					content: content,
					err:     errStr,
				}
				k.publishEvent(Event{
					Type:      EventToolCallEnded,
					Source:    "kernel",
					Data:      map[string]interface{}{"tool": call.Function.Name, "success": r.Error == "", "session_id": session.ID},
					Timestamp: time.Now(),
				})
			}(i, tc)
		}
		wg.Wait()
		totalToolCalls += len(results)

		// 按原始顺序添加 tool 结果
		for _, r := range results {
			messages = append(messages, Message{
				Role:       "tool",
				Content:    r.content,
				ToolCallID: r.id,
			})
		}
	}

	// 超出最大轮次
	k.setState(StateIdle)
	lastMsg := messages[len(messages)-1]
	return &Response{
		Content:    lastMsg.Content,
		ToolCalls:  totalToolCalls,
		TokensUsed: totalTokens,
		Duration:   time.Since(start),
		Model:      k.llmProvider.GetModelID(),
	}, nil
}

// ProcessStream 处理用户查询（流式）
func (k *AgentKernel) ProcessStream(ctx context.Context, query *Query) (<-chan StreamChunk, error) {
	start := time.Now()

	// 1. 发布查询接收事件
	k.publishEvent(Event{
		Type:      EventQueryReceived,
		Source:    "kernel",
		Data:      map[string]interface{}{"session_id": query.SessionID, "content": query.Content},
		Timestamp: time.Now(),
	})

	// 2. 获取或创建会话
	session, err := k.getOrCreateSession(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("session error: %w", err)
	}

	// 3. 构建消息列表
	messages := k.buildMessages(session, query)

	// 4. 获取工具定义
	tools := k.toolExecutor.GetDefinitions()
	if len(query.Options.ToolFilter) > 0 {
		tools = k.toolExecutor.GetDefinitionsByNames(query.Options.ToolFilter)
	}

	// 5. 调用 LLM 流式接口
	k.setState(StateThinking)

	llmStream, err := k.llmProvider.ChatStream(ctx, messages, tools, k.buildOptions(query.Options))
	if err != nil {
		k.setState(StateError)
		return nil, fmt.Errorf("llm stream error: %w", err)
	}

	resultChan := make(chan StreamChunk, 100)

	go func() {
		defer close(resultChan)

		var fullContent strings.Builder
		var reasoningContent strings.Builder
		var totalTokens int
		k.setState(StateResponding)

		for chunk := range llmStream {
			if chunk.Error != nil {
				resultChan <- StreamChunk{Error: chunk.Error, Done: true}
				k.setState(StateError)
				return
			}

			if chunk.Done {
				break
			}

			// 累积内容
			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)
			}
			if chunk.ReasoningContent != "" {
				reasoningContent.WriteString(chunk.ReasoningContent)
			}
			if chunk.Usage != nil {
				totalTokens = chunk.Usage.TotalTokens
			}

			// 转发流式块
			select {
			case resultChan <- chunk:
			case <-ctx.Done():
				return
			}
		}

		// 保存到记忆
		messages = append(messages, Message{
			Role:             "assistant",
			Content:          fullContent.String(),
			ReasoningContent: reasoningContent.String(),
		})
		k.saveToMemory(ctx, session.ID, messages)

		// 更新会话
		session.Messages = messages
		session.UpdatedAt = time.Now()
		k.sessionStore.Update(ctx, session)

		k.setState(StateIdle)
		k.publishEvent(Event{
			Type:      EventResponseEnded,
			Source:    "kernel",
			Data:      map[string]interface{}{"session_id": session.ID, "tokens": totalTokens},
			Timestamp: time.Now(),
		})

		// 发送结束标记
		resultChan <- StreamChunk{
			Done:  true,
			Usage: &TokenUsage{TotalTokens: totalTokens},
		}
	}()

	_ = start // 避免未使用变量警告，实际统计在 goroutine 中处理
	return resultChan, nil
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
	if k.systemPrompt != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: k.systemPrompt,
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

	// 注入相关知识库上下文
	if k.knowledgeCollector != nil {
		ctx := context.Background()
		kbCtx, err := k.knowledgeCollector.InjectContext(ctx, query.Content, 500)
		if err == nil && kbCtx != "" {
			messages = append(messages, Message{
				Role:    "system",
				Content: kbCtx,
			})
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

func (k *AgentKernel) doReflection(ctx context.Context, sessionID, query, response string, toolCalls int) {
	if k.reflection == nil {
		return
	}

	record := ExecutionRecord{
		Query:     query,
		Response:  response,
		Success:   true,
		ToolCalls: make([]ToolCall, 0),
	}

	result, err := k.reflection.Reflect(ctx, sessionID, record)
	if err != nil {
		slog.Warn("Reflection failed", "error", err)
		return
	}

	// 存储反思结果到会话
	if result != nil && k.sessionStore != nil {
		session, err := k.sessionStore.Get(ctx, sessionID)
		if err == nil && session != nil {
			if session.Metadata == nil {
				session.Metadata = make(map[string]interface{})
			}
			session.Metadata["reflection"] = result
			k.sessionStore.Update(ctx, session)
		}
	}

	// 自动知识抽取：质量门控通过后存入知识库
	k.autoSaveKnowledge(ctx, sessionID, query, response, toolCalls)
}

// autoSaveKnowledge 自动知识抽取 — 质量门控通过后存入知识库
func (k *AgentKernel) autoSaveKnowledge(ctx context.Context, sessionID, query, response string, toolCalls int) {
	if k.knowledgeCollector == nil {
		return
	}

	// 构建质量评估快照
	toolSuccesses := toolCalls // 简化：有工具调用就计为成功尝试
	toolFailures := 0

	var reflectResult *ReflectionResult
	if session, err := k.sessionStore.Get(ctx, sessionID); err == nil && session != nil {
		if ref, ok := session.Metadata["reflection"]; ok {
			if r, ok := ref.(*ReflectionResult); ok {
				reflectResult = r
				if r.Quality < 5 {
					toolFailures = 1 // 低质量标记
					toolSuccesses = 0
				}
			}
		}
	}

	// 质量门控判断
	if k.qualityGate != nil {
		if !k.qualityGate.Pass(query, response, toolSuccesses, toolFailures, reflectResult) {
			return
		}
	}

	// 存入知识库
	title := query
	if len(title) > 80 {
		title = title[:80] + "..."
	}
	tags := []string{"auto", "session:" + sessionID}
	if reflectResult != nil && reflectResult.Quality >= 7 {
		tags = append(tags, "high-quality")
	}

	if _, err := k.knowledgeCollector.AddKnowledge(ctx, title, response, "auto-extract", tags); err != nil {
		slog.Debug("Auto knowledge save failed", "error", err)
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
		go h.HandleEvent(event)
	}
}

func defaultSystemPrompt() string {
	return `你是 OpenAIDE，一个专业的 AI 编程助手。

## 身份
你是用户的编程伙伴，擅长代码分析、调试、重构和技术方案设计。

## 思考方式（ReAct）
面对任务，按以下步骤思考：
1. **理解**：用户真正想要什么？核心问题是什么？
2. **分析**：有哪些约束条件？需要什么信息？
3. **计划**：如何分步骤完成？步骤间有什么依赖？
4. **执行**：调用工具获取信息，执行操作
5. **验证**：结果是否正确？是否满足需求？
6. **总结**：给出清晰的最终结果

## 行为准则
1. **先理解后行动**：确保完全理解需求后再执行
2. **主动思考**：复杂问题先分解，逐步解决
3. **工具优先**：优先使用工具获取准确信息
4. **结果验证**：每次工具调用后检查结果
5. **完整性**：覆盖用户所有问题，不遗漏
6. **诚实透明**：不确定时说明，不编造

## 输出格式
- 使用清晰的 Markdown 格式
- 代码块标注语言类型
- 列表项简洁明了
- 重要信息用加粗或代码块
- 长回答先给结论，再展开`
}
