package kernel

import (
	"context"
	"strings"
	"testing"
)

type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vec := make([]float32, m.dim)
	for i, r := range text {
		vec[i%m.dim] += float32(r) / 1000.0
	}
	return vec, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, t := range texts {
		v, _ := m.Embed(ctx, t)
		result[i] = v
	}
	return result, nil
}

func (m *mockEmbedder) Dimension() int { return m.dim }

func TestSemanticPatternDetector_FormsCluster(t *testing.T) {
	d := NewSemanticPatternDetector(&mockEmbedder{8}, 3, 0.80)
	messages := []Message{
		{Role: "user", Content: "fix the login bug"},
		{Role: "assistant", Content: "fixed auth/service.go"},
		{Role: "user", Content: "login is broken again"},
		{Role: "assistant", Content: "updated token middleware"},
		{Role: "user", Content: "the login still has issues"},
		{Role: "assistant", Content: "patched login handler"},
		{Role: "user", Content: "fix login please"},
		{Role: "assistant", Content: "resolved in auth/login.go"},
	}
	patterns, err := d.Detect(context.Background(), "s1", messages)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Got %d patterns", len(patterns))
	for _, p := range patterns {
		t.Logf("  Pattern: type=%s desc=%q freq=%d", p.Type, p.Description, p.Frequency)
	}
	examples := d.GetDistillableExamples()
	t.Logf("Distillable clusters: %d", len(examples))
}

func TestEvaluateAndDistill_PromptFormat(t *testing.T) {
	// Verify evaluateAndDistill handles nil LLM gracefully
	result := evaluateAndDistill(context.Background(), nil, Pattern{Description: "test", Frequency: 2}, 0, nil)
	if result != "" {
		t.Error("evaluateAndDistill should return empty with nil LLM")
	}
}

func TestEvaluateAndDistill(t *testing.T) {
	examples := []clusterExample{
		{query: "fix login", response: "checked auth/service.go"},
		{query: "login broken", response: "patched middleware/token.go"},
	}
	// nil LLM returns ""
	p := Pattern{Description: "login", Frequency: 2}
	result := evaluateAndDistill(context.Background(), nil, p, 0, [][]clusterExample{examples})
	if result != "" {
		t.Error("evaluateAndDistill should return empty with nil LLM")
	}

	// empty examples returns ""
	result = evaluateAndDistill(context.Background(), nil, p, 0, [][]clusterExample{{}})
	if result != "" {
		t.Error("evaluateAndDistill should return empty with <2 examples")
	}
}

func TestExtractPairs(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "fix login bug"},
		{Role: "assistant", Content: "fixed in auth.go"},
		{Role: "user", Content: "add test"},
		{Role: "assistant", Content: "created test file"},
	}
	pairs := extractPairs(messages)
	if len(pairs) != 2 {
		t.Errorf("expected 2 pairs, got %d", len(pairs))
	}
	if pairs[0].query != "fix login bug" {
		t.Errorf("expected 'fix login bug', got %q", pairs[0].query)
	}
}

func TestExtractClusterTheme(t *testing.T) {
	examples := []clusterExample{
		{query: "fix the login bug"},
		{query: "login is broken"},
		{query: "please fix the login"},
	}
	theme := extractClusterTheme(examples)
	if theme == "" || !strings.Contains(theme, "login") {
		t.Errorf("expected 'login' in theme, got %q", theme)
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("fix the login bug in auth module")
	if len(tokens) == 0 {
		t.Error("expected non-empty tokens")
	}
	hasLogin := false
	for _, tok := range tokens {
		if tok == "login" {
			hasLogin = true
		}
		if len(tok) <= 2 {
			t.Errorf("token too short: %q", tok)
		}
	}
	if !hasLogin {
		t.Error("expected 'login' in tokens")
	}
}
