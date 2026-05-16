package kernel

import (
	"context"
	"fmt"
	"log/slog"
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
	reflection     Reflection
	learner        Learner
	patternDetector PatternDetector

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

	for round := 0; round < k.maxRounds; round++ {
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

		// 执行工具调用
		k.setState(StateToolCalling)
		for _, tc := range llmResp.ToolCalls {
			k.publishEvent(Event{
				Type:      EventToolCallStarted,
				Source:    "kernel",
				Data:      map[string]interface{}{"tool": tc.Function.Name, "session_id": session.ID},
				Timestamp: time.Now(),
			})

			result := k.executeTool(ctx, tc, session.ID)
			totalToolCalls++

			// 添加 tool 结果到消息
			toolContent := fmt.Sprintf("%v", result.Content)
			if result.Error != "" {
				toolContent = fmt.Sprintf("Error: %s", result.Error)
			}

			messages = append(messages, Message{
				Role:       "tool",
				Content:    toolContent,
				ToolCallID: tc.ID,
			})

			k.publishEvent(Event{
				Type:      EventToolCallEnded,
				Source:    "kernel",
				Data:      map[string]interface{}{"tool": tc.Function.Name, "success": result.Error == "", "session_id": session.ID},
				Timestamp: time.Now(),
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
	// 流式实现：先走同步流程，然后模拟流式输出
	// 后续可优化为真正的流式 ReAct
	resultChan := make(chan StreamChunk, 100)

	go func() {
		defer close(resultChan)

		// 发送开始事件
		resultChan <- StreamChunk{Content: "", Done: false}

		// 执行同步处理
		resp, err := k.Process(ctx, query)
		if err != nil {
			resultChan <- StreamChunk{Error: err, Done: true}
			return
		}

		// 模拟流式输出（按字符分块）
		content := resp.Content
		chunkSize := 10
		for i := 0; i < len(content); i += chunkSize {
			end := i + chunkSize
			if end > len(content) {
				end = len(content)
			}
			select {
			case resultChan <- StreamChunk{Content: content[i:end], Done: false}:
			case <-ctx.Done():
				return
			}
			time.Sleep(10 * time.Millisecond)
		}

		// 发送结束标记
		resultChan <- StreamChunk{
			Content: "",
			Done:    true,
			Usage: &TokenUsage{TotalTokens: resp.TokensUsed},
		}
	}()

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
func (k *AgentKernel) Unsubscribe(handler EventHandler) {
	k.eventMu.Lock()
	defer k.eventMu.Unlock()
	for i, h := range k.eventHandlers {
		if h == handler {
			k.eventHandlers = append(k.eventHandlers[:i], k.eventHandlers[i+1:]...)
			break
		}
	}
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

	// 加载历史记忆
	if k.memory != nil && len(session.Messages) > 0 {
		history, err := k.memory.Load(context.Background(), session.ID, 20)
		if err == nil && len(history) > 0 {
			messages = append(messages, history...)
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

	_, err := k.reflection.Reflect(ctx, sessionID, record)
	if err != nil {
		slog.Warn("Reflection failed", "error", err)
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
