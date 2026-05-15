package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
)

// Level 记忆级别
type Level int

const (
	LevelWorking  Level = iota // L1: 工作记忆（当前会话上下文）
	LevelShortTerm             // L2: 短期记忆（最近会话摘要）
	LevelLongTerm              // L3: 长期记忆（重要信息持久化）
)

func (l Level) String() string {
	switch l {
	case LevelWorking:
		return "working"
	case LevelShortTerm:
		return "short_term"
	case LevelLongTerm:
		return "long_term"
	default:
		return "unknown"
	}
}

// MemoryItem 记忆条目
type MemoryItem struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Level     Level     `json:"level"`
	Content   string    `json:"content"`
	Type      string    `json:"type"` // fact, preference, pattern, summary
	Tags      []string  `json:"tags,omitempty"`
	Importance float64  `json:"importance"` // 0.0 - 1.0
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AccessCount int     `json:"access_count"`
	LastAccessed time.Time `json:"last_accessed"`
}

// Manager 记忆管理器
type Manager struct {
	dataDir string
	items   map[string]*MemoryItem // 内存缓存
	mu      sync.RWMutex
}

// NewManager 创建记忆管理器
func NewManager(dataDir string) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create memory dir: %w", err)
	}

	m := &Manager{
		dataDir: dataDir,
		items:   make(map[string]*MemoryItem),
	}

	// 加载已有记忆
	if err := m.loadAll(); err != nil {
		return nil, err
	}

	return m, nil
}

// Save 保存记忆
func (m *Manager) Save(ctx context.Context, sessionID string, level Level, content, itemType string, tags []string, importance float64) (*MemoryItem, error) {
	item := &MemoryItem{
		ID:           fmt.Sprintf("%s_%d", sessionID, time.Now().UnixNano()),
		SessionID:    sessionID,
		Level:        level,
		Content:      content,
		Type:         itemType,
		Tags:         tags,
		Importance:   importance,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		AccessCount:  0,
		LastAccessed: time.Now(),
	}

	m.mu.Lock()
	m.items[item.ID] = item
	m.mu.Unlock()

	// 持久化
	if err := m.saveItem(item); err != nil {
		return nil, err
	}

	return item, nil
}

// Get 获取记忆
func (m *Manager) Get(ctx context.Context, itemID string) (*MemoryItem, error) {
	m.mu.RLock()
	item, ok := m.items[itemID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("memory item not found: %s", itemID)
	}

	// 更新访问统计
	m.mu.Lock()
	item.AccessCount++
	item.LastAccessed = time.Now()
	m.mu.Unlock()

	return item, nil
}

// Search 搜索记忆
func (m *Manager) Search(ctx context.Context, query string, level Level, limit int) ([]*MemoryItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*MemoryItem
	queryLower := strings.ToLower(query)

	for _, item := range m.items {
		if level >= 0 && item.Level != level {
			continue
		}

		// 简单文本匹配（后续可替换为向量搜索）
		contentLower := strings.ToLower(item.Content)
		if strings.Contains(contentLower, queryLower) {
			results = append(results, item)
			continue
		}

		// 标签匹配
		for _, tag := range item.Tags {
			if strings.Contains(strings.ToLower(tag), queryLower) {
				results = append(results, item)
				break
			}
		}
	}

	// 按重要性排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Importance > results[j].Importance
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetBySession 获取会话的所有记忆
func (m *Manager) GetBySession(ctx context.Context, sessionID string) ([]*MemoryItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*MemoryItem
	for _, item := range m.items {
		if item.SessionID == sessionID {
			results = append(results, item)
		}
	}

	// 按时间排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.Before(results[j].CreatedAt)
	})

	return results, nil
}

// Delete 删除记忆
func (m *Manager) Delete(ctx context.Context, itemID string) error {
	m.mu.Lock()
	delete(m.items, itemID)
	m.mu.Unlock()

	// 删除文件
	path := m.itemPath(itemID)
	return os.Remove(path)
}

// Compress 压缩会话记忆（将工作记忆提升为短期记忆）
func (m *Manager) Compress(ctx context.Context, sessionID string) error {
	items, err := m.GetBySession(ctx, sessionID)
	if err != nil {
		return err
	}

	// 收集工作记忆
	var workingItems []*MemoryItem
	for _, item := range items {
		if item.Level == LevelWorking {
			workingItems = append(workingItems, item)
		}
	}

	if len(workingItems) == 0 {
		return nil
	}

	// 生成摘要（简单拼接，后续可用 LLM 生成）
	var contents []string
	for _, item := range workingItems {
		contents = append(contents, item.Content)
	}
	summary := strings.Join(contents, "\n")
	if len(summary) > 1000 {
		summary = summary[:1000] + "..."
	}

	// 保存为短期记忆
	_, err = m.Save(ctx, sessionID, LevelShortTerm, summary, "summary", []string{"compressed"}, 0.7)
	return err
}

// ============ 内部方法 ============

func (m *Manager) itemPath(itemID string) string {
	// 按层级分目录存储
	return filepath.Join(m.dataDir, itemID+".json")
}

func (m *Manager) saveItem(item *MemoryItem) error {
	path := m.itemPath(item.ID)
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (m *Manager) loadAll() error {
	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(m.dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var item MemoryItem
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}

		m.items[item.ID] = &item
	}

	return nil
}

// FileMemory 文件存储的记忆实现（符合 kernel.Memory 接口）
type FileMemory struct {
	manager *Manager
}

// NewFileMemory 创建文件记忆
func NewFileMemory(dataDir string) (*FileMemory, error) {
	manager, err := NewManager(dataDir)
	if err != nil {
		return nil, err
	}
	return &FileMemory{manager: manager}, nil
}

// Save 保存消息到记忆
func (f *FileMemory) Save(ctx context.Context, sessionID string, messages []kernel.Message) error {
	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}

		itemType := "message"
		if msg.Role == "assistant" {
			itemType = "response"
		}

		_, err := f.manager.Save(ctx, sessionID, LevelWorking, msg.Content, itemType, nil, 0.5)
		if err != nil {
			return err
		}
	}
	return nil
}

// Load 加载记忆
func (f *FileMemory) Load(ctx context.Context, sessionID string, limit int) ([]kernel.Message, error) {
	items, err := f.manager.GetBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// 只返回工作记忆
	var messages []kernel.Message
	count := 0
	for i := len(items) - 1; i >= 0 && count < limit; i-- {
		if items[i].Level != LevelWorking {
			continue
		}

		role := "user"
		if items[i].Type == "response" {
			role = "assistant"
		}

		messages = append([]kernel.Message{{
			Role:    role,
			Content: items[i].Content,
		}}, messages...)
		count++
	}

	return messages, nil
}

// Search 搜索记忆
func (f *FileMemory) Search(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	items, err := f.manager.Search(ctx, query, LevelLongTerm, limit)
	if err != nil {
		return nil, 0, err
	}

	messages := make([]kernel.Message, len(items))
	for i, item := range items {
		messages[i] = kernel.Message{
			Role:    "system",
			Content: fmt.Sprintf("[记忆] %s", item.Content),
		}
	}

	return messages, 0.8, nil
}

// Compress 压缩记忆
func (f *FileMemory) Compress(ctx context.Context, sessionID string) error {
	return f.manager.Compress(ctx, sessionID)
}
