package kernel

import (
	"context"
	"strings"
	"testing"
)

func TestBuildFinalMessage(t *testing.T) {
	msg := buildFinalMessage("hello", "thinking...", []ToolCall{})
	if msg.Content != "hello" {
		t.Errorf("expected 'hello', got '%s'", msg.Content)
	}
	if msg.ReasoningContent != "thinking..." {
		t.Errorf("expected reasoning content preserved, got '%s'", msg.ReasoningContent)
	}
}

func TestBuildSynthesisPrompt(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "test"}}
	result := buildSynthesisPrompt(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[1].Role != "user" {
		t.Error("synthesis prompt should be user message")
	}
}

func TestPrepareReActRound_CompressionNotTriggered(t *testing.T) {
	k := &AgentKernel{maxTokens: 1000000, maxRounds: 10}
	// Short messages shouldn't trigger compression at 90% of 1M
	msgs := []Message{
		{Role: "user", Content: "short message"},
	}
	result := k.prepareReActRound(context.Background(), msgs, 0, 0, nil)
	if len(result) != 1 {
		t.Errorf("expected no change for short context, got %d msgs", len(result))
	}
}

func TestPrepareReActRound_BudgetInjection(t *testing.T) {
	k := &AgentKernel{maxTokens: 1000000}
	msgs := []Message{
		{Role: "user", Content: "test"},
	}
	// Round 10 should trigger first budget hint (>=10 threshold)
	result := k.prepareReActRound(context.Background(), msgs, 10, 0, nil)
	if len(result) != 2 {
		t.Fatalf("expected budget injection at round 10, got %d msgs", len(result))
	}
	if result[1].Role != "user" {
		t.Error("budget message should be user role")
	}

	// Round 21 should trigger second warning (>=20 threshold)
	result2 := k.prepareReActRound(context.Background(), msgs, 21, 0, nil)
	if len(result2) != 2 {
		t.Fatalf("expected warning at round 21, got %d msgs", len(result2))
	}
}

func TestExecuteToolBatch_Empty(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	results, errs := k.executeToolBatch(context.Background(), nil, "s1", 0, nil)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
	if errs != 0 {
		t.Errorf("expected 0 errors, got %d", errs)
	}
}

func TestExecuteToolBatch_SkipEmpty(t *testing.T) {
	k := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())
	calls := []ToolCall{
		{ID: "1", Function: FunctionCall{Name: ""}},
	}
	results, _ := k.executeToolBatch(context.Background(), calls, "s1", 0, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == "" {
		t.Error("empty tool name should be skipped with error")
	}
}

// countingCompressor 统计 Compress 调用次数,用于验证渐进式重摘要。
type countingCompressor struct {
	calls  int
	limit  int // 每次压缩后的 token 数
	onCompress func()
}

func (c *countingCompressor) Compress(_ context.Context, messages []Message, maxTokens int) ([]Message, int, error) {
	c.calls++
	if c.onCompress != nil {
		c.onCompress()
	}
	return messages, c.limit, nil
}

func (c *countingCompressor) EstimateTokens(messages []Message) int {
	return c.limit
}

func TestPrepareReActRound_ProgressiveCompression(t *testing.T) {
	k := &AgentKernel{maxTokens: 500, maxRounds: 10} // 90% = 450;2000 ASCII chars ≈ 500 tokens
	// 每次压缩后仍超限 → 应循环重摘要最多 3 次
	comp := &countingCompressor{limit: 480}
	k.SetContextCompressor(comp)

	msgs := []Message{
		{Role: "user", Content: strings.Repeat("a", 2000)},
	}
	result := k.prepareReActRound(context.Background(), msgs, 1, 0, nil)
	if comp.calls != 3 {
		t.Errorf("expected 3 compress attempts (still over budget), got %d", comp.calls)
	}
	if len(result) != 2 { // 原始消息 + 压缩提示(taskCtx 未设置,无重注入)
		t.Errorf("expected compression notice appended, got %d msgs", len(result))
	}
}

func TestPrepareReActRound_CompressionStopsWhenInBudget(t *testing.T) {
	k := &AgentKernel{maxTokens: 500, maxRounds: 10}
	comp := &countingCompressor{limit: 100} // 一次压缩即进入预算
	k.SetContextCompressor(comp)

	msgs := []Message{
		{Role: "user", Content: strings.Repeat("a", 2000)},
	}
	k.prepareReActRound(context.Background(), msgs, 1, 0, nil)
	if comp.calls != 1 {
		t.Errorf("expected 1 compress attempt (in budget after first), got %d", comp.calls)
	}
}

func TestSummarizeChunk(t *testing.T) {
	// 跨行结构:优先取包含 Symbol 的声明行
	c := CodeChunk{
		Symbol:  "Authenticate",
		Content: "package auth\n\nfunc Authenticate(user, pass string) bool {\n\t// validate\n\treturn true\n}",
	}
	got := summarizeChunk(c, 80)
	if got != "func Authenticate(user, pass string) bool {" {
		t.Errorf("expected symbol declaration line, got: %q", got)
	}

	// Symbol 不在 content 中 → 回退第一行
	c2 := CodeChunk{Symbol: "missing", Content: "package auth\nfunc foo() {}"}
	if got := summarizeChunk(c2, 80); got != "package auth" {
		t.Errorf("expected fallback first line, got: %q", got)
	}

	// 超长截断
	c3 := CodeChunk{Symbol: "X", Content: "func X() { " + strings.Repeat("y", 100) + " }"}
	got3 := summarizeChunk(c3, 20)
	if len(got3) > 20 {
		t.Errorf("summary not truncated: %q (%d chars)", got3, len(got3))
	}
}
