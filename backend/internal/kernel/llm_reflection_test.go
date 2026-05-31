package kernel

import (
	"context"
	"testing"
)

type mockReflectionLLM struct {
	json string
}

func (m *mockReflectionLLM) Chat(ctx context.Context, msgs []Message, tools []ToolDefinition, opts map[string]interface{}) (*LLMResponse, error) {
	return &LLMResponse{
		Content: "",
		ToolCalls: []ToolCall{{
			Function: FunctionCall{Name: "submit_evaluation", Arguments: m.json},
		}},
	}, nil
}
func (m *mockReflectionLLM) ChatStream(ctx context.Context, msgs []Message, tools []ToolDefinition, opts map[string]interface{}) (<-chan StreamChunk, error) {
	return nil, nil
}
func (m *mockReflectionLLM) GetModelID() string { return "mock" }
func (m *mockReflectionLLM) SetModelID(s string) {}

func TestLLMReflection_Success(t *testing.T) {
	llm := &mockReflectionLLM{json: `{"quality":8,"issues":[],"suggestions":["good work"],"learned":"test learning"}`}
	r := NewLLMReflection(llm)

	result, err := r.Reflect(context.Background(), "s1", ExecutionRecord{
		Query:    "test query",
		Response: "test response",
		Success:  true,
	})
	if err != nil {
		t.Fatal("Reflect:", err)
	}
	if result.Quality != 8 {
		t.Errorf("expected quality 8, got %d", result.Quality)
	}
}

func TestLLMReflection_ParseError(t *testing.T) {
	llm := &mockReflectionLLM{json: `{invalid`}
	r := NewLLMReflection(llm)

	_, err := r.Reflect(context.Background(), "s1", ExecutionRecord{})
	if err == nil {
		t.Error("expected parse error")
	}
}
