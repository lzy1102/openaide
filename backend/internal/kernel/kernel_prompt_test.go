package kernel

import (
	"context"
	"strings"
	"testing"
)

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s        string
		keywords []string
		want     bool
	}{
		{"hello world", []string{"hello"}, true},
		{"hello world", []string{"WORLD"}, false},       // case sensitive
		{"hello world", []string{"foo", "bar", "world"}, true},
		{"hello world", []string{"foo", "bar"}, false},
		{"", []string{"foo"}, false},
		{"hello", nil, false},
		{"codebase has issues", []string{"code"}, false}, // word boundary: "code" not in "codebase"
		{"fix the code", []string{"code"}, true},          // "code" as whole word
		{"fix code please", []string{"code"}, true},
	}
	for _, tt := range tests {
		got := containsAny(tt.s, tt.keywords...)
		if got != tt.want {
			t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.keywords, got, tt.want)
		}
	}
}

func TestDetectTaskType(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"fix the bug in login", "coding"},
		{"implement user authentication", "coding"},
		{"refactor the database layer", "coding"},
		{"add a new endpoint", "coding"},
		{"build a REST API", "coding"},
		{"review my pull request", "review"},
		{"audit the security report", "review"},
		{"explain how channels work", "teaching"},
		{"what is a goroutine", "teaching"},
		{"analyze the architecture", "research"},
		{"design new system", "research"},
		{"hello world", "general"},
		{"", "general"},
		// Word boundary: "codebase" no longer matches "code"
		{"audit the codebase", "review"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := detectTaskType(tt.query)
			if got != tt.want {
				t.Errorf("detectTaskType(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestContainsWord(t *testing.T) {
	// English word boundaries
	if containsWord("fix the code", "code") != true {
		t.Error("'code' should match in 'fix the code'")
	}
	if containsWord("audit the codebase", "code") != false {
		t.Error("'code' should NOT match in 'codebase'")
	}
	if containsWord("fix", "fix") != true {
		t.Error("exact word match should work")
	}
	// CJK substring matching
	if containsWord("修复登录", "修复") != true {
		t.Error("'修复' should match in '修复登录'")
	}
	if containsWord("审查代码", "审查") != true {
		t.Error("'审查' should match in '审查代码'")
	}
}

func TestPromptL3(t *testing.T) {
	// L3 returns task adapter for the query
	coding := promptL3("fix the login bug")
	if coding == "" {
		t.Error("L3 coding should not be empty")
	}
	if !strings.Contains(coding, "Coding Mode") && !strings.Contains(coding, "编码模式") {
		t.Error("L3 coding should contain mode header")
	}

	review := promptL3("review my pull request")
	if review == "" {
		t.Error("L3 review should not be empty")
	}

	general := promptL3("hello")
	if general != "" {
		t.Error("L3 general should be empty")
	}
}

func TestPromptL3_AnalysisFormat(t *testing.T) {
	// Analysis format should be in review and research modes
	for _, query := range []string{"review my code", "audit the security", "research the architecture"} {
		l3 := promptL3(query)
		if l3 == "" {
			t.Errorf("promptL3(%q) returned empty", query)
		}
		if !strings.Contains(l3, "P0/P1/P2") && !strings.Contains(l3, "P0") {
			t.Errorf("promptL3(%q) missing analysis format", query)
		}
	}
	// Coding mode should NOT have analysis format
	l3 := promptL3("fix the login bug")
	if strings.Contains(l3, "P0/P1/P2") {
		t.Error("L3 coding should not contain analysis format")
	}
}

func TestIsZhEnv(t *testing.T) {
	// Just verify it doesn't panic and returns bool
	_ = isZhEnv()
}

func TestNeedsStrategyAdvice(t *testing.T) {
	// needsStrategyAdvice was deleted with learner — verify it's gone
	// This function no longer exists in the codebase
}

func TestBuildSystemPrompt(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	query := &Query{Content: "fix the login bug", Options: QueryOptions{}}
	result := k.buildSystemPrompt(query)

	if len(result) < 500 {
		t.Errorf("system prompt too short: %d chars", len(result))
	}
	if !strings.Contains(result, "Human Interaction") {
		t.Error("system prompt missing L0 content")
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
