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

// VectorService 向量服务接口
type VectorService interface {
	CreateCollection(name string, dimension int) error
	Insert(collectionName string, id string, content string, metadata map[string]interface{}) error
	Search(collectionName string, query []float32, k int) ([]SearchResult, error)
	SemanticSearch(collectionName string, queryText string, k int) ([]SearchResult, error)
}

// KnowledgeService 知识库服务接口
type KnowledgeService interface {
	// CreateKnowledge 创建知识条目
	CreateKnowledge(knowledge *models.Knowledge) error

	// CreateKnowledgeWithEmbedding 创建知识条目并生成向量
	CreateKnowledgeWithEmbedding(ctx context.Context, title, content, summary, categoryID, source, sourceID, userID string) (*models.Knowledge, error)

	// GetKnowledge 获取知识条目
	GetKnowledge(id string) (*models.Knowledge, error)

	// UpdateKnowledge 更新知识条目
	UpdateKnowledge(knowledge *models.Knowledge) error

	// DeleteKnowledge 删除知识条目
	DeleteKnowledge(id string) error

	// ListKnowledge 列出知识条目
	ListKnowledge(userID, categoryID string, limit, offset int) ([]models.Knowledge, error)

	// SearchKnowledge 语义搜索知识
	SearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeSearchResult, error)

	// HybridSearchKnowledge 混合搜索（语义+关键词）
	HybridSearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeSearchResult, error)

	// IncrementAccessCount 增加访问次数
	IncrementAccessCount(id string) error

	// CreateCategory 创建分类
	CreateCategory(category *models.KnowledgeCategory) error

	// GetCategory 获取分类
	GetCategory(id string) (*models.KnowledgeCategory, error)

	// ListCategories 列出分类
	ListCategories(userID string) ([]models.KnowledgeCategory, error)

	// DeleteCategory 删除分类
	DeleteCategory(id string) error

	// CreateDocument 创建文档
	CreateDocument(document *models.Document) error

	// GetDocument 获取文档
	GetDocument(id string) (*models.Document, error)

	// ListDocuments 列出文档
	ListDocuments(userID string) ([]models.Document, error)
}

// KnowledgeSearchResult 知识搜索结果
type KnowledgeSearchResult struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Summary     string    `json:"summary"`
	CategoryID  string    `json:"category_id"`
	Source      string    `json:"source"`
	SourceID    string    `json:"source_id"`
	Score       float64   `json:"score"`
	Confidence  float64   `json:"confidence"`
	AccessCount int       `json:"access_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// knowledgeService 知识库服务实现
type knowledgeService struct {
	db        *gorm.DB
	embedding EmbeddingService
	vector    VectorService
	cache     *CacheService
}

// NewKnowledgeService 创建知识库服务
func NewKnowledgeService(db *gorm.DB, embedding EmbeddingService, vector VectorService, cache *CacheService) KnowledgeService {
	// 自动迁移
	db.AutoMigrate(
		&models.Knowledge{},
		&models.KnowledgeCategory{},
		&models.KnowledgeTag{},
		&models.KnowledgeTagRelation{},
		&models.Document{},
	)

	return &knowledgeService{
		db:        db,
		embedding: embedding,
		vector:    vector,
		cache:     cache,
	}
}

// CreateKnowledge 创建知识条目
func (s *knowledgeService) CreateKnowledge(knowledge *models.Knowledge) error {
	if knowledge.ID == "" {
		knowledge.ID = uuid.New().String()
	}

	if knowledge.CreatedAt.IsZero() {
		knowledge.CreatedAt = time.Now()
	}
	knowledge.UpdatedAt = time.Now()

	result := s.db.Create(knowledge)
	if result.Error != nil {
		return fmt.Errorf("failed to create knowledge: %w", result.Error)
	}

	// 清除缓存
	s.cache.Delete("knowledge:all")
	s.cache.Delete("knowledge:user:" + knowledge.UserID)

	return nil
}

// CreateKnowledgeWithEmbedding 创建知识条目并生成向量
func (s *knowledgeService) CreateKnowledgeWithEmbedding(ctx context.Context, title, content, summary, categoryID, source, sourceID, userID string) (*models.Knowledge, error) {
	// 生成组合文本用于 embedding
	combinedText := title
	if content != "" {
		combinedText += "\n\n" + content
	}

	// 生成 embedding
	embedding, err := s.embedding.GenerateEmbedding(ctx, combinedText)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 创建知识条目
	knowledge := &models.Knowledge{
		ID:         uuid.New().String(),
		Title:      title,
		Content:    content,
		Summary:    summary,
		CategoryID: categoryID,
		Source:     source,
		SourceID:   sourceID,
		Embedding:  embedding,
		Confidence: 0.8, // 默认置信度
		UserID:     userID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := s.CreateKnowledge(knowledge); err != nil {
		return nil, err
	}

	return knowledge, nil
}

// GetKnowledge 获取知识条目
func (s *knowledgeService) GetKnowledge(id string) (*models.Knowledge, error) {
	// 检查缓存
	cacheKey := "knowledge:" + id
	if cached, found := s.cache.Get(cacheKey); found {
		if knowledge, ok := cached.(*models.Knowledge); ok {
			return knowledge, nil
		}
	}

	var knowledge models.Knowledge
	result := s.db.First(&knowledge, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("knowledge not found")
		}
		return nil, fmt.Errorf("failed to get knowledge: %w", result.Error)
	}

	// 缓存结果
	s.cache.Set(cacheKey, &knowledge, 5*time.Minute)

	return &knowledge, nil
}

// UpdateKnowledge 更新知识条目
func (s *knowledgeService) UpdateKnowledge(knowledge *models.Knowledge) error {
	knowledge.UpdatedAt = time.Now()

	result := s.db.Save(knowledge)
	if result.Error != nil {
		return fmt.Errorf("failed to update knowledge: %w", result.Error)
	}

	// 清除缓存
	s.cache.Delete("knowledge:" + knowledge.ID)
	s.cache.Delete("knowledge:all")
	s.cache.Delete("knowledge:user:" + knowledge.UserID)

	return nil
}

// DeleteKnowledge 删除知识条目
func (s *knowledgeService) DeleteKnowledge(id string) error {
	result := s.db.Delete(&models.Knowledge{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete knowledge: %w", result.Error)
	}

	// 清除缓存
	s.cache.Delete("knowledge:" + id)
	s.cache.Delete("knowledge:all")

	return nil
}

// ListKnowledge 列出知识条目
func (s *knowledgeService) ListKnowledge(userID, categoryID string, limit, offset int) ([]models.Knowledge, error) {
	if limit <= 0 {
		limit = 100
	}

	var knowledges []models.Knowledge
	query := s.db.Model(&models.Knowledge{})

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	result := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&knowledges)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list knowledge: %w", result.Error)
	}

	return knowledges, nil
}

// SearchKnowledge 语义搜索知识
func (s *knowledgeService) SearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// 生成查询向量
	queryVector, err := s.embedding.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// 获取所有知识条目
	var knowledges []models.Knowledge
	result := s.db.Find(&knowledges)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to fetch knowledge: %w", result.Error)
	}

	// 计算相似度
	scoredResults := make([]scoredResult, 0, len(knowledges))
	for _, k := range knowledges {
		if len(k.Embedding) != len(queryVector) {
			continue
		}

		score := CosineSimilarity(queryVector, k.Embedding)
		if score > 0.1 { // 最低相似度阈值
			scoredResults = append(scoredResults, scoredResult{
				knowledge: k,
				score:     score,
			})
		}
	}

	// 按相似度降序排序
	sortScoredResults(scoredResults)

	// 转换为搜索结果
	results := make([]KnowledgeSearchResult, 0, min(limit, len(scoredResults)))
	for i := 0; i < min(limit, len(scoredResults)); i++ {
		k := scoredResults[i].knowledge
		results = append(results, KnowledgeSearchResult{
			ID:          k.ID,
			Title:       k.Title,
			Content:     k.Content,
			Summary:     k.Summary,
			CategoryID:  k.CategoryID,
			Source:      k.Source,
			SourceID:    k.SourceID,
			Score:       scoredResults[i].score,
			Confidence:  k.Confidence,
			AccessCount: k.AccessCount,
			CreatedAt:   k.CreatedAt,
		})
	}

	return results, nil
}

// HybridSearchKnowledge 混合搜索（语义+关键词）
func (s *knowledgeService) HybridSearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// 权重配置
	vectorWeight := 0.7
	keywordWeight := 0.3

	// 生成查询向量
	queryVector, err := s.embedding.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// 获取所有知识条目
	var knowledges []models.Knowledge
	result := s.db.Find(&knowledges)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to fetch knowledge: %w", result.Error)
	}

	// 计算混合得分
	scoredResults := make([]scoredResult, 0, len(knowledges))
	queryLower := strings.ToLower(query)

	for _, k := range knowledges {
		vectorScore := 0.0
		keywordScore := 0.0

		// 计算向量相似度
		if len(k.Embedding) == len(queryVector) {
			vectorScore = CosineSimilarity(queryVector, k.Embedding)
		}

		// 计算关键词匹配度
		titleLower := strings.ToLower(k.Title)
		contentLower := strings.ToLower(k.Content)

		if strings.Contains(titleLower, queryLower) {
			// 标题完全匹配
			keywordScore = 1.0
		} else if strings.Contains(contentLower, queryLower) {
			// 内容完全匹配
			keywordScore = 0.8
		} else {
			// 部分匹配（按单词）
			queryWords := strings.Fields(queryLower)
			matchedWords := 0
			for _, word := range queryWords {
				if strings.Contains(titleLower, word) || strings.Contains(contentLower, word) {
					matchedWords++
				}
			}
			if len(queryWords) > 0 {
				keywordScore = float64(matchedWords) / float64(len(queryWords)) * 0.6
			}
		}

		// 计算混合得分
		combinedScore := vectorScore*vectorWeight + keywordScore*keywordWeight

		if combinedScore > 0.1 { // 最低相似度阈值
			scoredResults = append(scoredResults, scoredResult{
				knowledge: k,
				score:     combinedScore,
			})
		}
	}

	// 按混合得分降序排序
	sortScoredResults(scoredResults)

	// 转换为搜索结果
	results := make([]KnowledgeSearchResult, 0, min(limit, len(scoredResults)))
	for i := 0; i < min(limit, len(scoredResults)); i++ {
		k := scoredResults[i].knowledge
		results = append(results, KnowledgeSearchResult{
			ID:          k.ID,
			Title:       k.Title,
			Content:     k.Content,
			Summary:     k.Summary,
			CategoryID:  k.CategoryID,
			Source:      k.Source,
			SourceID:    k.SourceID,
			Score:       scoredResults[i].score,
			Confidence:  k.Confidence,
			AccessCount: k.AccessCount,
			CreatedAt:   k.CreatedAt,
		})
	}

	return results, nil
}

// IncrementAccessCount 增加访问次数
func (s *knowledgeService) IncrementAccessCount(id string) error {
	result := s.db.Model(&models.Knowledge{}).
		Where("id = ?", id).
		UpdateColumn("access_count", gorm.Expr("access_count + ?", 1))
	if result.Error != nil {
		return fmt.Errorf("failed to increment access count: %w", result.Error)
	}

	// 清除缓存
	s.cache.Delete("knowledge:" + id)

	return nil
}

// CreateCategory 创建分类
func (s *knowledgeService) CreateCategory(category *models.KnowledgeCategory) error {
	if category.ID == "" {
		category.ID = uuid.New().String()
	}

	if category.CreatedAt.IsZero() {
		category.CreatedAt = time.Now()
	}

	result := s.db.Create(category)
	if result.Error != nil {
		return fmt.Errorf("failed to create category: %w", result.Error)
	}

	// 清除缓存
	s.cache.Delete("categories:all")
	s.cache.Delete("categories:user:" + category.UserID)

	return nil
}

// GetCategory 获取分类
func (s *knowledgeService) GetCategory(id string) (*models.KnowledgeCategory, error) {
	var category models.KnowledgeCategory
	result := s.db.First(&category, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("category not found")
		}
		return nil, fmt.Errorf("failed to get category: %w", result.Error)
	}

	return &category, nil
}

// ListCategories 列出分类
func (s *knowledgeService) ListCategories(userID string) ([]models.KnowledgeCategory, error) {
	var categories []models.KnowledgeCategory
	query := s.db.Model(&models.KnowledgeCategory{})

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	result := query.Order("created_at DESC").Find(&categories)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list categories: %w", result.Error)
	}

	return categories, nil
}

// DeleteCategory 删除分类
func (s *knowledgeService) DeleteCategory(id string) error {
	var count int64
	s.db.Model(&models.Knowledge{}).Where("category_id = ?", id).Count(&count)
	if count > 0 {
		return fmt.Errorf("cannot delete category: %d knowledge entries are using it", count)
	}
	return s.db.Where("id = ?", id).Delete(&models.KnowledgeCategory{}).Error
}

// CreateDocument 创建文档
func (s *knowledgeService) CreateDocument(document *models.Document) error {
	if document.ID == "" {
		document.ID = uuid.New().String()
	}

	if document.CreatedAt.IsZero() {
		document.CreatedAt = time.Now()
	}
	document.UpdatedAt = time.Now()

	result := s.db.Create(document)
	if result.Error != nil {
		return fmt.Errorf("failed to create document: %w", result.Error)
	}

	// 清除缓存
	s.cache.Delete("documents:all")
	s.cache.Delete("documents:user:" + document.UserID)

	return nil
}

// GetDocument 获取文档
func (s *knowledgeService) GetDocument(id string) (*models.Document, error) {
	var document models.Document
	result := s.db.First(&document, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("document not found")
		}
		return nil, fmt.Errorf("failed to get document: %w", result.Error)
	}

	return &document, nil
}

// ListDocuments 列出文档
func (s *knowledgeService) ListDocuments(userID string) ([]models.Document, error) {
	var documents []models.Document
	query := s.db.Model(&models.Document{})

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	result := query.Order("created_at DESC").Find(&documents)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list documents: %w", result.Error)
	}

	return documents, nil
}

// scoredResult 带分数的结果
type scoredResult struct {
	knowledge models.Knowledge
	score     float64
}

// sortScoredResults 对结果进行排序
func sortScoredResults(results []scoredResult) {
	// 使用冒泡排序按分数降序
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].score < results[j+1].score {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
