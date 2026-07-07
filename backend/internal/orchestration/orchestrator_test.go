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
func (m *mockKernel) Subscribe(handler kernel.EventHandler) uint64 { return 0 }
func (m *mockKernel) Unsubscribe(id uint64)                        {}
func (m *mockKernel) GetSlashCommands() map[string]string  { return nil }
func (m *mockKernel) TaskMetricsSummary() map[string]interface{} { return nil }
func (m *mockKernel) RecentTasks(n int) []kernel.TaskMetrics { return nil }

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
func (m *mockLLMProvider) SetModelID(model string)  {}

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

// ============ Pure function tests ============

func TestPickModel(t *testing.T) {
	tests := []struct{ role, want string }{
		{"analyst", "reasoning"}, {"reviewer", "reasoning"},
		{"coder", "execution"}, {"executor", "execution"},
		{"synthesis", "execution"}, {"unknown", "execution"},
	}
	for _, tt := range tests {
		if got := pickModel(tt.role); got != tt.want {
			t.Errorf("pickModel(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

func TestGroupByDependency(t *testing.T) {
	if g := groupByDependency(nil); g != nil {
		t.Error("nil should return nil")
	}
	if g := groupByDependency([]SubTask{}); g != nil {
		t.Error("empty should return nil")
	}

	single := []SubTask{{ID: 1, Title: "t1"}}
	if g := groupByDependency(single); len(g) != 1 || len(g[0]) != 1 {
		t.Error("single task: expected 1 group of 1")
	}

	// Two independent
	indep := []SubTask{{ID: 1}, {ID: 2}}
	if g := groupByDependency(indep); len(g) != 1 || len(g[0]) != 2 {
		t.Error("independent tasks: expected 1 group of 2")
	}

	// Dependent
	dep := []SubTask{{ID: 1}, {ID: 2, DependsOn: []int{1}}}
	if g := groupByDependency(dep); len(g) != 2 {
		t.Errorf("dependent: expected 2 groups, got %d", len(g))
	}

	// Cycle
	cyc := []SubTask{{ID: 1, DependsOn: []int{2}}, {ID: 2, DependsOn: []int{1}}}
	if g := groupByDependency(cyc); len(g) == 0 {
		t.Error("cycle should not hang")
	}
}

func TestDetectBranchSignal(t *testing.T) {
	tests := []struct{ content string; hit bool; text string }{
		{"normal", false, ""},
		{"[DISCOVERY:] found bug", true, "[DISCOVERY:] found bug"},
		{"DISCOVERY: important\nmore", true, "DISCOVERY: important"},
		{"[REPLAN:] new approach", true, "[REPLAN:] new approach"},
	}
	for _, tt := range tests {
		hit, text := detectBranchSignal(tt.content)
		if hit != tt.hit || (hit && text != tt.text) {
			t.Errorf("detectBranchSignal(%q) = (%v, %q)", tt.content, hit, text)
		}
	}
}

func TestFormatResults(t *testing.T) {
	f := formatResults([]string{"r1", "", "r3"})
	if f == "" || !stringsContains(f, "Result 1") || stringsContains(f, "Result 2") {
		t.Errorf("formatResults unexpected: %s", f)
	}
}

func TestMin(t *testing.T) {
	if min(1, 2) != 1 || min(5, 3) != 3 || min(0, 0) != 0 {
		t.Error("min failed")
	}
}

func TestMinStr(t *testing.T) {
	if minStr("hello", 3) != "hel" || minStr("hi", 10) != "hi" {
		t.Error("minStr failed")
	}
}

func TestTruncateForLearning(t *testing.T) {
	if got := truncateForLearning("short"); got != "short" {
		t.Error("short text should not be truncated")
	}
}

// ============ Plan parsing tests ============

func TestParsePlan(t *testing.T) {
	valid := `{"goal":"g","subtasks":[{"id":1,"title":"t","description":"d"}]}`
	p, err := parsePlan(valid)
	if err != nil || p.Goal != "g" || len(p.Subtasks) != 1 {
		t.Error("parsePlan failed")
	}
	if _, err := parsePlan("no json"); err == nil {
		t.Error("expected error for no JSON")
	}
	if _, err := parsePlan(`{"goal":"g","subtasks":[]}`); err == nil {
		t.Error("expected error for empty subtasks")
	}
}

func TestParsePlanFromFC(t *testing.T) {
	valid := `{"goal":"g","subtasks":[{"id":1,"title":"t","description":"d"}]}`
	p, err := parsePlanFromFC(valid)
	if err != nil || p.Goal != "g" {
		t.Error("parsePlanFromFC failed")
	}
}

func TestExtractJSON(t *testing.T) {
	if got := extractJSON(`{"k":"v"}`); got != `{"k":"v"}` {
		t.Errorf("plain JSON: got %q", got)
	}
	if got := extractJSON("```json\n{\"k\":\"v\"}\n```"); got != `{"k":"v"}` {
		t.Errorf("markdown JSON: got %q", got)
	}
	if got := extractJSON("no json"); got != "" {
		t.Error("expected empty for no JSON")
	}
}

func TestParseResearch(t *testing.T) {
	valid := `{"findings":"clean","modules":"m","risks":"none","complexity":"low"}`
	r, err := parseResearch(valid)
	if err != nil || r.Findings != "clean" {
		t.Error("parseResearch failed")
	}
	if _, err := parseResearch(`{"findings":"","modules":"","risks":"","complexity":""}`); err == nil {
		t.Error("expected error for empty findings")
	}
}

func TestParseProposals(t *testing.T) {
	valid := `{"goal":"g","options":[{"name":"A","description":"d","reasoning":"r","pros":"p","cons":"c","risk":"low","effort":"1d"}]}`
	p, err := parseProposals(valid)
	if err != nil || len(p.Options) != 1 || p.Options[0].Name != "A" {
		t.Error("parseProposals failed")
	}
	if _, err := parseProposals("no json"); err == nil {
		t.Error("expected error for no JSON")
	}
}

// ============ Team tests ============

func TestNewTeam(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	if team == nil || team.orchestrator != o {
		t.Error("NewTeam failed")
	}
}

func TestTeam_GetRole(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	if team.GetRole("analyst") == nil || team.GetRole("nonexistent") != nil {
		t.Error("GetRole failed")
	}
	var nilTeam *Team
	if nilTeam.GetRole("analyst") != nil {
		t.Error("nil team should return nil")
	}
}

func TestTeam_BuildAllChain(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	g := team.buildAllChain("analyst")
	if g == nil || len(g.Nodes) != 3 {
		t.Errorf("buildAllChain: expected 3 nodes, got %d", len(g.Nodes))
	}
}

func stringsContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
