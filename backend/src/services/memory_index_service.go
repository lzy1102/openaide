package services

import (
	"context"
	"fmt"
	"log"
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
	log.Println("[MemoryIndex] Creating memory indexes...")

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

	// 内容全文搜索索引（SQLite FTS5）
	if err := s.createFTS5Index(); err != nil {
		log.Printf("[MemoryIndex] FTS5 index creation skipped or failed: %v", err)
	}

	// 短期记忆索引
	if err := s.createIndexIfNotExists("idx_short_term_user_id", "short_term_memories", "user_id"); err != nil {
		return err
	}

	if err := s.createIndexIfNotExists("idx_short_term_expires", "short_term_memories", "expires_at"); err != nil {
		return err
	}

	log.Println("[MemoryIndex] Memory indexes created successfully")
	return nil
}

// createIndexIfNotExists 如果不存在则创建索引
func (s *MemoryIndexService) createIndexIfNotExists(indexName, tableName, columns string) error {
	// 检查索引是否存在
	var count int64
	s.db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?",
		indexName,
	).Scan(&count)

	if count > 0 {
		log.Printf("[MemoryIndex] Index %s already exists", indexName)
		return nil
	}

	// 创建索引
	sql := fmt.Sprintf("CREATE INDEX %s ON %s(%s)", indexName, tableName, columns)
	if err := s.db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to create index %s: %w", indexName, err)
	}

	log.Printf("[MemoryIndex] Created index: %s", indexName)
	return nil
}

// createFTS5Index 创建全文搜索索引
func (s *MemoryIndexService) createFTS5Index() error {
	// 检查 FTS5 表是否存在
	var count int64
	s.db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='memories_fts'",
	).Scan(&count)

	if count > 0 {
		log.Println("[MemoryIndex] FTS5 index already exists")
		return nil
	}

	// 创建 FTS5 虚拟表
	sql := `
		CREATE VIRTUAL TABLE memories_fts USING fts5(
			content,
			content='memories',
			content_rowid='id'
		)
	`
	if err := s.db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to create FTS5 table: %w", err)
	}

	// 创建触发器保持同步
	triggers := []string{
		`CREATE TRIGGER memories_fts_insert AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
		END`,
		`CREATE TRIGGER memories_fts_delete AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content) VALUES ('delete', old.id, old.content);
		END`,
		`CREATE TRIGGER memories_fts_update AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content) VALUES ('delete', old.id, old.content);
			INSERT INTO memories_fts(rowid, content) VALUES (new.id, new.content);
		END`,
	}

	for _, trigger := range triggers {
		if err := s.db.Exec(trigger).Error; err != nil {
			log.Printf("[MemoryIndex] Failed to create trigger: %v", err)
		}
	}

	// 填充现有数据
	if err := s.db.Exec("INSERT INTO memories_fts(rowid, content) SELECT id, content FROM memories").Error; err != nil {
		log.Printf("[MemoryIndex] Failed to populate FTS5: %v", err)
	}

	log.Println("[MemoryIndex] FTS5 index created successfully")
	return nil
}

// FullTextSearch 全文搜索
func (s *MemoryIndexService) FullTextSearch(userID, query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}

	// 使用 FTS5 进行全文搜索
	var memoryIDs []string
	sql := `
		SELECT m.id FROM memories m
		JOIN memories_fts fts ON m.id = fts.rowid
		WHERE m.user_id = ? AND memories_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`

	result := s.db.Raw(sql, userID, query, limit).Scan(&memoryIDs)
	if result.Error != nil {
		// FTS5 可能不可用，回退到普通搜索
		return s.fallbackSearch(userID, query, limit)
	}

	return memoryIDs, nil
}

// fallbackSearch 回退搜索
func (s *MemoryIndexService) fallbackSearch(userID, query string, limit int) ([]string, error) {
	var memoryIDs []string
	sql := `
		SELECT id FROM memories
		WHERE user_id = ? AND content LIKE ?
		ORDER BY importance DESC, last_accessed DESC
		LIMIT ?
	`

	result := s.db.Raw(sql, userID, "%"+query+"%", limit).Scan(&memoryIDs)
	return memoryIDs, result.Error
}

// OptimizeIndexes 优化索引
func (s *MemoryIndexService) OptimizeIndexes() error {
	// 运行 VACUUM 优化数据库
	if err := s.db.Exec("VACUUM").Error; err != nil {
		log.Printf("[MemoryIndex] VACUUM failed: %v", err)
	}

	// 优化 FTS5 索引
	if err := s.db.Exec("INSERT INTO memories_fts(memories_fts) VALUES('optimize')").Error; err != nil {
		log.Printf("[MemoryIndex] FTS5 optimize failed: %v", err)
	}

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
			log.Println("[MemoryIndex] Running scheduled maintenance...")
			if err := s.OptimizeIndexes(); err != nil {
				log.Printf("[MemoryIndex] Maintenance failed: %v", err)
			}
		}
	}
}
