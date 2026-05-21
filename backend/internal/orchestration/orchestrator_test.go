package orchestration

import (
	"context"
	"testing"

	"openaide/backend/internal/kernel"
)

type mockKernel struct {
	state kernel.KernelState
}

func (m *mockKernel) Process(ctx context.Context, query *kernel.Query) (*kernel.Response, error) {
	return &kernel.Response{Content: "mock response"}, nil
}

func (m *mockKernel) ProcessStream(ctx context.Context, query *kernel.Query) (<-chan kernel.StreamChunk, error) {
	ch := make(chan kernel.StreamChunk, 1)
	ch <- kernel.StreamChunk{Content: "mock stream", Done: true}
	close(ch)
	return ch, nil
}

func (m *mockKernel) GetState() kernel.KernelState { return m.state }
func (m *mockKernel) Subscribe(handler kernel.EventHandler) {}
func (m *mockKernel) Unsubscribe(handler kernel.EventHandler) {}

type mockLLMProvider struct{}

func (m *mockLLMProvider) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	return &kernel.LLMResponse{Content: "mock"}, nil
}
func (m *mockLLMProvider) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	ch := make(chan kernel.StreamChunk, 1)
	ch <- kernel.StreamChunk{Content: "mock", Done: true}
	close(ch)
	return ch, nil
}
func (m *mockLLMProvider) GetModelID() string { return "mock" }

type mockToolExecutor struct {
	defs []kernel.ToolDefinition
}

func (m *mockToolExecutor) GetDefinitions() []kernel.ToolDefinition { return m.defs }
func (m *mockToolExecutor) GetDefinitionsByNames(names []string) []kernel.ToolDefinition {
	return m.defs
}
func (m *mockToolExecutor) Execute(ctx context.Context, call kernel.ToolCall, sessionID string) (*kernel.ToolResult, error) {
	return &kernel.ToolResult{Content: "mock result"}, nil
}
func (m *mockToolExecutor) Register(tool kernel.ToolDefinition, handler kernel.ToolHandler) error {
	return nil
}

type mockMemory struct{}

func (m *mockMemory) Save(ctx context.Context, sessionID string, messages []kernel.Message) error {
	return nil
}
func (m *mockMemory) Load(ctx context.Context, sessionID string, limit int) ([]kernel.Message, error) {
	return nil, nil
}
func (m *mockMemory) Search(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	return nil, 0, nil
}
func (m *mockMemory) Compress(ctx context.Context, sessionID string) error { return nil }

func TestOrchestrator_CreateSession(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())

	session, err := o.CreateSession(context.Background(), "proj1", "user1")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if session.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if session.ProjectID != "proj1" {
		t.Errorf("expected proj1, got %s", session.ProjectID)
	}
}

func TestOrchestrator_DeleteSession(t *testing.T) {
	store := kernel.NewSessionStoreAdapter()
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, store)

	session, _ := o.CreateSession(context.Background(), "proj1", "user1")

	if err := o.DeleteSession(context.Background(), session.ID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	if _, err := o.GetSession(context.Background(), session.ID); err == nil {
		t.Error("expected error after delete")
	}
}

func TestOrchestrator_GetSession(t *testing.T) {
	store := kernel.NewSessionStoreAdapter()
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, store)

	created, _ := o.CreateSession(context.Background(), "proj1", "user1")

	got, err := o.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.ID != created.ID {
		t.Error("session ID mismatch")
	}
}

func TestOrchestrator_GetToolDefinitions(t *testing.T) {
	defs := []kernel.ToolDefinition{
		{Type: "function", Function: kernel.FunctionDef{Name: "tool1"}},
	}
	exec := &mockToolExecutor{defs: defs}
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, exec, &mockMemory{}, kernel.NewSessionStoreAdapter())

	got := o.GetToolDefinitions()
	if len(got) != 1 || got[0].Function.Name != "tool1" {
		t.Errorf("expected 1 tool def named tool1, got %#v", got)
	}
}

func TestOrchestrator_GetSessionHistory(t *testing.T) {
	store := kernel.NewSessionStoreAdapter()
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, store)

	session, _ := o.CreateSession(context.Background(), "proj1", "user1")

	history, err := o.GetSessionHistory(context.Background(), session.ID, 10)
	if err != nil {
		t.Fatalf("GetSessionHistory failed: %v", err)
	}
	if history == nil {
		t.Error("expected non-nil history")
	}
}

func TestOrchestrator_ListSessions(t *testing.T) {
	store := kernel.NewSessionStoreAdapter()
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, store)

	o.CreateSession(context.Background(), "proj1", "user1")
	o.CreateSession(context.Background(), "proj1", "user1")

	sessions, err := o.ListSessions(context.Background(), "proj1", "user1", 10, 0)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestOrchestrator_ProcessQuery(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())

	resp, err := o.ProcessQuery(context.Background(), "user1", "proj1", "hello", kernel.QueryOptions{})
	if err != nil {
		t.Fatalf("ProcessQuery failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
}

func TestOrchestrator_ProcessQueryStream(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())

	ch, err := o.ProcessQueryStream(context.Background(), "user1", "proj1", "hello", kernel.QueryOptions{})
	if err != nil {
		t.Fatalf("ProcessQueryStream failed: %v", err)
	}

	chunks := make([]kernel.StreamChunk, 0)
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
}

func TestOrchestrator_GetStats(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())

	stats := o.GetStats()
	if stats == nil {
		t.Error("expected non-nil stats")
	}
	if _, ok := stats["kernel_state"]; !ok {
		t.Error("expected kernel_state in stats")
	}
}
