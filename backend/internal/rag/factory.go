package rag

import (
	"context"
	"log/slog"
	"time"
)

// NewFromConfig builds a Retriever from a config struct. When the store is
// unreachable or not configured, it logs and returns NoopRetriever so the
// agent degrades to empty results instead of erroring.
func NewFromConfig(cfg PGVectorConfig) Retriever {
	if cfg.DSN == "" {
		return NoopRetriever{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pv, err := NewPGVector(cfg)
	if err != nil {
		slog.Warn("RAG store unavailable, using NoopRetriever", "error", err)
		return NoopRetriever{}
	}
	if err := pv.Ping(ctx); err != nil {
		slog.Warn("RAG store ping failed, using NoopRetriever", "error", err)
		return NoopRetriever{}
	}
	slog.Info("RAG store connected", "collection", cfg.Collection)
	return pv
}
