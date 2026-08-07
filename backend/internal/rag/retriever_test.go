package rag

import (
	"context"
	"testing"
)

func TestNoopRetriever_AllMethods(t *testing.T) {
	n := NoopRetriever{}
	ctx := context.Background()

	if err := n.Index(ctx, "col", []Document{{ID: "1", Content: "x"}}); err != nil {
		t.Fatalf("Index returned error: %v", err)
	}
	if err := n.Delete(ctx, "col", []string{"1"}); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := n.Ping(ctx); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}

	results, err := n.Search(ctx, "col", "query", 5)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results from NoopRetriever, got %v", results)
	}
}

func TestNewFromConfig_EmptyDSNReturnsNoop(t *testing.T) {
	r := NewFromConfig(PGVectorConfig{})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever for empty DSN, got %T", r)
	}
}

func TestNewFromConfig_UnreachableReturnsNoop(t *testing.T) {
	r := NewFromConfig(PGVectorConfig{
		DSN:          "postgres://invalid:invalid@127.0.0.1:1/db?connect_timeout=1",
		EmbeddingURL: "http://127.0.0.1:1/embeddings",
	})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever for unreachable store, got %T", r)
	}
}
