package services

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"openaide/backend/src/config"
)

// VectorManager 向量管理器（生产级）
type VectorManager struct {
	mu sync.RWMutex

	// 存储后端
	indexes map[string]*PersistentHNSW // collection name -> index

	// 依赖服务
	embeddingSvc EmbeddingService

	// 配置
	dataDir string
}

// NewVectorManager 创建向量管理器
func NewVectorManager(dataDir string, embeddingSvc EmbeddingService) (*VectorManager, error) {
	if dataDir == "" {
		dataDir = config.DefaultPaths.VectorDir
	}

	vm := &VectorManager{
		indexes:      make(map[string]*PersistentHNSW),
		embeddingSvc: embeddingSvc,
		dataDir:      dataDir,
	}

	// 加载已有集合
	if err := vm.loadCollections(); err != nil {
		log.Printf("[VectorManager] Failed to load collections: %v", err)
	}

	return vm, nil
}

// CreateCollection 创建向量集合
func (vm *VectorManager) CreateCollection(name string, dimension int) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if _, exists := vm.indexes[name]; exists {
		return fmt.Errorf("collection %s already exists", name)
	}

	index, err := NewPersistentHNSW(dimension, vm.getCollectionDir(name))
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	vm.indexes[name] = index
	log.Printf("[VectorManager] Created collection: %s (dimension: %d)", name, dimension)
	return nil
}

type noopVectorService struct{}

func NewNoopVectorService() VectorService {
	return &noopVectorService{}
}

func (n *noopVectorService) CreateCollection(name string, dimension int) error {
	return fmt.Errorf("vector service not available")
}

func (n *noopVectorService) Insert(collectionName string, id string, content string, metadata map[string]interface{}) error {
	return fmt.Errorf("vector service not available")
}

func (n *noopVectorService) Search(collectionName string, query []float32, k int) ([]SearchResult, error) {
	return nil, fmt.Errorf("vector service not available")
}

func (n *noopVectorService) SemanticSearch(collectionName string, queryText string, k int) ([]SearchResult, error) {
	return nil, fmt.Errorf("vector service not available")
}

// GetCollection 获取集合
func (vm *VectorManager) GetCollection(name string) (*PersistentHNSW, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	index, exists := vm.indexes[name]
	if !exists {
		return nil, fmt.Errorf("collection %s not found", name)
	}

	return index, nil
}

// DeleteCollection 删除集合
func (vm *VectorManager) DeleteCollection(name string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	index, exists := vm.indexes[name]
	if !exists {
		return fmt.Errorf("collection %s not found", name)
	}

	// 关闭并删除
	if err := index.Close(); err != nil {
		return err
	}

	delete(vm.indexes, name)
	log.Printf("[VectorManager] Deleted collection: %s", name)
	return nil
}

// ListCollections 列出所有集合
func (vm *VectorManager) ListCollections() []map[string]interface{} {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	result := make([]map[string]interface{}, 0, len(vm.indexes))
	for name, index := range vm.indexes {
		stats := index.GetStats()
		stats["name"] = name
		result = append(result, stats)
	}

	return result
}

// Insert 插入向量（带向量化）
func (vm *VectorManager) Insert(collectionName string, id string, content string, metadata map[string]interface{}) error {
	if vm.embeddingSvc == nil {
		return fmt.Errorf("embedding service not available")
	}

	// 获取集合
	index, err := vm.GetCollection(collectionName)
	if err != nil {
		return err
	}

	// 生成向量
	embedding, err := vm.embeddingSvc.GenerateEmbedding(context.Background(), content)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 转换为 float32
	embedding32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embedding32[i] = float32(v)
	}

	// 插入
	return index.Insert(id, embedding32, metadata)
}

// InsertVector 直接插入向量
func (vm *VectorManager) InsertVector(collectionName string, id string, vector []float32, metadata map[string]interface{}) error {
	index, err := vm.GetCollection(collectionName)
	if err != nil {
		return err
	}

	return index.Insert(id, vector, metadata)
}

// Search 向量搜索
func (vm *VectorManager) Search(collectionName string, query []float32, k int) ([]SearchResult, error) {
	index, err := vm.GetCollection(collectionName)
	if err != nil {
		return nil, err
	}

	return index.Search(query, k)
}

// SemanticSearch 语义搜索（自动向量化）
func (vm *VectorManager) SemanticSearch(collectionName string, queryText string, k int) ([]SearchResult, error) {
	if vm.embeddingSvc == nil {
		return nil, fmt.Errorf("embedding service not available")
	}

	// 生成查询向量
	embedding, err := vm.embeddingSvc.GenerateEmbedding(context.Background(), queryText)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// 转换为 float32
	embedding32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embedding32[i] = float32(v)
	}

	return vm.Search(collectionName, embedding32, k)
}

// Delete 删除文档
func (vm *VectorManager) Delete(collectionName string, id string) error {
	index, err := vm.GetCollection(collectionName)
	if err != nil {
		return err
	}

	return index.Delete(id)
}

// GetStats 获取统计信息
func (vm *VectorManager) GetStats() map[string]interface{} {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	totalDocs := 0
	for _, index := range vm.indexes {
		totalDocs += index.Count
	}

	return map[string]interface{}{
		"collections": len(vm.indexes),
		"total_docs":  totalDocs,
		"data_dir":    vm.dataDir,
	}
}

// Close 关闭所有集合
func (vm *VectorManager) Close() error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	var lastErr error
	for name, index := range vm.indexes {
		if err := index.Close(); err != nil {
			lastErr = err
			log.Printf("[VectorManager] Failed to close collection %s: %v", name, err)
		}
	}

	return lastErr
}

// loadCollections 加载已有集合
func (vm *VectorManager) loadCollections() error {
	// 这里可以实现扫描目录加载已有集合的逻辑
	// 目前简化处理，需要手动创建集合
	return nil
}

// getCollectionDir 获取集合数据目录
func (vm *VectorManager) getCollectionDir(name string) string {
	return filepath.Join(vm.dataDir, name)
}

// Backup 备份所有集合
func (vm *VectorManager) Backup(backupDir string) error {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	for name, index := range vm.indexes {
		backupPath := filepath.Join(backupDir, name)
		if err := index.Backup(backupPath); err != nil {
			log.Printf("[VectorManager] Failed to backup collection %s: %v", name, err)
		}
	}

	return nil
}
