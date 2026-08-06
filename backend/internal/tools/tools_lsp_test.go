package tools

import (
	"context"
	"strings"
	"testing"
)

// LSP tools need a live LSP server; tests assert graceful degradation
// (clear errors) when no server is registered for a file type.

func TestLSPHandlersArgValidation(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		call func() string
		want string
	}{
		{"definition_missing_file", func() string { r, _ := handleLSPDefinition(ctx, `{}`); return r.Error }, "file parameter required"},
		{"references_missing_file", func() string { r, _ := handleLSPReferences(ctx, `{}`); return r.Error }, "file parameter required"},
		{"hover_missing_file", func() string { r, _ := handleLSPHover(ctx, `{}`); return r.Error }, "file parameter required"},
		{"definition_bad_json", func() string { r, _ := handleLSPDefinition(ctx, `{oops`); return r.Error }, ""},
		{"diagnostics_bad_json", func() string { r, _ := handleLSPDiagnostics(ctx, `{oops`); return r.Error }, ""},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, c.want) {
			t.Errorf("%s: error = %q, want containing %q", c.name, msg, c.want)
		}
	}
}

func TestLSPHandlersNoServer(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		call func() string
		want string
	}{
		{"definition", func() string {
			r, _ := handleLSPDefinition(ctx, `{"file":"/tmp/foo.go","line":1,"character":0}`)
			return r.Error
		}, "no LSP server"},
		{"references", func() string {
			r, _ := handleLSPReferences(ctx, `{"file":"/tmp/foo.py","line":1,"character":0}`)
			return r.Error
		}, "no LSP server"},
		{"hover", func() string {
			r, _ := handleLSPHover(ctx, `{"file":"/tmp/foo.ts","line":1,"character":0}`)
			return r.Error
		}, "no LSP server"},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, c.want) {
			t.Errorf("%s: error = %q, want %q", c.name, msg, c.want)
		}
	}
}

func TestLSPDiagnosticsNoClients(t *testing.T) {
	// Active clients is a package-level map; ensure the test does not
	// depend on what other tests registered.
	ctx := context.Background()
	r, _ := handleLSPDiagnostics(ctx, `{"file":"/tmp/foo.go"}`)
	if r.Error != "" {
		t.Fatalf("diagnostics should never error: %v", r.Error)
	}
	if !strings.Contains(r.Content.(string), "No diagnostics") {
		t.Errorf("content = %q, want 'No diagnostics yet'", r.Content)
	}
}

func TestSymbolHandlersArgValidation(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		call func() string
		want string
	}{
		{"search_symbols_missing_query", func() string { r, _ := handleSearchSymbols(ctx, `{}`); return r.Error }, "query is required"},
		{"search_symbols_bad_json", func() string { r, _ := handleSearchSymbols(ctx, `{oops`); return r.Error }, ""},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, c.want) {
			t.Errorf("%s: error = %q, want containing %q", c.name, msg, c.want)
		}
	}
}
