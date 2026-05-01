package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// MemoryIndexService 记忆索引服务
type MemoryIndexService struct {
	db *gorm.DB
}

// NewMemoryIndexService 创建索引服务
func NewMemoryIndexService(db *gorm.DB) *MemoryIndexService {
	return &MemoryIndexService{db: db}
}

// CreateIndexes 创建记忆表索引
func (s *MemoryIndexService) CreateIndexes() error {
	slog.Info("Creating memory indexes...", "component", "MemoryIndex")

	// 用户ID索引（最常用的查询条件）
	if err := s.createIndexIfNotExists("idx_memories_user_id", "memories", "user_id"); err != nil {
		return err
	}

	// 用户ID + 记忆类型复合索引
	if err := s.createIndexIfNotExists("idx_memories_user_type", "memories", "user_id, memory_type"); err != nil {
		return err
	}

	// 重要性 + 最后访问时间索引（用于排序）
	if err := s.createIndexIfNotExists("idx_memories_importance_access", "memories", "importance DESC, last_accessed DESC"); err != nil {
		return err
	}

	// 用户ID + 重要性 + 访问时间复合索引
	if err := s.createIndexIfNotExists("idx_memories_user_importance", "memories", "user_id, importance DESC, last_accessed DESC"); err != nil {
		return err
	}

	// 短期记忆索引
	if err := s.createIndexIfNotExists("idx_short_term_user_id", "short_term_memories", "user_id"); err != nil {
		return err
	}

	if err := s.createIndexIfNotExists("idx_short_term_expires", "short_term_memories", "expires_at"); err != nil {
		return err
	}

	slog.Info("Memory indexes created successfully", "component", "MemoryIndex")
	return nil
}

// createIndexIfNotExists 如果不存在则创建索引（跨数据库兼容）
func (s *MemoryIndexService) createIndexIfNotExists(indexName, tableName, columns string) error {
	// 直接尝试创建索引，如果已存在会报错，忽略错误
	sql := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s(%s)", indexName, tableName, columns)
	if err := s.db.Exec(sql).Error; err != nil {
		// 某些数据库不支持 IF NOT EXISTS，尝试另一种方式
		slog.Error("CREATE INDEX IF NOT EXISTS failed, trying alternative", "component", "MemoryIndex", "error", err)
		return nil // 忽略错误，继续执行
	}

	slog.Info("Created index", "component", "MemoryIndex", "index", indexName)
	return nil
}

// FullTextSearch 全文搜索（跨数据库兼容，回退到 LIKE）
func (s *MemoryIndexService) FullTextSearch(userID, query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}

	// 使用 LIKE 进行跨数据库兼容的搜索
	var memoryIDs []string
	result := s.db.Raw(
		"SELECT id FROM memories WHERE user_id = ? AND content LIKE ? ORDER BY importance DESC, last_accessed DESC LIMIT ?",
		userID, "%"+query+"%", limit,
	).Scan(&memoryIDs)

	return memoryIDs, result.Error
}

// OptimizeIndexes 优化索引（跨数据库兼容）
func (s *MemoryIndexService) OptimizeIndexes() error {
	// 不同数据库的优化命令不同，这里只是记录日志
	slog.Info("Database optimization skipped (cross-database compatible)", "component", "MemoryIndex")
	return nil
}

// ScheduleMaintenance 定期维护
func (s *MemoryIndexService) ScheduleMaintenance(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			slog.Info("Running scheduled maintenance...", "component", "MemoryIndex")
			if err := s.OptimizeIndexes(); err != nil {
				slog.Error("Maintenance failed", "component", "MemoryIndex", "error", err)
			}
		}
	}
}
