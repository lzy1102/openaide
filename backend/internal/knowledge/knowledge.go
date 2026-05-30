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
	"log/slog"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/llm"
)

// Document 知识文档
type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content,omitempty"`   // 搜索时为空，Get/Inject 时按需加载
	Source    string    `json:"source"` // file, url, manual
	Tags      []string  `json:"tags,omitempty"`
	Embedding []float32 `json:"embedding,omitempty"` // LLM embedding 向量，用于语义搜索
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 使用反馈 — 用于评估知识有效性
	UsesCount        int     `json:"uses_count"`          // 被检索/注入次数
	TotalQualityScore float64 `json:"total_quality_score"` // 累计使用后质量分，avg = total / uses
}

// loadContent 从文件读取完整文档内容（Search 返回的 doc.Content 为空，需要时主动加载）
func (kb *Base) loadContent(doc *Document) error {
	path := filepath.Join(kb.dataDir, doc.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var full Document
	if err := json.Unmarshal(data, &full); err != nil {
		return err
	}
	doc.Content = full.Content
	doc.UpdatedAt = full.UpdatedAt
	return nil
}

// Base 知识库
type Base struct {
	dataDir     string
	docs        map[string]*Document
	embedder    llm.Embedder
	mu          sync.RWMutex
	invertedIdx map[string]map[string]bool // word → set of doc IDs
}

// NewBase 创建知识库
func NewBase(dataDir string) (*Base, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	kb := &Base{
		dataDir:     dataDir,
		docs:        make(map[string]*Document),
		embedder:    llm.NoopEmbedder{},
		invertedIdx: make(map[string]map[string]bool),
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
	slog.Debug("Knowledge add", "title", title, "source", source)
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
	kb.indexDoc(doc)
	kb.mu.Unlock()

	if err := kb.saveDoc(doc); err != nil {
		return nil, err
	}

	doc.Content = "" // 释放内存
	return doc, nil
}

// Get 获取文档（从文件读取完整内容）
func (kb *Base) Get(ctx context.Context, docID string) (*Document, error) {
	path := filepath.Join(kb.dataDir, docID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("document not found: %s", docID)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("document corrupt: %s", docID)
	}
	return &doc, nil
}

// Search 搜索文档（语义 + 文本混合搜索）
func (kb *Base) Search(ctx context.Context, query string, limit int) ([]*Document, error) {
	slog.Debug("Knowledge search", "query", query[:min(50, len(query))], "limit", limit)

	if limit <= 0 {
		limit = 10
	}

	// Embed query outside lock — do NOT hold lock across LLM embedding call.
	queryVec, hasQueryVec := kb.embedQuery(ctx, query)

	kb.mu.RLock()
	defer kb.mu.RUnlock()

	// 1. 倒排索引快速定位候选文档
	candidates := kb.searchByIndex(query)

	// 2. 分值排序（有向量则语义打分，否则按索引匹配度）
	type scoredDoc struct {
		doc   *Document
		score float64
	}
	var scored []scoredDoc
	seen := make(map[string]bool)

	for _, id := range candidates {
		doc := kb.docs[id]
		if doc == nil {
			continue
		}
		seen[id] = true
		score := float64(0)
		if hasQueryVec && len(doc.Embedding) == len(queryVec) {
			score = llm.CosineSimilarity(queryVec, doc.Embedding)
		} else {
			score = 0.5 // 无向量时给予基础分
		}
		scored = append(scored, scoredDoc{doc, score})
	}

	// 3. 全局向量搜索（仅当索引找不到足够结果时全量扫描）
	if len(scored) < limit/2 && hasQueryVec {
		for _, doc := range kb.docs {
			if seen[doc.ID] || len(doc.Embedding) == 0 || len(doc.Embedding) != len(queryVec) {
				continue
			}
			sim := llm.CosineSimilarity(queryVec, doc.Embedding)
			if sim > 0.5 {
				scored = append(scored, scoredDoc{doc, sim})
				seen[doc.ID] = true
			}
		}
	}

	// 4. 按分数排序取 top
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	results := make([]*Document, 0, len(scored))
	for i := 0; i < len(scored) && len(results) < limit; i++ {
		results = append(results, scored[i].doc)
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

// InjectToPrompt 将相关知识注入提示词，返回 (contextText, docIDs, error)
func (kb *Base) InjectToPrompt(ctx context.Context, query string, maxTokens int) (string, []string, error) {
	docs, err := kb.Search(ctx, query, 3)
	if err != nil {
		return "", nil, err
	}

	if len(docs) == 0 {
		return "", nil, nil
	}

	var parts []string
	parts = append(parts, "## 相关知识")
	docIDs := make([]string, 0, len(docs))

	for _, doc := range docs {
		// 搜索返回的 doc.Content 为空，需要从文件加载完整内容
		if doc.Content == "" {
			kb.loadContent(doc)
		}
		parts = append(parts, fmt.Sprintf("### %s\n%s", doc.Title, doc.Content))
		docIDs = append(docIDs, doc.ID)
	}

	result := strings.Join(parts, "\n\n")

	// 截断到最大 token
	if len(result) > maxTokens*4 { // 粗略估算
		result = result[:maxTokens*4] + "..."
	}

	return result, docIDs, nil
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
		kb.indexDoc(&doc)
		doc.Content = "" // 释放内存，Content 按需从文件读取
	}

	return nil
}

// ============ 倒排索引 ============

// tokenize 将文本拆分为单词（小写去重）
func tokenize(text string) map[string]bool {
	words := make(map[string]bool)
	var buf []rune
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			buf = append(buf, r)
		} else {
			if len(buf) >= 2 {
				words[string(buf)] = true
			}
			buf = buf[:0]
		}
	}
	if len(buf) >= 2 {
		words[string(buf)] = true
	}
	return words
}

// indexDoc 将文档加入倒排索引
func (kb *Base) indexDoc(doc *Document) {
	text := doc.Title + " " + doc.Content
	for tag := range doc.Tags {
		text += " " + doc.Tags[tag]
	}
	for word := range tokenize(text) {
		if kb.invertedIdx[word] == nil {
			kb.invertedIdx[word] = make(map[string]bool)
		}
		kb.invertedIdx[word][doc.ID] = true
	}
}

// searchByIndex 从倒排索引中查找匹配文档 ID（查询词之间取并集）
func (kb *Base) searchByIndex(query string) []string {
	queryWords := tokenize(query)
	if len(queryWords) == 0 {
		return nil
	}

	docSet := make(map[string]bool)
	var first bool = true
	for word := range queryWords {
		if docs, ok := kb.invertedIdx[word]; ok {
			if first {
				for id := range docs {
					docSet[id] = true
				}
				first = false
			} else {
				for id := range docSet {
					if !docs[id] {
						delete(docSet, id)
					}
				}
			}
		} else if !first {
			// 多词查询：某词无匹配则交集为空
			return nil
		}
	}

	// 单词无匹配时 first 仍为 true
	if first {
		return nil
	}

	ids := make([]string, 0, len(docSet))
	for id := range docSet {
		ids = append(ids, id)
	}
	return ids
}

// embedQuery 对查询文本做向量化
func (kb *Base) embedQuery(ctx context.Context, query string) ([]float32, bool) {
	if kb.embedder == nil || kb.embedder.Dimension() == 0 {
		return nil, false
	}
	vec, err := kb.embedder.Embed(ctx, query)
	return vec, err == nil && len(vec) > 0
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

// RecordUsage 记录知识被使用后的质量反馈
// 用于评估知识有效性：高频+高质量 = 好知识；高频+低质量 = 需优化
func (kb *Base) RecordUsage(ctx context.Context, docIDs []string, qualityScore float64) {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	for _, id := range docIDs {
		doc, ok := kb.docs[id]
		if !ok {
			continue
		}
		doc.UsesCount++
		doc.TotalQualityScore += qualityScore
		doc.UpdatedAt = time.Now()
		kb.saveDoc(doc)
	}
}

// RecordKnowledgeUsage 记录知识被使用后的质量反馈
func (kb *Base) RecordKnowledgeUsage(ctx context.Context, docIDs []string, qualityScore float64) {
	kb.RecordUsage(ctx, docIDs, qualityScore)
}

// InjectContext 将相关知识注入提示词，返回 (contextText, docIDs, error)
func (kb *Base) InjectContext(ctx context.Context, query string, maxTokens int) (string, []string, error) {
	slog.Debug("Knowledge inject", "query", query[:min(50, len(query))], "max_tokens", maxTokens)
	return kb.InjectToPrompt(ctx, query, maxTokens)
}
