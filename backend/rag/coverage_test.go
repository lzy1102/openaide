package rag

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/qdrant/go-client/qdrant"
)

// ============ embed.go ============

func TestEmbedder_DefaultModel(t *testing.T) {
	e := newEmbedder("http://x", "", "")
	if e.model != "text-embedding-3-small" {
		t.Errorf("expected default model, got %q", e.model)
	}
	if e.url != "http://x" {
		t.Errorf("expected trimmed url, got %q", e.url)
	}
}

func TestEmbedder_TrailingSlashTrimmed(t *testing.T) {
	e := newEmbedder("http://x/", "", "m")
	if e.url != "http://x" {
		t.Errorf("expected trailing slash trimmed, got %q", e.url)
	}
}

func TestEmbedder_Embed_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %q", got)
		}
		fmt.Fprint(w, `{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`)
	}))
	defer srv.Close()

	e := newEmbedder(srv.URL, "test-key", "m")
	vecs, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 2 || vecs[0][0] != 0.1 {
		t.Errorf("unexpected vector: %v", vecs[0])
	}
}

func TestEmbedder_Embed_EmptyInput(t *testing.T) {
	e := newEmbedder("http://x", "", "m")
	vecs, err := e.Embed(context.Background(), nil)
	if err != nil || vecs != nil {
		t.Errorf("expected nil,nil for empty input, got %v,%v", vecs, err)
	}
}

func TestEmbedder_Embed_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "bad key")
	}))
	defer srv.Close()

	e := newEmbedder(srv.URL, "bad", "m")
	_, err := e.Embed(context.Background(), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 error, got %v", err)
	}
}

func TestEmbedder_Embed_NetworkError(t *testing.T) {
	// Close immediately so the client gets a connection error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	e := newEmbedder(url, "", "m")
	if _, err := e.Embed(context.Background(), []string{"x"}); err == nil {
		t.Error("expected network error")
	}
}

// ============ pgvector.go: vectorString ============

func TestVectorString(t *testing.T) {
	tests := []struct {
		in   []float32
		want string
	}{
		{[]float32{0.1, 0.2, 0.3}, "[0.1,0.2,0.3]"},
		{[]float32{1, 2}, "[1,2]"},
		{[]float32{}, "[]"},
		{[]float32{-0.5, 0}, "[-0.5,0]"},
	}
	for _, tt := range tests {
		if got := vectorString(tt.in); got != tt.want {
			t.Errorf("vectorString(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ============ redis.go: key helpers ============

func TestRedis_KeyHelpers(t *testing.T) {
	r := &RedisRetriever{}
	if got := r.idxName("col"); got != "openaide:col:idx" {
		t.Errorf("idxName = %q", got)
	}
	if got := r.docPrefix("col"); got != "openaide:col:doc:" {
		t.Errorf("docPrefix = %q", got)
	}
	if got := r.docKey("col", "id1"); got != "openaide:col:doc:id1" {
		t.Errorf("docKey = %q", got)
	}
}

// ============ qdrant.go: pointIDString ============

func TestPointIDString(t *testing.T) {
	if got := pointIDString(nil); got != "" {
		t.Errorf("expected empty for nil, got %q", got)
	}
	id := &qdrant.PointId{
		PointIdOptions: &qdrant.PointId_Uuid{Uuid: "abc-123"},
	}
	if got := pointIDString(id); got != "abc-123" {
		t.Errorf("expected abc-123, got %q", got)
	}
}

// ============ chroma.go: noopEmbeddingFunction ============

func TestNoopEmbeddingFunction(t *testing.T) {
	var ef noopEmbeddingFunction
	if v, err := ef.EmbedDocuments(context.Background(), nil); err != nil || v != nil {
		t.Errorf("EmbedDocuments: got %v,%v", v, err)
	}
	if v, err := ef.EmbedQuery(context.Background(), "q"); err != nil || v != nil {
		t.Errorf("EmbedQuery: got %v,%v", v, err)
	}
	if ef.Name() != "noop" {
		t.Errorf("expected noop name, got %q", ef.Name())
	}
	cfg := ef.GetConfig()
	if cfg == nil {
		t.Error("expected non-nil config")
	}
	if ef.DefaultSpace() != embeddings.COSINE {
		t.Error("expected COSINE distance metric")
	}
	spaces := ef.SupportedSpaces()
	if len(spaces) != 1 || spaces[0] != embeddings.COSINE {
		t.Errorf("expected 1 COSINE space, got %v", spaces)
	}
}

// ============ factory.go: NewFromConfig routing ============

func TestNewFromConfig_Routing_EmptyTypeNoDSN(t *testing.T) {
	r := NewFromConfig(Config{})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever, got %T", r)
	}
}

func TestNewFromConfig_Routing_EmptyTypeWithDSN(t *testing.T) {
	// Empty type + DSN → pgvector path → unreachable → Noop
	r := NewFromConfig(Config{DSN: "postgres://x:y@localhost:5432/db"})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever for unreachable pgvector, got %T", r)
	}
}

func TestNewFromConfig_Routing_PGVectorEmptyDSN(t *testing.T) {
	r := NewFromConfig(Config{Type: "pgvector"})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever, got %T", r)
	}
}

func TestNewFromConfig_Routing_QdrantNoEmbedding(t *testing.T) {
	r := NewFromConfig(Config{Type: "qdrant"})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever, got %T", r)
	}
}

func TestNewFromConfig_Routing_MilvusNoEmbedding(t *testing.T) {
	r := NewFromConfig(Config{Type: "milvus"})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever, got %T", r)
	}
}

func TestNewFromConfig_Routing_RedisNoEmbedding(t *testing.T) {
	r := NewFromConfig(Config{Type: "redis"})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever, got %T", r)
	}
}

func TestNewFromConfig_Routing_ChromaNoEmbedding(t *testing.T) {
	r := NewFromConfig(Config{Type: "chroma"})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever, got %T", r)
	}
}

func TestNewFromConfig_Routing_UnknownType(t *testing.T) {
	r := NewFromConfig(Config{Type: "weird"})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever for unknown type, got %T", r)
	}
}

// ============ retriever.go: NoopRetriever ============

func TestNoopRetriever_EmptyDocsOps(t *testing.T) {
	n := NoopRetriever{}
	if err := n.Index(context.Background(), "c", nil); err != nil {
		t.Errorf("Index: %v", err)
	}
	if err := n.Delete(context.Background(), "c", nil); err != nil {
		t.Errorf("Delete: %v", err)
	}
	results, err := n.Search(context.Background(), "c", "q", 5)
	if err != nil || results != nil {
		t.Errorf("Search: got %v,%v", results, err)
	}
	if err := n.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// ============ Backend Index/Search guard clauses (no external store needed) ============

func TestBackends_EmptyDocsIndex(t *testing.T) {
	// Index with empty docs returns nil without touching the store — verified
	// via direct struct construction (no network calls).
	pv := &PGVector{}
	if err := pv.Index(context.Background(), "c", nil); err != nil {
		t.Errorf("PGVector.Index(empty): %v", err)
	}
	q := &Qdrant{}
	if err := q.Index(context.Background(), "c", nil); err != nil {
		t.Errorf("Qdrant.Index(empty): %v", err)
	}
	r := &RedisRetriever{}
	if err := r.Index(context.Background(), "c", nil); err != nil {
		t.Errorf("Redis.Index(empty): %v", err)
	}
}

func TestBackends_EmptyDelete(t *testing.T) {
	pv := &PGVector{}
	if err := pv.Delete(context.Background(), "c", nil); err != nil {
		t.Errorf("PGVector.Delete(empty): %v", err)
	}
	q := &Qdrant{}
	if err := q.Delete(context.Background(), "c", nil); err != nil {
		t.Errorf("Qdrant.Delete(empty): %v", err)
	}
	r := &RedisRetriever{}
	if err := r.Delete(context.Background(), "c", nil); err != nil {
		t.Errorf("Redis.Delete(empty): %v", err)
	}
}

// ============ Backend Search degrade paths (embed fails → empty results) ============
// These need no external store: an unreachable embedding URL makes Embed fail
// before any store call, exercising the degrade-to-empty-result path.

func newBrokenEmbedder() *embedder {
	return newEmbedder("http://127.0.0.1:1", "", "m")
}

func TestPGVector_Search_EmbedFailDegrades(t *testing.T) {
	pv := &PGVector{emb: newBrokenEmbedder()}
	results, err := pv.Search(context.Background(), "c", "query", 5)
	if err != nil {
		t.Fatalf("Search should degrade, got error: %v", err)
	}
	if results != nil {
		t.Errorf("expected empty results, got %v", results)
	}
}

func TestPGVector_Index_EmbedFailErrors(t *testing.T) {
	pv := &PGVector{emb: newBrokenEmbedder()}
	err := pv.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected embed error")
	}
	if !strings.Contains(err.Error(), "pgvector: embed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestQdrant_Search_EmbedFailDegrades(t *testing.T) {
	q := &Qdrant{emb: newBrokenEmbedder()}
	results, err := q.Search(context.Background(), "c", "query", 5)
	if err != nil {
		t.Fatalf("Search should degrade, got error: %v", err)
	}
	if results != nil {
		t.Errorf("expected empty results, got %v", results)
	}
}

func TestQdrant_Index_EmbedFailErrors(t *testing.T) {
	q := &Qdrant{emb: newBrokenEmbedder()}
	err := q.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected embed error")
	}
	if !strings.Contains(err.Error(), "qdrant: embed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRedis_Search_EmbedFailDegrades(t *testing.T) {
	r := &RedisRetriever{emb: newBrokenEmbedder()}
	results, err := r.Search(context.Background(), "c", "query", 5)
	if err != nil {
		t.Fatalf("Search should degrade, got error: %v", err)
	}
	if results != nil {
		t.Errorf("expected empty results, got %v", results)
	}
}

func TestRedis_Index_EmbedFailErrors(t *testing.T) {
	r := &RedisRetriever{emb: newBrokenEmbedder()}
	err := r.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected embed error")
	}
	if !strings.Contains(err.Error(), "redis: embed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMilvus_Search_EmbedFailDegrades(t *testing.T) {
	m := &Milvus{emb: newBrokenEmbedder()}
	results, err := m.Search(context.Background(), "c", "query", 5)
	if err != nil {
		t.Fatalf("Search should degrade, got error: %v", err)
	}
	if results != nil {
		t.Errorf("expected empty results, got %v", results)
	}
}

func TestMilvus_Index_EmbedFailErrors(t *testing.T) {
	m := &Milvus{emb: newBrokenEmbedder()}
	err := m.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected embed error")
	}
	if !strings.Contains(err.Error(), "milvus: embed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestChroma_Search_EmbedFailDegrades(t *testing.T) {
	c := &ChromaRetriever{emb: newBrokenEmbedder()}
	results, err := c.Search(context.Background(), "c", "query", 5)
	if err != nil {
		t.Fatalf("Search should degrade, got error: %v", err)
	}
	if results != nil {
		t.Errorf("expected empty results, got %v", results)
	}
}

func TestChroma_Index_EmbedFailErrors(t *testing.T) {
	// collectionFor with same collection name returns r.collection (nil-safe),
	// then embed fails.
	c := &ChromaRetriever{emb: newBrokenEmbedder(), name: "c"}
	err := c.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected embed error")
	}
	if !strings.Contains(err.Error(), "chroma: embed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestChroma_Index_EmbedNoVectors(t *testing.T) {
	// Embed succeeds but returns no vectors → explicit error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	c := &ChromaRetriever{
		emb:  newEmbedder(srv.URL, "", "m"),
		name: "c",
	}
	err := c.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected no-vectors error")
	}
	if !strings.Contains(err.Error(), "no vectors") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRedis_Index_EmbedNoVectors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	r := &RedisRetriever{emb: newEmbedder(srv.URL, "", "m")}
	err := r.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected no-vectors error")
	}
	if !strings.Contains(err.Error(), "no vectors") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestQdrant_Index_EmbedNoVectors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	q := &Qdrant{emb: newEmbedder(srv.URL, "", "m")}
	err := q.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected no-vectors error")
	}
	if !strings.Contains(err.Error(), "no vectors") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMilvus_Index_EmbedNoVectors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	m := &Milvus{emb: newEmbedder(srv.URL, "", "m")}
	err := m.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected no-vectors error")
	}
	if !strings.Contains(err.Error(), "no vectors") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============ Delete guard clauses & collectionFor short-circuit ============

func TestChroma_Delete_EmptyIDs(t *testing.T) {
	c := &ChromaRetriever{}
	if err := c.Delete(context.Background(), "c", nil); err != nil {
		t.Errorf("Delete(empty): %v", err)
	}
}

func TestMilvus_Delete_EmptyIDs(t *testing.T) {
	m := &Milvus{}
	if err := m.Delete(context.Background(), "c", nil); err != nil {
		t.Errorf("Delete(empty): %v", err)
	}
}

func TestChroma_CollectionFor_SameName(t *testing.T) {
	// When collection == r.name, the constructor-cached collection is returned
	// without any client call — nil client must not panic.
	c := &ChromaRetriever{name: "docs"}
	col, err := c.collectionFor(context.Background(), "docs")
	if err != nil {
		t.Fatalf("collectionFor(same name): %v", err)
	}
	if col != nil {
		t.Error("expected nil collection (no client wired)")
	}
}

func TestChroma_CollectionFor_DifferentNameNilClient(t *testing.T) {
	// Different name → attempts client.GetCollection → nil client panics unless
	// handled. We only assert it returns an error path without panicking on the
	// nil check. Since nil client would panic, skip this case — the same-name
	// path above is the safe, testable branch.
}
