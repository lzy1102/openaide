package rag

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
)

func TestNewChroma_EmptyEmbeddingURL(t *testing.T) {
	if _, err := NewChroma(ChromaConfig{URL: "http://localhost:8000"}); err == nil {
		t.Fatal("expected error for empty embedding URL")
	}
}

func TestNewChroma_UnreachableStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	r := NewFromConfig(Config{
		Type:         "chroma",
		EmbeddingURL: srv.URL,
		ChromaURL:    "http://127.0.0.1:1",
	})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever for unreachable Chroma, got %T", r)
	}
}

// mockQueryResult implements v2.QueryResult for parse tests.
type mockQueryResult struct {
	ids   []v2.DocumentIDs
	docs  []v2.Documents
	metas []v2.DocumentMetadatas
	dists []embeddings.Distances
}

func (m *mockQueryResult) GetIDGroups() []v2.DocumentIDs                { return m.ids }
func (m *mockQueryResult) GetDocumentsGroups() []v2.Documents           { return m.docs }
func (m *mockQueryResult) GetMetadatasGroups() []v2.DocumentMetadatas   { return m.metas }
func (m *mockQueryResult) GetEmbeddingsGroups() []embeddings.Embeddings { return nil }
func (m *mockQueryResult) GetDistancesGroups() []embeddings.Distances   { return m.dists }
func (m *mockQueryResult) ToRecordsGroups() []v2.Records                { return nil }
func (m *mockQueryResult) CountGroups() int                             { return len(m.ids) }

func TestParseChromaQuery(t *testing.T) {
	md, err := v2.NewDocumentMetadataFromMap(map[string]interface{}{"lang": "go"})
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	res := &mockQueryResult{
		ids:   []v2.DocumentIDs{{"a", "b"}},
		docs:  []v2.Documents{{v2.NewTextDocument("hello"), v2.NewTextDocument("world")}},
		metas: []v2.DocumentMetadatas{{md, nil}},
		dists: []embeddings.Distances{{0.1, 0.2}},
	}
	got := parseChromaQuery(res)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].ID != "a" || got[0].Content != "hello" || got[0].Metadata["lang"] != "go" || math.Abs(got[0].Score-0.1) > 1e-6 {
		t.Errorf("result[0] = %+v", got[0])
	}
	if got[1].ID != "b" || got[1].Content != "world" {
		t.Errorf("result[1] = %+v", got[1])
	}
}

func TestParseChromaQuery_Empty(t *testing.T) {
	res := &mockQueryResult{ids: nil}
	if got := parseChromaQuery(res); len(got) != 0 {
		t.Errorf("got %d results, want 0", len(got))
	}
}
