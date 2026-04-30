package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"openaide/backend/src/config"
	"openaide/backend/src/logger"
	"openaide/backend/src/services/storage"
)

// HNSWVectorStore 基于本地 HNSW 索引的向量存储实现
type HNSWVectorStore struct {
	mu sync.RWMutex

	// 存储后端：collection name -> PersistentHNSW index
	indexes map[string]*PersistentHNSW

	// 配置
	dataDir string
}

// NewHNSWVectorStore 创建 HNSW 向量存储
func NewHNSWVectorStore(dataDir string) (*HNSWVectorStore, error) {
	if dataDir == "" {
		dataDir = config.DefaultPaths.VectorDir
	}

	store := &HNSWVectorStore{
		indexes: make(map[string]*PersistentHNSW),
		dataDir: dataDir,
	}

	// 尝试加载已有集合
	if err := store.loadCollections(); err != nil {
		logger.WithComponent("HNSWVectorStore").Error("failed to load collections", "error", err)
	}

	return store, nil
}

// CreateCollection 创建向量集合
func (s *HNSWVectorStore) CreateCollection(name string, dimension int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.indexes[name]; exists {
		return fmt.Errorf("collection %s already exists", name)
	}

	index, err := NewPersistentHNSW(dimension, s.getCollectionDir(name))
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	s.indexes[name] = index
	logger.WithComponent("HNSWVectorStore").Info("created collection", "name", name, "dimension", dimension)
	return nil
}

// DeleteCollection 删除向量集合
func (s *HNSWVectorStore) DeleteCollection(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, exists := s.indexes[name]
	if !exists {
		return fmt.Errorf("collection %s not found", name)
	}

	// 关闭并删除
	if err := index.Close(); err != nil {
		return err
	}

	delete(s.indexes, name)
	logger.WithComponent("HNSWVectorStore").Info("deleted collection", "name", name)
	return nil
}

// ListCollections 列出所有集合
func (s *HNSWVectorStore) ListCollections() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.indexes))
	for name := range s.indexes {
		result = append(result, name)
	}
	return result, nil
}

// Insert 插入向量文档
func (s *HNSWVectorStore) Insert(ctx context.Context, collectionName string, doc storage.VectorDocument) error {
	s.mu.RLock()
	index, exists := s.indexes[collectionName]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("collection %s not found", collectionName)
	}

	return index.Insert(doc.ID, doc.Embedding, doc.Metadata)
}

// InsertBatch 批量插入向量文档
func (s *HNSWVectorStore) InsertBatch(ctx context.Context, collectionName string, docs []storage.VectorDocument) error {
	s.mu.RLock()
	index, exists := s.indexes[collectionName]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("collection %s not found", collectionName)
	}

	var lastErr error
	for _, doc := range docs {
		if err := index.Insert(doc.ID, doc.Embedding, doc.Metadata); err != nil {
			lastErr = err
			logger.WithComponent("HNSWVectorStore").Error("failed to insert doc", "id", doc.ID, "error", err)
		}
	}
	return lastErr
}

// Delete 删除向量文档
func (s *HNSWVectorStore) Delete(ctx context.Context, collectionName string, id string) error {
	s.mu.RLock()
	index, exists := s.indexes[collectionName]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("collection %s not found", collectionName)
	}

	return index.Delete(id)
}

// Search 向量相似度搜索
func (s *HNSWVectorStore) Search(ctx context.Context, collectionName string, query []float32, k int) ([]storage.VectorSearchResult, error) {
	s.mu.RLock()
	index, exists := s.indexes[collectionName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("collection %s not found", collectionName)
	}

	results, err := index.Search(query, k)
	if err != nil {
		return nil, err
	}

	return convertToStorageSearchResults(results), nil
}

// SearchWithFilter 带过滤条件的向量搜索（HNSW 简单实现：先搜索再过滤）
func (s *HNSWVectorStore) SearchWithFilter(ctx context.Context, collectionName string, query []float32, k int, filter map[string]interface{}) ([]storage.VectorSearchResult, error) {
	// 先进行普通搜索，获取更多结果用于过滤
	results, err := s.Search(ctx, collectionName, query, k*3)
	if err != nil {
		return nil, err
	}

	// 过滤结果
	var filtered []storage.VectorSearchResult
	for _, r := range results {
		if storage.MatchesFilter(r.Document.Metadata, filter) {
			filtered = append(filtered, r)
			if len(filtered) >= k {
				break
			}
		}
	}

	return filtered, nil
}

// GetDocument 获取单个文档
func (s *HNSWVectorStore) GetDocument(ctx context.Context, collectionName string, id string) (*storage.VectorDocument, error) {
	// HNSW 索引目前不直接支持按 ID 获取，需要通过搜索或其他方式
	// 这里返回未实现，实际使用中可以扩展 HNSWIndex
	return nil, fmt.Errorf("GetDocument not implemented for HNSWVectorStore")
}

// Count 获取集合文档数量
func (s *HNSWVectorStore) Count(ctx context.Context, collectionName string) (int, error) {
	s.mu.RLock()
	index, exists := s.indexes[collectionName]
	s.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("collection %s not found", collectionName)
	}

	stats := index.GetStats()
	if count, ok := stats["count"].(int); ok {
		return count, nil
	}
	return 0, nil
}

// GetStats 获取统计信息
func (s *HNSWVectorStore) GetStats(ctx context.Context, collectionName string) (map[string]interface{}, error) {
	s.mu.RLock()
	index, exists := s.indexes[collectionName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("collection %s not found", collectionName)
	}

	return index.GetStats(), nil
}

// Close 关闭所有集合
func (s *HNSWVectorStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastErr error
	for name, index := range s.indexes {
		if err := index.Close(); err != nil {
			lastErr = err
			logger.WithComponent("HNSWVectorStore").Error("failed to close collection", "name", name, "error", err)
		}
	}

	return lastErr
}

// loadCollections 加载已有集合
func (s *HNSWVectorStore) loadCollections() error {
	// 目前简化处理，需要手动创建集合
	return nil
}

// getCollectionDir 获取集合数据目录
func (s *HNSWVectorStore) getCollectionDir(name string) string {
	return filepath.Join(s.dataDir, name)
}

// convertToStorageSearchResults 转换搜索结果类型
func convertToStorageSearchResults(results []SearchResult) []storage.VectorSearchResult {
	converted := make([]storage.VectorSearchResult, len(results))
	for i, r := range results {
		converted[i] = storage.VectorSearchResult{
			Document: storage.VectorDocument{
				ID:        r.Document.ID,
				Content:   r.Document.Content,
				Embedding: r.Document.Embedding,
				Metadata:  r.Document.Metadata,
				Score:     r.Document.Score,
			},
			Score:    r.Score,
			Distance: r.Distance,
		}
	}
	return converted
}

// matchesFilter 检查文档元数据是否匹配过滤条件
func matchesFilter(metadata, filter map[string]interface{}) bool {
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
