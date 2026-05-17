package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"openaide/backend/internal/kernel"
)

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	def := kernel.ToolDefinition{
		Type: "function",
		Function: kernel.FunctionDef{Name: "test_tool", Description: "test"},
	}
	err := r.Register(def, func(ctx context.Context, args string) (*kernel.ToolResult, error) {
		return &kernel.ToolResult{Content: "ok"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Count() != 1 {
		t.Errorf("expected 1, got %d", r.Count())
	}
}

func TestRegistry_Execute(t *testing.T) {
	r := NewRegistry()
	r.Register(kernel.ToolDefinition{
		Type: "function",
		Function: kernel.FunctionDef{Name: "echo"},
	}, func(ctx context.Context, args string) (*kernel.ToolResult, error) {
		return &kernel.ToolResult{Content: args}, nil
	})
	result, err := r.Execute(context.Background(), kernel.ToolCall{
		Function: kernel.FunctionCall{Name: "echo", Arguments: `{"msg":"hello"}`},
	}, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != `{"msg":"hello"}` {
		t.Errorf("unexpected: %v", result.Content)
	}
}

func TestBuiltinTools_AllHaveHandlers(t *testing.T) {
	tools := BuiltinTools()
	handlers := BuiltinHandlers()
	if len(tools) < 14 {
		t.Errorf("expected at least 14 tools, got %d", len(tools))
	}
	for _, tool := range tools {
		if _, ok := handlers[tool.Function.Name]; !ok {
			t.Errorf("tool %s has no handler", tool.Function.Name)
		}
	}
}

func TestHandleReadFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	os.WriteFile(file, []byte("line1\nline2\nline3"), 0644)
	result, _ := handleReadFile(context.Background(), `{"path":"`+file+`"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
}

func TestHandleWriteFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "out.txt")
	result, _ := handleWriteFile(context.Background(), `{"path":"`+file+`","content":"hello world"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "hello world" {
		t.Error("content mismatch")
	}
}

func TestHandleListDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	result, _ := handleListDirectory(context.Background(), `{"path":"`+dir+`"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
}

func TestHandleSearchFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("package main\nfunc main(){}"), 0644)
	result, _ := handleSearchFiles(context.Background(), `{"pattern":"func main","path":"`+dir+`"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
}

func TestHandleExecuteCommand(t *testing.T) {
	result, _ := handleExecuteCommand(context.Background(), `{"command":"echo hello"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
}

func TestHandleDiffEdit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	os.WriteFile(file, []byte("old code here"), 0644)
	result, _ := handleDiffEdit(context.Background(), `{"path":"`+file+`","search_text":"old code here","replace_text":"new code here"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "new code here" {
		t.Errorf("diff edit failed: got %q", data)
	}
}

func TestHandleDiffEdit_Duplicate(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "dup.go")
	os.WriteFile(file, []byte("same\nsame"), 0644)
	result, _ := handleDiffEdit(context.Background(), `{"path":"`+file+`","search_text":"same","replace_text":"diff"}`)
	if result.Error == "" {
		t.Error("should reject duplicate match")
	}
}

func TestSafeAbsPath(t *testing.T) {
	p, err := safeAbsPath(".")
	if err != nil {
		t.Fatal(err)
	}
	if p == "" || p == "." {
		t.Error("expected absolute path")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0B"}, {500, "500B"}, {1024, "1.0K"}, {1048576, "1.0M"}, {1073741824, "1.0G"},
	}
	for _, tc := range tests {
		got := formatBytes(tc.n)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %s, want %s", tc.n, got, tc.want)
		}
	}
}
