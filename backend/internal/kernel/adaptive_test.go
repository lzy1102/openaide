package kernel

import (
	"context"
	"testing"
)

func TestAdaptiveRounds_Simple(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	r := ar.Calculate(context.Background(), "hello", 0)
	if r != 5 {
		t.Errorf("simple query should use min rounds, got %d", r)
	}
}

func TestAdaptiveRounds_WithHistory(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	r := ar.Calculate(context.Background(), "some complex task that needs many steps", 15)
	if r < 5 {
		t.Errorf("should at least be min rounds, got %d", r)
	}
	// history > 10 adds 3 base rounds
	if r < 8 && ar.llm == nil {
		t.Logf("no LLM available, history bonus only, got %d", r)
	}
}

func TestAdaptiveRounds_MaxCap(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	r := ar.Calculate(context.Background(), "test", 0)
	if r > 30 {
		t.Errorf("should cap at 30, got %d", r)
	}
}

func TestAdaptiveRounds_LLMEstimate(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	// Without LLM, should fall back to min + history
	r := ar.Calculate(context.Background(), "refactor the entire authentication system", 5)
	if r < 5 {
		t.Errorf("should use min rounds, got %d", r)
	}
	if r > 30 {
		t.Errorf("should cap at max, got %d", r)
	}
}
