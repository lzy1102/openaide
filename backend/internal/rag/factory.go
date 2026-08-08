package rag

import (
	"context"
	"log/slog"
	"time"
)

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

	var (
		r   Retriever
		err error
	)
	switch typ {
	case "pgvector":
		r, err = NewPGVector(PGVectorConfig{
			DSN:            cfg.DSN,
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
		})
	case "qdrant":
		r, err = NewQdrant(QdrantConfig{
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
			Host:           cfg.QdrantHost,
			Port:           cfg.QdrantPort,
			APIKey:         cfg.QdrantAPIKey,
			UseTLS:         cfg.QdrantTLS,
		})
	case "milvus":
		r, err = NewMilvus(MilvusConfig{
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
			Address:        cfg.MilvusAddress,
			Username:       cfg.MilvusUsername,
			Password:       cfg.MilvusPassword,
		})
	case "redis":
		r, err = NewRedisRetriever(RedisConfig{
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
			Addr:           cfg.RedisAddr,
			Password:       cfg.RedisPassword,
			DB:             cfg.RedisDB,
		})
	case "chroma":
		r, err = NewChroma(ChromaConfig{
			EmbeddingURL:   cfg.EmbeddingURL,
			EmbeddingKey:   cfg.EmbeddingKey,
			EmbeddingModel: cfg.EmbeddingModel,
			Collection:     cfg.Collection,
			URL:            cfg.ChromaURL,
			Token:          cfg.ChromaToken,
		})
	case "":
		return NoopRetriever{}
	default:
		slog.Warn("Unknown RAG backend type, using NoopRetriever", "type", typ)
		return NoopRetriever{}
	}
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
