package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// DialogueService 对话服务
type DialogueService struct {
	db             *gorm.DB
	modelService   *ModelService
	logger         *LoggerService
	usageService   *UsageService
	tokenEstimator *TokenEstimator
	smartCache     *SmartCacheService
}

// NewDialogueService 创建对话服务实例
func NewDialogueService(db *gorm.DB, modelService *ModelService, logger *LoggerService) *DialogueService {
	return &DialogueService{
		db:             db,
		modelService:   modelService,
		logger:         logger,
		tokenEstimator: NewTokenEstimator(),
	}
}

// SetCacheService 设置缓存服务（用于初始化SmartCache）
func (s *DialogueService) SetCacheService(cacheService *CacheService) {
	if cacheService != nil {
		s.smartCache = NewSmartCacheService(cacheService)
	}
}

// SetUsageService 设置使用量统计服务
func (s *DialogueService) SetUsageService(usageService *UsageService) {
	s.usageService = usageService
}

// CreateDialogue 创建新对话
func (s *DialogueService) CreateDialogue(userID, title string) models.Dialogue {
	dialogue := models.Dialogue{
		ID:        uuid.New().String(),
		UserID:    userID,
		Title:     title,
		Messages:  []models.Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s.db.Create(&dialogue)
	return dialogue
}

// GetDialogue 获取对话详情
func (s *DialogueService) GetDialogue(id string) (models.Dialogue, bool) {
	var dialogue models.Dialogue
	err := s.db.Preload("Messages").First(&dialogue, id).Error
	return dialogue, err == nil
}

// UpdateDialogue 更新对话
func (s *DialogueService) UpdateDialogue(id, title string) (models.Dialogue, bool) {
	var dialogue models.Dialogue
	err := s.db.First(&dialogue, id).Error
	if err != nil {
		return models.Dialogue{}, false
	}

	dialogue.Title = title
	dialogue.UpdatedAt = time.Now()
	s.db.Save(&dialogue)
	return dialogue, true
}

// ListDialogues 列出所有对话
func (s *DialogueService) ListDialogues() []models.Dialogue {
	var dialogues []models.Dialogue
	s.db.Order("updated_at DESC").Find(&dialogues)
	return dialogues
}

// ListDialoguesByUser 列出用户的所有对话
func (s *DialogueService) ListDialoguesByUser(userID string) []models.Dialogue {
	var dialogues []models.Dialogue
	s.db.Where("user_id = ?", userID).Order("updated_at DESC").Find(&dialogues)
	return dialogues
}

// AddMessage 添加消息到对话
func (s *DialogueService) AddMessage(dialogueID, sender, content string) (models.Message, error) {
	message := models.Message{
		ID:         uuid.New().String(),
		DialogueID: dialogueID,
		Sender:     sender,
		Content:    content,
		CreatedAt:  time.Now(),
	}

	if err := s.db.Create(&message).Error; err != nil {
		return message, fmt.Errorf("failed to create message: %w", err)
	}

	// 更新对话的更新时间
	if err := s.db.Model(&models.Dialogue{}).Where("id = ?", dialogueID).Update("updated_at", time.Now()).Error; err != nil {
		s.logger.Error(context.Background(), "Failed to update dialogue updated_at: %v", err)
	}

	return message, nil
}

// GetMessages 获取对话消息
func (s *DialogueService) GetMessages(dialogueID string) []models.Message {
	// 限制消息数量，只获取最近的100条消息
	var messages []models.Message
	s.db.Where("dialogue_id = ?", dialogueID).Order("created_at ASC").Limit(100).Find(&messages)
	return messages
}

// GetMessagesWithPagination 分页获取对话消息
func (s *DialogueService) GetMessagesWithPagination(dialogueID string, page, pageSize int) []models.Message {
	var messages []models.Message
	offset := (page - 1) * pageSize
	s.db.Where("dialogue_id = ?", dialogueID).Order("created_at ASC").Offset(offset).Limit(pageSize).Find(&messages)
	return messages
}

// SendMessage 发送消息并获取 AI 回复
func (s *DialogueService) SendMessage(ctx context.Context, dialogueID, userID, content string, modelID string, options map[string]interface{}) (*models.Message, error) {
	// 保存用户消息
	if _, err := s.AddMessage(dialogueID, "user", content); err != nil {
		s.logger.Error(ctx, "Failed to save user message: %v", err)
	}

	// 检查智能缓存
	if s.smartCache != nil {
		if resp, cached, err := s.smartCache.CacheMiddleware(content, modelID, options, func() (*llm.ChatResponse, error) {
			return s.executeChat(ctx, dialogueID, userID, content, modelID, options)
		}); err == nil {
			if cached {
				// 缓存命中，保存缓存的响应
				assistantMessage, err := s.AddMessage(dialogueID, "assistant", resp.Choices[0].Message.Content)
				if err != nil {
					s.logger.Error(ctx, "Failed to save assistant message: %v", err)
				}
				s.logger.Info(ctx, "Cache hit for dialogue %s, returning cached response", dialogueID)
				return &assistantMessage, nil
			}
			// 非缓存响应，正常处理
			assistantMessage, err := s.AddMessage(dialogueID, "assistant", resp.Choices[0].Message.Content)
			if err != nil {
				s.logger.Error(ctx, "Failed to save assistant message: %v", err)
			}

			// 记录响应
			if resp.Usage != nil {
				s.logger.Info(ctx, "LLM response received, tokens: %d+%d=%d",
					resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)

				// 记录Token使用量
				if s.usageService != nil {
					go s.recordUsage(ctx, userID, dialogueID, assistantMessage.ID, modelID, resp, 0, false)
				}
			}

			return &assistantMessage, nil
		}
	}

	// 无缓存，直接执行
	return s.executeChatAndSave(ctx, dialogueID, userID, content, modelID, options)
}

// executeChat 执行聊天请求（不保存消息）
func (s *DialogueService) executeChat(ctx context.Context, dialogueID, userID, content string, modelID string, options map[string]interface{}) (*llm.ChatResponse, error) {
	// 构建对话历史
	messages := s.GetMessages(dialogueID)

	// 转换为 LLM 消息格式
	llmMessages := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		role := "user"
		switch msg.Sender {
		case "assistant":
			role = "assistant"
		case "system":
			role = "system"
		case "tool":
			role = "tool"
		}
		llmMsg := llm.Message{
			Role:    role,
			Content: msg.Content,
		}
		if msg.ToolCallID != "" {
			llmMsg.ToolCallID = msg.ToolCallID
		}
		llmMessages = append(llmMessages, llmMsg)
	}

	// 调用 LLM 获取回复
	resp, err := s.modelService.Chat(modelID, llmMessages, options)
	if err != nil {
		s.logger.Error(ctx, "Failed to get LLM response: %v", err)
		return nil, err
	}

	return resp, nil
}

// executeChatAndSave 执行聊天并保存结果
func (s *DialogueService) executeChatAndSave(ctx context.Context, dialogueID, userID, content string, modelID string, options map[string]interface{}) (*models.Message, error) {
	startTime := time.Now()
	resp, err := s.executeChat(ctx, dialogueID, userID, content, modelID, options)
	duration := time.Since(startTime)

	if err != nil {
		return nil, err
	}

	// 保存助手回复
	assistantMessage, err := s.AddMessage(dialogueID, "assistant", resp.Choices[0].Message.Content)
	if err != nil {
		s.logger.Error(ctx, "Failed to save assistant message: %v", err)
	}

	// 记录响应
	if resp.Usage != nil {
		s.logger.Info(ctx, "LLM response received in %v, tokens: %d+%d=%d",
			duration, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)

		// 记录Token使用量
		if s.usageService != nil {
			go s.recordUsage(ctx, userID, dialogueID, assistantMessage.ID, modelID, resp, duration, false)
		}
	}

	return &assistantMessage, nil
}

// SendMessageStream 发送消息并获取流式 AI 回复
func (s *DialogueService) SendMessageStream(ctx context.Context, dialogueID, userID, content string, modelID string, options map[string]interface{}) (<-chan llm.ChatStreamChunk, error) {
	// 保存用户消息
	if _, err := s.AddMessage(dialogueID, "user", content); err != nil {
		s.logger.Error(ctx, "Failed to save user message: %v", err)
	}

	// 构建对话历史（最多最近20条消息，控制token消耗）
	messages := s.GetMessages(dialogueID)
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
	}

	// 转换为 LLM 消息格式
	llmMessages := make([]llm.Message, 0, len(messages)+1)

	// 注入系统提示词（增强版）
	systemPrompt := `你是 OpenAIDE AI 助手。

## 行为准则
1. 先理解用户意图，再决定行动
2. 复杂问题分解为小步骤，逐步解决
3. 使用工具获取准确信息，不要凭记忆猜测
4. 每次工具调用后检查结果是否正确
5. 确保回答覆盖用户所有问题，不遗漏需求
6. 不确定的信息要说明，不要编造事实

## 输出格式
- 使用 Markdown 格式，代码块标注语言
- 长回答先给结论，再展开说明
- 重要信息用加粗或代码块突出`

	llmMessages = append(llmMessages, llm.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	for _, msg := range messages {
		role := "user"
		switch msg.Sender {
		case "assistant":
			role = "assistant"
		case "system":
			role = "system"
		case "tool":
			role = "tool"
		}
		llmMsg := llm.Message{
			Role:    role,
			Content: msg.Content,
		}
		if msg.ToolCallID != "" {
			llmMsg.ToolCallID = msg.ToolCallID
		}
		llmMessages = append(llmMessages, llmMsg)
	}

	// Token预估和智能截断
	llmMessages = s.smartTruncateMessages(ctx, llmMessages, modelID, dialogueID)

	// 调用 LLM 获取流式回复
	startTime := time.Now()
	chunkChan, err := s.modelService.ChatStream(modelID, llmMessages, options)
	if err != nil {
		return nil, err
	}

	// 包装流式通道，在最后统计token
	if s.usageService != nil {
		return s.wrapStreamWithUsage(chunkChan, userID, dialogueID, modelID, startTime), nil
	}

	return chunkChan, nil
}

// smartTruncateMessages 智能截断消息列表
// 预估token数，如果超过模型限制则智能截断
func (s *DialogueService) smartTruncateMessages(ctx context.Context, messages []llm.Message, modelID string, dialogueID string) []llm.Message {
	if s.tokenEstimator == nil || len(messages) == 0 {
		return messages
	}

	// 获取模型信息
	model, err := s.modelService.GetModel(modelID)
	if err != nil {
		return messages
	}

	// 将消息转换为估算格式
	msgMaps := make([]map[string]string, 0, len(messages))
	for _, msg := range messages {
		msgMaps = append(msgMaps, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	// 检查是否需要截断
	shouldTruncate, estimatedTokens := s.tokenEstimator.ShouldTruncate(msgMaps, model.Name)
	if !shouldTruncate {
		s.logger.Info(ctx, "Token estimate for dialogue %s: %d tokens (within limit)", dialogueID, estimatedTokens)
		return messages
	}

	// 获取模型限制并计算安全限制
	limit := s.tokenEstimator.GetModelLimit(model.Name)
	safeLimit := int(float64(limit) * 0.8)

	s.logger.Warn(ctx, "Token estimate for dialogue %s: %d tokens exceeds safe limit %d, truncating...",
		dialogueID, estimatedTokens, safeLimit)

	// 执行截断
	truncatedMaps := s.tokenEstimator.TruncateMessages(msgMaps, model.Name, safeLimit)

	// 转换回LLM消息格式
	truncatedMessages := make([]llm.Message, 0, len(truncatedMaps))
	for _, msgMap := range truncatedMaps {
		truncatedMessages = append(truncatedMessages, llm.Message{
			Role:    msgMap["role"],
			Content: msgMap["content"],
		})
	}

	newEstimate := s.tokenEstimator.EstimateMessagesTokens(truncatedMaps, model.Name)
	s.logger.Info(ctx, "Truncated dialogue %s from %d to %d messages, estimated tokens: %d",
		dialogueID, len(messages), len(truncatedMessages), newEstimate)

	return truncatedMessages
}

// wrapStreamWithUsage 包装流式通道，在最后统计token使用量
func (s *DialogueService) wrapStreamWithUsage(
	chunkChan <-chan llm.ChatStreamChunk,
	userID, dialogueID, modelID string,
	startTime time.Time,
) <-chan llm.ChatStreamChunk {
	wrapped := make(chan llm.ChatStreamChunk)

	go func() {
		defer close(wrapped)

		var lastChunk llm.ChatStreamChunk
		var hasUsage bool

		for chunk := range chunkChan {
			lastChunk = chunk
			if chunk.Usage != nil {
				hasUsage = true
			}
			wrapped <- chunk
		}

		// 流结束后统计token
		if hasUsage && lastChunk.Usage != nil {
			duration := time.Since(startTime)
			resp := &llm.ChatResponse{
				Usage: lastChunk.Usage,
			}
			// 流式消息ID用对话ID+时间戳
			messageID := fmt.Sprintf("stream_%d", time.Now().Unix())
			s.recordUsage(context.Background(), userID, dialogueID, messageID, modelID, resp, duration, true)
		}
	}()

	return wrapped
}

// SaveStreamMessage 保存流式消息的完整内容
func (s *DialogueService) SaveStreamMessage(dialogueID string, content string) (models.Message, error) {
	return s.AddMessage(dialogueID, "assistant", content)
}

// recordUsage 记录Token使用量
func (s *DialogueService) recordUsage(ctx context.Context, userID, dialogueID, messageID, modelID string, resp *llm.ChatResponse, duration time.Duration, isStreaming bool) {
	if s.usageService == nil {
		return
	}

	// 获取模型信息
	model, err := s.modelService.GetModel(modelID)
	if err != nil {
		s.logger.Error(ctx, "Failed to get model for usage recording: %v", err)
		model = &models.Model{Name: modelID, Provider: "unknown"}
	}

	record := &models.UsageRecord{
		ID:               uuid.New().String(),
		UserID:           userID,
		DialogueID:       dialogueID,
		MessageID:        messageID,
		Provider:         model.Provider,
		ModelID:          model.ID,
		ModelName:        model.Name,
		PromptTokens:     int64(resp.Usage.PromptTokens),
		CompletionTokens: int64(resp.Usage.CompletionTokens),
		TotalTokens:      int64(resp.Usage.TotalTokens),
		RequestType:      "chat",
		IsStreaming:      isStreaming,
		Duration:         duration.Milliseconds(),
		Success:          true,
	}

	if err := s.usageService.RecordUsage(record); err != nil {
		s.logger.Error(ctx, "Failed to record usage: %v", err)
	}
}

// DeleteDialogue 删除对话
func (s *DialogueService) DeleteDialogue(id string) error {
	// 删除对话的所有消息
	s.db.Where("dialogue_id = ?", id).Delete(&models.Message{})
	// 删除对话
	return s.db.Where("id = ?", id).Delete(&models.Dialogue{}).Error
}

// ClearMessages 清空对话消息
func (s *DialogueService) ClearMessages(dialogueID string) error {
	return s.db.Where("dialogue_id = ?", dialogueID).Delete(&models.Message{}).Error
}
