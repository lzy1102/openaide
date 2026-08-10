package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/orchestration"
)

// ============ Mock implementations (same pattern as orchestration tests) ============

type apiMockKernel struct {
	state      kernel.KernelState
	lastQuery  *kernel.Query
	processErr error
	tasks      []kernel.TaskMetrics
}

func (m *apiMockKernel) Process(ctx context.Context, query *kernel.Query) (*kernel.Response, error) {
	m.lastQuery = query
	if m.processErr != nil {
		return nil, m.processErr
	}
	return &kernel.Response{Content: "mock response", TokensUsed: 42, Model: "mock-model"}, nil
}

func (m *apiMockKernel) ProcessStream(ctx context.Context, query *kernel.Query) (<-chan kernel.StreamChunk, error) {
	m.lastQuery = query
	ch := make(chan kernel.StreamChunk, 2)
	ch <- kernel.StreamChunk{Type: kernel.ChunkTypeContent, Content: "mock stream"}
	ch <- kernel.StreamChunk{Type: kernel.ChunkTypeDone, Done: true, Usage: &kernel.TokenUsage{TotalTokens: 7}}
	close(ch)
	return ch, nil
}

func (m *apiMockKernel) GetState() kernel.KernelState                 { return m.state }
func (m *apiMockKernel) Subscribe(handler kernel.EventHandler) uint64 { return 0 }
func (m *apiMockKernel) Unsubscribe(id uint64)                        {}
func (m *apiMockKernel) GetSlashCommands() map[string]string {
	return map[string]string{"/x": "skill-x"}
}
func (m *apiMockKernel) TaskMetricsSummary() map[string]interface{} {
	return map[string]interface{}{"total_tasks": len(m.tasks)}
}
func (m *apiMockKernel) RecentTasks(n int) []kernel.TaskMetrics { return m.tasks }

type apiMockLLMProvider struct{}

func (m *apiMockLLMProvider) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	return &kernel.LLMResponse{Content: "mock"}, nil
}
func (m *apiMockLLMProvider) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	ch := make(chan kernel.StreamChunk, 1)
	ch <- kernel.StreamChunk{Content: "mock", Done: true}
	close(ch)
	return ch, nil
}
func (m *apiMockLLMProvider) GetModelID() string { return "mock-model" }

type apiMockToolExecutor struct {
	defs []kernel.ToolDefinition
}

func (m *apiMockToolExecutor) GetDefinitions() []kernel.ToolDefinition { return m.defs }
func (m *apiMockToolExecutor) GetDefinitionsByNames(names []string) []kernel.ToolDefinition {
	return m.defs
}
func (m *apiMockToolExecutor) Execute(ctx context.Context, call kernel.ToolCall, sessionID string) (*kernel.ToolResult, error) {
	return &kernel.ToolResult{Content: "mock result"}, nil
}
func (m *apiMockToolExecutor) Register(tool kernel.ToolDefinition, handler kernel.ToolHandler) error {
	return nil
}

type apiMockMemory struct{}

func (m *apiMockMemory) Save(ctx context.Context, sessionID string, messages []kernel.Message) error {
	return nil
}
func (m *apiMockMemory) Load(ctx context.Context, sessionID string, limit int) ([]kernel.Message, error) {
	return nil, nil
}
func (m *apiMockMemory) Search(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error) {
	return []kernel.Message{{Role: "user", Content: "remembered fact"}}, 0.85, nil
}

// newTestServer builds a real Server with a real Orchestrator over mock deps.
func newTestServer(t *testing.T) (*Server, *apiMockKernel) {
	t.Helper()
	mk := &apiMockKernel{}
	toolExec := &apiMockToolExecutor{defs: []kernel.ToolDefinition{
		{Type: "function", Function: kernel.FunctionDef{Name: "read_file"}},
	}}
	orch := orchestration.NewOrchestrator(mk, &apiMockLLMProvider{}, toolExec, &apiMockMemory{}, kernel.NewSessionStoreAdapter())
	s := NewServer(orch, "127.0.0.1:0", nil)
	s.SetKernel(mk)
	return s, mk
}

func doRequest(t *testing.T, s *Server, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

// resetMetrics zeroes the package-level metric counters so tests are isolated.
func resetMetrics() {
	metricsRequests.Store(0)
	metricsTokens.Store(0)
	metricsToolCalls.Store(0)
	metricsErrors.Store(0)
}

// ============ handleChat ============

func TestHandleChat_Success(t *testing.T) {
	s, _ := newTestServer(t)
	body := strings.NewReader(`{"message":"hello","user_id":"u1","project_id":"p1"}`)
	w := doRequest(t, s, http.MethodPost, "/api/v1/chat", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ChatResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Content != "mock response" {
		t.Errorf("expected mock response, got %q", resp.Content)
	}
	if resp.TokensUsed != 42 {
		t.Errorf("expected 42 tokens, got %d", resp.TokensUsed)
	}
	if resp.Model != "mock-model" {
		t.Errorf("expected mock-model, got %q", resp.Model)
	}
}

func TestHandleChat_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/chat", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleChat_BadJSON(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/v1/chat", strings.NewReader(`{"message":`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChat_OrchestratorError(t *testing.T) {
	s, _ := newTestServer(t)
	mk := &apiMockKernel{processErr: fmt.Errorf("kernel boom")}
	orch := orchestration.NewOrchestrator(mk, &apiMockLLMProvider{}, &apiMockToolExecutor{}, &apiMockMemory{}, kernel.NewSessionStoreAdapter())
	s.orchestrator = orch

	w := doRequest(t, s, http.MethodPost, "/api/v1/chat", strings.NewReader(`{"message":"hi"}`))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	var er ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &er)
	if !strings.Contains(er.Error, "kernel boom") {
		t.Errorf("expected kernel boom in error, got %q", er.Error)
	}
}

// ============ handleChatStream ============

func TestHandleChatStream_SSE(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/v1/chat/stream", strings.NewReader(`{"message":"hello"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "mock stream") {
		t.Errorf("expected mock stream chunk, got: %s", body)
	}
	if !strings.Contains(body, `"type":"done"`) && !strings.Contains(body, `"type": "done"`) {
		t.Errorf("expected done event, got: %s", body)
	}
}

func TestHandleChatStream_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPut, "/api/v1/chat/stream", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleChatStream_BadJSON(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/v1/chat/stream", strings.NewReader("not json"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChatStream_StreamError(t *testing.T) {
	s, _ := newTestServer(t)
	ch := make(chan kernel.StreamChunk, 1)
	ch <- kernel.StreamChunk{Type: kernel.ChunkTypeError, Error: fmt.Errorf("stream fail")}
	close(ch)

	mk := &streamErrKernel{ch: ch}
	orch := orchestration.NewOrchestrator(mk, &apiMockLLMProvider{}, &apiMockToolExecutor{}, &apiMockMemory{}, kernel.NewSessionStoreAdapter())
	s.orchestrator = orch

	w := doRequest(t, s, http.MethodPost, "/api/v1/chat/stream", strings.NewReader(`{"message":"hi"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for SSE start, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "stream fail") {
		t.Errorf("expected stream fail in body, got: %s", w.Body.String())
	}
}

type streamErrKernel struct {
	apiMockKernel
	ch <-chan kernel.StreamChunk
}

func (m *streamErrKernel) ProcessStream(ctx context.Context, query *kernel.Query) (<-chan kernel.StreamChunk, error) {
	return m.ch, nil
}

// ============ handleSessions ============

func TestHandleSessions_List(t *testing.T) {
	s, _ := newTestServer(t)
	orch := s.orchestrator
	orch.CreateSession(context.Background(), "p1", "u1")
	orch.CreateSession(context.Background(), "p1", "u1")

	w := doRequest(t, s, http.MethodGet, "/api/v1/sessions?project_id=p1&user_id=u1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var sessions []SessionInfo
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestHandleSessions_ListLimitBounds(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/sessions?limit=999&offset=-1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleSessions_Create(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/v1/sessions", strings.NewReader(`{"project_id":"p9","user_id":"u9"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var si SessionInfo
	if err := json.Unmarshal(w.Body.Bytes(), &si); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if si.ProjectID != "p9" || si.UserID != "u9" {
		t.Errorf("unexpected session info: %+v", si)
	}
	if si.ID == "" {
		t.Error("expected non-empty session ID")
	}
}

func TestHandleSessions_CreateBadJSON(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/v1/sessions", strings.NewReader(`{`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSessions_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodDelete, "/api/v1/sessions", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ============ handleSessionDetail ============

func TestHandleSessionDetail_Get(t *testing.T) {
	s, _ := newTestServer(t)
	sess, _ := s.orchestrator.CreateSession(context.Background(), "p1", "u1")

	w := doRequest(t, s, http.MethodGet, "/api/v1/sessions/"+sess.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Session  SessionInfo   `json:"session"`
		Messages []MessageInfo `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Session.ID != sess.ID {
		t.Errorf("session ID mismatch: %s vs %s", body.Session.ID, sess.ID)
	}
}

func TestHandleSessionDetail_GetForbiddenForOtherUser(t *testing.T) {
	s, _ := newTestServer(t)
	sess, _ := s.orchestrator.CreateSession(context.Background(), "p1", "owner")

	w := doRequest(t, s, http.MethodGet, "/api/v1/sessions/"+sess.ID+"?user_id=other", nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandleSessionDetail_GetOwnedByUser(t *testing.T) {
	s, _ := newTestServer(t)
	sess, _ := s.orchestrator.CreateSession(context.Background(), "p1", "owner")

	w := doRequest(t, s, http.MethodGet, "/api/v1/sessions/"+sess.ID+"?user_id=owner", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for owner, got %d", w.Code)
	}
}

func TestHandleSessionDetail_GetNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/sessions/does-not-exist", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSessionDetail_MissingID(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/sessions/", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSessionDetail_Delete(t *testing.T) {
	s, _ := newTestServer(t)
	sess, _ := s.orchestrator.CreateSession(context.Background(), "p1", "u1")

	w := doRequest(t, s, http.MethodDelete, "/api/v1/sessions/"+sess.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "deleted") {
		t.Errorf("expected deleted marker, got %s", w.Body.String())
	}
	if _, err := s.orchestrator.GetSession(context.Background(), sess.ID); err == nil {
		t.Error("expected session to be deleted")
	}
}

func TestHandleSessionDetail_DeleteForbidden(t *testing.T) {
	s, _ := newTestServer(t)
	sess, _ := s.orchestrator.CreateSession(context.Background(), "p1", "owner")

	w := doRequest(t, s, http.MethodDelete, "/api/v1/sessions/"+sess.ID+"?user_id=other", nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandleSessionDetail_DeleteNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodDelete, "/api/v1/sessions/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============ handleMemorySearch ============

func TestHandleMemorySearch_Success(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/memory/search?q=something", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp MemorySearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Query != "something" {
		t.Errorf("expected query echo, got %q", resp.Query)
	}
	if len(resp.Results) != 1 || resp.Results[0].Content != "remembered fact" {
		t.Errorf("unexpected results: %+v", resp.Results)
	}
	if resp.Score != 0.85 {
		t.Errorf("expected 0.85, got %v", resp.Score)
	}
}

func TestHandleMemorySearch_MissingQuery(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/memory/search", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMemorySearch_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/v1/memory/search?q=x", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ============ handleTools / handleStats / handleMetrics ============

func TestHandleTools(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/tools", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var defs []kernel.ToolDefinition
	if err := json.Unmarshal(w.Body.Bytes(), &defs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if defs == nil {
		t.Error("expected non-nil tool defs (empty slice ok)")
	}
}

func TestHandleTools_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/v1/tools", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleStats(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/stats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var stats map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := stats["kernel_state"]; !ok {
		t.Errorf("expected kernel_state in stats, got %v", stats)
	}
}

func TestHandleMetrics(t *testing.T) {
	s, _ := newTestServer(t)
	resetMetrics()
	RecordMetrics(100, 3, false)
	RecordMetrics(50, 1, true)
	defer func() {
		metricsRequests.Store(0)
		metricsTokens.Store(0)
		metricsToolCalls.Store(0)
		metricsErrors.Store(0)
	}()

	w := doRequest(t, s, http.MethodGet, "/api/v1/metrics", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var stats map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stats["requests_total"] != float64(2) {
		t.Errorf("expected 2 requests, got %v", stats["requests_total"])
	}
	if stats["tokens_total"] != float64(150) {
		t.Errorf("expected 150 tokens, got %v", stats["tokens_total"])
	}
	if stats["tool_calls_total"] != float64(4) {
		t.Errorf("expected 4 tool calls, got %v", stats["tool_calls_total"])
	}
	if stats["errors_total"] != float64(1) {
		t.Errorf("expected 1 error, got %v", stats["errors_total"])
	}
}

// ============ handleTasks ============

func TestHandleTasks_NoKernel(t *testing.T) {
	s, _ := newTestServer(t)
	s.kernel = nil
	w := doRequest(t, s, http.MethodGet, "/api/v1/tasks", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["total_tasks"] != float64(0) {
		t.Errorf("expected 0 tasks, got %v", body["total_tasks"])
	}
}

func TestHandleTasks_Summary(t *testing.T) {
	s, mk := newTestServer(t)
	mk.tasks = []kernel.TaskMetrics{{ID: "t1", TaskType: "coding", Success: true}}
	w := doRequest(t, s, http.MethodGet, "/api/v1/tasks?summary=true", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["total_tasks"] != float64(1) {
		t.Errorf("expected 1 task, got %v", body["total_tasks"])
	}
}

func TestHandleTasks_List(t *testing.T) {
	s, mk := newTestServer(t)
	mk.tasks = []kernel.TaskMetrics{{ID: "t1", TaskType: "coding", Success: true}, {ID: "t2", TaskType: "debugging", Success: false}}
	w := doRequest(t, s, http.MethodGet, "/api/v1/tasks?n=5", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var tasks []kernel.TaskMetrics
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

// ============ handlePrometheus ============

func TestHandlePrometheus(t *testing.T) {
	s, _ := newTestServer(t)
	resetMetrics()
	RecordMetrics(10, 1, false)
	defer func() {
		metricsRequests.Store(0)
		metricsTokens.Store(0)
		metricsToolCalls.Store(0)
		metricsErrors.Store(0)
	}()

	w := doRequest(t, s, http.MethodGet, "/metrics", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"openaide_requests_total", "openaide_tokens_total",
		"openaide_tool_calls_total", "openaide_errors_total",
		"openaide_requests_total 1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in prometheus output", want)
		}
	}
}

func TestHandlePrometheus_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/metrics", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ============ handleChannels ============

func TestHandleChannels_NoRegistry(t *testing.T) {
	s, _ := newTestServer(t)
	s.channelRegistry = nil
	w := doRequest(t, s, http.MethodGet, "/api/v1/channels", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "[]") && body != "[]\n" {
		t.Errorf("expected empty array, got %s", body)
	}
}

func TestHandleChannels_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodPost, "/api/v1/channels", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ============ handleConfig ============

func TestHandleConfig_GetNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	w := doRequest(t, s, http.MethodGet, "/api/v1/config", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleConfig_PutThenGet(t *testing.T) {
	s, _ := newTestServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".openaide"), 0755)

	w := doRequest(t, s, http.MethodPut, "/api/v1/config", strings.NewReader("llm:\n  model: test\n"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on PUT, got %d: %s", w.Code, w.Body.String())
	}

	w2 := doRequest(t, s, http.MethodGet, "/api/v1/config", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "model: test") {
		t.Errorf("expected config content, got %s", w2.Body.String())
	}
}

func TestHandleConfig_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodDelete, "/api/v1/config", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ============ handleProjects ============

func TestHandleProjects_EmptyList(t *testing.T) {
	s, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	w := doRequest(t, s, http.MethodGet, "/api/v1/projects", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var projects []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &projects); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestHandleProjects_CreateAndList(t *testing.T) {
	s, _ := newTestServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".openaide"), 0755)
	projDir := t.TempDir()

	payload := fmt.Sprintf(`{"name":"myproj","path":%q}`, projDir)
	w := doRequest(t, s, http.MethodPost, "/api/v1/projects", strings.NewReader(payload))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created["name"] != "myproj" {
		t.Errorf("expected name myproj, got %v", created["name"])
	}
	if id, _ := created["id"].(string); id == "" {
		t.Error("expected non-empty project id")
	}

	w2 := doRequest(t, s, http.MethodGet, "/api/v1/projects", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	var projects []map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &projects); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}
}

func TestHandleProjects_CreateInvalidDir(t *testing.T) {
	s, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	w := doRequest(t, s, http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"x","path":"/nonexistent/dir/xyz"}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid dir, got %d", w.Code)
	}
}

func TestHandleProjects_CreateBadJSON(t *testing.T) {
	s, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	w := doRequest(t, s, http.MethodPost, "/api/v1/projects", strings.NewReader(`{bad`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ============ handleProjectDetail ============

func TestHandleProjectDetail_CRUD(t *testing.T) {
	s, _ := newTestServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".openaide"), 0755)
	projDir := t.TempDir()

	w := doRequest(t, s, http.MethodPost, "/api/v1/projects", strings.NewReader(fmt.Sprintf(`{"name":"proj","path":%q}`, projDir)))
	if w.Code != http.StatusOK {
		t.Fatalf("create failed: %d", w.Code)
	}
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	w2 := doRequest(t, s, http.MethodGet, "/api/v1/projects/"+id, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET detail, got %d: %s", w2.Code, w2.Body.String())
	}
	var got map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &got)
	if got["id"] != id {
		t.Errorf("detail ID mismatch: %v", got["id"])
	}

	w3 := doRequest(t, s, http.MethodPut, "/api/v1/projects/"+id, strings.NewReader(fmt.Sprintf(`{"name":"renamed","path":%q}`, projDir)))
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 on PUT, got %d: %s", w3.Code, w3.Body.String())
	}
	var updated map[string]interface{}
	json.Unmarshal(w3.Body.Bytes(), &updated)
	if updated["name"] != "renamed" {
		t.Errorf("expected renamed, got %v", updated["name"])
	}

	w4 := doRequest(t, s, http.MethodDelete, "/api/v1/projects/"+id, nil)
	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200 on DELETE, got %d: %s", w4.Code, w4.Body.String())
	}

	w5 := doRequest(t, s, http.MethodGet, "/api/v1/projects/"+id, nil)
	if w5.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w5.Code)
	}
}

func TestHandleProjectDetail_GetNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	w := doRequest(t, s, http.MethodGet, "/api/v1/projects/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleProjectDetail_MissingID(t *testing.T) {
	s, _ := newTestServer(t)
	w := doRequest(t, s, http.MethodGet, "/api/v1/projects/", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectDetail_UpdateNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	w := doRequest(t, s, http.MethodPut, "/api/v1/projects/nope", strings.NewReader(`{"name":"x"}`))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleProjectDetail_DeleteNotFound(t *testing.T) {
	s, _ := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	w := doRequest(t, s, http.MethodDelete, "/api/v1/projects/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============ sessionToInfo / trace helpers ============

func TestSessionToInfo(t *testing.T) {
	sess := &kernel.Session{
		ID:        "s1",
		ProjectID: "p1",
		UserID:    "u1",
		Messages:  []kernel.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	info := sessionToInfo(sess)
	if info.ID != "s1" || info.ProjectID != "p1" || info.UserID != "u1" {
		t.Errorf("unexpected session info: %+v", info)
	}
	if info.MessageCount != 2 {
		t.Errorf("expected 2 messages, got %d", info.MessageCount)
	}
	if info.CreatedAt == "" || info.UpdatedAt == "" {
		t.Error("expected timestamps to be formatted")
	}
}

func TestSessionToInfo_ZeroTime(t *testing.T) {
	info := sessionToInfo(&kernel.Session{ID: "x"})
	if info.CreatedAt != "" || info.UpdatedAt != "" {
		t.Errorf("expected empty timestamps for zero time, got %q %q", info.CreatedAt, info.UpdatedAt)
	}
}

func TestWithTraceAndTraceID(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Trace-ID", "abc123")
	ctx := withTrace(req)
	if tid := traceID(ctx); tid != "abc123" {
		t.Errorf("expected abc123, got %q", tid)
	}

	// No header → generated
	req2 := httptest.NewRequest("GET", "/", nil)
	ctx2 := withTrace(req2)
	if tid := traceID(ctx2); tid == "-" || len(tid) != 12 {
		t.Errorf("expected generated 12-char trace id, got %q", tid)
	}
}

func TestTraceID_Empty(t *testing.T) {
	if tid := traceID(context.Background()); tid != "-" {
		t.Errorf("expected -, got %q", tid)
	}
}

// ============ sendSSE ============

func TestSendSSE(t *testing.T) {
	w := httptest.NewRecorder()
	sendSSE(w, w, StreamEvent{Type: "content", Content: "hi"})
	body := w.Body.String()
	if !strings.Contains(body, "data: ") || !strings.Contains(body, "hi") {
		t.Errorf("unexpected SSE body: %q", body)
	}
	if !strings.Contains(body, "\n\n") {
		t.Errorf("expected blank line after SSE event: %q", body)
	}
}
