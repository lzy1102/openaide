package services

import (
	"context"
	"sync"
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

// MemoryProvider 记忆提供者插件接口 (扩展自 MemoryStore, 支持插件化注册)
type MemoryProvider interface {
	MemoryStore

	Initialize(config map[string]interface{}) error
	Name() string
	SemanticSearch(ctx context.Context, userID, query string, limit int) ([]models.Memory, error)
	BatchUpsert(ctx context.Context, memories []models.Memory) error
	Close() error
}

// MemoryProviderRegistry 记忆提供者注册表
type MemoryProviderRegistry struct {
	providers map[string]func() MemoryProvider
	active    MemoryProvider
	mu        sync.RWMutex
}

// NewMemoryProviderRegistry 创建注册表
func NewMemoryProviderRegistry() *MemoryProviderRegistry {
	return &MemoryProviderRegistry{
		providers: make(map[string]func() MemoryProvider),
	}
}

// Register 注册提供者工厂
func (r *MemoryProviderRegistry) Register(name string, factory func() MemoryProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = factory
}

// GetProvider 获取提供者实例
func (r *MemoryProviderRegistry) GetProvider(name string) (MemoryProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, exists := r.providers[name]
	if !exists {
		return nil, &ModelNotFoundError{Model: name}
	}
	return factory(), nil
}

// SetActiveProvider 设置当前活跃的提供者
func (r *MemoryProviderRegistry) SetActiveProvider(provider MemoryProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active != nil {
		r.active.Close()
	}
	r.active = provider
}

// GetActiveProvider 获取当前活跃的提供者
func (r *MemoryProviderRegistry) GetActiveProvider() MemoryProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

type ModelNotFoundError struct {
	Model string
}

func (e *ModelNotFoundError) Error() string {
	return "provider not found: " + e.Model
}

// GormMemoryProvider 基于 GORM 的记忆提供者实现
type GormMemoryProvider struct {
	db *gorm.DB
}

// NewGormMemoryProvider 创建 GORM 提供者
func NewGormMemoryProvider(db *gorm.DB) *GormMemoryProvider {
	return &GormMemoryProvider{db: db}
}

func (p *GormMemoryProvider) Name() string {
	return "gorm"
}

func (p *GormMemoryProvider) Initialize(config map[string]interface{}) error {
	return nil
}

func (p *GormMemoryProvider) Close() error {
	return nil
}

func (p *GormMemoryProvider) SemanticSearch(_ context.Context, userID, query string, limit int) ([]models.Memory, error) {
	var memories []models.Memory
	err := p.db.Where("user_id = ? AND (content LIKE ? OR tags LIKE ?)", userID, "%"+query+"%", "%"+query+"%").
		Order("importance DESC, last_accessed DESC").
		Limit(limit).
		Find(&memories).Error
	return memories, err
}

func (p *GormMemoryProvider) BatchUpsert(_ context.Context, memories []models.Memory) error {
	for _, mem := range memories {
		if mem.ID == "" {
			if err := p.db.Create(&mem).Error; err != nil {
				return err
			}
		} else {
			if err := p.db.Save(&mem).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *GormMemoryProvider) Create(_ context.Context, memory *models.Memory) error {
	memory.CreatedAt = time.Now()
	memory.UpdatedAt = time.Now()
	memory.LastAccessed = time.Now()
	return p.db.Create(memory).Error
}

func (p *GormMemoryProvider) GetByUser(_ context.Context, userID, memoryType string) ([]models.Memory, error) {
	query := p.db.Where("user_id = ?", userID)
	if memoryType != "" {
		query = query.Where("memory_type = ?", memoryType)
	}
	var memories []models.Memory
	err := query.Order("importance DESC, last_accessed DESC").Find(&memories).Error
	return memories, err
}

func (p *GormMemoryProvider) Search(_ context.Context, userID, keyword string) ([]models.Memory, error) {
	var memories []models.Memory
	err := p.db.Where("user_id = ? AND content LIKE ?", userID, "%"+keyword+"%").
		Order("importance DESC, last_accessed DESC").Find(&memories).Error
	return memories, err
}

func (p *GormMemoryProvider) SearchByType(_ context.Context, userID, keyword, memoryType string) ([]models.Memory, error) {
	var memories []models.Memory
	query := p.db.Where("user_id = ? AND content LIKE ?", userID, "%"+keyword+"%")
	if memoryType != "" {
		query = query.Where("memory_type = ?", memoryType)
	}
	err := query.Order("importance DESC, last_accessed DESC").Find(&memories).Error
	return memories, err
}

func (p *GormMemoryProvider) Update(_ context.Context, memory *models.Memory) error {
	memory.UpdatedAt = time.Now()
	return p.db.Save(memory).Error
}

func (p *GormMemoryProvider) GetByID(_ context.Context, id string) (*models.Memory, error) {
	var memory models.Memory
	err := p.db.First(&memory, id).Error
	return &memory, err
}

func (p *GormMemoryProvider) Delete(_ context.Context, id string) error {
	return p.db.Where("id = ?", id).Delete(&models.Memory{}).Error
}

func (p *GormMemoryProvider) UpdateAccess(_ context.Context, id string) error {
	return p.db.Model(&models.Memory{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_accessed": time.Now(),
		"access_count":  gorm.Expr("access_count + 1"),
	}).Error
}

func (p *GormMemoryProvider) CreateShortTerm(_ context.Context, stm *models.ShortTermMemory) error {
	return p.db.Create(stm).Error
}

func (p *GormMemoryProvider) GetShortTermByUser(_ context.Context, userID string) ([]models.ShortTermMemory, error) {
	var memories []models.ShortTermMemory
	err := p.db.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now()).
		Order("created_at DESC").
		Limit(20).
		Find(&memories).Error
	return memories, err
}

func (p *GormMemoryProvider) GetRecentSummaries(_ context.Context, userID string, limit int) ([]models.ShortTermMemory, error) {
	var memories []models.ShortTermMemory
	err := p.db.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now()).
		Order("created_at DESC").
		Limit(limit).
		Find(&memories).Error
	return memories, err
}

func (p *GormMemoryProvider) CleanupExpiredShortTerm(_ context.Context) error {
	return p.db.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Delete(&models.ShortTermMemory{}).Error
}

func (p *GormMemoryProvider) AdjustPriority(_ context.Context) error {
	if err := p.db.Exec(`UPDATE memories SET importance = MIN(5, importance + 1) WHERE last_accessed > datetime('now', '-7 days')`).Error; err != nil {
		return err
	}
	return p.db.Exec(`UPDATE memories SET importance = MAX(1, importance - 1) WHERE last_accessed < datetime('now', '-30 days')`).Error
}

func (p *GormMemoryProvider) FindDuplicates(_ context.Context, userID string) ([]models.Memory, error) {
	var memories []models.Memory
	err := p.db.Where("user_id = ?", userID).
		Where("memory_type IN ?", []string{models.MemoryTypeFact, models.MemoryTypePreference}).
		Find(&memories).Error
	return memories, err
}

func (p *GormMemoryProvider) DeleteByIDs(_ context.Context, ids []string) error {
	return p.db.Where("id IN ?", ids).Delete(&models.Memory{}).Error
}

// 保留原有 GormMemoryStore 以兼容旧代码
var _ MemoryStore = (*GormMemoryProvider)(nil)

// GormMemoryStore 为兼容旧代码,已重定向至 GormMemoryProvider
type GormMemoryStore = GormMemoryProvider

// NewGormMemoryStore 创建 GORM 记忆存储 (兼容旧接口)
func NewGormMemoryStore(db *gorm.DB) *GormMemoryStore {
	return NewGormMemoryProvider(db)
}
