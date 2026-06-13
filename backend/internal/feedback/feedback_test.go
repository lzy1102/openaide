package feedback

import (
	"testing"

	"openaide/backend/internal/kernel"
)

func TestQualityScore_Perfect(t *testing.T) {
	r := &Result{
		ToolSuccesses: 3, ToolFailures: 0,
		UserVerdict: VerdictGood,
		Reflection:  &kernel.ReflectionResult{Quality: 9},
	}
	s := r.QualityScore()
	if s < 0.8 {
		t.Errorf("expected high score, got %.2f", s)
	}
}

func TestQualityScore_Terrible(t *testing.T) {
	r := &Result{
		ToolSuccesses: 0, ToolFailures: 3,
		UserVerdict: VerdictBad,
		Reflection:  &kernel.ReflectionResult{Quality: 2},
	}
	s := r.QualityScore()
	if s > 0.3 {
		t.Errorf("expected low score, got %.2f", s)
	}
}

func TestQualityScore_Neutral(t *testing.T) {
	r := &Result{
		ToolSuccesses: 1, ToolFailures: 1,
		UserVerdict: VerdictNone,
		Reflection:  &kernel.ReflectionResult{Quality: 5},
	}
	s := r.QualityScore()
	if s < 0.4 || s > 0.6 {
		t.Errorf("expected medium score, got %.2f", s)
	}
}

func TestShouldKeep(t *testing.T) {
	r := &Result{
		ToolSuccesses: 3, ToolFailures: 0,
		UserVerdict: VerdictGood,
		Reflection:  &kernel.ReflectionResult{Quality: 8},
	}
	if !r.ShouldKeep() {
		t.Error("should keep")
	}
}

func TestShouldDiscard(t *testing.T) {
	r := &Result{
		ToolSuccesses: 0, ToolFailures: 3,
		UserVerdict: VerdictBad,
		Reflection:  &kernel.ReflectionResult{Quality: 1},
	}
	if !r.ShouldDiscard() {
		t.Error("should discard")
	}
}

func TestGate_Pass(t *testing.T) {
	g := NewGate()
	// LLM-first: Reflection Quality >= 5 passes (trust LLM judgment)
	if !g.Pass("test", "response", 1, 0, &kernel.ReflectionResult{Quality: 7}) {
		t.Error("Quality 7 should pass")
	}
	if g.Pass("test", "response", 0, 3, &kernel.ReflectionResult{Quality: 3}) {
		t.Error("Quality 3 should fail")
	}
	// No reflection + no LLM: falls back to formula
	if !g.Pass("test", "response", 3, 0, nil) {
		t.Error("3 success, 0 failures should pass formula")
	}
}

func TestGate_ExtractKnowledge(t *testing.T) {
	g := NewGate()
	r := &Result{
		Query: "test query", Response: "test response",
		ToolSuccesses: 2, ToolFailures: 0,
		UserVerdict: VerdictGood,
		Reflection:  &kernel.ReflectionResult{Quality: 8, Learned: "good work"},
	}
	title, content, ok := g.ExtractKnowledge(r)
	if !ok {
		t.Error("should extract")
	}
	if title == "" || content == "" {
		t.Error("empty title or content")
	}
}
