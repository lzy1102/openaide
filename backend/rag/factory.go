package rag

import (
	"context"
	"log/slog"
	"time"
)

// retrieverBuilder 从统一 Config 构造一个后端检索器。
type retrieverBuilder func(cfg Config) (Retriever, error)

// backendFactories 将后端类型映射到其构造函数。
// 阶段 0 把 switch 收敛为注册表,以便阶段 1 按 build tag 裁剪各后端。
// 各后端实现仍位于独立文件(pgvector.go / qdrant.go / milvus.go / redis.go / chroma.go)。
var backendFactories = map[string]retrieverBuilder{
	"pgvector": func(cfg Config) (Retriever, error) {
		return NewPGVector(PGVectorConfig{
			DSN:            cfg.DSN,
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
		})
	},
	"qdrant": func(cfg Config) (Retriever, error) {
		return NewQdrant(QdrantConfig{
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
			Host:           cfg.QdrantHost,
			Port:           cfg.QdrantPort,
			APIKey:         cfg.QdrantAPIKey,
			UseTLS:         cfg.QdrantTLS,
		})
	},
	"milvus": func(cfg Config) (Retriever, error) {
		return NewMilvus(MilvusConfig{
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
			Address:        cfg.MilvusAddress,
			Username:       cfg.MilvusUsername,
			Password:       cfg.MilvusPassword,
		})
	},
	"redis": func(cfg Config) (Retriever, error) {
		return NewRedisRetriever(RedisConfig{
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
			Addr:           cfg.RedisAddr,
			Password:       cfg.RedisPassword,
			DB:             cfg.RedisDB,
		})
	},
	"chroma": func(cfg Config) (Retriever, error) {
		return NewChroma(ChromaConfig{
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
			URL:            cfg.ChromaURL,
			Token:          cfg.ChromaToken,
		})
	},
}

// NewFromConfig builds a Retriever from a unified Config. When the store is
// unreachable or not configured, it logs and returns NoopRetriever so the
// agent degrades to empty results instead of erroring.
//
// Type selects the backend: ""/pgvector, qdrant, milvus, redis, chroma.
// An empty Type with a non-empty DSN defaults to pgvector (backward
// compatible with the previous single-backend config).
func NewFromConfig(cfg Config) Retriever {
	typ := cfg.Type
	if typ == "" && cfg.DSN != "" {
		typ = "pgvector"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if typ == "" {
		return NoopRetriever{}
	}
	builder, ok := backendFactories[typ]
	if !ok {
		slog.Warn("Unknown RAG backend type, using NoopRetriever", "type", typ)
		return NoopRetriever{}
	}

	r, err := builder(cfg)
	if err != nil {
		slog.Warn("RAG store unavailable, using NoopRetriever", "type", typ, "error", err)
		return NoopRetriever{}
	}
	if err := r.Ping(ctx); err != nil {
		slog.Warn("RAG store ping failed, using NoopRetriever", "type", typ, "error", err)
		return NoopRetriever{}
	}
	slog.Info("RAG store connected", "type", typ, "collection", cfg.Collection)
	return r
}
