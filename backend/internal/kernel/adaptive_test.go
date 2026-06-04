package kernel

import (
	"context"
	"fmt"
	"testing"
)

type mockAdaptiveLLM struct{ estimate int }

func (m *mockAdaptiveLLM) Chat(ctx context.Context, msgs []Message, tools []ToolDefinition, opts map[string]interface{}) (*LLMResponse, error) {
	return &LLMResponse{Content: fmt.Sprintf("%d", m.estimate)}, nil
}
func (m *mockAdaptiveLLM) ChatStream(ctx context.Context, msgs []Message, tools []ToolDefinition, opts map[string]interface{}) (<-chan StreamChunk, error) {
	return nil, nil
}
func (m *mockAdaptiveLLM) GetModelID() string { return "mock" }
func (m *mockAdaptiveLLM) SetModelID(mdl string) {}

func TestAdaptiveRounds_LLM(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	ar.SetLLM(&mockAdaptiveLLM{estimate: 7})
	r := ar.Calculate(context.Background(), "hello", 0)
	if r != 7 {
		t.Errorf("expected LLM estimate 7, got %d", r)
	}
}

func TestAdaptiveRounds_MinCap(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	ar.SetLLM(&mockAdaptiveLLM{estimate: 1})
	r := ar.Calculate(context.Background(), "hello", 0)
	if r != 5 {
		t.Errorf("should cap at min 5, got %d", r)
	}
}

func TestAdaptiveRounds_MaxCap(t *testing.T) {
	ar := NewAdaptiveRounds(5, 30)
	ar.SetLLM(&mockAdaptiveLLM{estimate: 50})
	r := ar.Calculate(context.Background(), "test", 0)
	if r != 30 {
		t.Errorf("should cap at max 30, got %d", r)
	}
}
