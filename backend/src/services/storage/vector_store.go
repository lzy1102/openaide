package storage

import (
	"context"
	"fmt"
)

// VectorDocument 向量文档
type VectorDocument struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content,omitempty"`
	Embedding []float32              `json:"embedding"`
	Metadata  map[string]interface{} `json:"metadata"`
	Score     float64                `json:"score,omitempty"`
}

// VectorSearchResult 向量搜索结果
type VectorSearchResult struct {
	Document VectorDocument `json:"document"`
	Score    float64        `json:"score"`
	Distance float64        `json:"distance"`
}

// VectorStore 向量存储接口（抽象层，支持本地 HNSW 和外部向量数据库）
type VectorStore interface {
	// CreateCollection 创建向量集合
	CreateCollection(name string, dimension int) error

	// DeleteCollection 删除向量集合
	DeleteCollection(name string) error

	// ListCollections 列出所有集合
	ListCollections() ([]string, error)

	// Insert 插入向量文档
	Insert(ctx context.Context, collectionName string, doc VectorDocument) error

	// InsertBatch 批量插入向量文档
	InsertBatch(ctx context.Context, collectionName string, docs []VectorDocument) error

	// Delete 删除向量文档
	Delete(ctx context.Context, collectionName string, id string) error

	// Search 向量相似度搜索
	Search(ctx context.Context, collectionName string, query []float32, k int) ([]VectorSearchResult, error)

	// SearchWithFilter 带过滤条件的向量搜索
	SearchWithFilter(ctx context.Context, collectionName string, query []float32, k int, filter map[string]interface{}) ([]VectorSearchResult, error)

	// GetDocument 获取单个文档
	GetDocument(ctx context.Context, collectionName string, id string) (*VectorDocument, error)

	// Count 获取集合文档数量
	Count(ctx context.Context, collectionName string) (int, error)

	// GetStats 获取统计信息
	GetStats(ctx context.Context, collectionName string) (map[string]interface{}, error)

	// Close 关闭连接/释放资源
	Close() error
}

// VectorStoreType 向量存储类型
type VectorStoreType string

const (
	VectorStoreHNSW    VectorStoreType = "hnsw"    // 本地 HNSW 索引（默认）
	VectorStoreMemory  VectorStoreType = "memory"  // 内存存储（基于数据库）
	VectorStorePinecone VectorStoreType = "pinecone" // Pinecone
	VectorStoreWeaviate VectorStoreType = "weaviate" // Weaviate
	VectorStoreMilvus   VectorStoreType = "milvus"   // Milvus
	VectorStoreQdrant   VectorStoreType = "qdrant"   // Qdrant
	VectorStoreChroma   VectorStoreType = "chroma"   // ChromaDB
)

// VectorStoreConfig 向量存储配置
type VectorStoreConfig struct {
	Type    VectorStoreType `json:"type"`    // 存储类型
	DataDir string          `json:"data_dir"` // 数据目录（HNSW 使用）

	// 外部向量数据库配置
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Cloud     string `json:"cloud,omitempty"`     // Pinecone cloud
	Region    string `json:"region,omitempty"`    // Pinecone region
}

// NewVectorStore 创建向量存储实例（工厂模式）
// 注意：HNSW 实现位于 services 包中，这里只返回通用实现
func NewVectorStore(cfg VectorStoreConfig) (VectorStore, error) {
	switch cfg.Type {
	case VectorStoreMemory:
		return NewMemoryVectorStore(), nil
	case VectorStoreHNSW:
		return nil, fmt.Errorf("HNSW vector store must be created using services.NewHNSWVectorStore() to avoid import cycles")
	case VectorStorePinecone, VectorStoreWeaviate, VectorStoreMilvus, VectorStoreQdrant, VectorStoreChroma:
		return nil, fmt.Errorf("vector store type '%s' is not yet implemented, please use 'hnsw' (via services.NewHNSWVectorStore) or 'memory'", cfg.Type)
	default:
		return nil, fmt.Errorf("unsupported vector store type: %s", cfg.Type)
	}
}
