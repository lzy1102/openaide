package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

type PersistentMemoryCategory string

const (
	MemoryCatPreference   PersistentMemoryCategory = "preference"
	MemoryCatFact         PersistentMemoryCategory = "fact"
	MemoryCatPattern      PersistentMemoryCategory = "pattern"
	MemoryCatCorrection   PersistentMemoryCategory = "correction"
	MemoryCatContext      PersistentMemoryCategory = "context"
	MemoryCatWorkflow     PersistentMemoryCategory = "workflow"
)

type PersistentMemory struct {
	ID        string                  `json:"id" gorm:"primaryKey"`
	UserID    string                  `json:"user_id" gorm:"index"`
	Category  PersistentMemoryCategory `json:"category" gorm:"index"`
	Key       string                  `json:"key" gorm:"index"`
	Value     string                  `json:"value" gorm:"type:text"`
	Source    string                  `json:"source"`
	Confidence float64               `json:"confidence"`
	AccessCount int                   `json:"access_count"`
	LastAccessed time.Time            `json:"last_accessed"`
	CreatedAt time.Time               `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time               `json:"updated_at" gorm:"autoUpdateTime"`
	ExpiresAt *time.Time              `json:"expires_at,omitempty" gorm:"index"`
	Tags      string                  `json:"tags,omitempty" gorm:"type:varchar(500)"`
}

func (PersistentMemory) TableName() string {
	return "persistent_memories"
}

type PersistentMemoryService struct {
	BaseService
	mu       sync.RWMutex
	fileDir  string
	eventBus *EventBus
}

func NewPersistentMemoryService(db *gorm.DB, cache *CacheService, eventBus *EventBus) *PersistentMemoryService {
	s := &PersistentMemoryService{
		BaseService: BaseService{DB: db, Cache: cache},
		eventBus:    eventBus,
		fileDir:     ".openaide/memories",
	}
	s.ensureDir()
	s.autoMigrate()
	return s
}

func (s *PersistentMemoryService) ensureDir() {
	dirs := []string{s.fileDir}
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		dirs = append(dirs, filepath.Join(homeDir, ".openaide", "memories"))
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}
}

func (s *PersistentMemoryService) autoMigrate() {
	if s.DB != nil {
		s.DB.AutoMigrate(&PersistentMemory{})
	}
}

func (s *PersistentMemoryService) Remember(ctx context.Context, userID string, category PersistentMemoryCategory, key, value, source string) (*PersistentMemory, error) {
	mem := &PersistentMemory{
		ID:         uuid.New().String(),
		UserID:     userID,
		Category:   category,
		Key:        key,
		Value:      value,
		Source:     source,
		Confidence: 0.8,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	var existing PersistentMemory
	err := s.DB.Where("user_id = ? AND category = ? AND key = ?", userID, category, key).First(&existing).Error
	if err == nil {
		existing.Value = value
		existing.Source = source
		existing.Confidence = existing.Confidence + 0.05
		if existing.Confidence > 1.0 {
			existing.Confidence = 1.0
		}
		existing.UpdatedAt = time.Now()
		s.DB.Save(&existing)
		mem = &existing
	} else {
		if err := s.DB.Create(mem).Error; err != nil {
			return nil, fmt.Errorf("failed to save memory: %w", err)
		}
	}

	s.saveToFile(mem)

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicMemory, "memory_created", "persistent_memory", map[string]interface{}{
			"memory_id": mem.ID,
			"category":  string(category),
			"key":       key,
		})
	}

	slog.Info("Memory saved", "component", "PersistentMemory", "category", string(category), "key", key)
	return mem, nil
}

func (s *PersistentMemoryService) Recall(ctx context.Context, userID string, category PersistentMemoryCategory, key string) (*PersistentMemory, error) {
	var mem PersistentMemory
	err := s.DB.Where("user_id = ? AND category = ? AND key = ?", userID, category, key).First(&mem).Error
	if err != nil {
		return nil, fmt.Errorf("memory not found: %s/%s", category, key)
	}

	s.DB.Model(&mem).Updates(map[string]interface{}{
		"access_count":   gorm.Expr("access_count + 1"),
		"last_accessed":  time.Now(),
	})

	return &mem, nil
}

func (s *PersistentMemoryService) RecallByCategory(ctx context.Context, userID string, category PersistentMemoryCategory) ([]PersistentMemory, error) {
	var memories []PersistentMemory
	err := s.DB.Where("user_id = ? AND category = ?", userID, category).
		Order("confidence DESC, updated_at DESC").
		Find(&memories).Error
	return memories, err
}

func (s *PersistentMemoryService) RecallAll(ctx context.Context, userID string) ([]PersistentMemory, error) {
	var memories []PersistentMemory
	err := s.DB.Where("user_id = ?", userID).
		Order("category, confidence DESC, updated_at DESC").
		Find(&memories).Error
	return memories, err
}

func (s *PersistentMemoryService) Search(ctx context.Context, userID, query string, limit int) ([]PersistentMemory, error) {
	var memories []PersistentMemory
	q := s.DB.Where("user_id = ? AND (key LIKE ? OR value LIKE ?)", userID, "%"+query+"%", "%"+query+"%")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Order("confidence DESC, access_count DESC").Find(&memories).Error
	return memories, err
}

func (s *PersistentMemoryService) Forget(ctx context.Context, userID, memoryID string) error {
	result := s.DB.Where("user_id = ? AND id = ?", userID, memoryID).Delete(&PersistentMemory{})
	if result.RowsAffected == 0 {
		return fmt.Errorf("memory not found: %s", memoryID)
	}
	return nil
}

func (s *PersistentMemoryService) BuildContextForUser(ctx context.Context, userID string) string {
	memories, err := s.RecallAll(ctx, userID)
	if err != nil || len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[User Memories]\n")

	categories := map[PersistentMemoryCategory][]PersistentMemory{}
	for _, m := range memories {
		categories[m.Category] = append(categories[m.Category], m)
	}

	for cat, mems := range categories {
		sb.WriteString(fmt.Sprintf("\n## %s\n", cat))
		for _, m := range mems {
			if m.ExpiresAt != nil && m.ExpiresAt.Before(time.Now()) {
				continue
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", m.Key, m.Value))
		}
	}

	return sb.String()
}

func (s *PersistentMemoryService) ExtractAndRemember(ctx context.Context, userID, dialogueID, userMsg, assistantMsg string) int {
	count := 0

	corrections := s.extractCorrections(userMsg, assistantMsg)
	for _, c := range corrections {
		s.Remember(ctx, userID, MemoryCatCorrection, c.Key, c.Value, "dialogue:"+dialogueID)
		count++
	}

	preferences := s.extractPreferences(userMsg)
	for _, p := range preferences {
		s.Remember(ctx, userID, MemoryCatPreference, p.Key, p.Value, "dialogue:"+dialogueID)
		count++
	}

	facts := s.extractFacts(assistantMsg)
	for _, f := range facts {
		s.Remember(ctx, userID, MemoryCatFact, f.Key, f.Value, "dialogue:"+dialogueID)
		count++
	}

	return count
}

type memoryExtraction struct {
	Key   string
	Value string
}

func (s *PersistentMemoryService) extractCorrections(userMsg, assistantMsg string) []memoryExtraction {
	var results []memoryExtraction
	lower := strings.ToLower(userMsg)
	correctionPatterns := []string{
		"don't ", "do not ", "never ", "always ", "i prefer ", "i like ",
		"i want ", "please use ", "use ", "instead of ", "no, ",
		"that's wrong", "incorrect", "not like that",
	}
	for _, pat := range correctionPatterns {
		if strings.Contains(lower, pat) {
			idx := strings.Index(lower, pat)
			value := strings.TrimSpace(userMsg[idx:])
			if len(value) > 200 {
				value = value[:200]
			}
			results = append(results, memoryExtraction{
				Key:   fmt.Sprintf("correction_%d", len(results)),
				Value: value,
			})
		}
	}
	return results
}

func (s *PersistentMemoryService) extractPreferences(userMsg string) []memoryExtraction {
	var results []memoryExtraction
	lower := strings.ToLower(userMsg)
	prefPatterns := map[string]string{
		"i prefer":      "preference",
		"i like":        "preference",
		"i always":      "habit",
		"i never":       "habit",
		"my style is":   "style",
		"my convention": "convention",
		"we use ":       "convention",
		"our standard":  "standard",
	}
	for pat, cat := range prefPatterns {
		if strings.Contains(lower, pat) {
			idx := strings.Index(lower, pat)
			value := strings.TrimSpace(userMsg[idx:])
			if len(value) > 200 {
				value = value[:200]
			}
			results = append(results, memoryExtraction{
				Key:   fmt.Sprintf("%s_%d", cat, len(results)),
				Value: value,
			})
		}
	}
	return results
}

func (s *PersistentMemoryService) extractFacts(assistantMsg string) []memoryExtraction {
	var results []memoryExtraction
	return results
}

func (s *PersistentMemoryService) saveToFile(mem *PersistentMemory) {
	dir := s.fileDir
	filename := filepath.Join(dir, fmt.Sprintf("%s.json", mem.Category))
	data, err := os.ReadFile(filename)
	existing := make(map[string]PersistentMemory)
	if err == nil {
		json.Unmarshal(data, &existing)
	}
	existing[mem.Key] = *mem
	if newData, err := json.MarshalIndent(existing, "", "  "); err == nil {
		os.WriteFile(filename, newData, 0644)
	}
}

func (s *PersistentMemoryService) ExportMemories(ctx context.Context, userID string) (string, error) {
	memories, err := s.RecallAll(ctx, userID)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(memories, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *PersistentMemoryService) ImportMemories(ctx context.Context, userID string, jsonData string) (int, error) {
	var memories []PersistentMemory
	if err := json.Unmarshal([]byte(jsonData), &memories); err != nil {
		return 0, fmt.Errorf("invalid JSON: %w", err)
	}
	count := 0
	for _, m := range memories {
		m.UserID = userID
		m.ID = uuid.New().String()
		m.CreatedAt = time.Now()
		m.UpdatedAt = time.Now()
		if err := s.DB.Create(&m).Error; err == nil {
			count++
		}
	}
	return count, nil
}

func (s *PersistentMemoryService) Cleanup(ctx context.Context, userID string) int {
	result := s.DB.Where("user_id = ? AND expires_at IS NOT NULL AND expires_at < ?", userID, time.Now()).Delete(&PersistentMemory{})
	return int(result.RowsAffected)
}
