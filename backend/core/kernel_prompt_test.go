package kernel

import (
	"context"
	"os"
	"path/filepath"
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

func TestPromptIntent(t *testing.T) {
	if got := promptIntent(nil); got != "" {
		t.Errorf("promptIntent(nil) = %q, want empty", got)
	}
	if got := promptIntent(&QueryAnalysis{}); got != "" {
		t.Errorf("promptIntent(empty analysis) = %q, want empty", got)
	}

	got := promptIntent(&QueryAnalysis{
		TaskType:   "coding",
		Complexity: 12,
		Strategy:   "locate token validation, then check expiry branches",
	})
	for _, want := range []string{"[Intent]", "task: coding", "complexity: 12", "interpreted: locate token validation, then check expiry branches"} {
		if !strings.Contains(got, want) {
			t.Errorf("promptIntent missing %q, got: %s", want, got)
		}
	}
}

func TestPromptL3_ActiveContext(t *testing.T) {
	mock := &MockLLMProvider{
		responses: []LLMResponse{{Content: "coding"}},
	}
	k := NewAgentKernel(mock, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())

	l3 := k.promptL3(context.Background(), "fix the login bug", "")
	if !strings.Contains(l3, "Active context: fix the login bug") {
		t.Errorf("promptL3 missing active context anchor, got: %s", l3)
	}

	// General mode: no mode text, so no anchor and still empty
	mock2 := &MockLLMProvider{
		responses: []LLMResponse{{Content: "general"}},
	}
	k2 := NewAgentKernel(mock2, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	if got := k2.promptL3(context.Background(), "hello", ""); got != "" {
		t.Errorf("promptL3(general) = %q, want empty", got)
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

// mockPersonaProvider is a test double for PersonaProvider.
type mockPersonaProvider struct{ prompt string }

func (m *mockPersonaProvider) ActiveSystemPrompt() string { return m.prompt }

func TestActiveL0_FallsBackToDefault(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	if got := k.activeL0(); got == "" {
		t.Fatal("expected non-empty default L0 when no persona provider")
	}
	// Provider present but no active persona -> fall back to default.
	k.SetPersona(&mockPersonaProvider{prompt: ""})
	if got := k.activeL0(); got == "" {
		t.Fatal("expected fallback to default L0 when persona inactive")
	}
}

func TestActiveL0_UsesPersonaPrompt(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	persona := "You are the Architect. Design-focused."
	k.SetPersona(&mockPersonaProvider{prompt: persona})

	query := &Query{Content: "design the module", Options: QueryOptions{}}
	result := k.buildSystemPrompt(query)
	if !strings.Contains(result, persona) {
		t.Fatalf("expected persona L0 in system prompt, got: %q", result[:min(200, len(result))])
	}
	// Persona replaces the default L0 identity.
	if strings.Contains(result, "一个全能的 AI 编码助手") || strings.Contains(result, "versatile AI coding assistant") {
		t.Error("persona prompt should replace the default L0, not append to it")
	}
}

func TestDetectProjectLangs_SharedAnchors(t *testing.T) {
	dir := t.TempDir()
	// 盲区补齐:go.work / setup.py / requirements.txt 应被识别
	for _, f := range []string{"go.work", "setup.py", "requirements.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	langs := detectProjectLangs(dir)
	for _, want := range []string{"go-workspace", "python"} {
		found := false
		for _, l := range langs {
			if l == want {
				found = true
			}
		}
		if !found {
			t.Errorf("detectProjectLangs missing %q, got %v", want, langs)
		}
	}
}

func TestDetectProjectLangs_GlobFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.csproj"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range detectProjectLangs(dir) {
		if l == "csharp" {
			found = true
		}
	}
	if !found {
		t.Error("glob anchor *.csproj should detect csharp")
	}
}

func TestPromptL1_RulePrecedence(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// 两个规则文件存在,声明优先级
	for _, f := range []string{"CLAUDE.md", "OPENAIDE.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("rule: x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	l1 := promptL1()
	if !strings.Contains(l1, "CLAUDE.md > OPENAIDE.md") {
		t.Errorf("promptL1 missing rule precedence declaration, got: %s", l1)
	}
	if !strings.Contains(l1, "CLAUDE.md") || !strings.Contains(l1, "OPENAIDE.md") {
		t.Error("promptL1 should include both rule file contents")
	}
}

func TestDedupeProjectContext(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// 规则文件含一条长约定
	rule := "Always use context.Context for async operations"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(rule+"\nshort: x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := "## 项目已知信息\n- 框架: go\n" + "- " + rule + "\n- 独立约定: 使用表驱动测试\n"
	got := dedupeProjectContext(ctx)
	if strings.Contains(got, rule) {
		t.Errorf("duplicate rule line not removed: %s", got)
	}
	if !strings.Contains(got, "框架: go") {
		t.Error("non-duplicate lines should be kept")
	}
	if !strings.Contains(got, "独立约定") {
		t.Error("independent convention should be kept")
	}
	// 短行不受影响
	if !strings.Contains(got, "short: x") && strings.Contains(ctx, "short: x") {
		t.Error("short line should not be filtered from project context (only from rules)")
	}
}

func TestScanSubdirRules(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "api"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "vendor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api/CLAUDE.md"), []byte("api rules here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// vendor 应被跳过
	if err := os.WriteFile(filepath.Join(dir, "vendor/CLAUDE.md"), []byte("vendor rules\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rules := scanSubdirRules(dir)
	if len(rules) != 1 || rules[0].dir != "api" {
		t.Errorf("expected only api subdir rule, got %+v", rules)
	}
	if !strings.Contains(rules[0].content, "api rules") {
		t.Errorf("api rule content missing: %+v", rules)
	}
}

func TestPivotLimitReached(t *testing.T) {
	d := NewStuckDetector()
	if d.PivotLimitReached() {
		t.Error("fresh detector should not be at pivot limit")
	}
	// 连续失败触发 pivot 直至上限(round 间隔 >3 避开冷却期)
	for i := 0; i < 5; i++ {
		d.RecordResult("read_file", "x", "boom", i*10)
		d.IsStuck(i * 10)
	}
	if !d.PivotLimitReached() {
		t.Error("expected pivot limit reached after repeated failures")
	}
	// 达到上限后 IsStuck 不再触发
	if stuck, _ := d.IsStuck(500); stuck {
		t.Error("IsStuck should be silent after pivot limit")
	}
}
