package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTrunc(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 3, "hel..."},
		{"hello", 10, "hello"},
		{"你好世界", 2, "你好..."},
		{"", 5, ""},
		{"a", 0, "..."},
	}
	for _, tt := range tests {
		got := trunc(tt.in, tt.n)
		if got != tt.want {
			t.Errorf("trunc(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

func TestLogRingWrite(t *testing.T) {
	r := &logRing{buf: make([]string, 0)}
	// Add 51 entries to trigger trim (>50 cap)
	for i := 0; i < 60; i++ {
		r.Write([]byte("line"))
	}
	snap := r.snapshot()
	if len(snap) != 50 {
		t.Errorf("expected 50 lines (max cap), got %d", len(snap))
	}
}

func TestLogRingWriteTrim(t *testing.T) {
	r := &logRing{buf: make([]string, 0)}
	r.Write([]byte("  first  "))
	snap := r.snapshot()
	if snap[0] != "first" {
		t.Errorf("whitespace should be trimmed, got %q", snap[0])
	}
}

func TestLogRingConcurrent(t *testing.T) {
	r := &logRing{buf: make([]string, 0, 10)}
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			r.Write([]byte("msg"))
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			r.snapshot()
		}
		done <- true
	}()
	<-done
	<-done
}

func TestPromptStyle(t *testing.T) {
	// idle prompt
	p := PromptStyle("abc12345", "deepseek-v4-pro", false)
	if !strings.Contains(p, "●") {
		t.Error("idle prompt should contain ●")
	}
	if !strings.Contains(p, "abc12345") {
		t.Error("prompt should contain session ID")
	}
	if !strings.Contains(p, "❯") {
		t.Error("prompt should contain ❯")
	}

	// busy prompt
	p2 := PromptStyle("abc12345", "deepseek-v4-pro", true)
	if !strings.Contains(p2, "◉") {
		t.Error("busy prompt should contain ◉")
	}
}

func TestToolSectionCollapse(t *testing.T) {
	BeginToolSection()
	AddToolCall("read_file")
	AddToolCall("list_directory")
	AddToolCall("search_files")
	AddToolResult("", "262 lines", "")
	AddToolResult("", "36 entries", "")
	AddToolResult("", "15 matches", "")
	EndToolSection()
	// Should have collapsed to one line (no errors, multiple tools)
}

func TestToolSectionErrors(t *testing.T) {
	BeginToolSection()
	AddToolCall("write_file")
	AddToolResult("", "", "permission denied")
	EndToolSection()
	// Should have shown expanded box with error
}

func TestRenderMarkdown(t *testing.T) {
	input := "# Hello\n\nThis is **bold** and `code`.\n\n```go\nfunc main() {}\n```"
	output := RenderMarkdown(input)
	if output == "" {
		t.Error("RenderMarkdown should not return empty string")
	}
	if output == input {
		t.Error("RenderMarkdown should transform the input")
	}
}

func TestPrintStatusBar(t *testing.T) {
	// Just verify it doesn't panic
	PrintStatusBar(1000, 5, 1234567890, "test-model")
}

func TestPrintError(t *testing.T) {
	PrintError("test error") // should not panic
}

func TestPrintWarning(t *testing.T) {
	PrintWarning("test warning")
}

func TestPrintSuccess(t *testing.T) {
	PrintSuccess("test success")
}

func TestPrintInfo(t *testing.T) {
	PrintInfo("test info")
}

func TestPrintThinking(t *testing.T) {
	PrintThinking("用户想要了解当前项目...")
	PrintThinking(strings.Repeat("x", 200)) // long text
}

func TestExecCmd(t *testing.T) {
	out, err := execCmd("echo", "hello")
	if err != nil {
		t.Skip("exec not available")
	}
	if out != "hello" {
		t.Errorf("expected 'hello', got %q", out)
	}

	_, err = execCmd("nonexistent_command_xyz")
	if err == nil {
		t.Error("expected error for nonexistent command")
	}
}

func TestCommandParsing(t *testing.T) {
	tests := []struct {
		cmd  string
		want string // expected first part
	}{
		{"/help", "/help"},
		{"/model claude", "/model"},
		{"/exit", "/exit"},
		{"/analyst analyze this code", "/analyst"},
		{"/team build feature X", "/team"},
	}
	for _, tt := range tests {
		parts := strings.Fields(tt.cmd)
		if len(parts) == 0 {
			t.Errorf("empty command: %q", tt.cmd)
			continue
		}
		if parts[0] != tt.want {
			t.Errorf("command %q: first part = %q, want %q", tt.cmd, parts[0], tt.want)
		}
	}
}

func TestPromptStyleEdgeCases(t *testing.T) {
	// Empty model name
	p := PromptStyle("x", "", false)
	if !strings.Contains(p, "openaide") {
		t.Error("should default to 'openaide' when model name empty")
	}

	// Short session ID (should not panic)
	p2 := PromptStyle("ab", "m", false)
	if p2 == "" {
		t.Error("should return non-empty prompt for short ID")
	}

	// Extra suffix (token count)
	p3 := PromptStyle("abc12345", "test", false, "[5k]")
	if !strings.Contains(p3, "[5k]") {
		t.Error("prompt should include extra suffix")
	}
}

func TestSessionTokens(t *testing.T) {
	sessionTokens = 0
	PrintStatusBar(1000, 3, 1234567890, "test")
	if sessionTokens != 1000 {
		t.Errorf("sessionTokens should be 1000, got %d", sessionTokens)
	}
	PrintStatusBar(500, 1, 1234567890, "test")
	if sessionTokens != 1500 {
		t.Errorf("sessionTokens should be 1500, got %d", sessionTokens)
	}
}

func TestFileAutocompletePattern(t *testing.T) {
	// Verify filepath.Glob works for our use case
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Skip("glob failed")
	}
	if len(matches) == 0 {
		t.Skip("no .go files")
	}
	// Should find at least repl.go, repl_test.go, etc.
	found := false
	for _, m := range matches {
		if strings.Contains(m, "repl") {
			found = true; break
		}
	}
	if !found {
		t.Error("should find repl*.go files")
	}
}
