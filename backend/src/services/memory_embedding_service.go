package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// MemoryEmbeddingService 记忆向量嵌入服务
type MemoryEmbeddingService struct {
	db             *gorm.DB
	embeddingSvc   EmbeddingService
	cache          *CacheService
	batchSize      int
	workerCount    int
}

// MemoryWithEmbedding 带向量的记忆
type MemoryWithEmbedding struct {
	Memory    models.Memory
	Embedding []float64
	Score     float64
}

// SemanticSearchResult 语义搜索结果
type SemanticSearchResult struct {
	Memory       models.Memory `json:"memory"`
	Score        float64       `json:"score"`
	MatchType    string        `json:"match_type"` // semantic, keyword, hybrid
	KeywordScore float64       `json:"keyword_score"`
	VectorScore  float64       `json:"vector_score"`
}

// NewMemoryEmbeddingService 创建记忆向量服务
func NewMemoryEmbeddingService(db *gorm.DB, embeddingSvc EmbeddingService, cache *CacheService) *MemoryEmbeddingService {
	return &MemoryEmbeddingService{
		db:           db,
		embeddingSvc: embeddingSvc,
		cache:        cache,
		batchSize:    100,
		workerCount:  4,
	}
}

// ==================== 向量生成 ====================

// GenerateEmbedding 为记忆内容生成向量
func (s *MemoryEmbeddingService) GenerateEmbedding(content string) ([]float64, error) {
	if s.embeddingSvc == nil {
		return nil, fmt.Errorf("embedding service not available")
	}

	// 使用 embedding 服务
	embedding, err := s.embeddingSvc.GenerateEmbedding(context.Background(), content)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	return embedding, nil
}

// BatchGenerateEmbeddings 批量生成向量
func (s *MemoryEmbeddingService) BatchGenerateEmbeddings(contents []string) ([][]float64, error) {
	if s.embeddingSvc == nil {
		return nil, fmt.Errorf("embedding service not available")
	}

	// 使用 embedding 服务的批量接口
	return s.embeddingSvc.GenerateEmbeddings(context.Background(), contents)
}

// ==================== 向量存储 ====================

// SaveMemoryEmbedding 保存记忆向量
func (s *MemoryEmbeddingService) SaveMemoryEmbedding(memoryID string, embedding []float64) error {
	// 将向量序列化为 JSON
	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	// 保存到数据库
	result := s.db.Exec(
		"UPDATE memories SET embedding = ? WHERE id = ?",
		string(embeddingJSON),
		memoryID,
	)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// GetMemoryEmbedding 获取记忆向量
func (s *MemoryEmbeddingService) GetMemoryEmbedding(memoryID string) ([]float64, error) {
	cacheKey := fmt.Sprintf("memory:embedding:%s", memoryID)
	
	// 检查缓存
	if cached, found := s.cache.Get(cacheKey); found {
		return cached.([]float64), nil
	}

	// 从数据库获取
	var embeddingStr string
	result := s.db.Raw(
		"SELECT embedding FROM memories WHERE id = ?",
		memoryID,
	).Scan(&embeddingStr)
	
	if result.Error != nil {
		return nil, result.Error
	}

	if embeddingStr == "" {
		return nil, fmt.Errorf("embedding not found for memory %s", memoryID)
	}

	var embedding []float64
	if err := json.Unmarshal([]byte(embeddingStr), &embedding); err != nil {
		return nil, err
	}

	// 缓存
	s.cache.Set(cacheKey, embedding, 30*time.Minute)

	return embedding, nil
}

// ==================== 语义搜索 ====================

// SemanticSearch 语义搜索
func (s *MemoryEmbeddingService) SemanticSearch(
	ctx context.Context,
	userID string,
	query string,
	limit int,
) ([]SemanticSearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	// 生成查询向量
	queryEmbedding, err := s.GenerateEmbedding(query)
	if err != nil {
		return nil, err
	}

	// 获取用户的所有记忆
	var memories []models.Memory
	result := s.db.Where("user_id = ?", userID).Find(&memories)
	if result.Error != nil {
		return nil, result.Error
	}

	// 计算相似度
	var results []SemanticSearchResult
	for _, memory := range memories {
		// 获取向量
		memoryEmbedding, err := s.GetMemoryEmbedding(memory.ID)
		if err != nil {
			// 如果没有向量，尝试生成
			memoryEmbedding, err = s.GenerateEmbedding(memory.Content)
			if err != nil {
				continue
			}
			s.SaveMemoryEmbedding(memory.ID, memoryEmbedding)
		}

		// 计算余弦相似度
		similarity := CosineSimilarity(queryEmbedding, memoryEmbedding)
		
		if similarity > 0.7 { // 阈值
			results = append(results, SemanticSearchResult{
				Memory:    memory,
				Score:     similarity,
				MatchType: "semantic",
				VectorScore: similarity,
			})
		}
	}

	// 按相似度排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 限制数量
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// HybridSearch 混合搜索（语义 + 关键词）
func (s *MemoryEmbeddingService) HybridSearch(
	ctx context.Context,
	userID string,
	query string,
	limit int,
	semanticWeight float64,
) ([]SemanticSearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if semanticWeight < 0 || semanticWeight > 1 {
		semanticWeight = 0.7
	}
	keywordWeight := 1 - semanticWeight

	// 获取语义搜索结果
	semanticResults, err := s.SemanticSearch(ctx, userID, query, limit*2)
	if err != nil {
		semanticResults = []SemanticSearchResult{}
	}

	// 获取关键词搜索结果
	keywordResults, err := s.keywordSearch(userID, query, limit*2)
	if err != nil {
		keywordResults = []SemanticSearchResult{}
	}

	// 合并结果
	resultMap := make(map[string]*SemanticSearchResult)

	// 添加语义结果
	for i := range semanticResults {
		r := &semanticResults[i]
		r.Score = r.Score * semanticWeight
		resultMap[r.Memory.ID] = r
	}

	// 添加关键词结果
	for i := range keywordResults {
		r := keywordResults[i]
		if existing, ok := resultMap[r.Memory.ID]; ok {
			// 已存在，混合分数
			existing.Score += r.Score * keywordWeight
			existing.KeywordScore = r.Score
			existing.MatchType = "hybrid"
		} else {
			r.Score = r.Score * keywordWeight
			r.KeywordScore = r.Score
			resultMap[r.Memory.ID] = &r
		}
	}

	// 转换为列表并排序
	var results []SemanticSearchResult
	for _, r := range resultMap {
		results = append(results, *r)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 限制数量
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// keywordSearch 关键词搜索
func (s *MemoryEmbeddingService) keywordSearch(userID, query string, limit int) ([]SemanticSearchResult, error) {
	keywords := strings.Fields(strings.ToLower(query))
	if len(keywords) == 0 {
		return nil, nil
	}

	// 构建 SQL
	var conditions []string
	var args []interface{}
	args = append(args, userID)

	for _, keyword := range keywords {
		conditions = append(conditions, "LOWER(content) LIKE ?")
		args = append(args, "%"+keyword+"%")
	}

	queryStr := fmt.Sprintf(
		"SELECT * FROM memories WHERE user_id = ? AND (%s)",
		strings.Join(conditions, " OR "),
	)

	var memories []models.Memory
	result := s.db.Raw(queryStr, args...).Scan(&memories)
	if result.Error != nil {
		return nil, result.Error
	}

	// 计算关键词匹配分数
	var results []SemanticSearchResult
	for _, memory := range memories {
		score := s.calculateKeywordScore(memory.Content, keywords)
		results = append(results, SemanticSearchResult{
			Memory:       memory,
			Score:        score,
			MatchType:    "keyword",
			KeywordScore: score,
		})
	}

	// 排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// calculateKeywordScore 计算关键词匹配分数
func (s *MemoryEmbeddingService) calculateKeywordScore(content string, keywords []string) float64 {
	contentLower := strings.ToLower(content)
	score := 0.0

	for _, keyword := range keywords {
		if strings.Contains(contentLower, keyword) {
			score += 0.3
			
			// 完全匹配加分
			if strings.Contains(contentLower, " "+keyword+" ") {
				score += 0.2
			}
		}
	}

	// 归一化
	if len(keywords) > 0 {
		score = score / float64(len(keywords))
	}

	return math.Min(score, 1.0)
}

// ==================== 批量处理 ====================

// BatchEmbedUserMemories 批量向量化用户的所有记忆
func (s *MemoryEmbeddingService) BatchEmbedUserMemories(userID string) (int, error) {
	// 获取没有向量的记忆
	var memories []models.Memory
	result := s.db.Where(
		"user_id = ? AND (embedding IS NULL OR embedding = '')",
		userID,
	).Find(&memories)
	
	if result.Error != nil {
		return 0, result.Error
	}

	if len(memories) == 0 {
		return 0, nil
	}

	// 提取内容
	contents := make([]string, len(memories))
	for i, m := range memories {
		contents[i] = m.Content
	}

	// 批量生成向量
	embeddings, err := s.BatchGenerateEmbeddings(contents)
	if err != nil {
		return 0, err
	}

	// 保存向量
	successCount := 0
	for i, memory := range memories {
		if i < len(embeddings) && embeddings[i] != nil {
			if err := s.SaveMemoryEmbedding(memory.ID, embeddings[i]); err == nil {
				successCount++
			}
		}
	}

	return successCount, nil
}

// AutoEmbedNewMemories 自动向量化新记忆
func (s *MemoryEmbeddingService) AutoEmbedNewMemories(memoryID string) error {
	// 获取记忆
	var memory models.Memory
	result := s.db.First(&memory, memoryID)
	if result.Error != nil {
		return result.Error
	}

	// 生成向量
	embedding, err := s.GenerateEmbedding(memory.Content)
	if err != nil {
		return err
	}

	// 保存
	return s.SaveMemoryEmbedding(memory.ID, embedding)
}

// ==================== 相似度计算 ====================




