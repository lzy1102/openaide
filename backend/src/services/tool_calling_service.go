package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// ToolCallingService 工具调用循环服务
// 将 ToolService 与 LLM 对话连接，实现完整的 tool calling 闭环
type ToolCallingService struct {
	toolSvc      *ToolService
	modelSvc     *ModelService
	logger       *LoggerService
	usageService *UsageService
	eventBus     *EventBus
	maxRounds    int // 最大工具调用轮次，防止无限循环
}

// SetEventBus 设置事件总线（可选依赖注入）
func (s *ToolCallingService) SetEventBus(bus *EventBus) {
	s.eventBus = bus
}

// SetUsageService 设置使用量统计服务
func (s *ToolCallingService) SetUsageService(usageService *UsageService) {
	s.usageService = usageService
}

// NewToolCallingService 创建工具调用服务
func NewToolCallingService(toolSvc *ToolService, modelSvc *ModelService, logger *LoggerService) *ToolCallingService {
	return &ToolCallingService{
		toolSvc:   toolSvc,
		modelSvc:  modelSvc,
		logger:    logger,
		maxRounds: 5,
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

	// 3. 构建初始消息
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: content},
	}

	// 4. 工具调用循环
	var totalUsage llm.Usage
	startTime := time.Now()

	for round := 0; round < s.maxRounds; round++ {
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

		// 执行工具调用
		for _, tc := range assistantMsg.ToolCalls {
			toolResult := s.executeToolCall(ctx, tc, dialogueID)

			// 将工具结果作为 tool 消息追加
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    toolResult,
				ToolCallID: tc.ID,
			})
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

	// 发布工具完成事件
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicTool, models.EventTypeToolCompleted, "tool_calling", map[string]interface{}{
			"tool_name":    tc.Function.Name,
			"tool_call_id": tc.ID,
			"result":       result.Content,
		})
	}

	// 序列化结果为 JSON 字符串
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
