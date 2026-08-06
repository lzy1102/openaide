package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTransportCall(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/message" {
			t.Errorf("path = %s, want /message", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req jsonrpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("request not valid JSON-RPC: %v", err)
		}
		if req.Method != "tools/list" {
			t.Errorf("method = %q, want tools/list", req.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
	}))
	defer ts.Close()

	tr, err := newHTTPTransport(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tr.Call("tools/list", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(string(result), "tools") {
		t.Errorf("result = %s, want tools key", result)
	}
}

func TestHTTPTransportSSEPrefix(t *testing.T) {
	// Some servers wrap responses in SSE "data: " format — must be stripped.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `data: {"jsonrpc":"2.0","id":1,"result":{"ok":true}}`)
	}))
	defer ts.Close()

	tr, _ := newHTTPTransport(ts.URL)
	result, err := tr.Call("ping", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(string(result), "ok") {
		t.Errorf("result = %s, want ok key after SSE strip", result)
	}
}

func TestHTTPTransportNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	tr, _ := newHTTPTransport(ts.URL)
	_, err := tr.Call("ping", nil)
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want to mention status 500", err)
	}
}

func TestHTTPTransportErrorCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found"}}`)
	}))
	defer ts.Close()

	tr, _ := newHTTPTransport(ts.URL)
	_, err := tr.Call("bogus", nil)
	if err == nil {
		t.Fatal("expected error for JSON-RPC error response")
	}
	if !strings.Contains(err.Error(), "-32601") {
		t.Errorf("error = %v, want to mention -32601", err)
	}
}

func TestHTTPTransportTrailingSlash(t *testing.T) {
	gotPath := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	}))
	defer ts.Close()

	// Trailing slash must be trimmed so /message is appended correctly.
	tr, _ := newHTTPTransport(ts.URL + "/")
	if _, err := tr.Call("ping", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if gotPath != "/message" {
		t.Errorf("path = %q, want /message (trailing slash trimmed)", gotPath)
	}
}
