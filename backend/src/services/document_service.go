package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"openaide/backend/src/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DocumentService 文档服务接口
type DocumentService interface {
	// ImportDocument 导入文档
	ImportDocument(ctx context.Context, req ImportRequest) (*ImportResult, error)

	// ParseDocument 解析文档
	ParseDocument(fileType string, content []byte) (string, error)

	// ChunkDocument 文档分块
	ChunkDocument(content string, chunkSize int) []DocumentChunk

	// IndexDocument 索引文档（生成向量）
	IndexDocument(ctx context.Context, docID string) error

	// DeleteDocument 删除文档及其知识
	DeleteDocument(ctx context.Context, docID string) error
}

// ImportRequest 导入请求
type ImportRequest struct {
	Title    string
	FileType string // txt, md, pdf, docx
	Content  []byte
	UserID   string
}

// ImportResult 导入结果
type ImportResult struct {
	DocumentID  string    `json:"document_id"`
	Title       string    `json:"title"`
	ChunkCount  int       `json:"chunk_count"`
	KnowledgeIDs []string `json:"knowledge_ids"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// DocumentChunk 文档块
type DocumentChunk struct {
	Content  string
	Position int
}

// documentService 文档服务实现
type documentService struct {
	db        *gorm.DB
	embedding EmbeddingService
	knowledge KnowledgeService
	cache     *CacheService
}

// NewDocumentService 创建文档服务
func NewDocumentService(db *gorm.DB, embedding EmbeddingService, knowledge KnowledgeService, cache *CacheService) DocumentService {
	return &documentService{
		db:        db,
		embedding: embedding,
		knowledge: knowledge,
		cache:     cache,
	}
}

// ImportDocument 导入文档
func (s *documentService) ImportDocument(ctx context.Context, req ImportRequest) (*ImportResult, error) {
	// 验证请求
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if req.Content == nil || len(req.Content) == 0 {
		return nil, fmt.Errorf("content is required")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	// 解析文档内容
	parsedContent, err := s.ParseDocument(req.FileType, req.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse document: %w", err)
	}

	// 创建文档记录
	document := &models.Document{
		ID:        uuid.New().String(),
		Title:     req.Title,
		FileType:  strings.ToLower(req.FileType),
		Content:   parsedContent,
		Status:    "pending",
		UserID:    req.UserID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.db.Create(document).Error; err != nil {
		return nil, fmt.Errorf("failed to create document: %w", err)
	}

	// 清除缓存
	s.cache.Delete("documents:all")
	s.cache.Delete("documents:user:" + req.UserID)

	// 开始索引
	if err := s.IndexDocument(ctx, document.ID); err != nil {
		// 更新状态为失败
		s.db.Model(document).Updates(map[string]interface{}{
			"status":    "failed",
			"updated_at": time.Now(),
		})
		return nil, fmt.Errorf("failed to index document: %w", err)
	}

	// 获取更新后的文档
	if err := s.db.First(document, "id = ?", document.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve document: %w", err)
	}

	// 获取关联的知识条目ID
	var knowledges []models.Knowledge
	if err := s.db.Where("source = ? AND source_id = ?", "document", document.ID).Find(&knowledges).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve knowledges: %w", err)
	}

	knowledgeIDs := make([]string, len(knowledges))
	for i, k := range knowledges {
		knowledgeIDs[i] = k.ID
	}

	return &ImportResult{
		DocumentID:   document.ID,
		Title:        document.Title,
		ChunkCount:   document.ChunkCount,
		KnowledgeIDs: knowledgeIDs,
		Status:       document.Status,
		CreatedAt:    document.CreatedAt,
	}, nil
}

// ParseDocument 解析文档
func (s *documentService) ParseDocument(fileType string, content []byte) (string, error) {
	switch strings.ToLower(fileType) {
	case "txt", "text":
		return s.parseTxt(content)
	case "md", "markdown":
		return s.parseMarkdown(content)
	case "pdf":
		return "", fmt.Errorf("PDF format not yet supported")
	case "docx":
		return "", fmt.Errorf("DOCX format not yet supported")
	default:
		// 尝试作为纯文本处理
		return s.parseTxt(content)
	}
}

// parseTxt 解析纯文本
func (s *documentService) parseTxt(content []byte) (string, error) {
	// 转换为字符串并清理
	text := string(content)
	// 移除 BOM 标记 (UTF-8)
	if len(text) > 0 && text[0] == '\xEF' && text[1] == '\xBB' && text[2] == '\xBF' {
		text = text[1:]
	}
	// 标准化换行符
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSpace(text)
	return text, nil
}

// parseMarkdown 解析 Markdown
func (s *documentService) parseMarkdown(content []byte) (string, error) {
	text := string(content)

	// 移除 BOM 标记 (UTF-8)
	if len(text) > 0 && text[0] == '\xEF' && text[1] == '\xBB' && text[2] == '\xBF' {
		text = text[1:]
	}

	// 标准化换行符
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// 构建段落
	var paragraphs []string
	lines := strings.Split(text, "\n")
	var currentParagraph strings.Builder
	inCodeBlock := false

	for _, line := range lines {
		// 检测代码块
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
			currentParagraph.WriteString(line + "\n")
			continue
		}

		if inCodeBlock {
			currentParagraph.WriteString(line + "\n")
			continue
		}

		// 跳过空的标题标记行
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if currentParagraph.Len() > 0 {
				paragraphs = append(paragraphs, strings.TrimSpace(currentParagraph.String()))
				currentParagraph.Reset()
			}
			continue
		}

		// 处理标题
		if strings.HasPrefix(trimmed, "#") {
			if currentParagraph.Len() > 0 {
				paragraphs = append(paragraphs, strings.TrimSpace(currentParagraph.String()))
				currentParagraph.Reset()
			}
			// 保留标题内容，移除 # 符号
			titleContent := strings.TrimLeft(trimmed, "#")
			titleContent = strings.TrimSpace(titleContent)
			if titleContent != "" {
				paragraphs = append(paragraphs, titleContent)
			}
			continue
		}

		// 处理列表项
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") {
			if currentParagraph.Len() > 0 {
				paragraphs = append(paragraphs, strings.TrimSpace(currentParagraph.String()))
				currentParagraph.Reset()
			}
			listContent := strings.TrimLeft(trimmed, "-*")
			listContent = strings.TrimSpace(listContent)
			paragraphs = append(paragraphs, listContent)
			continue
		}

		// 处理有序列表
		if len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && trimmed[1] == '.' {
			if currentParagraph.Len() > 0 {
				paragraphs = append(paragraphs, strings.TrimSpace(currentParagraph.String()))
				currentParagraph.Reset()
			}
			listContent := strings.TrimPrefix(trimmed, trimmed[:2])
			listContent = strings.TrimSpace(listContent)
			paragraphs = append(paragraphs, listContent)
			continue
		}

		// 普通文本行
		if currentParagraph.Len() > 0 {
			currentParagraph.WriteString(" ")
		}
		currentParagraph.WriteString(line)
	}

	// 添加最后一个段落
	if currentParagraph.Len() > 0 {
		paragraphs = append(paragraphs, strings.TrimSpace(currentParagraph.String()))
	}

	// 合并非空段落
	var result strings.Builder
	for i, p := range paragraphs {
		if p != "" {
			if i > 0 {
				result.WriteString("\n\n")
			}
			result.WriteString(p)
		}
	}

	return strings.TrimSpace(result.String()), nil
}

// ChunkDocument 文档分块
func (s *documentService) ChunkDocument(content string, chunkSize int) []DocumentChunk {
	if chunkSize <= 0 {
		chunkSize = 500 // 默认分块大小
	}

	// 按段落分割
	paragraphs := strings.Split(content, "\n\n")

	var chunks []DocumentChunk
	var currentChunk strings.Builder
 currentPosition := 0
 chunkStartPos := 0

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		// 如果单个段落超过分块大小，需要进一步分割
		if len(paragraph) > chunkSize {
			// 先保存当前累积的内容
			if currentChunk.Len() > 0 {
				chunks = append(chunks, DocumentChunk{
					Content:  strings.TrimSpace(currentChunk.String()),
					Position: chunkStartPos,
				})
				chunkStartPos = currentPosition
				currentChunk.Reset()
			}

			// 分割长段落
			sentences := s.splitParagraph(paragraph, chunkSize)
			for _, sentence := range sentences {
				chunks = append(chunks, DocumentChunk{
					Content:  sentence,
					Position: currentPosition,
				})
				currentPosition += len(sentence)
			}
			chunkStartPos = currentPosition
			continue
		}

		// 检查添加该段落后是否会超过分块大小
		testLength := currentChunk.Len() + len(paragraph)
		if currentChunk.Len() > 0 {
			testLength += 2 // 添加 "\n\n"
		}

		if testLength > chunkSize && currentChunk.Len() > 0 {
			// 保存当前分块
			chunks = append(chunks, DocumentChunk{
				Content:  strings.TrimSpace(currentChunk.String()),
				Position: chunkStartPos,
			})
			currentChunk.Reset()
			chunkStartPos = currentPosition
		}

		// 添加段落到当前分块
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
			currentPosition += 2
		}
		currentChunk.WriteString(paragraph)
		currentPosition += len(paragraph)
	}

	// 添加最后一个分块
	if currentChunk.Len() > 0 {
		chunks = append(chunks, DocumentChunk{
			Content:  strings.TrimSpace(currentChunk.String()),
			Position: chunkStartPos,
		})
	}

	return chunks
}

// splitParagraph 将长段落分割成适合的大小
func (s *documentService) splitParagraph(paragraph string, maxSize int) []string {
	// 首先尝试按句子分割
	sentences := strings.Split(paragraph, "。")

	var result []string
	var currentSentence strings.Builder

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		testLength := currentSentence.Len() + len(sentence) + 1 // +1 for "。"
		if testLength > maxSize && currentSentence.Len() > 0 {
			result = append(result, strings.TrimSpace(currentSentence.String())+"。")
			currentSentence.Reset()
		}

		currentSentence.WriteString(sentence)
	}

	if currentSentence.Len() > 0 {
		result = append(result, strings.TrimSpace(currentSentence.String())+"。")
	}

	// 如果仍有超长的分块，强制按字符分割
	var finalResult []string
	for _, chunk := range result {
		if len(chunk) <= maxSize {
			finalResult = append(finalResult, chunk)
		} else {
			// 强制分割
			for i := 0; i < len(chunk); i += maxSize {
				end := i + maxSize
				if end > len(chunk) {
					end = len(chunk)
				}
				finalResult = append(finalResult, chunk[i:end])
			}
		}
	}

	return finalResult
}

// IndexDocument 索引文档（生成向量）
func (s *documentService) IndexDocument(ctx context.Context, docID string) error {
	// 获取文档
	var document models.Document
	if err := s.db.First(&document, "id = ?", docID).Error; err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// 更新状态为处理中
	s.db.Model(&document).Updates(map[string]interface{}{
		"status":    "processing",
		"updated_at": time.Now(),
	})

	// 分块文档
	chunks := s.ChunkDocument(document.Content, 500)
	document.ChunkCount = len(chunks)

	// 批量生成 Embedding
	chunkTexts := make([]string, len(chunks))
	for i, chunk := range chunks {
		chunkTexts[i] = chunk.Content
	}

	embeddings, err := s.embedding.GenerateEmbeddings(ctx, chunkTexts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// 创建知识条目
	knowledgeIDs := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		// 生成标题（使用文档标题 + 分块编号）
		chunkTitle := document.Title
		if len(chunks) > 1 {
			chunkTitle = fmt.Sprintf("%s (第%d部分)", document.Title, i+1)
		}

		knowledge := &models.Knowledge{
			ID:        uuid.New().String(),
			Title:     chunkTitle,
			Content:   chunk.Content,
			Summary:   s.generateSummary(chunk.Content),
			Source:    "document",
			SourceID:  document.ID,
			Embedding: embeddings[i],
			Confidence: 0.8,
			UserID:    document.UserID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.db.Create(knowledge).Error; err != nil {
			return fmt.Errorf("failed to create knowledge: %w", err)
		}

		knowledgeIDs = append(knowledgeIDs, knowledge.ID)
	}

	// 更新文档状态为完成
	s.db.Model(&document).Updates(map[string]interface{}{
		"status":     "completed",
		"chunk_count": len(chunks),
		"updated_at":  time.Now(),
	})

	// 清除缓存
	s.cache.Delete("documents:all")
	s.cache.Delete("documents:user:" + document.UserID)
	s.cache.Delete("knowledge:all")

	return nil
}

// generateSummary 生成摘要
func (s *documentService) generateSummary(content string) string {
	// 简单的摘要生成：取前100个字符
	maxLength := 100
	if len(content) <= maxLength {
		return content
	}

	summary := content[:maxLength]
	// 确保在完整的词或句子处截断
	lastSpace := strings.LastIndexAny(summary, "。.!！??\n\t ")
	if lastSpace > maxLength/2 {
		summary = summary[:lastSpace]
	}

	return summary + "..."
}

// DeleteDocument 删除文档及其知识
func (s *documentService) DeleteDocument(ctx context.Context, docID string) error {
	// 获取文档
	var document models.Document
	if err := s.db.First(&document, "id = ?", docID).Error; err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	// 删除关联的知识条目
	if err := s.db.Where("source = ? AND source_id = ?", "document", docID).Delete(&models.Knowledge{}).Error; err != nil {
		return fmt.Errorf("failed to delete related knowledge: %w", err)
	}

	// 删除文档
	if err := s.db.Delete(&document).Error; err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	// 清除缓存
	s.cache.Delete("documents:all")
	s.cache.Delete("documents:user:" + document.UserID)
	s.cache.Delete("knowledge:all")

	return nil
}
