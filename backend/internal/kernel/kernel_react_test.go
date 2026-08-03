package kernel

import (
	"context"
	"testing"
)

func TestBuildFinalMessage(t *testing.T) {
	msg := buildFinalMessage("hello", "thinking...", []ToolCall{})
	if msg.Content != "hello" {
		t.Errorf("expected 'hello', got '%s'", msg.Content)
	}
	if msg.ReasoningContent != "thinking..." {
		t.Errorf("expected reasoning content preserved, got '%s'", msg.ReasoningContent)
	}
}

func TestBuildSynthesisPrompt(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "test"}}
	result := buildSynthesisPrompt(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[1].Role != "user" {
		t.Error("synthesis prompt should be user message")
	}
}

func TestPrepareReActRound_CompressionNotTriggered(t *testing.T) {
	k := &AgentKernel{maxTokens: 1000000, maxRounds: 10}
	// Short messages shouldn't trigger compression at 90% of 1M
	msgs := []Message{
		{Role: "user", Content: "short message"},
	}
	result := k.prepareReActRound(context.Background(), msgs, 0, 0, nil)
	if len(result) != 1 {
		t.Errorf("expected no change for short context, got %d msgs", len(result))
	}
}

func TestPrepareReActRound_BudgetInjection(t *testing.T) {
	k := &AgentKernel{maxTokens: 1000000}
	msgs := []Message{
		{Role: "user", Content: "test"},
	}
	// Round 10 should trigger first budget hint (>=10 threshold)
	result := k.prepareReActRound(context.Background(), msgs, 10, 0, nil)
	if len(result) != 2 {
		t.Fatalf("expected budget injection at round 10, got %d msgs", len(result))
	}
	if result[1].Role != "user" {
		t.Error("budget message should be user role")
	}

	// Round 21 should trigger second warning (>=20 threshold)
	result2 := k.prepareReActRound(context.Background(), msgs, 21, 0, nil)
	if len(result2) != 2 {
		t.Fatalf("expected warning at round 21, got %d msgs", len(result2))
	}
}

func TestExecuteToolBatch_Empty(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	results, errs := k.executeToolBatch(context.Background(), nil, "s1", 0, nil)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
	if errs != 0 {
		t.Errorf("expected 0 errors, got %d", errs)
	}
}

func TestExecuteToolBatch_SkipEmpty(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: ""}},
	}
	results, _ := k.executeToolBatch(context.Background(), calls, "s1", 0, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == "" {
		t.Error("empty tool name should be skipped with error")
	}
}
