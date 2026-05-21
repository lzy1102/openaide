package knowledge

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
	"openaide/backend/internal/llm"
)

// Document 知识文档
type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Source    string    `json:"source"` // file, url, manual
	Tags      []string  `json:"tags,omitempty"`
	Embedding []float32 `json:"embedding,omitempty"` // LLM embedding 向量，用于语义搜索
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Base 知识库
type Base struct {
	dataDir  string
	docs     map[string]*Document
	embedder llm.Embedder
	mu       sync.RWMutex
}

// NewBase 创建知识库
func NewBase(dataDir string) (*Base, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	kb := &Base{
		dataDir:  dataDir,
		docs:     make(map[string]*Document),
		embedder: llm.NoopEmbedder{},
	}

	kb.load()
	return kb, nil
}

// SetEmbedder 设置向量嵌入器（语义搜索增强）
func (kb *Base) SetEmbedder(e llm.Embedder) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	kb.embedder = e
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

	// 如果有嵌入器，生成向量（使用标题+内容合成文本）
	if kb.embedder != nil && kb.embedder.Dimension() > 0 {
		embedText := title
		if content != "" {
			embedText = title + "\n" + content
		}
		vec, err := kb.embedder.Embed(ctx, embedText)
		if err == nil && len(vec) > 0 {
			doc.Embedding = vec
		}
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

// Search 搜索文档（语义 + 文本混合搜索）
func (kb *Base) Search(ctx context.Context, query string, limit int) ([]*Document, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	var results []*Document
	seen := make(map[string]bool)

	// 1. LLM 向量语义搜索
	if kb.embedder != nil {
		queryVec, err := kb.embedder.Embed(ctx, query)
		if err == nil && len(queryVec) > 0 {
			type scoredDoc struct {
				doc   *Document
				score float64
			}
			var scored []scoredDoc
			for _, doc := range kb.docs {
				if len(doc.Embedding) == 0 || len(doc.Embedding) != len(queryVec) {
					continue
				}
				sim := llm.CosineSimilarity(queryVec, doc.Embedding)
				if sim > 0.5 {
					scored = append(scored, scoredDoc{doc, sim})
				}
			}
			sort.Slice(scored, func(i, j int) bool {
				return scored[i].score > scored[j].score
			})
			for i := 0; i < len(scored) && (limit <= 0 || len(results) < limit); i++ {
				results = append(results, scored[i].doc)
				seen[scored[i].doc.ID] = true
			}
		}
	}

	// 2. 文本匹配回退
	if len(results) < limit || limit == 0 {
		queryLower := strings.ToLower(query)
		for _, doc := range kb.docs {
			if seen[doc.ID] {
				continue
			}

			if strings.Contains(strings.ToLower(doc.Title), queryLower) ||
				strings.Contains(strings.ToLower(doc.Content), queryLower) {
				results = append(results, doc)
				seen[doc.ID] = true
				continue
			}

			for _, tag := range doc.Tags {
				if strings.Contains(strings.ToLower(tag), queryLower) {
					results = append(results, doc)
					seen[doc.ID] = true
					break
				}
			}
		}
	}

	if limit > 0 && len(results) > limit {
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
