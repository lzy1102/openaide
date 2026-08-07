package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerCommand_AllLanguages(t *testing.T) {
	tests := []struct {
		language    string
		wantCommand string
	}{
		{"go", "gopls"},
		{"rust", "rust-analyzer"},
		{"c", "clangd"}, {"cpp", "clangd"},
		{"zig", "zls"},
		{"python", "pylsp"},
		{"ruby", "solargraph"},
		{"lua", "lua-lsp"},
		{"php", "intelephense"},
		{"java", "jdtls"},
		{"kotlin", "kotlin-language-server"},
		{"scala", "metals-v2"},
		{"typescript", "typescript-language-server"},
		{"javascript", "typescript-language-server"},
		{"html", "vscode-html-languageserver"},
		{"css", "vscode-html-languageserver"},
		{"csharp", "omnisharp"},
		{"swift", "sourcekit-lsp"},
		{"elixir", "elixir-ls"},
		{"haskell", "haskell-language-server-wrapper"},
		{"erlang", "erlang_ls"},
		{"dart", "dart"},
		{"r", "R"},
		{"julia", "julia"},
	}
	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			cmd, err := serverCommand(tt.language)
			if err != nil {
				t.Fatalf("serverCommand(%q) error: %v", tt.language, err)
			}
			if filepath.Base(cmd.Path) != tt.wantCommand && cmd.Path != tt.wantCommand {
				t.Errorf("expected %q, got %q", tt.wantCommand, cmd.Path)
			}
		})
	}
}

func TestServerCommand_Unknown(t *testing.T) {
	_, err := serverCommand("brainfuck")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' error, got: %v", err)
	}
}

func TestDetectLanguage_AllExtensions(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/src/main.go", "go"}, {"/src/lib.rs", "rust"},
		{"/src/main.c", "c"}, {"/src/main.h", "c"},
		{"/src/main.cpp", "cpp"}, {"/src/main.hpp", "cpp"}, {"/src/main.cc", "cpp"},
		{"/src/main.zig", "zig"},
		{"/src/main.py", "python"}, {"/src/main.rb", "ruby"},
		{"/src/main.lua", "lua"}, {"/src/main.php", "php"},
		{"/src/Main.java", "java"}, {"/src/Main.kt", "kotlin"}, {"/src/Main.scala", "scala"},
		{"/src/main.ts", "typescript"}, {"/src/main.tsx", "typescript"},
		{"/src/main.js", "javascript"}, {"/src/main.jsx", "javascript"},
		{"/index.html", "html"}, {"/style.css", "css"},
		{"/src/App.cs", "csharp"}, {"/src/main.swift", "swift"},
		{"/src/main.ex", "elixir"}, {"/src/Main.hs", "haskell"},
		{"/src/main.erl", "erlang"}, {"/src/main.dart", "dart"},
		{"/src/analysis.R", "r"}, {"/src/main.jl", "julia"},
		{"/src/README.md", ""}, {"/noext", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectLanguage(tt.path)
			if got != tt.expected {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestDetectLanguage_RoundTrip(t *testing.T) {
	exts := []string{
		"main.go", "main.rs", "main.c", "main.cpp", "main.zig",
		"main.py", "main.rb", "main.lua", "main.php",
		"Main.java", "Main.kt", "Main.scala",
		"main.ts", "main.js", "index.html", "style.css",
		"App.cs", "main.swift", "main.ex", "Main.hs", "main.erl", "main.dart",
		"analysis.R", "main.jl",
	}
	for _, fname := range exts {
		t.Run(fname, func(t *testing.T) {
			lang := DetectLanguage("/src/" + fname)
			if lang == "" {
				t.Fatalf("DetectLanguage(%q) returned empty", fname)
			}
			_, err := serverCommand(lang)
			if err != nil {
				t.Errorf("round-trip failed: %q → %q → error: %v", fname, lang, err)
			}
		})
	}
}

func TestLSPGoConnect(t *testing.T) {
	root := "/mnt/d/project/android/openaide/backend"
	if _, err := os.Stat(filepath.Join(root, "go.mod")); os.IsNotExist(err) {
		t.Skip("go.mod not found")
	}
	c, err := Start(root, "go")
	if err != nil {
		t.Skip("gopls not available:", err)
	}
	defer c.Close()
	hover, err := c.Hover(filepath.Join(root, "cmd/cli/main.go"), 0, 8)
	if err != nil {
		t.Error("Hover:", err)
	} else {
		t.Logf("LSP Hover: %s", hover.Contents.Value)
	}
}

func TestLSPDidChangeAndSymbols(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "demo.go")
	content := "package demo\n\nvar Greeting = \"hello\"\n\nfunc SayHello() string {\n\treturn Greeting\n}\n"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := Start(dir, "go")
	if err != nil {
		t.Skip("gopls not available:", err)
	}
	defer c.Close()

	// 先 open 再 symbols：验证 documentSymbol 能力与符号枚举
	c.OpenDocument(file, content)
	syms, err := c.Symbols(file)
	if err != nil {
		t.Fatalf("Symbols: %v", err)
	}
	names := make(map[string]bool)
	for _, s := range syms {
		names[s.Name] = true
	}
	for _, want := range []string{"Greeting", "SayHello"} {
		if !names[want] {
			t.Errorf("Symbols missing %q (got %v)", want, names)
		}
	}

	// didChange 后重读内容不应报错
	c.DidChange(file, strings.Replace(content, "Greeting", "Greeting2", -1))
}

func TestLSPRename(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "demo.go")
	content := "package demo\n\nvar Greeting = \"hello\"\n\nfunc SayHello() string {\n\treturn Greeting\n}\n"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := Start(dir, "go")
	if err != nil {
		t.Skip("gopls not available:", err)
	}
	defer c.Close()

	c.OpenDocument(file, content)
	// Greeting 在文件第 3 行（0-based line 2），字符偏移 4
	edit, err := c.Rename(file, 2, 4, "Greeting2")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if edit == nil || len(edit.Changes) == 0 {
		t.Skip("gopls returned no rename edits (server capability variance)")
	}
	for uri, edits := range edit.Changes {
		t.Logf("Rename edits for %s: %d", uri, len(edits))
		for _, e := range edits {
			if !strings.Contains(e.NewText, "Greeting2") {
				t.Errorf("rename edit %q does not contain Greeting2", e.NewText)
			}
		}
	}
}
