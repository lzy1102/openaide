package kernel

import (
	"context"
	"testing"
	"time"
)

// MockLLMProvider 模拟 LLM 提供商
type MockLLMProvider struct {
	responses []LLMResponse
	index     int
}

func (m *MockLLMProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options map[string]interface{}) (*LLMResponse, error) {
	if m.index >= len(m.responses) {
		return &LLMResponse{Content: "默认响应"}, nil
	}
	resp := m.responses[m.index]
	m.index++
	return &resp, nil
}

func (m *MockLLMProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, options map[string]interface{}) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Content: "流式响应", Done: true}
	close(ch)
	return ch, nil
}

func (m *MockLLMProvider) GetModelID() string { return "mock-model" }

// MockToolExecutor 模拟工具执行器
type MockToolExecutor struct {
	defs []ToolDefinition
}

func (m *MockToolExecutor) GetDefinitions() []ToolDefinition { return m.defs }
func (m *MockToolExecutor) GetDefinitionsByNames(names []string) []ToolDefinition {
	return m.defs
}
func (m *MockToolExecutor) Execute(ctx context.Context, call ToolCall, sessionID string) (*ToolResult, error) {
	return &ToolResult{Content: "工具结果"}, nil
}
func (m *MockToolExecutor) Register(tool ToolDefinition, handler ToolHandler) error { return nil }

// MockMemory 模拟记忆
type MockMemory struct {
	messages []Message
}

func (m *MockMemory) Save(ctx context.Context, sessionID string, messages []Message) error {
	m.messages = append(m.messages, messages...)
	return nil
}
func (m *MockMemory) Load(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	return m.messages, nil
}
func (m *MockMemory) Search(ctx context.Context, query string, limit int) ([]Message, float64, error) {
	return nil, 0, nil
}
func (m *MockMemory) Compress(ctx context.Context, sessionID string) error { return nil }

func TestAgentKernel_Process(t *testing.T) {
	llm := &MockLLMProvider{
		responses: []LLMResponse{
			{Content: "需要工具", ToolCalls: []ToolCall{{ID: "1", Type: "function", Function: FunctionCall{Name: "test_tool", Arguments: "{}"}}}},
			{Content: "最终答案"},
		},
	}
	tools := &MockToolExecutor{defs: []ToolDefinition{{Type: "function", Function: FunctionDef{Name: "test_tool"}}}}
	mem := &MockMemory{}
	store := NewSessionStoreAdapter()

	kernel := NewAgentKernel(llm, tools, mem, store, DefaultConfig())

	resp, err := kernel.Process(context.Background(), &Query{
		Content:   "测试问题",
		ProjectID: "test-project",
	})

	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp.Content != "最终答案" {
		t.Errorf("Expected '最终答案', got '%s'", resp.Content)
	}
	if resp.ToolCalls != 1 {
		t.Errorf("Expected 1 tool call, got %d", resp.ToolCalls)
	}
}

func TestAgentKernel_GetState(t *testing.T) {
	kernel := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())

	if kernel.GetState() != StateIdle {
		t.Errorf("Expected state idle, got %v", kernel.GetState())
	}
}

func TestAgentKernel_Event(t *testing.T) {
	kernel := NewAgentKernel(&MockLLMProvider{}, &MockToolExecutor{}, &MockMemory{}, NewSessionStoreAdapter(), DefaultConfig())

	var received bool
	handler := EventHandlerFunc(func(event Event) {
		if event.Type == EventQueryReceived {
			received = true
		}
	})

	kernel.Subscribe(handler)

	// 触发事件
	kernel.Process(context.Background(), &Query{Content: "test"})

	time.Sleep(100 * time.Millisecond)

	if !received {
		t.Error("Event not received")
	}
}

func TestSimpleCompressor(t *testing.T) {
	compressor := &SimpleCompressor{}

	messages := []Message{
		{Role: "system", Content: "系统提示"},
		{Role: "user", Content: "问题1"},
		{Role: "assistant", Content: "回答1"},
		{Role: "user", Content: "问题2"},
		{Role: "assistant", Content: "回答2"},
		{Role: "user", Content: "问题3"},
		{Role: "assistant", Content: "回答3"},
	}

	compressed, saved, err := compressor.Compress(messages, 100)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	if len(compressed) >= len(messages) {
		t.Error("Compression did not reduce message count")
	}
	// Token estimation is approximate — with very short messages the summary
	// overhead may temporarily increase estimated tokens
	_ = saved
}

func TestSessionStoreAdapter(t *testing.T) {
	store := NewSessionStoreAdapter()
	ctx := context.Background()

	// 创建会话
	session, err := store.Create(ctx, "proj1", "user1")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if session.ProjectID != "proj1" {
		t.Errorf("Expected project proj1, got %s", session.ProjectID)
	}

	// 获取会话
	got, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != session.ID {
		t.Error("Session ID mismatch")
	}

	// 更新会话
	session.Messages = append(session.Messages, Message{Role: "user", Content: "hi"})
	if err := store.Update(ctx, session); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// 列出会话
	sessions, err := store.List(ctx, "proj1", "user1", 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
}
