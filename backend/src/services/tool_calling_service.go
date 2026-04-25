package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// ToolCallingService 工具调用循环服务
// 将 ToolService 与 LLM 对话连接，实现完整的 tool calling 闭环
type ToolCallingService struct {
	toolSvc      *ToolService
	modelSvc     *ModelService
	dialogueSvc  *DialogueService
	logger       *LoggerService
	usageService *UsageService
	eventBus     *EventBus
	maxRounds    int
}

func (s *ToolCallingService) SetEventBus(bus *EventBus) {
	s.eventBus = bus
}

func (s *ToolCallingService) SetUsageService(usageService *UsageService) {
	s.usageService = usageService
}

func (s *ToolCallingService) SetDialogueService(dialogueSvc *DialogueService) {
	s.dialogueSvc = dialogueSvc
}

// NewToolCallingService 创建工具调用服务
func NewToolCallingService(toolSvc *ToolService, modelSvc *ModelService, logger *LoggerService) *ToolCallingService {
	return &ToolCallingService{
		toolSvc:   toolSvc,
		modelSvc:  modelSvc,
		logger:    logger,
		maxRounds: 20,
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

	// 4. 工具调用循环（ReAct 模式，参考 Hermes Agent）
	var totalUsage llm.Usage
	startTime := time.Now()

	for round := 0; round < s.maxRounds; round++ {
		// 上下文压缩（参考 OpenCode 两阶段压缩）
		// Phase 1: 简单修剪旧工具输出
		messages = s.compressToolOutputs(messages)
		// Phase 2: 当接近上下文窗口溢出时，使用 LLM 摘要
		if s.isContextOverflow(messages, modelID) {
			log.Printf("[ToolCalling] Context overflow detected, triggering LLM summarization")
			messages = s.summarizeWithLLM(ctx, messages, modelID)
		}

		resp, err := s.modelSvc.ChatWithTools(modelID, messages, llmTools, options)
		if err != nil {
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		// 累计token使用量
		if resp.Usage != nil {
			totalUsage.PromptTokens += resp.Usage.PromptTokens
			totalUsage.CompletionTokens += resp.Usage.CompletionTokens
			totalUsage.TotalTokens += resp.Usage.TotalTokens
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("empty response from LLM")
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

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

			return s.saveToolCallingResult(dialogueID, "assistant", result), nil
		}

		// 执行工具调用（并行执行多个工具，参考 Hermes Agent 的并发模式）
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
	}

	// 超出最大轮次，返回最后一条 assistant 消息
	lastMsg := messages[len(messages)-1]

	// 记录总token使用量（即使超出轮次也要记录）
	if s.usageService != nil && totalUsage.TotalTokens > 0 {
		go s.recordToolCallingUsage(ctx, userID, dialogueID, modelID, &totalUsage, time.Since(startTime))
	}

	return s.saveToolCallingResult(dialogueID, "assistant", lastMsg.Content), nil
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

	result, err := s.toolSvc.ExecuteTool(ctx, toolCall, dialogueID, "", "")
	if err != nil {
		if confirmErr, ok := err.(*ConfirmationRequiredError); ok {
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

// saveToolCallingResult 保存工具调用的最终结果
func (s *ToolCallingService) saveToolCallingResult(dialogueID, sender, content string) *models.Message {
	// 通过数据库直接插入消息
	now := time.Now()
	msg := &models.Message{
		ID:         GenerateUUID(),
		DialogueID: dialogueID,
		Sender:     sender,
		Content:    content,
		CreatedAt:  now,
	}

	// 使用 toolSvc 的 db 连接保存
	if s.toolSvc != nil {
		db := s.toolSvc.db
		if db != nil {
			db.Create(msg)
			return msg
		}
	}

	log.Printf("[ToolCalling] Warning: could not save message, no database connection")
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
func (s *ToolCallingService) compressToolOutputs(messages []llm.Message) []llm.Message {
	const maxMessages = 40
	const keepRecent = 10

	if len(messages) <= maxMessages {
		return messages
	}

	compressed := make([]llm.Message, 0, len(messages))
	oldCount := len(messages) - keepRecent
	prunedCount := 0

	for i, msg := range messages {
		if i < oldCount && msg.Role == llm.RoleTool && len(msg.Content) > 200 {
			compressed = append(compressed, llm.Message{
				Role:       msg.Role,
				Content:    "[Old tool output cleared for context compression]",
				ToolCallID: msg.ToolCallID,
			})
			prunedCount++
		} else {
			compressed = append(compressed, msg)
		}
	}

	log.Printf("[ToolCalling] Context compression: %d messages, %d old tool outputs pruned", len(messages), prunedCount)
	return compressed
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
func (s *ToolCallingService) summarizeWithLLM(ctx context.Context, messages []llm.Message, modelID string) []llm.Message {
	if len(messages) <= 10 {
		return messages
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
			historyText.WriteString(fmt.Sprintf("User: %s\n", msg.Content))
		case llm.RoleAssistant:
			content := msg.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			historyText.WriteString(fmt.Sprintf("Assistant: %s\n", content))
		case llm.RoleTool:
			content := msg.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			historyText.WriteString(fmt.Sprintf("Tool(%s): %s\n", msg.ToolCallID, content))
		}
	}

	summaryPrompt := fmt.Sprintf(`Summarize the following conversation history concisely. Preserve:
1. Key decisions and conclusions
2. Important tool results (IP addresses, file paths, error messages)
3. User's original intent

Conversation history:
%s

Provide a detailed summary for continuing the conversation:`, historyText.String())

	summaryModelID := modelID
	models, err := s.modelSvc.ListModels()
	if err == nil {
		for _, m := range models {
			for _, tag := range m.Tags {
				if strings.TrimSpace(tag) == "fast" {
					summaryModelID = m.ID
					break
				}
			}
		}
	}

	resp, err := s.modelSvc.Chat(summaryModelID, []llm.Message{
		{Role: llm.RoleUser, Content: summaryPrompt},
	}, map[string]interface{}{"max_tokens": 2000})

	if err != nil {
		log.Printf("[ToolCalling] LLM summarization failed, falling back to simple compression: %v", err)
		return s.compressToolOutputs(messages)
	}

	summary := ""
	if len(resp.Choices) > 0 {
		summary = resp.Choices[0].Message.Content
	}

	result := []llm.Message{
		{Role: llm.RoleSystem, Content: fmt.Sprintf("[Conversation Summary]\n%s\n[End of Summary - Continue from here]", summary)},
	}
	result = append(result, recentMessages...)

	log.Printf("[ToolCalling] LLM summarization: %d old messages -> summary + %d recent messages", len(oldMessages), len(recentMessages))
	return result
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
			if role == llm.RoleAssistant && len(content) > 1000 {
				content = content[:1000] + "..."
			}
			if role == llm.RoleTool && len(content) > 500 {
				content = content[:500] + "..."
			}

			messages = append(messages, llm.Message{
				Role:    role,
				Content: content,
			})
		}
	}

	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: currentContent,
	})

	return messages
}

func (s *ToolCallingService) getDialogueService() *DialogueService {
	if s.dialogueSvc != nil {
		return s.dialogueSvc
	}
	return nil
}
