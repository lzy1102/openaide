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

func TestLLMReflection_ProcessSupervision(t *testing.T) {
	llm := &mockReflectionLLM{json: `{"quality":7,"issues":["round 2 tool choice was suboptimal"],"suggestions":["read middleware first"],"learned":"Always check middleware before handler"}`}
	r := NewLLMReflection(llm)

	// Include full message history for process supervision
	record := ExecutionRecord{
		Query:    "fix login bug",
		Response: "fixed auth/service.go",
		Success:  true,
		Messages: []Message{
			{Role: "assistant", Content: "Let me read the login handler", ToolCalls: []ToolCall{{Function: FunctionCall{Name: "read_file"}}}},
			{Role: "tool", Content: "func LoginHandler..."},
			{Role: "assistant", Content: "I see the issue — need to check middleware too", ToolCalls: []ToolCall{{Function: FunctionCall{Name: "read_file"}}}},
			{Role: "tool", Content: "func ValidateToken..."},
			{Role: "assistant", Content: "Fixed by updating token validation in middleware/token.go"},
		},
	}

	result, err := r.Reflect(context.Background(), "s1", record)
	if err != nil {
		t.Fatal("Reflect with process supervision:", err)
	}
	if result.Quality != 7 {
		t.Errorf("expected quality 7, got %d", result.Quality)
	}
	if len(result.Issues) == 0 {
		t.Error("expected step-level issues")
	}
}

func TestLLMReflection_NoMessages(t *testing.T) {
	llm := &mockReflectionLLM{json: `{"quality":5,"issues":[],"suggestions":[],"learned":"test"}`}
	r := NewLLMReflection(llm)

	// Without messages, should still work (no process supervision)
	result, err := r.Reflect(context.Background(), "s1", ExecutionRecord{
		Query: "test", Response: "test", Success: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Quality != 5 {
		t.Errorf("expected quality 5, got %d", result.Quality)
	}
}

func TestLLMReflection_Stateless(t *testing.T) {
	// LLMReflection is stateless — no shared mutable state, no criteria map.
	// Each Reflect call is independent. Safe for concurrent use.
	r := NewLLMReflection(nil)
	if r.llm != nil {
		t.Error("expected nil llm")
	}
}

func stringsContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub { return true }
	}
	return false
}
