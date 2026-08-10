package compress

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"openaide/backend/internal/kernel"
)

// mockLLM implements kernel.LLMProvider with scripted responses.
type mockLLM struct {
	content string
	err     error
}

func (m *mockLLM) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &kernel.LLMResponse{Content: m.content}, nil
}

func (m *mockLLM) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	ch := make(chan kernel.StreamChunk, 1)
	ch <- kernel.StreamChunk{Content: m.content, Done: true}
	close(ch)
	return ch, nil
}

func (m *mockLLM) GetModelID() string { return "mock" }

// ============ NovelCompressor: Compress full path ============

func TestNovelCompressor_Compress_MoreThan4(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "question one about database design"},
		{Role: "assistant", Content: "answer one with some detail", ToolCalls: []kernel.ToolCall{{ID: "t1"}}},
		{Role: "tool", Content: "tool result one"},
		{Role: "user", Content: "follow up question two"},
		{Role: "assistant", Content: "answer two"},
		{Role: "user", Content: "final question three"},
		{Role: "assistant", Content: "final answer three"},
	}
	compressed, saved, err := c.Compress(context.Background(), messages, 0)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if len(compressed) <= 4 {
		t.Errorf("expected system + summary + recent, got %d", len(compressed))
	}
	if saved < 0 {
		t.Errorf("saved must be >= 0, got %d", saved)
	}
	// System messages preserved
	for _, msg := range compressed {
		if msg.Role == "system" && strings.Contains(msg.Content, "前文摘要") {
			return // summary injected
		}
	}
	t.Error("expected chapter summary injected")
}

func TestNovelCompressor_Compress_MaxTokensTrim(t *testing.T) {
	c := NewNovelCompressor()
	var messages []kernel.Message
	messages = append(messages, kernel.Message{Role: "system", Content: "rules"})
	for i := 0; i < 10; i++ {
		messages = append(messages,
			kernel.Message{Role: "user", Content: fmt.Sprintf("long user message number %d with plenty of words here", i)},
			kernel.Message{Role: "assistant", Content: fmt.Sprintf("long assistant reply number %d with lots of words and detail", i)},
		)
	}
	compressed, _, err := c.Compress(context.Background(), messages, 200)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if c.estimateTokens(compressed) > 200 {
		t.Errorf("result exceeds maxTokens budget: %d > 200", c.estimateTokens(compressed))
	}
}

func TestNovelCompressor_Compress_SystemOnlyPreserved(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{
		{Role: "system", Content: "core rules"},
		{Role: "system", Content: "more rules"},
	}
	compressed, _, err := c.Compress(context.Background(), messages, 0)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if len(compressed) != 2 {
		t.Errorf("expected 2 system messages, got %d", len(compressed))
	}
}

// ============ NovelCompressor: helpers ============

func TestGenerateChapterSummary(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{
		{Role: "user", Content: "setup database schema"},
		{Role: "assistant", Content: "created tables", ToolCalls: []kernel.ToolCall{{ID: "t1"}, {ID: "t2"}}},
		{Role: "user", Content: "add index on users table"},
		{Role: "assistant", Content: "done adding index"},
	}
	s := c.generateChapterSummary(messages)
	if s == "" {
		t.Fatal("expected non-empty summary")
	}
	if !strings.Contains(s, "2 轮对话") {
		t.Errorf("expected round count, got %q", s)
	}
	if !strings.Contains(s, "2 次工具") {
		t.Errorf("expected tool count, got %q", s)
	}
	if !strings.Contains(s, "涉及主题") {
		t.Errorf("expected topics, got %q", s)
	}
}

func TestGenerateChapterSummary_Truncate(t *testing.T) {
	c := &NovelCompressor{maxSummaryLength: 10}
	long := strings.Repeat("verylongword ", 50)
	messages := []kernel.Message{{Role: "user", Content: long}}
	s := c.generateChapterSummary(messages)
	if len([]rune(s)) > 13 {
		t.Errorf("summary should be truncated, got %d chars", len([]rune(s)))
	}
	if !strings.HasSuffix(s, "...") {
		t.Errorf("expected ellipsis suffix, got %q", s)
	}
}

func TestGenerateCliffhanger_UnansweredQuestion(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{
		{Role: "user", Content: "how does auth work here"},
		{Role: "assistant", Content: "let me check"},
		{Role: "user", Content: "what about rate limiting?"},
	}
	hook := c.generateCliffhanger(messages)
	if hook != "what about rate limiting?" {
		t.Errorf("expected unanswered question, got %q", hook)
	}
}

func TestGenerateCliffhanger_AllAnswered(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{
		{Role: "user", Content: "question"},
		{Role: "assistant", Content: "answer"},
	}
	if hook := c.generateCliffhanger(messages); hook != "" {
		t.Errorf("expected empty hook, got %q", hook)
	}
}

func TestGenerateCliffhanger_ToolError(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{
		{Role: "user", Content: "run the build"},
		{Role: "assistant", Content: "running it"},
		{Role: "tool", Content: "Error: build failed"},
	}
	hook := c.generateCliffhanger(messages)
	if !strings.Contains(hook, "工具调用错误") {
		t.Errorf("expected tool error hook, got %q", hook)
	}
}

func TestGenerateCliffhanger_TruncateLong(t *testing.T) {
	c := NewNovelCompressor()
	long := strings.Repeat("x", 150)
	messages := []kernel.Message{{Role: "user", Content: long}}
	hook := c.generateCliffhanger(messages)
	if len([]rune(hook)) > 103 {
		t.Errorf("expected truncated hook, got %d chars", len([]rune(hook)))
	}
}

func TestExtractTopics(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{
		{Role: "user", Content: "database schema migration strategy"},
		{Role: "assistant", Content: "database index strategy works well"},
		{Role: "user", Content: "strategy for caching database queries"},
	}
	topics := c.extractTopics(messages)
	if len(topics) == 0 {
		t.Fatal("expected topics extracted")
	}
	// "strategy" and "database" appear 3x each — should be top
	if topics[0] != "strategy" && topics[0] != "database" {
		t.Errorf("expected high-frequency word first, got %v", topics)
	}
}

func TestExtractTopics_ShortWordsSkipped(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{{Role: "user", Content: "a b c d e f g h i j"}}
	topics := c.extractTopics(messages)
	if len(topics) != 0 {
		t.Errorf("expected no topics for short words, got %v", topics)
	}
}

func TestNovelCompressor_EstimateTokens_CJK(t *testing.T) {
	c := NewNovelCompressor()
	msgs := []kernel.Message{{Role: "user", Content: "你好世界"}}
	if got := c.EstimateTokens(msgs); got != 12 {
		t.Errorf("expected 12 (4 CJK×2 + 4 overhead), got %d", got)
	}
}

// ============ LLMCompressor ============

func TestLLMCompressor_Compress_Success(t *testing.T) {
	llm := &mockLLM{content: "[用户意图] finish the task\n[关键事实] a.go has the bug\n[当前状态] fixing\n[注意事项] none"}
	fallback := NewNovelCompressor()
	c := NewLLMCompressor(llm, fallback)

	messages := []kernel.Message{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "msg 1"},
		{Role: "assistant", Content: "reply 1"},
		{Role: "user", Content: "msg 2"},
		{Role: "assistant", Content: "reply 2"},
		{Role: "user", Content: "msg 3"},
		{Role: "assistant", Content: "reply 3"},
	}
	compressed, saved, err := c.Compress(context.Background(), messages, 0)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if saved < 0 {
		t.Errorf("saved must be >= 0, got %d", saved)
	}
	// LLM summary injected as system message
	foundSummary := false
	for _, msg := range compressed {
		if strings.Contains(msg.Content, "[前文摘要]") {
			foundSummary = true
		}
	}
	if !foundSummary {
		t.Error("expected LLM summary injected")
	}
}

func TestLLMCompressor_Compress_LLMFails_Fallback(t *testing.T) {
	llm := &mockLLM{err: fmt.Errorf("llm down")}
	fallback := NewNovelCompressor()
	c := NewLLMCompressor(llm, fallback)

	messages := []kernel.Message{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "msg 1"},
		{Role: "assistant", Content: "reply 1"},
		{Role: "user", Content: "msg 2"},
		{Role: "assistant", Content: "reply 2"},
		{Role: "user", Content: "msg 3"},
		{Role: "assistant", Content: "reply 3"},
	}
	compressed, saved, err := c.Compress(context.Background(), messages, 0)
	if err != nil {
		t.Fatalf("Compress fallback failed: %v", err)
	}
	if len(compressed) == 0 || saved < 0 {
		t.Errorf("unexpected fallback result: len=%d saved=%d", len(compressed), saved)
	}
}

func TestLLMCompressor_Compress_EmptySummary(t *testing.T) {
	llm := &mockLLM{content: "   "}
	fallback := NewNovelCompressor()
	c := NewLLMCompressor(llm, fallback)

	messages := []kernel.Message{
		{Role: "user", Content: "1"},
		{Role: "assistant", Content: "a"},
		{Role: "user", Content: "2"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "3"},
		{Role: "assistant", Content: "c"},
	}
	// Empty summary → fallback path via error, still works
	compressed, _, err := c.Compress(context.Background(), messages, 0)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if len(compressed) == 0 {
		t.Error("expected fallback compression")
	}
}

func TestLLMCompressor_Compress_TooFewMessages(t *testing.T) {
	c := NewLLMCompressor(&mockLLM{content: "x"}, NewNovelCompressor())
	messages := []kernel.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}}
	compressed, saved, err := c.Compress(context.Background(), messages, 0)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if len(compressed) != 2 || saved != 0 {
		t.Errorf("expected passthrough, got len=%d saved=%d", len(compressed), saved)
	}
}

func TestLLMCompressor_Compress_PendingQuestions(t *testing.T) {
	llm := &mockLLM{content: "summary text"}
	c := NewLLMCompressor(llm, NewNovelCompressor())
	messages := []kernel.Message{
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "unanswered question here"},
		{Role: "assistant", Content: "reply"},
		{Role: "user", Content: "still open question"},
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "pending query"},
	}
	compressed, _, err := c.Compress(context.Background(), messages, 0)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	foundPending := false
	for _, msg := range compressed {
		if strings.Contains(msg.Content, "[待解决问题]") {
			foundPending = true
		}
	}
	if !foundPending {
		t.Error("expected pending question injected")
	}
}

func TestLLMCompressor_EstimateTokens_Delegates(t *testing.T) {
	fallback := NewNovelCompressor()
	c := NewLLMCompressor(&mockLLM{content: "x"}, fallback)
	msgs := []kernel.Message{{Role: "user", Content: "hello world"}}
	if got := c.EstimateTokens(msgs); got != fallback.EstimateTokens(msgs) {
		t.Errorf("expected delegated estimate %d, got %d", fallback.EstimateTokens(msgs), got)
	}
}

// ============ extractPendingQuestions ============

func TestExtractPendingQuestions_Unanswered(t *testing.T) {
	messages := []kernel.Message{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "unanswered final question"},
	}
	got := extractPendingQuestions(messages)
	if got != "unanswered final question" {
		t.Errorf("expected pending question, got %q", got)
	}
}

func TestExtractPendingQuestions_Answered(t *testing.T) {
	messages := []kernel.Message{
		{Role: "user", Content: "question"},
		{Role: "assistant", Content: "answer"},
	}
	if got := extractPendingQuestions(messages); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractPendingQuestions_Empty(t *testing.T) {
	if got := extractPendingQuestions(nil); got != "" {
		t.Errorf("expected empty for nil, got %q", got)
	}
}

func TestExtractPendingQuestions_TruncateLong(t *testing.T) {
	long := strings.Repeat("y", 150)
	messages := []kernel.Message{{Role: "user", Content: long}}
	got := extractPendingQuestions(messages)
	if len([]rune(got)) > 103 {
		t.Errorf("expected truncated, got %d chars", len([]rune(got)))
	}
}
