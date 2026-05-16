package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
)

// Document 知识文档
type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Source    string    `json:"source"` // file, url, manual
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Base 知识库
type Base struct {
	dataDir   string
	docs      map[string]*Document
	mu        sync.RWMutex
}

// NewBase 创建知识库
func NewBase(dataDir string) (*Base, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	kb := &Base{
		dataDir: dataDir,
		docs:    make(map[string]*Document),
	}

	kb.load()
	return kb, nil
}

// Add 添加文档
func (kb *Base) Add(ctx context.Context, title, content, source string, tags []string) (*Document, error) {
	doc := &Document{
		ID:        fmt.Sprintf("doc_%d", time.Now().UnixNano()),
		Title:     title,
		Content:   content,
		Source:    source,
		Tags:      tags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	kb.mu.Lock()
	kb.docs[doc.ID] = doc
	kb.mu.Unlock()

	if err := kb.saveDoc(doc); err != nil {
		return nil, err
	}

	return doc, nil
}

// Get 获取文档
func (kb *Base) Get(ctx context.Context, docID string) (*Document, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	if doc, ok := kb.docs[docID]; ok {
		return doc, nil
	}
	return nil, fmt.Errorf("document not found: %s", docID)
}

// Search 搜索文档
func (kb *Base) Search(ctx context.Context, query string, limit int) ([]*Document, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	var results []*Document
	queryLower := strings.ToLower(query)

	for _, doc := range kb.docs {
		score := 0

		// 标题匹配
		if strings.Contains(strings.ToLower(doc.Title), queryLower) {
			score += 10
		}

		// 内容匹配
		if strings.Contains(strings.ToLower(doc.Content), queryLower) {
			score += 5
		}

		// 标签匹配
		for _, tag := range doc.Tags {
			if strings.Contains(strings.ToLower(tag), queryLower) {
				score += 3
			}
		}

		if score > 0 {
			results = append(results, doc)
		}
	}

	// 按相关性排序（简单实现）
	// 实际应使用向量相似度
	if len(results) > limit && limit > 0 {
		results = results[:limit]
	}

	return results, nil
}

// Delete 删除文档
func (kb *Base) Delete(ctx context.Context, docID string) error {
	kb.mu.Lock()
	delete(kb.docs, docID)
	kb.mu.Unlock()

	path := filepath.Join(kb.dataDir, docID+".json")
	return os.Remove(path)
}

// List 列出所有文档
func (kb *Base) List(ctx context.Context, limit int) ([]*Document, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	var results []*Document
	for _, doc := range kb.docs {
		results = append(results, doc)
		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results, nil
}

// InjectToPrompt 将相关知识注入提示词
func (kb *Base) InjectToPrompt(ctx context.Context, query string, maxTokens int) (string, error) {
	docs, err := kb.Search(ctx, query, 3)
	if err != nil {
		return "", err
	}

	if len(docs) == 0 {
		return "", nil
	}

	var parts []string
	parts = append(parts, "## 相关知识")

	for _, doc := range docs {
		parts = append(parts, fmt.Sprintf("### %s\n%s", doc.Title, doc.Content))
	}

	result := strings.Join(parts, "\n\n")

	// 截断到最大 token
	if len(result) > maxTokens*4 { // 粗略估算
		result = result[:maxTokens*4] + "..."
	}

	return result, nil
}

// ImportFromFile 从文件导入知识
func (kb *Base) ImportFromFile(ctx context.Context, filePath string) (*Document, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	title := filepath.Base(filePath)
	content := string(data)

	return kb.Add(ctx, title, content, "file", []string{"imported"})
}

// ImportFromDirectory 从目录导入知识
func (kb *Base) ImportFromDirectory(ctx context.Context, dir string, extensions []string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		for _, allowed := range extensions {
			if ext == allowed {
				_, err := kb.ImportFromFile(ctx, path)
				return err
			}
		}

		return nil
	})
}

func (kb *Base) saveDoc(doc *Document) error {
	path := filepath.Join(kb.dataDir, doc.ID+".json")
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (kb *Base) load() error {
	entries, err := os.ReadDir(kb.dataDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(kb.dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var doc Document
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}

		kb.docs[doc.ID] = &doc
	}

	return nil
}

// ============ kernel.KnowledgeCollector 接口实现 ============

// AddKnowledge 添加知识条目
func (kb *Base) AddKnowledge(ctx context.Context, title, content, source string, tags []string) (string, error) {
	doc, err := kb.Add(ctx, title, content, source, tags)
	if err != nil {
		return "", err
	}
	return doc.ID, nil
}

// SearchKnowledge 搜索知识库
func (kb *Base) SearchKnowledge(ctx context.Context, query string, limit int) ([]kernel.KnowledgeItem, error) {
	docs, err := kb.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	items := make([]kernel.KnowledgeItem, len(docs))
	for i, doc := range docs {
		items[i] = kernel.KnowledgeItem{
			ID:      doc.ID,
			Title:   doc.Title,
			Content: doc.Content,
			Tags:    doc.Tags,
		}
	}
	return items, nil
}

// InjectContext 将相关知识注入提示词
func (kb *Base) InjectContext(ctx context.Context, query string, maxTokens int) (string, error) {
	return kb.InjectToPrompt(ctx, query, maxTokens)
}
