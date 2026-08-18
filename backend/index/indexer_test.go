package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		lang string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"component.tsx", "typescript"},
		{"lib.rs", "rust"},
		{"data.json", "json"},
		{"unknown.xyz", "unknown"},
	}
	for _, tt := range tests {
		got := detectLanguage(tt.path)
		if got != tt.lang {
			t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.lang)
		}
	}
}

func TestParseGoSymbols(t *testing.T) {
	goCode := []string{
		"package main",
		"",
		"import \"fmt\"",
		"",
		"func Hello(name string) string {",
		"\treturn fmt.Sprintf(\"Hello, %s\", name)",
		"}",
		"",
		"type Greeter struct {",
		"\tName string",
		"}",
		"",
		"const Version = \"1.0\"",
	}
	symbols := parseGoSymbols("test.go", goCode)
	if len(symbols) == 0 {
		t.Fatal("expected some symbols")
	}

	found := map[string]bool{}
	for _, s := range symbols {
		found[s.Name] = true
	}
	if !found["Hello"] {
		t.Error("expected 'Hello' function")
	}
	if !found["Greeter"] {
		t.Error("expected 'Greeter' type")
	}
	if !found["Version"] {
		t.Error("expected 'Version' constant")
	}
}

func TestIndexer_IndexFile(t *testing.T) {
	dir := t.TempDir()
	idx, err := NewIndexer(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a Go file
	src := filepath.Join(t.TempDir(), "hello.go")
	os.WriteFile(src, []byte("package main\n\nfunc Hello() string { return \"hi\" }\n"), 0644)

	fi, err := idx.IndexFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Language != "go" {
		t.Errorf("expected go, got %s", fi.Language)
	}
	if len(fi.Symbols) == 0 {
		t.Error("expected symbols")
	}
}

func TestIndexer_SearchSymbol(t *testing.T) {
	dir := t.TempDir()
	idx, _ := NewIndexer(dir)

	// Index a file with known symbols
	src := filepath.Join(t.TempDir(), "main.go")
	os.WriteFile(src, []byte("package main\n\nfunc HelloWorld() {}\nfunc GoodbyeWorld() {}\n"), 0644)
	idx.IndexFile(src)

	results := idx.SearchSymbol("HelloWorld")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "HelloWorld" {
		t.Errorf("expected HelloWorld, got %s", results[0].Name)
	}
}

func TestIndexer_GetStats(t *testing.T) {
	dir := t.TempDir()
	idx, _ := NewIndexer(dir)

	stats := idx.GetStats()
	if _, ok := stats["total_files"]; !ok {
		t.Error("expected total_files in stats")
	}
}
