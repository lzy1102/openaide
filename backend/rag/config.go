package rag

// Config 是外部检索后端的统一配置。
// Type 选择后端实现;未配置对应后端参数或后端不可达时,NewFromConfig 降级为 NoopRetriever。
// 所有后端共用 EmbeddingURL/EmbeddingKey/EmbeddingModel(外部 OpenAI 兼容 /embeddings 端点)。
type Config struct {
	Type           string // ""/pgvector(默认) / qdrant / milvus / redis / chroma
	EmbeddingURL   string
	EmbeddingKey   string
	EmbeddingModel string // 默认 text-embedding-3-small
	Collection     string // 默认 openaide_docs

	DSN string // pgvector: PostgreSQL 连接串

	QdrantHost   string // qdrant: 主机
	QdrantPort   int    // qdrant: gRPC 端口,默认 6334
	QdrantAPIKey string // qdrant: API key
	QdrantTLS    bool   // qdrant: 启用 TLS

	MilvusAddress  string // milvus: 如 localhost:19530
	MilvusUsername string // milvus: 用户名
	MilvusPassword string // milvus: 密码

	RedisAddr     string // redis: 如 localhost:6379
	RedisPassword string // redis: 密码
	RedisDB       int    // redis: DB 编号

	ChromaURL   string // chroma: 如 http://localhost:8000
	ChromaToken string // chroma: 认证 token
}
