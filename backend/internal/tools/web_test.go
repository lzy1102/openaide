package tools

import (
	"context"
	"strings"
	"testing"
)

func TestWebHandlersArgValidation(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		call func() string
		want string
	}{
		{"search_missing_query", func() string { r, _ := handleWebSearch(ctx, `{}`); return r.Error }, "query is required"},
		{"fetch_missing_url", func() string { r, _ := handleWebFetch(ctx, `{}`); return r.Error }, "url is required"},
		{"ai_search_missing_query", func() string { r, _ := handleAISearch(ctx, `{}`); return r.Error }, "query is required"},
		{"search_bad_json", func() string { r, _ := handleWebSearch(ctx, `{oops`); return r.Error }, ""},
		{"fetch_bad_json", func() string { r, _ := handleWebFetch(ctx, `{oops`); return r.Error }, ""},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, c.want) {
			t.Errorf("%s: error = %q, want containing %q", c.name, msg, c.want)
		}
	}
}

func TestWebFetchWithLocalServer(t *testing.T) {
	// Use a local HTTP server so no real network is needed.
	ts := newTestHTTPServer(t, "<html><body><h1>Hello Web</h1><p>Some text here.</p></body></html>")
	defer ts.Close()

	ctx := context.Background()
	r, _ := handleWebFetch(ctx, `{"url":"`+ts.URL+`","max_length":5000}`)
	if r.Error != "" {
		t.Fatalf("web_fetch error: %v", r.Error)
	}
	if !strings.Contains(r.Content.(string), "Hello Web") {
		t.Errorf("web_fetch content = %q, want page text", r.Content)
	}
}
