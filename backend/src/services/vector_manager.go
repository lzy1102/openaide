package services

import (
	"context"
	"fmt"
	"log"

	"openaide/backend/src/logger"
	"openaide/backend/src/services/storage"
)

// VectorManager 向量管理器（基于 VectorStore 接口，支持本地 HNSW 和外部向量数据库）
type VectorManager struct {
	// 存储后端（通过接口抽象，支持多种实现）
	store storage.VectorStore

	// 依赖服务
	embeddingSvc EmbeddingService
}

// NewVectorManager 创建向量管理器
// 兼容旧版调用：如果传入 dataDir，则使用 HNSW 本地存储
func NewVectorManager(dataDir string, embeddingSvc EmbeddingService) (*VectorManager, error) {
	// 创建默认的 HNSW 存储（HNSWVectorStore 在当前包中）
	store, err := NewHNSWVectorStore(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create vector store: %w", err)
	}

	return NewVectorManagerWithStore(store, embeddingSvc)
}

// NewVectorManagerWithStore 使用指定的 VectorStore 创建向量管理器
func NewVectorManagerWithStore(store storage.VectorStore, embeddingSvc EmbeddingService) (*VectorManager, error) {
	vm := &VectorManager{
		store:        store,
		embeddingSvc: embeddingSvc,
	}

	return vm, nil
}

// NewVectorManagerWithConfig 根据配置创建向量管理器（推荐方式）
func NewVectorManagerWithConfig(cfg storage.VectorStoreConfig, embeddingSvc EmbeddingService) (*VectorManager, error) {
	var store storage.VectorStore
	var err error

	switch cfg.Type {
	case storage.VectorStoreHNSW, "":
		// HNSW 实现在 services 包中
		store, err = NewHNSWVectorStore(cfg.DataDir)
	default:
		// 其他实现在 storage 包中
		store, err = storage.NewVectorStore(cfg)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create vector store: %w", err)
	}

	return NewVectorManagerWithStore(store, embeddingSvc)
}

// CreateCollection 创建向量集合
func (vm *VectorManager) CreateCollection(ctx context.Context, name string, dimension int) error {
	return vm.store.CreateCollection(name, dimension)
}

// GetCollection 获取集合（兼容旧版，返回底层 HNSW 索引）
// 注意：当使用非 HNSW 存储时，此方法返回错误
func (vm *VectorManager) GetCollection(name string) (*PersistentHNSW, error) {
	// 尝试获取底层 HNSW 存储（HNSWVectorStore 在当前包中）
	if hnswStore, ok := vm.store.(*HNSWVectorStore); ok {
		// 通过反射或内部方法获取（这里简化处理）
		_ = hnswStore
		return nil, fmt.Errorf("GetCollection is deprecated, please use VectorStore interface methods")
	}
	return nil, fmt.Errorf("GetCollection only supported with HNSW backend")
}

// DeleteCollection 删除集合
func (vm *VectorManager) DeleteCollection(ctx context.Context, name string) error {
	return vm.store.DeleteCollection(name)
}

// ListCollections 列出所有集合
func (vm *VectorManager) ListCollections(ctx context.Context) []map[string]interface{} {
	collections, err := vm.store.ListCollections()
	if err != nil {
		log.Printf("[VectorManager] Failed to list collections: %v", err)
		return nil
	}

	result := make([]map[string]interface{}, 0, len(collections))
	for _, name := range collections {
		stats, err := vm.store.GetStats(ctx, name)
		if err != nil {
			stats = map[string]interface{}{"name": name, "error": err.Error()}
		} else {
			stats["name"] = name
		}
		result = append(result, stats)
	}

	return result
}

// Insert 插入向量（带向量化）
func (vm *VectorManager) Insert(ctx context.Context, collectionName string, id string, content string, metadata map[string]interface{}) error {
	if vm.embeddingSvc == nil {
		return fmt.Errorf("embedding service not available")
	}

	// 生成向量
	embedding, err := vm.embeddingSvc.GenerateEmbedding(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 转换为 float32
	embedding32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embedding32[i] = float32(v)
	}

	// 插入到存储
	doc := storage.VectorDocument{
		ID:        id,
		Content:   content,
		Embedding: embedding32,
		Metadata:  metadata,
	}
	return vm.store.Insert(ctx, collectionName, doc)
}

// InsertVector 直接插入向量
func (vm *VectorManager) InsertVector(ctx context.Context, collectionName string, id string, vector []float32, metadata map[string]interface{}) error {
	doc := storage.VectorDocument{
		ID:        id,
		Embedding: vector,
		Metadata:  metadata,
	}
	return vm.store.Insert(ctx, collectionName, doc)
}

// Search 向量搜索
func (vm *VectorManager) Search(ctx context.Context, collectionName string, query []float32, k int) ([]SearchResult, error) {
	results, err := vm.store.Search(ctx, collectionName, query, k)
	if err != nil {
		return nil, err
	}

	return convertToSearchResults(results), nil
}

// SemanticSearch 语义搜索（自动向量化）
func (vm *VectorManager) SemanticSearch(ctx context.Context, collectionName string, queryText string, k int) ([]SearchResult, error) {
	if vm.embeddingSvc == nil {
		return nil, fmt.Errorf("embedding service not available")
	}

	// 生成查询向量
	embedding, err := vm.embeddingSvc.GenerateEmbedding(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// 转换为 float32
	embedding32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embedding32[i] = float32(v)
	}

	return vm.Search(ctx, collectionName, embedding32, k)
}

// Delete 删除文档
func (vm *VectorManager) Delete(ctx context.Context, collectionName string, id string) error {
	return vm.store.Delete(ctx, collectionName, id)
}

// GetStats 获取统计信息
func (vm *VectorManager) GetStats(ctx context.Context) map[string]interface{} {
	collections, err := vm.store.ListCollections()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	totalDocs := 0
	for _, name := range collections {
		count, err := vm.store.Count(ctx, name)
		if err == nil {
			totalDocs += count
		}
	}

	return map[string]interface{}{
		"collections": len(collections),
		"total_docs":  totalDocs,
	}
}

// Close 关闭所有集合
func (vm *VectorManager) Close() error {
	if vm.store != nil {
		return vm.store.Close()
	}
	return nil
}

// Backup 备份所有集合（仅 HNSW 支持）
func (vm *VectorManager) Backup(backupDir string) error {
	if hnswStore, ok := vm.store.(*HNSWVectorStore); ok {
		_ = hnswStore
		// HNSWVectorStore 的备份逻辑可以通过扩展接口实现
		logger.WithComponent("VectorManager").Warn("backup not yet implemented via interface")
	}
	return fmt.Errorf("backup not supported for current vector store type")
}

// GetStore 获取底层 VectorStore（用于高级操作）
func (vm *VectorManager) GetStore() storage.VectorStore {
	return vm.store
}

// convertToSearchResults 将 storage.VectorSearchResult 转换为 services.SearchResult
func convertToSearchResults(results []storage.VectorSearchResult) []SearchResult {
	converted := make([]SearchResult, len(results))
	for i, r := range results {
		converted[i] = SearchResult{
			Document: VectorDocument{
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

// ==================== Noop 实现（当向量服务不可用时）====================

type noopVectorService struct{}

func NewNoopVectorService() VectorService {
	return &noopVectorService{}
}

func (n *noopVectorService) CreateCollection(ctx context.Context, name string, dimension int) error {
	return fmt.Errorf("vector service not available")
}

func (n *noopVectorService) Insert(ctx context.Context, collectionName string, id string, content string, metadata map[string]interface{}) error {
	return fmt.Errorf("vector service not available")
}

func (n *noopVectorService) Search(ctx context.Context, collectionName string, query []float32, k int) ([]SearchResult, error) {
	return nil, fmt.Errorf("vector service not available")
}

func (n *noopVectorService) SemanticSearch(ctx context.Context, collectionName string, queryText string, k int) ([]SearchResult, error) {
	return nil, fmt.Errorf("vector service not available")
}

func (n *noopVectorService) Delete(ctx context.Context, collectionName string, id string) error {
	return fmt.Errorf("vector service not available")
}

func (n *noopVectorService) DeleteCollection(ctx context.Context, name string) error {
	return fmt.Errorf("vector service not available")
}

func (n *noopVectorService) ListCollections(ctx context.Context) []map[string]interface{} {
	return nil
}

func (n *noopVectorService) GetStats(ctx context.Context) map[string]interface{} {
	return map[string]interface{}{"error": "vector service not available"}
}

func (n *noopVectorService) Close() error {
	return nil
}
