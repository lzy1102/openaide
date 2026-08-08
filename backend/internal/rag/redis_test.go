package rag

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRedis_EmptyEmbeddingURL(t *testing.T) {
	if _, err := NewRedisRetriever(RedisConfig{Addr: "localhost:6379"}); err == nil {
		t.Fatal("expected error for empty embedding URL")
	}
}

func TestNewRedis_UnreachableStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	r := NewFromConfig(Config{
		Type:         "redis",
		EmbeddingURL: srv.URL,
		RedisAddr:    "127.0.0.1:1",
	})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever for unreachable Redis, got %T", r)
	}
}

func TestParseSearchResults(t *testing.T) {
	raw := []interface{}{
		int64(2),
		"openaide:code:doc:a", []interface{}{"content", "hello", "metadata", `{"lang":"go"}`, "score", "0.25"},
		"openaide:code:doc:b", []interface{}{"content", "world", "metadata", `{}`, "score", "0.5"},
	}
	got, err := parseSearchResults("code", raw)
	if err != nil {
		t.Fatalf("parseSearchResults: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].ID != "a" || got[0].Content != "hello" || got[0].Metadata["lang"] != "go" || got[0].Score != 0.25 {
		t.Errorf("result[0] = %+v", got[0])
	}
	if got[1].ID != "b" || got[1].Content != "world" {
		t.Errorf("result[1] = %+v", got[1])
	}
}

func TestParseSearchResults_Empty(t *testing.T) {
	got, err := parseSearchResults("code", []interface{}{int64(0)})
	if err != nil {
		t.Fatalf("parseSearchResults: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d results, want 0", len(got))
	}
}

func TestFloat32Bytes(t *testing.T) {
	b := float32Bytes([]float32{1.0, 0.5})
	if len(b) != 8 {
		t.Fatalf("bytes length = %d, want 8", len(b))
	}
	if b[0] != 0x00 || b[3] != 0x3f { // little-endian 1.0 = 0x3f800000
		t.Errorf("unexpected bytes: %v", b)
	}
}
