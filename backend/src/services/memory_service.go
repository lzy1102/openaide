package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// MemoryService 记忆服务 - 三层记忆架构
type MemoryService struct {
	BaseService        BaseService
	store              MemoryStore
	embeddingSvc       *MemoryEmbeddingService
}

// NewMemoryService 创建记忆服务实例
func NewMemoryService(db *gorm.DB, cache *CacheService) *MemoryService {
	return &MemoryService{
		BaseService: BaseService{DB: db, Cache: cache},
		store:       NewGormMemoryStore(db),
	}
}

// NewMemoryServiceWithStore 创建记忆服务实例（可注入自定义存储）
func NewMemoryServiceWithStore(db *gorm.DB, store MemoryStore, cache *CacheService) *MemoryService {
	return &MemoryService{
		BaseService: BaseService{DB: db, Cache: cache},
		store:       store,
	}
}

// SetEmbeddingService 设置向量嵌入服务
func (s *MemoryService) SetEmbeddingService(embeddingSvc *MemoryEmbeddingService) {
	s.embeddingSvc = embeddingSvc
}

// ==================== 长期记忆（第三层）====================

// CreateMemory 创建记忆
func (s *MemoryService) CreateMemory(memory *models.Memory) error {
	memory.ID = uuid.New().String()
	err := s.store.Create(context.Background(), memory)
	if err != nil {
		return err
	}

	// 自动生成向量
	if s.embeddingSvc != nil {
		go func() {
			if err := s.embeddingSvc.AutoEmbedNewMemories(memory.ID); err != nil {
				log.Printf("[Memory] failed to embed memory %s: %v", memory.ID, err)
			}
		}()
	}

	return nil
}

// CreateFactMemory 创建事实记忆
func (s *MemoryService) CreateFactMemory(userID, content string, importance int, tags []string) (*models.Memory, error) {
	memory := &models.Memory{
		UserID:     userID,
		Content:    content,
		Category:   models.MemoryCategoryPersonal,
		MemoryType: models.MemoryTypeFact,
		Importance: importance,
		Tags:       tags,
	}
	err := s.CreateMemory(memory)
	return memory, err
}

// CreatePreferenceMemory 创建偏好记忆
func (s *MemoryService) CreatePreferenceMemory(userID, content string, tags []string) (*models.Memory, error) {
	memory := &models.Memory{
		UserID:     userID,
		Content:    content,
		Category:   models.MemoryCategoryHabit,
		MemoryType: models.MemoryTypePreference,
		Importance: 4,
		Tags:       tags,
	}
	err := s.CreateMemory(memory)
	return memory, err
}

// CreateProcedureMemory 创建流程记忆
func (s *MemoryService) CreateProcedureMemory(userID, content string, tags []string) (*models.Memory, error) {
	memory := &models.Memory{
		UserID:     userID,
		Content:    content,
		Category:   models.MemoryCategoryTechnical,
		MemoryType: models.MemoryTypeProcedure,
		Importance: 3,
		Tags:       tags,
	}
	err := s.CreateMemory(memory)
	return memory, err
}

// CreateContextMemory 创建上下文记忆
func (s *MemoryService) CreateContextMemory(userID, content string, tags []string) (*models.Memory, error) {
	memory := &models.Memory{
		UserID:     userID,
		Content:    content,
		Category:   models.MemoryCategoryProject,
		MemoryType: models.MemoryTypeContext,
		Importance: 2,
		Tags:       tags,
	}
	err := s.CreateMemory(memory)
	return memory, err
}

// GetMemoriesByUser 获取用户的记忆
func (s *MemoryService) GetMemoriesByUser(userID string) ([]models.Memory, error) {
	return s.GetMemoriesByUserAndType(userID, "")
}

// GetMemoriesByUserAndType 按用户和类型获取记忆
func (s *MemoryService) GetMemoriesByUserAndType(userID, memoryType string) ([]models.Memory, error) {
	cacheKey := fmt.Sprintf("memories:user:%s:type:%s", userID, memoryType)

	if cached, found := s.Cache.Get(cacheKey); found {
		if mems, ok := cached.([]models.Memory); ok {
			return mems, nil
		}
	}

	memories, err := s.store.GetByUser(context.Background(), userID, memoryType)
	if err != nil {
		return nil, err
	}

	s.Cache.Set(cacheKey, memories, 10*time.Minute)
	return memories, nil
}

// GetMemoriesByType 按类型获取记忆
func (s *MemoryService) GetMemoriesByType(userID, memoryType string) ([]models.Memory, error) {
	return s.GetMemoriesByUserAndType(userID, memoryType)
}

// SearchMemories 搜索记忆（全文搜索）
func (s *MemoryService) SearchMemories(userID string, keyword string) ([]models.Memory, error) {
	cacheKey := fmt.Sprintf("memories:search:%s:%s", userID, keyword)

	if cached, found := s.Cache.Get(cacheKey); found {
		if mems, ok := cached.([]models.Memory); ok {
			return mems, nil
		}
	}

	memories, err := s.store.Search(context.Background(), userID, keyword)
	if err != nil {
		return nil, err
	}

	s.Cache.Set(cacheKey, memories, 5*time.Minute)
	return memories, err
}

// SearchMemoriesByType 按类型搜索记忆
func (s *MemoryService) SearchMemoriesByType(userID, keyword, memoryType string) ([]models.Memory, error) {
	return s.store.SearchByType(context.Background(), userID, keyword, memoryType)
}

// GetRelevantMemories 获取与输入相关的记忆（优先使用语义搜索）
func (s *MemoryService) GetRelevantMemories(userID, content string, limit int) ([]models.Memory, error) {
	if limit <= 0 {
		limit = 5
	}

	// 如果有向量服务，使用语义搜索
	if s.embeddingSvc != nil {
		results, err := s.embeddingSvc.HybridSearch(context.Background(), userID, content, limit, 0.7)
		if err == nil && len(results) > 0 {
			memories := make([]models.Memory, 0, len(results))
			for _, r := range results {
				memories = append(memories, r.Memory)
				s.AccessMemory(r.Memory.ID)
			}
			return memories, nil
		}
	}

	// 回退到传统字符串匹配
	memories, err := s.GetMemoriesByUser(userID)
	if err != nil {
		return nil, err
	}

	contentLower := strings.ToLower(content)
	type scored struct {
		memory models.Memory
		score  float64
	}
	var scoredList []scored

	for _, m := range memories {
		score := 0.0
		contentLowerMem := strings.ToLower(m.Content)

		if strings.Contains(contentLower, contentLowerMem) || strings.Contains(contentLowerMem, contentLower) {
			score += 0.5
		}

		for _, tag := range m.Tags {
			if strings.Contains(contentLower, strings.ToLower(tag)) {
				score += 0.3
			}
		}

		score += float64(m.Importance) * 0.1
		score += float64(m.AccessCount) * 0.01

		if score > 0 {
			scoredList = append(scoredList, scored{memory: m, score: score})
		}
	}

	for i := 0; i < len(scoredList)-1; i++ {
		for j := i + 1; j < len(scoredList); j++ {
			if scoredList[j].score > scoredList[i].score {
				scoredList[i], scoredList[j] = scoredList[j], scoredList[i]
			}
		}
	}

	result := make([]models.Memory, 0, limit)
	for i := 0; i < limit && i < len(scoredList); i++ {
		result = append(result, scoredList[i].memory)
	}

	return result, nil
}

// UpdateMemory 更新记忆
func (s *MemoryService) UpdateMemory(id string, memory *models.Memory) error {
	err := s.store.Update(context.Background(), memory)
	if err != nil {
		return err
	}
	s.invalidateUserCache(memory.UserID)
	return nil
}

// DeleteMemory 删除记忆
func (s *MemoryService) DeleteMemory(id string) error {
	memory, err := s.store.GetByID(context.Background(), id)
	if err != nil {
		return err
	}
	err = s.store.Delete(context.Background(), id)
	if err != nil {
		return err
	}
	s.invalidateUserCache(memory.UserID)
	return nil
}

// AccessMemory 访问记忆（更新最后访问时间和计数）
func (s *MemoryService) AccessMemory(id string) error {
	memory, err := s.store.GetByID(context.Background(), id)
	if err != nil {
		return err
	}
	err = s.store.UpdateAccess(context.Background(), id)
	if err != nil {
		return err
	}
	s.invalidateUserCache(memory.UserID)
	return nil
}

// ==================== 短期记忆（第二层）====================

// CreateShortTermMemory 创建短期记忆（对话摘要）
func (s *MemoryService) CreateShortTermMemory(userID, dialogueID, summary string, messageCount int, ttl time.Duration) (*models.ShortTermMemory, error) {
	mem := &models.ShortTermMemory{
		ID:           uuid.New().String(),
		UserID:       userID,
		Summary:      summary,
		MessageCount: messageCount,
		DialogueID:   dialogueID,
		CreatedAt:    time.Now(),
	}
	if ttl > 0 {
		expires := time.Now().Add(ttl)
		mem.ExpiresAt = &expires
	}

	err := s.store.CreateShortTerm(context.Background(), mem)
	return mem, err
}

// GetShortTermMemories 获取用户的短期记忆
func (s *MemoryService) GetShortTermMemories(userID string) ([]models.ShortTermMemory, error) {
	return s.store.GetShortTermByUser(context.Background(), userID)
}

// GetRecentSummaries 获取最近的对话摘要
func (s *MemoryService) GetRecentSummaries(userID string, limit int) ([]models.ShortTermMemory, error) {
	if limit <= 0 {
		limit = 5
	}
	return s.store.GetRecentSummaries(context.Background(), userID, limit)
}

// CleanupExpiredShortTermMemories 清理过期的短期记忆
func (s *MemoryService) CleanupExpiredShortTermMemories() error {
	return s.store.CleanupExpiredShortTerm(context.Background())
}

// ==================== 记忆衰减与维护 ====================

// AdjustPriority 调整记忆优先级（衰减机制）
func (s *MemoryService) AdjustPriority() error {
	err := s.store.AdjustPriority(context.Background())
	if err != nil {
		return err
	}
	s.Cache.Flush()
	return nil
}

// MergeDuplicateMemories 合并相似记忆
func (s *MemoryService) MergeDuplicateMemories(userID string) (int, error) {
	memories, err := s.store.FindDuplicates(context.Background(), userID)
	if err != nil {
		return 0, err
	}

	merged := 0
	processed := make(map[string]bool)

	for i, m1 := range memories {
		if processed[m1.ID] {
			continue
		}

		for j := i + 1; j < len(memories); j++ {
			m2 := memories[j]
			if processed[m2.ID] || m1.MemoryType != m2.MemoryType {
				continue
			}

			similarity := calculateSimilarity(m1.Content, m2.Content)
			if similarity > 0.8 {
				if m2.CreatedAt.After(m1.CreatedAt) {
					s.store.Delete(context.Background(), m1.ID)
					processed[m1.ID] = true
				} else {
					s.store.Delete(context.Background(), m2.ID)
					processed[m2.ID] = true
				}
				merged++
			}
		}
	}

	if merged > 0 {
		s.invalidateUserCache(userID)
	}

	return merged, nil
}

// calculateSimilarity 计算两个字符串的相似度（简单 Jaccard）
func calculateSimilarity(s1, s2 string) float64 {
	words1 := splitWords(s1)
	words2 := splitWords(s2)

	if len(words1) == 0 && len(words2) == 0 {
		return 1.0
	}
	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}

	intersection := 0
	union := len(set1)
	set2 := make(map[string]bool)
	for _, w := range words2 {
		set2[w] = true
		if set1[w] {
			intersection++
		} else {
			union++
		}
	}

	return float64(intersection) / float64(union)
}

func splitWords(s string) []string {
	s = strings.ToLower(s)
	return strings.Fields(s)
}

// ==================== 组合记忆检索 ====================

// BuildMemoryContext 构建记忆上下文（组合工作记忆、短期记忆和长期记忆）
func (s *MemoryService) BuildMemoryContext(userID, currentContent string, maxTokens int) string {
	var parts []string

	longTermMem, err := s.GetRelevantMemories(userID, currentContent, 5)
	if err == nil && len(longTermMem) > 0 {
		var memParts []string
		for _, m := range longTermMem {
			memParts = append(memParts, fmt.Sprintf("[%s] %s", m.MemoryType, m.Content))
		}
		parts = append(parts, "已知信息：\n"+strings.Join(memParts, "\n"))
	}

	shortTermMem, err := s.GetRecentSummaries(userID, 3)
	if err == nil && len(shortTermMem) > 0 {
		var sumParts []string
		for _, sm := range shortTermMem {
			sumParts = append(sumParts, sm.Summary)
		}
		parts = append(parts, "近期对话摘要：\n"+strings.Join(sumParts, "\n---\n"))
	}

	result := strings.Join(parts, "\n\n")

	if maxTokens > 0 && len(result) > maxTokens*2 {
		result = result[:maxTokens*2] + "..."
	}

	return result
}

// ==================== 内部方法 ====================

func (s *MemoryService) invalidateUserCache(userID string) {
	s.Cache.Delete(fmt.Sprintf("memories:user:%s:type:", userID))
	s.Cache.Delete(fmt.Sprintf("memories:user:%s:type:fact", userID))
	s.Cache.Delete(fmt.Sprintf("memories:user:%s:type:preference", userID))
	s.Cache.Delete(fmt.Sprintf("memories:user:%s:type:procedure", userID))
	s.Cache.Delete(fmt.Sprintf("memories:user:%s:type:context", userID))
	_ = userID
}

func init() {
	log.Println("[MemoryService] three-layer memory architecture initialized")
}
