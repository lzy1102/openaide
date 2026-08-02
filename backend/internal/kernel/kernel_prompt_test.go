package kernel

import (
	"context"
	"strings"
	"testing"
)

func TestDetectTaskType_LLM(t *testing.T) {
	tests := []struct {
		query    string
		response string
		want     string
	}{
		{"fix the bug in login", "coding", "coding"},
		{"review my pull request", "review", "review"},
		{"explain how channels work", "think", "think"},
		{"analyze the architecture", "think", "think"},
		{"hello world", "general", "general"},
		{"", "general", "general"},
		// LLM response with extra whitespace or newlines should still match
		{"write a function", "  coding  ", "coding"},
		{"audit security", "review\n", "review"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			mock := &MockLLMProvider{
				responses: []LLMResponse{{Content: tt.response}},
			}
			k := NewAgentKernel(mock, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
			got := k.detectTaskType(context.Background(), tt.query)
			if got != tt.want {
				t.Errorf("detectTaskType(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestDetectTaskType_LLMError(t *testing.T) {
	// When LLM fails, should return "general"
	mock := &MockLLMProvider{
		responses: []LLMResponse{{Content: "something unexpected"}},
	}
	k := NewAgentKernel(mock, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	got := k.detectTaskType(context.Background(), "some query")
	if got != "general" {
		t.Errorf("unexpected LLM response should default to general, got %q", got)
	}
}

func TestPromptL3(t *testing.T) {
	mock := &MockLLMProvider{
		responses: []LLMResponse{{Content: "coding"}},
	}
	k := NewAgentKernel(mock, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())

	coding := k.promptL3(context.Background(), "fix the login bug", "")
	if coding == "" {
		t.Error("L3 coding should not be empty")
	}
	if !strings.Contains(coding, "Coding") && !strings.Contains(coding, "编码") {
		t.Error("L3 coding should contain mode header")
	}

	// General task type should return empty
	mock2 := &MockLLMProvider{
		responses: []LLMResponse{{Content: "general"}},
	}
	k2 := NewAgentKernel(mock2, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	general := k2.promptL3(context.Background(), "hello", "")
	if general != "" {
		t.Error("L3 general should be empty")
	}
}

func TestPromptL3_AnalysisFormat(t *testing.T) {
	for _, tc := range []struct {
		query    string
		response string
	}{
		{"review my code", "review"},
		{"audit the security", "review"},
		{"research the architecture", "think"},
	} {
		mock := &MockLLMProvider{
			responses: []LLMResponse{{Content: tc.response}},
		}
		k := NewAgentKernel(mock, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
		l3 := k.promptL3(context.Background(), tc.query, "")
		if l3 == "" {
			t.Errorf("promptL3(%q) returned empty", tc.query)
		}
		// Review should contain structured format, think should contain its header
		if !strings.Contains(l3, "P0/P1/P2") && !strings.Contains(l3, "P0") &&
			!strings.Contains(l3, "Think") && !strings.Contains(l3, "思考") {
			t.Errorf("promptL3(%q) missing analysis format", tc.query)
		}
	}

	// Coding mode should NOT have analysis format
	mock := &MockLLMProvider{
		responses: []LLMResponse{{Content: "coding"}},
	}
	k := NewAgentKernel(mock, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	l3 := k.promptL3(context.Background(), "fix the login bug", "")
	if strings.Contains(l3, "P0/P1/P2") {
		t.Error("L3 coding should not contain analysis format")
	}
}

func TestNeedsStrategyAdvice(t *testing.T) {
	// needsStrategyAdvice was deleted with learner — verify it's gone
}

func TestBuildSystemPrompt(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	query := &Query{Content: "fix the login bug", Options: QueryOptions{}}
	result := k.buildSystemPrompt(query)

	if len(result) < 500 {
		t.Errorf("system prompt too short: %d chars", len(result))
	}
	if !strings.Contains(result, "Grounding") {
		t.Error("system prompt missing L0 core rules")
	}
}

func TestBuildSystemLayer_FileOverride(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	custom := "You are a custom assistant. Be helpful."
	k.SetSystemPrompt(custom)

	result := k.buildSystemLayer(context.Background(), &Query{Content: "hello", Options: QueryOptions{}})
	if !strings.Contains(result, custom) {
		t.Error("buildSystemLayer should use custom prompt when set")
	}
}
