package rag

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
)

func TestNewMilvus_EmptyEmbeddingURL(t *testing.T) {
	if _, err := NewMilvus(MilvusConfig{Address: "localhost:19530"}); err == nil {
		t.Fatal("expected error for empty embedding URL")
	}
}

func TestNewMilvus_UnreachableStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	r := NewFromConfig(Config{
		Type:           "milvus",
		EmbeddingURL:   srv.URL,
		MilvusAddress:  "127.0.0.1:1",
		MilvusUsername: "",
		MilvusPassword: "",
	})
	if _, ok := r.(NoopRetriever); !ok {
		t.Errorf("expected NoopRetriever for unreachable Milvus, got %T", r)
	}
}

func TestMilvusConfigDefaults(t *testing.T) {
	// NewMilvus dials with grpc.WithBlock and now a 5s deadline, so the dial
	// must reach a live endpoint. A bare gRPC server satisfies the handshake;
	// the client's Connect RPC then returns Unimplemented, which the SDK
	// treats as a legacy server and accepts (no error, non-nil client).
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	m, err := NewMilvus(MilvusConfig{
		EmbeddingURL: "http://example.com/v1",
		Address:      lis.Addr().String(),
	})
	if err != nil {
		t.Fatalf("NewMilvus: %v", err)
	}
	if m.client == nil {
		t.Fatal("expected non-nil milvus client")
	}
	if m.collection != "openaide_docs" {
		t.Errorf("collection = %q, want openaide_docs", m.collection)
	}
}
