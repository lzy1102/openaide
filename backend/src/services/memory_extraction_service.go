package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// MemoryExtractionService 自动记忆提取服务
type MemoryExtractionService struct {
	db            *gorm.DB
	memoryService *MemoryService
	llmClient     llm.Client
	enabled       bool
}

// NewMemoryExtractionService 创建自动记忆提取服务
func NewMemoryExtractionService(db *gorm.DB, memoryService *MemoryService, llmClient llm.Client, enabled bool) *MemoryExtractionService {
	return &MemoryExtractionService{
		db:            db,
		memoryService: memoryService,
		llmClient:     llmClient,
		enabled:       enabled,
	}
}

// ExtractMemoriesFromDialogue 从对话中自动提取记忆
func (s *MemoryExtractionService) ExtractMemoriesFromDialogue(ctx context.Context, dialogueID string, userID string) error {
	if !s.enabled || s.llmClient == nil {
		return nil
	}

	var dialogue models.Dialogue
	if err := s.db.Preload("Messages").First(&dialogue, "id = ?", dialogueID).Error; err != nil {
		return fmt.Errorf("dialogue not found: %w", err)
	}

	if len(dialogue.Messages) < 2 {
		return nil
	}

	conversationText := s.buildConversationText(dialogue.Messages)

	extractedMemories, err := s.extractWithLLM(ctx, conversationText)
	if err != nil {
		return fmt.Errorf("LLM extraction failed: %w", err)
	}

	for _, mem := range extractedMemories {
		mem.UserID = userID
		mem.ID = uuid.New().String()
		mem.CreatedAt = time.Now()
		mem.UpdatedAt = time.Now()
		mem.LastAccessed = time.Now()

		if err := s.memoryService.CreateMemory(&mem); err != nil {
			log.Printf("[MemoryExtract] failed to save memory: %v", err)
		} else {
			log.Printf("[MemoryExtract] extracted %s memory: %s", mem.MemoryType, truncate(mem.Content, 50))
		}
	}

	return nil
}

// extractWithLLM 使用 LLM 提取记忆
func (s *MemoryExtractionService) extractWithLLM(ctx context.Context, conversationText string) ([]models.Memory, error) {
	prompt := fmt.Sprintf(`你是一个专业的记忆提取专家。请从以下对话中提取需要长期记住的重要信息。

提取规则：
1. 只提取真正重要的信息，避免琐碎内容
2. 按以下类别提取：
   - fact: 事实信息（用户告诉你的个人信息、工作、技能等）
   - preference: 用户偏好（语言偏好、工具偏好、工作习惯等）
   - procedure: 操作步骤（用户教你的特定流程）
   - context: 项目背景（项目名称、技术栈、架构等）

对话内容：
%s

请以 JSON 数组格式返回提取的记忆，格式如下：
[
  {
    "memory_type": "fact|preference|procedure|context",
    "content": "提取的内容",
    "importance": 1-5,
    "tags": ["标签1", "标签2"]
  }
]

如果没有值得提取的记忆，返回空数组 []。`, conversationText)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个记忆提取专家。从对话中提取重要信息作为长期记忆。只返回 JSON 数组。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   1500,
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var extracted []models.Memory
	if err := json.Unmarshal([]byte(content), &extracted); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return extracted, nil
}

// BatchExtractPendingDialogues 批量提取待处理对话的记忆
func (s *MemoryExtractionService) BatchExtractPendingDialogues(userID string, limit int) error {
	if !s.enabled {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}

	var dialogues []models.Dialogue
	s.db.Where("user_id = ? AND status = ? AND messages_extracted = ?", userID, "completed", false).
		Order("created_at DESC").
		Limit(limit).
		Find(&dialogues)

	for _, dialogue := range dialogues {
		ctx := context.Background()
		if err := s.ExtractMemoriesFromDialogue(ctx, dialogue.ID, userID); err != nil {
			log.Printf("[MemoryExtract] failed to extract from dialogue %s: %v", dialogue.ID, err)
		} else {
			s.db.Model(&dialogue).Update("messages_extracted", true)
		}
	}

	return nil
}

func (s *MemoryExtractionService) buildConversationText(messages []models.Message) string {
	var sb strings.Builder
	for i, msg := range messages {
		if i > 0 {
			sb.WriteString("\n")
		}
		sender := "用户"
		if msg.Sender == "assistant" {
			sender = "助手"
		}
		content := msg.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("%s: %s", sender, content))
	}
	return sb.String()
}
