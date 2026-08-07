package memory

import (
	"context"
	"strings"
	"sync"

	"openaide/backend/internal/rag"
)

// memRetriever 是测试用的内存 rag.Retriever 实现,按分词 AND 匹配内容。
type memRetriever struct {
	mu   sync.Mutex
	docs map[string]map[string]rag.Document // collection -> id -> doc
}

func newMemRetriever() *memRetriever {
	return &memRetriever{docs: make(map[string]map[string]rag.Document)}
}

func (m *memRetriever) Index(_ context.Context, collection string, docs []rag.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.docs[collection] == nil {
		m.docs[collection] = make(map[string]rag.Document)
	}
	for _, d := range docs {
		m.docs[collection][d.ID] = d
	}
	return nil
}

func (m *memRetriever) Search(_ context.Context, collection, query string, limit int) ([]rag.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	qTokens := strings.Fields(strings.ToLower(query))
	var out []rag.Result
	for _, d := range m.docs[collection] {
		content := strings.ToLower(d.Content)
		all := true
		for _, tok := range qTokens {
			if !strings.Contains(content, tok) {
				all = false
				break
			}
		}
		if all {
			out = append(out, rag.Result{ID: d.ID, Content: d.Content, Score: 1, Metadata: d.Metadata})
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memRetriever) Delete(_ context.Context, collection string, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.docs[collection], id)
	}
	return nil
}

func (m *memRetriever) Ping(context.Context) error { return nil }
