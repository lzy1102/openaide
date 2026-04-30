package storage

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
)

// MemoryVectorStore 基于内存的向量存储实现（简单实现，适用于测试和小数据量）
type MemoryVectorStore struct {
	mu sync.RWMutex

	// collections: name -> collection data
	collections map[string]*memoryCollection
}

type memoryCollection struct {
	dimension int
	documents map[string]VectorDocument
}

// NewMemoryVectorStore 创建内存向量存储
func NewMemoryVectorStore() *MemoryVectorStore {
	return &MemoryVectorStore{
		collections: make(map[string]*memoryCollection),
	}
}

// CreateCollection 创建向量集合
func (s *MemoryVectorStore) CreateCollection(name string, dimension int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.collections[name]; exists {
		return fmt.Errorf("collection %s already exists", name)
	}

	s.collections[name] = &memoryCollection{
		dimension: dimension,
		documents: make(map[string]VectorDocument),
	}
	return nil
}

// DeleteCollection 删除向量集合
func (s *MemoryVectorStore) DeleteCollection(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.collections[name]; !exists {
		return fmt.Errorf("collection %s not found", name)
	}

	delete(s.collections, name)
	return nil
}

// ListCollections 列出所有集合
func (s *MemoryVectorStore) ListCollections() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.collections))
	for name := range s.collections {
		result = append(result, name)
	}
	return result, nil
}

// Insert 插入向量文档
func (s *MemoryVectorStore) Insert(ctx context.Context, collectionName string, doc VectorDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return fmt.Errorf("collection %s not found", collectionName)
	}

	if len(doc.Embedding) != col.dimension {
		return fmt.Errorf("dimension mismatch: expected %d, got %d", col.dimension, len(doc.Embedding))
	}

	col.documents[doc.ID] = doc
	return nil
}

// InsertBatch 批量插入向量文档
func (s *MemoryVectorStore) InsertBatch(ctx context.Context, collectionName string, docs []VectorDocument) error {
	for _, doc := range docs {
		if err := s.Insert(ctx, collectionName, doc); err != nil {
			return err
		}
	}
	return nil
}

// Delete 删除向量文档
func (s *MemoryVectorStore) Delete(ctx context.Context, collectionName string, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return fmt.Errorf("collection %s not found", collectionName)
	}

	delete(col.documents, id)
	return nil
}

// Search 向量相似度搜索（暴力计算）
func (s *MemoryVectorStore) Search(ctx context.Context, collectionName string, query []float32, k int) ([]VectorSearchResult, error) {
	s.mu.RLock()
	col, exists := s.collections[collectionName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("collection %s not found", collectionName)
	}

	if len(query) != col.dimension {
		return nil, fmt.Errorf("dimension mismatch: expected %d, got %d", col.dimension, len(query))
	}

	// 暴力计算所有文档的相似度
	var results []VectorSearchResult
	for _, doc := range col.documents {
		distance := cosineDistance(query, doc.Embedding)
		score := 1.0 - distance
		results = append(results, VectorSearchResult{
			Document: doc,
			Score:    score,
			Distance: distance,
		})
	}

	// 按分数降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 限制数量
	if len(results) > k {
		results = results[:k]
	}

	return results, nil
}

// SearchWithFilter 带过滤条件的向量搜索
func (s *MemoryVectorStore) SearchWithFilter(ctx context.Context, collectionName string, query []float32, k int, filter map[string]interface{}) ([]VectorSearchResult, error) {
	results, err := s.Search(ctx, collectionName, query, len(s.collections[collectionName].documents))
	if err != nil {
		return nil, err
	}

	var filtered []VectorSearchResult
	for _, r := range results {
		if MatchesFilter(r.Document.Metadata, filter) {
			filtered = append(filtered, r)
			if len(filtered) >= k {
				break
			}
		}
	}

	return filtered, nil
}

// MatchesFilter 检查文档元数据是否匹配过滤条件
func MatchesFilter(metadata, filter map[string]interface{}) bool {
	if len(filter) == 0 {
		return true
	}
	if len(metadata) == 0 {
		return false
	}

	for key, expectedValue := range filter {
		actualValue, exists := metadata[key]
		if !exists {
			return false
		}
		if actualValue != expectedValue {
			return false
		}
	}
	return true
}

// GetDocument 获取单个文档
func (s *MemoryVectorStore) GetDocument(ctx context.Context, collectionName string, id string) (*VectorDocument, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return nil, fmt.Errorf("collection %s not found", collectionName)
	}

	doc, exists := col.documents[id]
	if !exists {
		return nil, fmt.Errorf("document %s not found in collection %s", id, collectionName)
	}

	return &doc, nil
}

// Count 获取集合文档数量
func (s *MemoryVectorStore) Count(ctx context.Context, collectionName string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return 0, fmt.Errorf("collection %s not found", collectionName)
	}

	return len(col.documents), nil
}

// GetStats 获取统计信息
func (s *MemoryVectorStore) GetStats(ctx context.Context, collectionName string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return nil, fmt.Errorf("collection %s not found", collectionName)
	}

	return map[string]interface{}{
		"type":      "memory",
		"dimension": col.dimension,
		"count":     len(col.documents),
	}, nil
}

// Close 关闭（内存存储无需清理）
func (s *MemoryVectorStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.collections = make(map[string]*memoryCollection)
	return nil
}

// cosineDistance 计算余弦距离
func cosineDistance(a, b []float32) float64 {
	if len(a) != len(b) {
		return math.MaxFloat64
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 1.0
	}

	similarity := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
	return 1.0 - similarity
}
