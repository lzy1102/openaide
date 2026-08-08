package kernel

import (
	"context"
	"strings"
	"testing"
)

func TestCheckDirection_OnTrack(t *testing.T) {
	mock := &MockLLMProvider{
		responses: []LLMResponse{{Content: "on_track"}},
	}
	k := NewAgentKernel(mock, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())

	messages := []Message{
		{Role: "assistant", Content: "I'll fix the login bug", ToolCalls: []ToolCall{{Function: FunctionCall{Name: "read_file"}}}},
		{Role: "tool", Content: "auth.go: login validation"},
	}
	if got := k.checkDirection(context.Background(), "fix the login bug", messages); got != "" {
		t.Errorf("on_track should return empty pivot, got: %q", got)
	}
}

func TestCheckDirection_OffTrack(t *testing.T) {
	mock := &MockLLMProvider{
		responses: []LLMResponse{{Content: "off_track exploring unrelated UI styling"}},
	}
	k := NewAgentKernel(mock, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())

	messages := []Message{
		{Role: "assistant", Content: "let me restyle the dashboard", ToolCalls: []ToolCall{{Function: FunctionCall{Name: "write_file"}}}},
	}
	pivot := k.checkDirection(context.Background(), "fix the login bug", messages)
	if !strings.Contains(pivot, "drifted") {
		t.Errorf("off_track should return drift pivot, got: %q", pivot)
	}
	if !strings.Contains(pivot, "fix the login bug") {
		t.Errorf("pivot should reference original request, got: %q", pivot)
	}
}

func TestCheckDirection_NoLLM(t *testing.T) {
	k := NewAgentKernel(nil, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	if got := k.checkDirection(context.Background(), "query", []Message{{Role: "user", Content: "hi"}}); got != "" {
		t.Errorf("nil LLM should return empty pivot, got: %q", got)
	}
}

func TestRecentActivitySummary(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "fix bug"},
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{{Function: FunctionCall{Name: "read_file"}}}},
		{Role: "tool", Content: "content of file"},
	}
	got := recentActivitySummary(messages, 8)
	if !strings.Contains(got, "read_file") || !strings.Contains(got, "fix bug") || !strings.Contains(got, "content of file") {
		t.Errorf("summary missing expected entries, got: %q", got)
	}
}

func TestTaskContextReinject(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())

	// 未设置 → 空
	if got := k.taskContextReinject(); got != "" {
		t.Errorf("empty task ctx should return empty, got: %q", got)
	}

	k.taskCtx.Store(TaskContext{
		Query:  "fix the login bug",
		Intent: "[Intent]\ntask: coding\ninterpreted: fix token validation\n",
	})
	got := k.taskContextReinject()
	for _, want := range []string{"[OriginalQuery]", "fix the login bug", "[Intent]", "task: coding"} {
		if !strings.Contains(got, want) {
			t.Errorf("reinject missing %q, got: %s", want, got)
		}
	}

	// 超长查询截断
	k.taskCtx.Store(TaskContext{Query: strings.Repeat("a", 300)})
	got = k.taskContextReinject()
	if len(got) > 220 {
		t.Errorf("long query not truncated, len=%d", len(got))
	}
}
