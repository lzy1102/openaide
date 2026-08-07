// Package rag provides a retrieval-augmented generation client interface.
//
// OpenAIDE does not implement retrieval itself (no local embedding, no
// vector search, no TF-IDF). All retrieval is delegated to an external
// store through the Retriever interface. The pgvector implementation
// (pgvector.go) owns embedding generation and vector search entirely.
package rag

import "context"

// Document is a retrievable unit of text.
type Document struct {
	ID       string            // stable identifier (e.g. "path.go:42-87")
	Content  string            // full text
	Metadata map[string]string // optional filters (path, symbol, language...)
}

// Result is a retrieved document with a similarity score.
type Result struct {
	ID       string
	Content  string
	Score    float64
	Metadata map[string]string
}

// Retriever is the external retrieval contract. Implementations must be
// safe for concurrent use.
type Retriever interface {
	// Index writes or replaces documents in a collection.
	Index(ctx context.Context, collection string, docs []Document) error

	// Search returns the top-k documents most similar to query.
	// Returns an empty slice (not an error) when the store is unreachable.
	Search(ctx context.Context, collection, query string, limit int) ([]Result, error)

	// Delete removes documents by ID from a collection.
	Delete(ctx context.Context, collection string, ids []string) error

	// Ping reports whether the backing store is reachable.
	Ping(ctx context.Context) error
}

// NoopRetriever is the default when no external store is configured.
// It always returns empty results so the agent degrades gracefully
// instead of failing retrieval-dependent flows.
type NoopRetriever struct{}

func (NoopRetriever) Index(context.Context, string, []Document) error { return nil }
func (NoopRetriever) Search(context.Context, string, string, int) ([]Result, error) {
	return nil, nil
}
func (NoopRetriever) Delete(context.Context, string, []string) error { return nil }
func (NoopRetriever) Ping(context.Context) error                     { return nil }
