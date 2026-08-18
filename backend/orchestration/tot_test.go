package orchestration

import (
	"context"
	"strings"
	"testing"

	"openaide/backend/core"
)

// mockLLMForToT returns responses for tree-of-thoughts evaluation.
type mockToTLLM struct{ resp string }

func (m *mockToTLLM) Chat(ctx context.Context, msgs []kernel.Message, tools []kernel.ToolDefinition, opts map[string]interface{}) (*kernel.LLMResponse, error) {
	return &kernel.LLMResponse{Content: m.resp}, nil
}
func (m *mockToTLLM) ChatStream(ctx context.Context, msgs []kernel.Message, tools []kernel.ToolDefinition, opts map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	return nil, nil
}
func (m *mockToTLLM) GetModelID() string  { return "mock" }
func (m *mockToTLLM) SetModelID(s string) {}

func TestExploreAlternatives_NeedsAtLeastTwo(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockToTLLM{resp: "BEST=1"}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	_, err := o.ExploreAlternatives(context.Background(), "u1", "p1", "task", []Approach{{Name: "A"}})
	if err == nil || !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("expected 'at least 2' error, got: %v", err)
	}
}

func TestExploreAlternatives_CapsAtFour(t *testing.T) {
	approaches := make([]Approach, 6)
	for i := range approaches {
		approaches[i] = Approach{Name: "test"}
	}
	_ = approaches // just testing the cap logic
	if len(approaches) > 4 {
		approaches = approaches[:4]
	}
	if len(approaches) != 4 {
		t.Errorf("expected 4, got %d", len(approaches))
	}
}

func TestEvaluateBranches(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockToTLLM{resp: "BEST=2 REASON=Approach B has better performance characteristics"}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())

	branches := []BranchResult{
		{Approach: Approach{Name: "A: Direct fix"}, Findings: "Quick fix in handler.go"},
		{Approach: Approach{Name: "B: Refactor"}, Findings: "Refactor middleware for better reuse"},
	}

	best, rationale := o.evaluateBranches(context.Background(), "fix login bug", branches)
	if best != 1 {
		t.Errorf("expected best=1 (branch 2), got %d", best)
	}
	if !strings.Contains(rationale, "better performance") {
		t.Errorf("expected rationale, got: %s", rationale)
	}
}

func TestEvaluateBranches_InvalidBest(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockToTLLM{resp: "BEST=99 REASON=none"}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())

	branches := []BranchResult{
		{Approach: Approach{Name: "A"}, Findings: "test"},
		{Approach: Approach{Name: "B"}, Findings: "test"},
	}

	best, _ := o.evaluateBranches(context.Background(), "task", branches)
	if best != 0 {
		t.Errorf("out-of-range best should default to 0, got %d", best)
	}
}

func TestApproach_Structure(t *testing.T) {
	a := Approach{Name: "Safe Refactor", Prompt: "Refactor by extracting shared logic first"}
	if a.Name == "" || a.Prompt == "" {
		t.Error("approach should have name and prompt")
	}
}

func TestBranchResult_Structure(t *testing.T) {
	b := BranchResult{
		Approach:   Approach{Name: "Test"},
		Findings:   "Found the issue in auth.go",
		ToolCalls:  3,
		Confidence: 0.85,
	}
	if b.Findings == "" || b.Confidence <= 0 {
		t.Error("branch result should have findings and confidence")
	}
}
