package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// ToolCallingService 工具调用循环服务
// 将 ToolService 与 LLM 对话连接，实现完整的 tool calling 闭环
type ToolCallingService struct {
	toolSvc      ToolProvider
	modelSvc     ModelCaller
	dialogueSvc  DialogueStore
	logger       Logger
	usageService UsageTracker
	eventBus     EventPublisher
	maxRounds    int
	sessionRecorder  *SessionRecorder  // ReAct 会话记录器
	metricsCollector *ToolMetricsCollector // 工具指标收集器
}

// SetEventBus 设置事件发布器
func (s *ToolCallingService) SetEventBus(bus EventPublisher) {
	s.eventBus = bus
}

// SetUsageService 设置用量追踪器
func (s *ToolCallingService) SetUsageService(usageService UsageTracker) {
	s.usageService = usageService
}

// SetDialogueService 设置对话存储
func (s *ToolCallingService) SetDialogueService(dialogueSvc DialogueStore) {
	s.dialogueSvc = dialogueSvc
}

// NewToolCallingService 创建工具调用服务（使用接口，降低耦合）
func NewToolCallingService(toolSvc ToolProvider, modelSvc ModelCaller, logger Logger) *ToolCallingService {
	return &ToolCallingService{
		toolSvc:          toolSvc,
		modelSvc:         modelSvc,
		logger:           logger,
		maxRounds:        20,
		sessionRecorder:  NewSessionRecorder(""),
		metricsCollector: NewToolMetricsCollector(),
	}
}

// SendMessageWithTools 发送消息并自动处理工具调用循环
func (s *ToolCallingService) SendMessageWithTools(
	ctx context.Context,
	dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (*models.Message, error) {
	// 1. 获取工具定义（支持技能工具过滤）
	var toolDefs []map[string]interface{}
	if filterRaw, ok := options["tool_filter"]; ok {
		if filter := toStringSlice(filterRaw); len(filter) > 0 {
			toolDefs = s.toolSvc.GetToolDefinitionsWithMCPByNames(filter)
		}
	}
	if len(toolDefs) == 0 {
		toolDefs = s.toolSvc.GetToolDefinitionsWithMCP()
	}
	if len(toolDefs) > 15 {
		toolDefs = s.filterRelevantTools(content, toolDefs)
	}
	if len(toolDefs) == 0 {
		// 发布工具调用事件
		if s.eventBus != nil {
			s.eventBus.Publish(ctx, models.EventTopicTool, models.EventTypeToolCalled, "tool_calling", map[string]interface{}{
				"tool_name": "unknown",
				"params":    map[string]interface{}{"content": content},
			})
		}
		// 无可用工具，退化为普通对话
		return nil, fmt.Errorf("no tools available")
	}

	// 2. 转换为 LLM ToolDefinition 格式
	llmTools := make([]llm.ToolDefinition, 0, len(toolDefs))
	for _, def := range toolDefs {
		fnMap, _ := def["function"].(map[string]interface{})
		if fnMap == nil {
			continue
		}
		name, _ := fnMap["name"].(string)
		desc, _ := fnMap["description"].(string)
		params, _ := fnMap["parameters"].(map[string]interface{})

		llmTools = append(llmTools, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}

	if len(llmTools) == 0 {
		return nil, fmt.Errorf("no valid tool definitions")
	}

	// 3. 构建消息（加载历史对话以保持上下文记忆）
	messages := s.buildMessagesWithHistory(ctx, dialogueID, content)

	// 4. 初始化 ReAct 状态机
	sessionID := GenerateUUID()
	stateMachine := s.sessionRecorder.StartSession(sessionID, dialogueID, userID, modelID)

	// 5. 工具调用循环（ReAct 模式，参考 Hermes Agent）
	var totalUsage llm.Usage
	startTime := time.Now()
	var lastToolSignature string
	var repeatCount int

	for round := 0; round < s.maxRounds; round++ {
		messages, _ = s.compressToolOutputs(messages)
		if s.isContextOverflow(messages, modelID) {
			slog.Info("Context overflow detected, triggering LLM summarization", "component", "ToolCalling")
			messages, _ = s.summarizeWithLLM(ctx, messages, modelID)
		}

		// === ReAct: Thinking 阶段 ===
		thinkStep := stateMachine.StartThinking()

		resp, err := s.modelSvc.ChatWithTools(modelID, messages, llmTools, options)
		if err != nil {
			stateMachine.Complete(StateError, totalUsage.TotalTokens)
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		// 累计token使用量
		if resp.Usage != nil {
			totalUsage.PromptTokens += resp.Usage.PromptTokens
			totalUsage.CompletionTokens += resp.Usage.CompletionTokens
			totalUsage.TotalTokens += resp.Usage.TotalTokens
		}

		if len(resp.Choices) == 0 {
			stateMachine.Complete(StateError, totalUsage.TotalTokens)
			return nil, fmt.Errorf("empty response from LLM")
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		// 结束 Thinking 阶段
		stateMachine.EndThinking(thinkStep, &assistantMsg)

		// 追加 assistant 消息到历史
		messages = append(messages, assistantMsg)

		// 检查是否有工具调用
		if len(assistantMsg.ToolCalls) == 0 {
			// 无工具调用，返回文本回复
			result := assistantMsg.Content
			if result == "" {
				result = "(无回复内容)"
			}

			// 记录总token使用量（所有轮次累计）
			if s.usageService != nil && totalUsage.TotalTokens > 0 {
				go s.recordToolCallingUsage(ctx, userID, dialogueID, modelID, &totalUsage, time.Since(startTime))
			}

			// 完成 ReAct 会话
			stateMachine.Complete(StateCompleted, totalUsage.TotalTokens)
			go s.saveSessionAsync(sessionID)

			return s.saveToolCallingResult(dialogueID, "assistant", result, assistantMsg.ReasoningContent), nil
		}

		// === ReAct: Tool Call 阶段 ===
		toolStep := stateMachine.StartToolCall(assistantMsg.ToolCalls)
		toolStart := time.Now()

		// 循环检测：检查是否重复调用同一工具
		currentSignature := toolCallSignature(assistantMsg.ToolCalls)
		if currentSignature == lastToolSignature {
			repeatCount++
			if repeatCount >= 3 {
				slog.Warn("Tool calling loop detected, breaking", "component", "ToolCalling", "round", round, "signature", currentSignature)
				messages = append(messages, llm.Message{
					Role:    llm.RoleSystem,
					Content: "检测到重复调用同一工具，请换一种方式回答用户问题，不要再调用相同工具。",
				})
				lastToolSignature = ""
				repeatCount = 0
				continue
			}
		} else {
			lastToolSignature = currentSignature
			repeatCount = 0
		}

		// 执行工具调用（并行执行多个工具，参考 Hermes Agent 的并发模式）
		var toolRecords []ToolCallRecord
		if len(assistantMsg.ToolCalls) > 1 {
			type toolResult struct {
				toolCallID string
				content    string
			}
			results := make([]toolResult, len(assistantMsg.ToolCalls))
			var wg sync.WaitGroup
			for i, tc := range assistantMsg.ToolCalls {
				wg.Add(1)
				go func(idx int, toolCall llm.ToolCall) {
					defer wg.Done()
					results[idx] = toolResult{
						toolCallID: toolCall.ID,
						content:    s.executeToolCall(ctx, toolCall, dialogueID),
					}
				}(i, tc)
			}
			wg.Wait()
			for _, r := range results {
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    r.content,
					ToolCallID: r.toolCallID,
				})
			}
		} else {
			for _, tc := range assistantMsg.ToolCalls {
				toolResult := s.executeToolCall(ctx, tc, dialogueID)
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    toolResult,
					ToolCallID: tc.ID,
				})
			}
		}

		// 结束 Tool Call 阶段
		stateMachine.EndToolCall(toolStep, toolRecords)

		// === ReAct: Observation 阶段 ===
		obsStep := stateMachine.StartObservation(fmt.Sprintf("Completed %d tool calls in round %d", len(assistantMsg.ToolCalls), round+1))
		stateMachine.EndObservation(obsStep)

		// 记录指标
		s.metricsCollector.RecordCall("batch", true, false, time.Since(toolStart))
	}

	// 超出最大轮次，查找最后一条 assistant 消息
	lastAssistantContent := ""
	lastAssistantReasoning := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleAssistant && messages[i].Content != "" {
			lastAssistantContent = messages[i].Content
			lastAssistantReasoning = messages[i].ReasoningContent
			break
		}
	}
	if lastAssistantContent == "" {
		lastAssistantContent = "I reached the maximum number of tool-calling rounds. Please continue the conversation if you need more work done."
	}

	// 记录总token使用量（即使超出轮次也要记录）
	if s.usageService != nil && totalUsage.TotalTokens > 0 {
		go s.recordToolCallingUsage(ctx, userID, dialogueID, modelID, &totalUsage, time.Since(startTime))
	}

	// 完成 ReAct 会话（超出轮次）
	stateMachine.Complete(StateMaxRounds, totalUsage.TotalTokens)
	go s.saveSessionAsync(sessionID)

	return s.saveToolCallingResult(dialogueID, "assistant", lastAssistantContent, lastAssistantReasoning), nil
}

// recordToolCallingUsage 记录工具调用的token使用量
func (s *ToolCallingService) recordToolCallingUsage(ctx context.Context, userID, dialogueID, modelID string, usage *llm.Usage, duration time.Duration) {
	if s.usageService == nil {
		return
	}

	// 获取模型信息
	model, err := s.modelSvc.GetModel(modelID)
	if err != nil {
		model = &models.Model{Name: modelID, Provider: "unknown"}
	}

	record := &models.UsageRecord{
		ID:               GenerateUUID(),
		UserID:           userID,
		DialogueID:       dialogueID,
		MessageID:        fmt.Sprintf("tool_call_%d", time.Now().Unix()),
		Provider:         model.Provider,
		ModelID:          model.ID,
		ModelName:        model.Name,
		PromptTokens:     int64(usage.PromptTokens),
		CompletionTokens: int64(usage.CompletionTokens),
		TotalTokens:      int64(usage.TotalTokens),
		RequestType:      "tool_calling",
		IsStreaming:      false,
		Duration:         duration.Milliseconds(),
		Success:          true,
	}

	if err := s.usageService.RecordUsage(record); err != nil {
		s.logger.Error(ctx, "Failed to record tool calling usage: %v", err)
	}
}

// executeToolCall 执行单个工具调用
func (s *ToolCallingService) executeToolCall(ctx context.Context, tc llm.ToolCall, dialogueID string) string {
	toolCall := &models.ToolCall{
		ID:        tc.ID,
		Name:      tc.Function.Name,
		Arguments: tc.Function.Arguments,
	}

	s.logger.Info(ctx, "Executing tool: %s", tc.Function.Name)

	toolCtx, toolCancel := context.WithTimeout(ctx, 30*time.Second)
	defer toolCancel()

	result, err := s.toolSvc.ExecuteTool(toolCtx, toolCall, dialogueID, "", "")
	if err != nil {
		var confirmErr *ConfirmationRequiredError
		if errors.As(err, &confirmErr) {
			warningMsg := fmt.Sprintf("⚠️ 需要用户确认才能执行此命令: %s\n风险: %s\n请使用 approved=true 参数重新调用，或在确认后再次请求。", confirmErr.Command, confirmErr.Risk)
			s.logger.Warn(ctx, "Tool %s requires confirmation: %s", tc.Function.Name, confirmErr.Command)
			return warningMsg
		}

		errMsg := fmt.Sprintf("Tool execution error: %v", err)
		s.logger.Error(ctx, "Tool %s failed: %v", tc.Function.Name, err)
		if s.eventBus != nil {
			s.eventBus.Publish(ctx, models.EventTopicTool, models.EventTypeToolFailed, "tool_calling", map[string]interface{}{
				"tool_name":    tc.Function.Name,
				"tool_call_id": tc.ID,
				"error":        errMsg,
			})
		}
		return errMsg
	}

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicTool, models.EventTypeToolCompleted, "tool_calling", map[string]interface{}{
			"tool_name":    tc.Function.Name,
			"tool_call_id": tc.ID,
			"result":       result.Content,
		})
	}

	resultJSON, err := json.Marshal(result.Content)
	if err != nil {
		return fmt.Sprintf("%v", result.Content)
	}

	return string(resultJSON)
}

func (s *ToolCallingService) SendMessageWithToolsStream(
	ctx context.Context,
	dialogueID, userID, content, modelID string,
	options map[string]interface{},
) (<-chan llm.ChatStreamChunk, error) {
	var toolDefs []map[string]interface{}
	if filterRaw, ok := options["tool_filter"]; ok {
		if filter := toStringSlice(filterRaw); len(filter) > 0 {
			toolDefs = s.toolSvc.GetToolDefinitionsWithMCPByNames(filter)
		}
	}
	if len(toolDefs) == 0 {
		toolDefs = s.toolSvc.GetToolDefinitionsWithMCP()
	}
	if len(toolDefs) > 15 {
		toolDefs = s.filterRelevantTools(content, toolDefs)
	}
	if len(toolDefs) == 0 {
		return nil, fmt.Errorf("no tools available")
	}

	llmTools := make([]llm.ToolDefinition, 0, len(toolDefs))
	for _, def := range toolDefs {
		fnMap, _ := def["function"].(map[string]interface{})
		if fnMap == nil {
			continue
		}
		name, _ := fnMap["name"].(string)
		desc, _ := fnMap["description"].(string)
		params, _ := fnMap["parameters"].(map[string]interface{})

		llmTools = append(llmTools, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}

	if len(llmTools) == 0 {
		return nil, fmt.Errorf("no valid tool definitions")
	}

	messages := s.buildMessagesWithHistory(ctx, dialogueID, content)
	sessionID := GenerateUUID()
	stateMachine := s.sessionRecorder.StartSession(sessionID, dialogueID, userID, modelID)

	ch := make(chan llm.ChatStreamChunk, 64)

	go func() {
		defer close(ch)

		var totalUsage llm.Usage
		startTime := time.Now()
		var lastToolSignature string
		var repeatCount int

		for round := 0; round < s.maxRounds; round++ {
			var compactInfo *llm.CompactInfo
			messages, compactInfo = s.compressToolOutputs(messages)
			if compactInfo != nil {
				ch <- llm.ChatStreamChunk{CompactInfo: compactInfo}
			}
			if s.isContextOverflow(messages, modelID) {
				slog.Info("Context overflow detected, triggering LLM summarization", "component", "ToolCalling")
				var summaryInfo *llm.CompactInfo
				messages, summaryInfo = s.summarizeWithLLM(ctx, messages, modelID)
				if summaryInfo != nil {
					ch <- llm.ChatStreamChunk{CompactInfo: summaryInfo}
				}
			}

			thinkStep := stateMachine.StartThinking()

			resp, err := s.modelSvc.ChatWithTools(modelID, messages, llmTools, options)
			if err != nil {
				stateMachine.Complete(StateError, totalUsage.TotalTokens)
				ch <- llm.ChatStreamChunk{Error: fmt.Errorf("LLM call failed: %w", err)}
				return
			}

			if resp.Usage != nil {
				totalUsage.PromptTokens += resp.Usage.PromptTokens
				totalUsage.CompletionTokens += resp.Usage.CompletionTokens
				totalUsage.TotalTokens += resp.Usage.TotalTokens
			}

			if len(resp.Choices) == 0 {
				stateMachine.Complete(StateError, totalUsage.TotalTokens)
				ch <- llm.ChatStreamChunk{Error: fmt.Errorf("empty response from LLM")}
				return
			}

			choice := resp.Choices[0]
			assistantMsg := choice.Message
			stateMachine.EndThinking(thinkStep, &assistantMsg)
			messages = append(messages, assistantMsg)

			if assistantMsg.ReasoningContent != "" {
				ch <- llm.ChatStreamChunk{
					Choices: []llm.StreamChoice{{
						Delta: llm.MessageDelta{ReasoningContent: assistantMsg.ReasoningContent},
					}},
				}
			}

			if len(assistantMsg.ToolCalls) == 0 {
				result := assistantMsg.Content
				if result == "" {
					result = "(无回复内容)"
				}

				if s.usageService != nil && totalUsage.TotalTokens > 0 {
					go s.recordToolCallingUsage(ctx, userID, dialogueID, modelID, &totalUsage, time.Since(startTime))
				}
				stateMachine.Complete(StateCompleted, totalUsage.TotalTokens)
				go s.saveSessionAsync(sessionID)
				s.saveToolCallingResult(dialogueID, "assistant", result, assistantMsg.ReasoningContent)

				ch <- llm.ChatStreamChunk{
					Choices: []llm.StreamChoice{{
						Delta:          llm.MessageDelta{Content: result, Role: "assistant"},
						FinishReason:   "stop",
					}},
				}
				return
			}

			toolStep := stateMachine.StartToolCall(assistantMsg.ToolCalls)
			toolStart := time.Now()

			currentSignature := toolCallSignature(assistantMsg.ToolCalls)
			if currentSignature == lastToolSignature {
				repeatCount++
				if repeatCount >= 3 {
					slog.Warn("Tool calling loop detected, breaking", "component", "ToolCalling", "round", round)
					messages = append(messages, llm.Message{
						Role:    llm.RoleSystem,
						Content: "检测到重复调用同一工具，请换一种方式回答用户问题，不要再调用相同工具。",
					})
					lastToolSignature = ""
					repeatCount = 0
					continue
				}
			} else {
				lastToolSignature = currentSignature
				repeatCount = 0
			}

			for _, tc := range assistantMsg.ToolCalls {
				toolName := tc.Function.Name
				toolArgs := tc.Function.Arguments
				ch <- llm.ChatStreamChunk{
					Choices: []llm.StreamChoice{{
						Delta: llm.MessageDelta{
							ToolCalls: []llm.ToolCallDelta{{
								ID:   tc.ID,
								Type: "function",
								Function: &llm.FunctionDelta{
									Name:      toolName,
									Arguments: toolArgs,
								},
							}},
						},
					}},
				}

				toolResult := s.executeToolCall(ctx, tc, dialogueID)

				resultPreview := toolResult
				if len(resultPreview) > 500 {
					resultPreview = resultPreview[:500] + "..."
				}
				ch <- llm.ChatStreamChunk{
					ToolDone: &llm.ToolDoneInfo{
						Tool:   toolName,
						Result: resultPreview,
					},
				}

				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    toolResult,
					ToolCallID: tc.ID,
				})
			}

			stateMachine.EndToolCall(toolStep, nil)
			stateMachine.StartObservation(fmt.Sprintf("Completed %d tool calls in round %d", len(assistantMsg.ToolCalls), round+1))
			s.metricsCollector.RecordCall("batch", true, false, time.Since(toolStart))
		}

		lastAssistantContent := ""
		lastAssistantReasoning := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleAssistant && messages[i].Content != "" {
				lastAssistantContent = messages[i].Content
				lastAssistantReasoning = messages[i].ReasoningContent
				break
			}
		}
		if lastAssistantContent == "" {
			lastAssistantContent = "I reached the maximum number of tool-calling rounds."
		}

		if s.usageService != nil && totalUsage.TotalTokens > 0 {
			go s.recordToolCallingUsage(ctx, userID, dialogueID, modelID, &totalUsage, time.Since(startTime))
		}
		stateMachine.Complete(StateMaxRounds, totalUsage.TotalTokens)
		go s.saveSessionAsync(sessionID)
		s.saveToolCallingResult(dialogueID, "assistant", lastAssistantContent, lastAssistantReasoning)

		ch <- llm.ChatStreamChunk{
			Choices: []llm.StreamChoice{{
				Delta:        llm.MessageDelta{Content: lastAssistantContent, Role: "assistant"},
				FinishReason: "stop",
			}},
		}
	}()

	return ch, nil
}

// saveToolCallingResult 保存工具调用的最终结果
func (s *ToolCallingService) saveToolCallingResult(dialogueID, sender, content string, reasoningContent ...string) *models.Message {
	// 通过数据库直接插入消息
	now := time.Now()
	msg := &models.Message{
		ID:         GenerateUUID(),
		DialogueID: dialogueID,
		Sender:     sender,
		Content:    content,
		CreatedAt:  now,
	}
	if len(reasoningContent) > 0 && reasoningContent[0] != "" {
		msg.ReasoningContent = reasoningContent[0]
	}

	// 使用 dialogueSvc 保存消息（通过接口，不直接访问 db）
	if s.dialogueSvc != nil {
		var rc string
		if len(reasoningContent) > 0 {
			rc = reasoningContent[0]
		}
		if savedMsg, err := s.dialogueSvc.AddMessage(dialogueID, sender, content, rc); err == nil {
			return &savedMsg
		}
	}

	slog.Warn("Warning: could not save message, no dialogue service", "component", "ToolCalling")
	return msg
}

// toStringSlice 将 interface{} 转为 []string（处理 JSON 反序列化后的 []interface{} 类型）
func toStringSlice(raw interface{}) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// compressToolOutputs 上下文压缩（参考 OpenCode Session Compaction）
// 两阶段压缩：Phase 1 修剪旧工具输出，Phase 2 LLM 摘要（当接近上下文窗口时）
func (s *ToolCallingService) compressToolOutputs(messages []llm.Message) ([]llm.Message, *llm.CompactInfo) {
	const maxMessages = 40
	const keepRecent = 10

	if len(messages) <= maxMessages {
		return messages, nil
	}

	compressSet := make(map[int]bool)
	skipSet := make(map[int]bool)
	oldCount := len(messages) - keepRecent

	for i, msg := range messages {
		if i < oldCount && msg.Role == llm.RoleTool && len(msg.Content) > 200 {
			compressSet[i] = true
			if i > 0 && messages[i-1].Role == llm.RoleAssistant && len(messages[i-1].ToolCalls) > 0 {
				skipSet[i-1] = true
			}
		}
	}

	compressed := make([]llm.Message, 0, len(messages))
	prunedCount := 0

	for i, msg := range messages {
		if skipSet[i] {
			continue
		}
		if compressSet[i] {
			compressedContent := compressToolResult(msg.Content, 150)
			compressed = append(compressed, llm.Message{
				Role:       msg.Role,
				Content:    compressedContent,
				ToolCallID: msg.ToolCallID,
			})
			if i > 0 && messages[i-1].Role == llm.RoleAssistant && len(messages[i-1].ToolCalls) > 0 {
				compressed = append(compressed, llm.Message{
					Role:      llm.RoleAssistant,
					Content:   messages[i-1].Content,
					ToolCalls: messages[i-1].ToolCalls,
				})
			}
			prunedCount++
		} else {
			compressed = append(compressed, msg)
		}
	}

	slog.Info("Context compression", "component", "ToolCalling", "messages", len(messages), "pruned", prunedCount)

	estimator := NewTokenEstimator()
	oldTokens := 0
	newTokens := 0
	for _, msg := range messages {
		oldTokens += estimator.EstimateTokens(msg.Content, "")
	}
	for _, msg := range compressed {
		newTokens += estimator.EstimateTokens(msg.Content, "")
	}

	info := &llm.CompactInfo{
		Reason:         "tool_output_compression",
		BeforeMessages: len(messages),
		AfterMessages:  len(compressed),
		SavedTokens:    oldTokens - newTokens,
	}
	return compressed, info
}

// isContextOverflow 检测是否接近上下文窗口溢出（参考 OpenCode isOverflow）
// 安全缓冲区：保留 20% 的上下文给响应
func (s *ToolCallingService) isContextOverflow(messages []llm.Message, modelID string) bool {
	estimator := NewTokenEstimator()
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += estimator.EstimateTokens(msg.Content, modelID)
	}

	contextLimit := 128000
	model, err := s.modelSvc.GetModel(modelID)
	if err == nil && model.Config != nil {
		if cl, ok := model.Config["context_length"].(float64); ok && cl > 0 {
			contextLimit = int(cl)
		}
	}

	safetyBuffer := 20000
	if model != nil && model.Config != nil {
		if mt, ok := model.Config["max_tokens"].(float64); ok && int(mt) > safetyBuffer {
			safetyBuffer = int(mt)
		}
	}

	usableQuota := contextLimit - safetyBuffer
	return totalTokens > usableQuota
}

// summarizeWithLLM 使用 LLM 对旧消息进行摘要压缩（参考 OpenCode SessionCompaction.create）
func (s *ToolCallingService) summarizeWithLLM(ctx context.Context, messages []llm.Message, modelID string) ([]llm.Message, *llm.CompactInfo) {
	if len(messages) <= 10 {
		return messages, nil
	}

	var oldMessages []llm.Message
	var recentMessages []llm.Message

	splitPoint := len(messages) - 10
	oldMessages = messages[:splitPoint]
	recentMessages = messages[splitPoint:]

	var historyText strings.Builder
	for _, msg := range oldMessages {
		switch msg.Role {
		case llm.RoleUser:
			historyText.WriteString(fmt.Sprintf("用户: %s\n", msg.Content))
		case llm.RoleAssistant:
			content := msg.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			historyText.WriteString(fmt.Sprintf("助手: %s\n", content))
		case llm.RoleTool:
			content := compressToolResult(msg.Content, 200)
			historyText.WriteString(fmt.Sprintf("工具(%s): %s\n", msg.ToolCallID, content))
		}
	}

	summaryPrompt := fmt.Sprintf(`请用中文简洁总结以下对话历史，必须保留：
1. 用户的核心需求和目标
2. 已做出的关键决策和结论
3. 重要的技术细节（IP地址、文件路径、端口号、配置值、错误信息）
4. 工具执行的关键结果（成功/失败、输出摘要）
5. 未解决的问题和待办事项

对话历史：
%s

请用要点格式总结，不超过500字：`, historyText.String())

	summaryModelID := modelID
	models, err := s.modelSvc.ListModels()
	if err == nil {
	outer:
		for _, m := range models {
			for _, tag := range m.Tags {
				if strings.TrimSpace(tag) == "fast" {
					summaryModelID = m.ID
					break outer
				}
			}
		}
	}

	resp, err := s.modelSvc.Chat(summaryModelID, []llm.Message{
		{Role: llm.RoleUser, Content: summaryPrompt},
	}, map[string]interface{}{"max_tokens": 2000})

	if err != nil {
		slog.Error("LLM summarization failed, using messages as-is", "component", "ToolCalling", "error", err)
		return messages, nil
	}

	summary := ""
	if len(resp.Choices) > 0 {
		summary = resp.Choices[0].Message.Content
	}

	result := []llm.Message{
		{Role: llm.RoleSystem, Content: fmt.Sprintf("[对话历史摘要]\n%s\n[摘要结束 - 从此处继续对话]", summary)},
	}

	for i, msg := range recentMessages {
		if msg.Role == llm.RoleTool {
			if i == 0 {
				continue
			}
			hasAssistantBefore := false
			for j := i - 1; j >= 0; j-- {
				if recentMessages[j].Role == llm.RoleAssistant && len(recentMessages[j].ToolCalls) > 0 {
					hasAssistantBefore = true
					break
				}
				if recentMessages[j].Role != llm.RoleTool {
					break
				}
			}
			if !hasAssistantBefore {
				continue
			}
		}
		result = append(result, msg)
	}

	slog.Info("LLM summarization", "component", "ToolCalling", "old_messages", len(oldMessages), "recent_messages", len(recentMessages))

	estimator := NewTokenEstimator()
	oldTokens := 0
	newTokens := 0
	for _, msg := range messages {
		oldTokens += estimator.EstimateTokens(msg.Content, "")
	}
	for _, msg := range result {
		newTokens += estimator.EstimateTokens(msg.Content, "")
	}

	info := &llm.CompactInfo{
		Reason:         "llm_summarization",
		BeforeMessages: len(messages),
		AfterMessages:  len(result),
		SavedTokens:    oldTokens - newTokens,
	}
	return result, info
}

// buildMessagesWithHistory 构建包含历史对话的消息列表，保持上下文记忆
func (s *ToolCallingService) buildMessagesWithHistory(ctx context.Context, dialogueID string, currentContent string) []llm.Message {
	messages := []llm.Message{}

	dialogueSvc := s.getDialogueService()
	if dialogueSvc != nil {
		history := dialogueSvc.GetMessages(dialogueID)
		const maxHistoryMessages = 50
		startIdx := 0
		if len(history) > maxHistoryMessages {
			startIdx = len(history) - maxHistoryMessages
		}

		estimator := NewTokenEstimator()
		modelID := ""
		if m, err := s.modelSvc.ListModels(); err == nil && len(m) > 0 {
			modelID = m[0].ID
		}

		for i := startIdx; i < len(history); i++ {
			msg := history[i]
			var role string
			switch msg.Sender {
			case "user":
				role = llm.RoleUser
			case "assistant":
				role = llm.RoleAssistant
			case "system":
				role = llm.RoleSystem
			case "tool":
				role = llm.RoleTool
			default:
				role = llm.RoleUser
			}

			content := msg.Content
			isRecent := i >= len(history)-8

			if !isRecent {
				if role == llm.RoleAssistant {
					if len(content) > 800 {
						keyPoints := extractKeyPoints(content)
						if keyPoints != "" {
							content = keyPoints
						} else {
							content = content[:600] + "\n...(回复已压缩)"
						}
					}
				}
				if role == llm.RoleTool {
					if len(content) > 400 {
						content = compressToolResult(content, 400)
					}
				}
			}

			llmMsg := llm.Message{
				Role:    role,
				Content: content,
			}
			if msg.ReasoningContent != "" {
				llmMsg.ReasoningContent = msg.ReasoningContent
			}
			if msg.ToolCallID != "" {
				llmMsg.ToolCallID = msg.ToolCallID
			}
			messages = append(messages, llmMsg)
		}

		totalTokens := 0
		for _, msg := range messages {
			totalTokens += estimator.EstimateTokens(msg.Content, modelID)
		}
		slog.Info("Built messages with history", "component", "ToolCalling",
			"history_count", len(history), "messages", len(messages),
			"estimated_tokens", totalTokens)
	}

	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx >= 0 && messages[lastUserIdx].Content == currentContent {
		return messages
	}

	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: currentContent,
	})

	return messages
}

func (s *ToolCallingService) getDialogueService() DialogueStore {
	if s.dialogueSvc != nil {
		return s.dialogueSvc
	}
	return nil
}

// saveSessionAsync 异步保存 ReAct 会话到文件
func (s *ToolCallingService) saveSessionAsync(sessionID string) {
	if s.sessionRecorder == nil {
		return
	}
	go func() {
		filename, err := s.sessionRecorder.SaveSessionToFile(sessionID)
		if err != nil {
			slog.Error("Failed to save session", "component", "ToolCalling", "session_id", sessionID, "error", err)
		} else {
			slog.Info("Session saved", "component", "ToolCalling", "session_id", sessionID, "file", filename)
		}
	}()
}

// GetSessionMetrics 获取工具调用指标
func (s *ToolCallingService) GetSessionMetrics() ToolCallMetrics {
	if s.metricsCollector == nil {
		return ToolCallMetrics{ToolBreakdown: make(map[string]int)}
	}
	return s.metricsCollector.GetMetrics()
}

// GetSessionExport 导出指定会话
func (s *ToolCallingService) GetSessionExport(sessionID string) ([]byte, error) {
	if s.sessionRecorder == nil {
		return nil, fmt.Errorf("session recorder not initialized")
	}
	return s.sessionRecorder.ExportSession(sessionID)
}

// ListReActSessions 列出所有 ReAct 会话
func (s *ToolCallingService) ListReActSessions() []string {
	if s.sessionRecorder == nil {
		return []string{}
	}
	return s.sessionRecorder.ListSessions()
}

func compressToolResult(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}

	lines := strings.Split(content, "\n")
	if len(lines) <= 3 {
		return content[:maxLen] + "..."
	}

	var important []string
	var tail []string
	tailCount := 5

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isImportant := false
		lower := strings.ToLower(trimmed)
		keywords := []string{"error", "fail", "warn", "success", "ok", "running",
			"active", "listen", "connect", "refused", "timeout", "denied",
			"permission", "not found", "no such", "already exist",
			"错误", "失败", "成功", "警告", "拒绝", "超时", "找不到"}
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				isImportant = true
				break
			}
		}
		if isImportant {
			important = append(important, trimmed)
		}
		if i >= len(lines)-tailCount {
			tail = append(tail, trimmed)
		}
	}

	var result strings.Builder
	if len(important) > 0 {
		result.WriteString("[关键信息] ")
		for idx, imp := range important {
			if idx > 0 {
				result.WriteString("; ")
			}
			result.WriteString(imp)
			if result.Len() > maxLen/2 {
				break
			}
		}
		result.WriteString("\n")
	}

	result.WriteString(fmt.Sprintf("[共%d行输出，显示最后%d行]\n", len(lines), len(tail)))
	for _, t := range tail {
		if result.Len()+len(t) > maxLen {
			break
		}
		result.WriteString(t)
		result.WriteString("\n")
	}

	compressed := result.String()
	if len(compressed) > maxLen+50 {
		compressed = compressed[:maxLen] + "..."
	}
	return compressed
}

func extractKeyPoints(content string) string {
	lines := strings.Split(content, "\n")
	var points []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "1.") || strings.HasPrefix(trimmed, "2.") ||
			strings.HasPrefix(trimmed, "3.") || strings.HasPrefix(trimmed, "①") ||
			strings.HasPrefix(trimmed, "②") || strings.HasPrefix(trimmed, "③") {
			points = append(points, trimmed)
		}
	}

	if len(points) == 0 {
		firstLine := ""
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				firstLine = strings.TrimSpace(line)
				break
			}
		}
		if firstLine != "" {
			return firstLine + "\n...(回复已压缩)"
		}
		return ""
	}

	result := strings.Join(points, "\n")
	if len(result) > 600 {
		result = result[:600] + "\n..."
	}
	return result
}

func toolCallSignature(toolCalls []llm.ToolCall) string {
	var sb strings.Builder
	for _, tc := range toolCalls {
		sb.WriteString(tc.Function.Name)
		sb.WriteString("(")
		sb.WriteString(tc.Function.Arguments)
		sb.WriteString(")")
	}
	return sb.String()
}

func (s *ToolCallingService) filterRelevantTools(content string, tools []map[string]interface{}) []map[string]interface{} {
	contentLower := strings.ToLower(content)

	alwaysInclude := map[string]bool{
		"web_search": true, "search": true, "http_request": true, "read_file": true,
		"write_file": true, "execute_code": true, "list_directory": true,
	}

	type scoredTool struct {
		tool  map[string]interface{}
		score float64
	}
	var scored []scoredTool

	for _, t := range tools {
		name, _ := t["name"].(string)
		nameLower := strings.ToLower(name)

		if alwaysInclude[nameLower] {
			scored = append(scored, scoredTool{tool: t, score: 100})
			continue
		}

		score := 0.0
		if strings.Contains(contentLower, nameLower) {
			score += 10
		}

		if desc, ok := t["description"].(string); ok {
			descLower := strings.ToLower(desc)
			words := strings.Fields(descLower)
			for _, w := range words {
				if strings.Contains(contentLower, w) && len(w) > 2 {
					score += 1
				}
			}
		}

		scored = append(scored, scoredTool{tool: t, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	limit := 15
	if limit > len(scored) {
		limit = len(scored)
	}

	result := make([]map[string]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, scored[i].tool)
	}
	return result
}
