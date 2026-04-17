package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// DialogueService 对话服务
type DialogueService struct {
	db            *gorm.DB
	modelService  *ModelService
	logger        *LoggerService
}

// NewDialogueService 创建对话服务实例
func NewDialogueService(db *gorm.DB, modelService *ModelService, logger *LoggerService) *DialogueService {
	return &DialogueService{
		db:           db,
		modelService: modelService,
		logger:       logger,
	}
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
func (s *DialogueService) AddMessage(dialogueID, sender, content string) models.Message {
	message := models.Message{
		ID:         uuid.New().String(),
		DialogueID: dialogueID,
		Sender:     sender,
		Content:    content,
		CreatedAt:  time.Now(),
	}

	s.db.Create(&message)

	// 更新对话的更新时间
	s.db.Model(&models.Dialogue{}).Where("id = ?", dialogueID).Update("updated_at", time.Now())

	return message
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
	s.AddMessage(dialogueID, "user", content)

	// 构建对话历史
	messages := s.GetMessages(dialogueID)

	// 转换为 LLM 消息格式
	llmMessages := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		role := "user"
		if msg.Sender == "assistant" {
			role = "assistant"
		}
		llmMessages = append(llmMessages, llm.Message{
			Role:    role,
			Content: msg.Content,
		})
	}

	// 调用 LLM 获取回复
	startTime := time.Now()
	resp, err := s.modelService.Chat(modelID, llmMessages, options)
	duration := time.Since(startTime)

	if err != nil {
		s.logger.Error(ctx, "Failed to get LLM response: %v", err)
		return nil, err
	}

	// 记录响应
	if resp.Usage != nil {
		s.logger.Info(ctx, "LLM response received in %v, tokens: %d+%d=%d",
			duration, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}

	// 保存助手回复
	assistantMessage := s.AddMessage(dialogueID, "assistant", resp.Choices[0].Message.Content)

	return &assistantMessage, nil
}

// SendMessageStream 发送消息并获取流式 AI 回复
func (s *DialogueService) SendMessageStream(ctx context.Context, dialogueID, userID, content string, modelID string, options map[string]interface{}) (<-chan llm.ChatStreamChunk, error) {
	// 保存用户消息
	s.AddMessage(dialogueID, "user", content)

	// 构建对话历史（最多最近20条消息，控制token消耗）
	messages := s.GetMessages(dialogueID)
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
	}

	// 转换为 LLM 消息格式
	llmMessages := make([]llm.Message, 0, len(messages)+1)

	// 注入系统提示词（增强版）
	llmMessages = append(llmMessages, llm.Message{
		Role: "system",
		Content: `你是 OpenAIDE AI 助手。

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
- 重要信息用加粗或代码块突出`,
	})

	for _, msg := range messages {
		role := "user"
		if msg.Sender == "assistant" {
			role = "assistant"
		}
		llmMessages = append(llmMessages, llm.Message{
			Role:    role,
			Content: msg.Content,
		})
	}

	// 调用 LLM 获取流式回复
	return s.modelService.ChatStream(modelID, llmMessages, options)
}

// SaveStreamMessage 保存流式消息的完整内容
func (s *DialogueService) SaveStreamMessage(dialogueID string, content string) models.Message {
	return s.AddMessage(dialogueID, "assistant", content)
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
