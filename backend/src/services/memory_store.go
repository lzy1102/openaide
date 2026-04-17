package services

import (
	"context"
	"time"

	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// MemoryStore 记忆存储接口
// 可插拔：替换 GORM 实现为 Redis/向量库等后端，无需修改 MemoryService
type MemoryStore interface {
	Create(ctx context.Context, memory *models.Memory) error
	GetByUser(ctx context.Context, userID, memoryType string) ([]models.Memory, error)
	Search(ctx context.Context, userID, keyword string) ([]models.Memory, error)
	SearchByType(ctx context.Context, userID, keyword, memoryType string) ([]models.Memory, error)
	Update(ctx context.Context, memory *models.Memory) error
	GetByID(ctx context.Context, id string) (*models.Memory, error)
	Delete(ctx context.Context, id string) error
	UpdateAccess(ctx context.Context, id string) error
	CreateShortTerm(ctx context.Context, stm *models.ShortTermMemory) error
	GetShortTermByUser(ctx context.Context, userID string) ([]models.ShortTermMemory, error)
	GetRecentSummaries(ctx context.Context, userID string, limit int) ([]models.ShortTermMemory, error)
	CleanupExpiredShortTerm(ctx context.Context) error
	AdjustPriority(ctx context.Context) error
	FindDuplicates(ctx context.Context, userID string) ([]models.Memory, error)
	DeleteByIDs(ctx context.Context, ids []string) error
}

// GormMemoryStore 基于 GORM 的记忆存储实现
type GormMemoryStore struct {
	db *gorm.DB
}

// NewGormMemoryStore 创建 GORM 记忆存储
func NewGormMemoryStore(db *gorm.DB) *GormMemoryStore {
	return &GormMemoryStore{db: db}
}

func (s *GormMemoryStore) Create(_ context.Context, memory *models.Memory) error {
	memory.CreatedAt = time.Now()
	memory.UpdatedAt = time.Now()
	memory.LastAccessed = time.Now()
	return s.db.Create(memory).Error
}

func (s *GormMemoryStore) GetByUser(_ context.Context, userID, memoryType string) ([]models.Memory, error) {
	query := s.db.Where("user_id = ?", userID)
	if memoryType != "" {
		query = query.Where("memory_type = ?", memoryType)
	}
	var memories []models.Memory
	err := query.Order("importance DESC, last_accessed DESC").Find(&memories).Error
	return memories, err
}

func (s *GormMemoryStore) Search(_ context.Context, userID, keyword string) ([]models.Memory, error) {
	var memories []models.Memory
	err := s.db.Where("user_id = ? AND content LIKE ?", userID, "%"+keyword+"%").
		Order("importance DESC, last_accessed DESC").Find(&memories).Error
	return memories, err
}

func (s *GormMemoryStore) SearchByType(_ context.Context, userID, keyword, memoryType string) ([]models.Memory, error) {
	var memories []models.Memory
	query := s.db.Where("user_id = ? AND content LIKE ?", userID, "%"+keyword+"%")
	if memoryType != "" {
		query = query.Where("memory_type = ?", memoryType)
	}
	err := query.Order("importance DESC, last_accessed DESC").Find(&memories).Error
	return memories, err
}

func (s *GormMemoryStore) Update(_ context.Context, memory *models.Memory) error {
	memory.UpdatedAt = time.Now()
	return s.db.Save(memory).Error
}

func (s *GormMemoryStore) GetByID(_ context.Context, id string) (*models.Memory, error) {
	var memory models.Memory
	err := s.db.First(&memory, id).Error
	return &memory, err
}

func (s *GormMemoryStore) Delete(_ context.Context, id string) error {
	return s.db.Where("id = ?", id).Delete(&models.Memory{}).Error
}

func (s *GormMemoryStore) UpdateAccess(_ context.Context, id string) error {
	return s.db.Model(&models.Memory{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_accessed": time.Now(),
		"access_count":  gorm.Expr("access_count + 1"),
	}).Error
}

func (s *GormMemoryStore) CreateShortTerm(_ context.Context, stm *models.ShortTermMemory) error {
	return s.db.Create(stm).Error
}

func (s *GormMemoryStore) GetShortTermByUser(_ context.Context, userID string) ([]models.ShortTermMemory, error) {
	var memories []models.ShortTermMemory
	err := s.db.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now()).
		Order("created_at DESC").
		Limit(20).
		Find(&memories).Error
	return memories, err
}

func (s *GormMemoryStore) GetRecentSummaries(_ context.Context, userID string, limit int) ([]models.ShortTermMemory, error) {
	var memories []models.ShortTermMemory
	err := s.db.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now()).
		Order("created_at DESC").
		Limit(limit).
		Find(&memories).Error
	return memories, err
}

func (s *GormMemoryStore) CleanupExpiredShortTerm(_ context.Context) error {
	return s.db.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Delete(&models.ShortTermMemory{}).Error
}

func (s *GormMemoryStore) AdjustPriority(_ context.Context) error {
	if err := s.db.Exec(`UPDATE memories SET importance = MIN(5, importance + 1) WHERE last_accessed > datetime('now', '-7 days')`).Error; err != nil {
		return err
	}
	return s.db.Exec(`UPDATE memories SET importance = MAX(1, importance - 1) WHERE last_accessed < datetime('now', '-30 days')`).Error
}

func (s *GormMemoryStore) FindDuplicates(_ context.Context, userID string) ([]models.Memory, error) {
	var memories []models.Memory
	err := s.db.Where("user_id = ?", userID).
		Where("memory_type IN ?", []string{models.MemoryTypeFact, models.MemoryTypePreference}).
		Find(&memories).Error
	return memories, err
}

func (s *GormMemoryStore) DeleteByIDs(_ context.Context, ids []string) error {
	return s.db.Where("id IN ?", ids).Delete(&models.Memory{}).Error
}
