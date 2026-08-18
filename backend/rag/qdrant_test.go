package rag

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewQdrant_EmptyEmbeddingURL(t *testing.T) {
	if _, err := NewQdrant(QdrantConfig{Host: "localhost", Port: 6334}); err == nil {
		t.Fatal("expected error for empty embedding URL")
	}
}

func TestNewQdrant_UnreachableStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	// NewQdrant itself does not ping (deferred to factory); it only builds the
	// gRPC client, which is lazy. Verify factory-level degradation instead.
	r := NewFromConfig(Config{
		Type:         "qdrant",
		EmbeddingURL: srv.URL,
		QdrantHost:   "127.0.0.1",
		QdrantPort:   1,
	})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever for unreachable Qdrant, got %T", r)
	}
}

func TestSanitizeCollection(t *testing.T) {
	cases := map[string]string{
		"code":          "code",
		"-bad.start":    "_bad.start",
		"my collection": "my_collection",
		"":              "openaide_docs",
	}
	for in, want := range cases {
		if got := sanitizeCollection(in); got != want {
			t.Errorf("sanitizeCollection(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDocUUID(t *testing.T) {
	a, b := docUUID("path.go:42"), docUUID("path.go:42")
	if a != b {
		t.Errorf("docUUID not deterministic: %s != %s", a, b)
	}
	if len(a) != 36 {
		t.Errorf("docUUID length = %d, want 36 (UUID format)", len(a))
	}
}

func TestQdrantConfigDefaults(t *testing.T) {
	q, err := NewQdrant(QdrantConfig{
		EmbeddingURL: "http://example.com/v1",
		Host:         "",
		Port:         0,
	})
	if err != nil {
		t.Fatalf("NewQdrant: %v", err)
	}
	if q.client == nil {
		t.Fatal("expected non-nil qdrant client")
	}
	_ = context.Background()
}
