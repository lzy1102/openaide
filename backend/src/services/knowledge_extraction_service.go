package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// KnowledgeExtractionService 知识提取服务接口
type KnowledgeExtractionService interface {
	// ExtractFromDialogue 从对话中提取知识
	ExtractFromDialogue(ctx context.Context, dialogueID string) ([]ExtractedKnowledge, error)

	// ExtractFromMessage 从单条消息中提取知识
	ExtractFromMessage(ctx context.Context, message string) ([]ExtractedKnowledge, error)

	// AutoSave 自动保存提取的知识
	AutoSave(ctx context.Context, knowledge []ExtractedKnowledge, userID string) error
}

// ExtractedKnowledge 提取的知识
type ExtractedKnowledge struct {
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Summary    string   `json:"summary"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	Confidence float64  `json:"confidence"`
	Source     string   `json:"source"`
}

// knowledgeExtractionService 知识提取服务实现
type knowledgeExtractionService struct {
	db              *gorm.DB
	llmClient       llm.LLMClient
	knowledgeService KnowledgeService
	dialogueService *DialogueService
	logger          *LoggerService
}

// NewKnowledgeExtractionService 创建知识提取服务
func NewKnowledgeExtractionService(
	db *gorm.DB,
	llmClient llm.LLMClient,
	knowledgeService KnowledgeService,
	dialogueService *DialogueService,
	logger *LoggerService,
) KnowledgeExtractionService {
	return &knowledgeExtractionService{
		db:               db,
		llmClient:        llmClient,
		knowledgeService: knowledgeService,
		dialogueService:  dialogueService,
		logger:           logger,
	}
}

// ExtractFromDialogue 从对话中提取知识
func (s *knowledgeExtractionService) ExtractFromDialogue(ctx context.Context, dialogueID string) ([]ExtractedKnowledge, error) {
	// 获取对话
	dialogue, exists := s.dialogueService.GetDialogue(dialogueID)
	if !exists {
		return nil, fmt.Errorf("dialogue not found: %s", dialogueID)
	}

	// 构建对话文本
	dialogueText := s.buildDialogueText(dialogue)

	// 调用 LLM 提取知识
	return s.extractKnowledgeWithLLM(ctx, dialogueText, "dialogue")
}

// ExtractFromMessage 从单条消息中提取知识
func (s *knowledgeExtractionService) ExtractFromMessage(ctx context.Context, message string) ([]ExtractedKnowledge, error) {
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("message cannot be empty")
	}

	return s.extractKnowledgeWithLLM(ctx, message, "message")
}

// extractKnowledgeWithLLM 使用 LLM 提取知识
func (s *knowledgeExtractionService) extractKnowledgeWithLLM(ctx context.Context, text, source string) ([]ExtractedKnowledge, error) {
	prompt := s.buildExtractionPrompt(text)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: s.getSystemPrompt(),
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Model:       "",
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		s.logger.Error(ctx, "Failed to extract knowledge with LLM: %v", err)
		return nil, fmt.Errorf("failed to extract knowledge: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	content := resp.Choices[0].Message.Content

	// 解析 LLM 响应
	knowledgeList, err := s.parseExtractionResponse(content)
	if err != nil {
		s.logger.Error(ctx, "Failed to parse extraction response: %v", err)
		return nil, fmt.Errorf("failed to parse extraction response: %w", err)
	}

	// 设置来源
	for i := range knowledgeList {
		knowledgeList[i].Source = source
	}

	return knowledgeList, nil
}

// AutoSave 自动保存提取的知识
func (s *knowledgeExtractionService) AutoSave(ctx context.Context, knowledgeList []ExtractedKnowledge, userID string) error {
	if len(knowledgeList) == 0 {
		return nil
	}

	savedCount := 0
	skippedCount := 0

	for _, extracted := range knowledgeList {
		// 检查置信度阈值
		if extracted.Confidence < 0.5 {
			skippedCount++
			continue
		}

		// 检查去重
		isDuplicate, err := s.checkDuplicate(ctx, extracted)
		if err != nil {
			s.logger.Error(ctx, "Failed to check duplicate: %v", err)
			continue
		}
		if isDuplicate {
			skippedCount++
			continue
		}

		// 查找或创建分类
		categoryID, err := s.getOrCreateCategory(ctx, extracted.Category, userID)
		if err != nil {
			s.logger.Error(ctx, "Failed to get or create category: %v", err)
			categoryID = ""
		}

		// 创建知识条目
		_, err = s.knowledgeService.CreateKnowledgeWithEmbedding(
			ctx,
			extracted.Title,
			extracted.Content,
			extracted.Summary,
			categoryID,
			extracted.Source,
			"",
			userID,
		)
		if err != nil {
			s.logger.Error(ctx, "Failed to create knowledge: %v", err)
			continue
		}

		// 处理标签
		if len(extracted.Tags) > 0 {
			for _, tagName := range extracted.Tags {
				if err := s.createTagIfNotExists(ctx, tagName, userID); err != nil {
					s.logger.Error(ctx, "Failed to create tag: %v", err)
				}
			}
		}

		savedCount++
	}

	s.logger.Info(ctx, "Knowledge auto-save completed: %d saved, %d skipped", savedCount, skippedCount)

	return nil
}

// buildDialogueText 构建对话文本
func (s *knowledgeExtractionService) buildDialogueText(dialogue models.Dialogue) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", dialogue.Title))

	for _, msg := range dialogue.Messages {
		role := "用户"
		if msg.Sender == "assistant" {
			role = "助手"
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", role, msg.Content))
	}

	return sb.String()
}

// getSystemPrompt 获取系统提示词
func (s *knowledgeExtractionService) getSystemPrompt() string {
	return `你是一个专业的知识提取助手。你的任务是从对话或文本中识别和提取有价值的知识。

请遵循以下规则：
1. 只提取真正有价值的知识点，避免提取琐碎的问候或无实质内容的信息
2. 为每个知识点生成简洁明了的标题
3. 保留关键信息作为内容
4. 生成简短的摘要（不超过50字）
5. 推荐合适的分类（如：技术、生活、工作、学习等）
6. 提取相关标签（3-5个）
7. 评估置信度（0-1之间的浮点数，表示知识的可靠性）

请以JSON格式返回，格式如下：
{
  "knowledge": [
    {
      "title": "知识标题",
      "content": "知识内容",
      "summary": "简短摘要",
      "category": "分类",
      "tags": ["标签1", "标签2", "标签3"],
      "confidence": 0.8
    }
  ]
}

如果没有找到有价值的知识，请返回：
{
  "knowledge": []
}`
}

// buildExtractionPrompt 构建提取提示词
func (s *knowledgeExtractionService) buildExtractionPrompt(text string) string {
	return fmt.Sprintf(`请从以下文本中提取有价值的知识点：

%s

请以JSON格式返回提取的知识。`, text)
}

// parseExtractionResponse 解析提取响应
func (s *knowledgeExtractionService) parseExtractionResponse(content string) ([]ExtractedKnowledge, error) {
	// 清理可能的 markdown 代码块标记
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	// 解析 JSON
	var result struct {
		Knowledge []ExtractedKnowledge `json:"knowledge"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return result.Knowledge, nil
}

// checkDuplicate 检查是否重复
func (s *knowledgeExtractionService) checkDuplicate(ctx context.Context, knowledge ExtractedKnowledge) (bool, error) {
	var count int64
	err := s.db.Model(&models.Knowledge{}).
		Where("LOWER(title) = ?", strings.ToLower(knowledge.Title)).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	// 如果标题完全相同，认为是重复
	if count > 0 {
		return true, nil
	}

	return false, nil
}

// mergeOrDuplicate 合并或判断重复
func (s *knowledgeExtractionService) mergeOrDuplicate(ctx context.Context, knowledge ExtractedKnowledge, userID string) error {
	// 查找相似的知识
	var existing models.Knowledge
	err := s.db.Where("user_id = ? AND LOWER(title) = ?", userID, strings.ToLower(knowledge.Title)).
		First(&existing).Error

	if err == nil {
		// 找到相同标题的知识，合并内容
		mergedContent := fmt.Sprintf("%s\n\n【补充】%s", existing.Content, knowledge.Content)
		existing.Content = mergedContent
		existing.UpdatedAt = time.Now()
		existing.Confidence = (existing.Confidence + knowledge.Confidence) / 2

		return s.knowledgeService.UpdateKnowledge(&existing)
	}

	if err == gorm.ErrRecordNotFound {
		// 没有找到，不是重复
		return nil
	}

	return err
}

// getOrCreateCategory 获取或创建分类
func (s *knowledgeExtractionService) getOrCreateCategory(ctx context.Context, categoryName, userID string) (string, error) {
	if categoryName == "" {
		return "", nil
	}

	// 查找现有分类
	var category models.KnowledgeCategory
	err := s.db.Where("user_id = ? AND name = ?", userID, categoryName).First(&category).Error
	if err == nil {
		return category.ID, nil
	}

	if err != gorm.ErrRecordNotFound {
		return "", err
	}

	// 创建新分类
	category = models.KnowledgeCategory{
		ID:          uuid.New().String(),
		Name:        categoryName,
		Description: "自动创建的分类",
		UserID:      userID,
		CreatedAt:   time.Now(),
	}

	if err := s.knowledgeService.CreateCategory(&category); err != nil {
		return "", err
	}

	return category.ID, nil
}

// createTagIfNotExists 创建标签（如果不存在）
func (s *knowledgeExtractionService) createTagIfNotExists(ctx context.Context, tagName, userID string) error {
	var count int64
	err := s.db.Model(&models.KnowledgeTag{}).
		Where("user_id = ? AND name = ?", userID, tagName).
		Count(&count).Error

	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	// 创建新标签
	tag := models.KnowledgeTag{
		ID:        uuid.New().String(),
		Name:      tagName,
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	return s.db.Create(&tag).Error
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
