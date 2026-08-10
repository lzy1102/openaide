package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openaide/backend/internal/kernel"
)

// scriptedLLM returns canned responses in order, then falls back to a default.
type scriptedLLM struct {
	responses []*kernel.LLMResponse
	idx       int
}

func (m *scriptedLLM) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	if m.idx < len(m.responses) {
		r := m.responses[m.idx]
		m.idx++
		return r, nil
	}
	return &kernel.LLMResponse{Content: "mock"}, nil
}

func (m *scriptedLLM) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	ch := make(chan kernel.StreamChunk, 1)
	ch <- kernel.StreamChunk{Content: "mock", Done: true}
	close(ch)
	return ch, nil
}

func (m *scriptedLLM) GetModelID() string { return "mock" }

// errLLM always fails, to exercise fallback paths.
type errLLM struct{}

func (m *errLLM) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	return nil, fmt.Errorf("llm down")
}
func (m *errLLM) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	return nil, fmt.Errorf("llm down")
}
func (m *errLLM) GetModelID() string { return "err" }

// ============ execute.go: routePipeline ============

func TestRoutePipeline_SingleSubtask(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	plan := &Plan{Goal: "g", Subtasks: []SubTask{{ID: 1, Title: "t", Description: "d"}}}
	m := o.routePipeline(context.Background(), plan)
	if len(m) != 1 {
		t.Fatalf("expected 1 role assignment, got %d", len(m))
	}
	if m[0] != o.firstRoleName() {
		t.Errorf("expected firstRoleName fallback, got %q", m[0])
	}
}

func TestRoutePipeline_EmptyPlan(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	m := o.routePipeline(context.Background(), &Plan{})
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestRoutePipeline_MultiSubtask_LLMFailure(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &errLLM{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	plan := &Plan{Goal: "g", Subtasks: []SubTask{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}}}
	m := o.routePipeline(context.Background(), plan)
	// LLM failure → fallback to firstRoleName for every subtask
	if len(m) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(m))
	}
	for i := 0; i < 2; i++ {
		if m[i] != o.firstRoleName() {
			t.Errorf("expected fallback role for %d, got %q", i, m[i])
		}
	}
}

func TestRoutePipeline_MultiSubtask_Parsed(t *testing.T) {
	llm := &scriptedLLM{responses: []*kernel.LLMResponse{
		{Content: "1=coder,2=reviewer"},
	}}
	o := NewOrchestrator(&mockKernel{}, llm, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	plan := &Plan{Goal: "g", Subtasks: []SubTask{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}}}
	m := o.routePipeline(context.Background(), plan)
	if m[0] != "coder" || m[1] != "reviewer" {
		t.Errorf("expected coder/reviewer, got %v", m)
	}
}

// ============ execute.go: executeBranch ============

func TestExecuteBranch(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	o.SetTeam(NewTeam(o))
	var branches []Branch
	b := o.executeBranch(context.Background(), "u1", "p1", "[DISCOVERY:] found bug", []string{"r1"}, &branches)
	if b.Trigger == "" {
		t.Error("expected non-empty trigger")
	}
	if b.Parent != "main" {
		t.Errorf("expected parent main, got %q", b.Parent)
	}
}

func TestExecuteBranch_SubAgentFails(t *testing.T) {
	// No team → RunSubAgent fails → branch returned with empty result, no panic
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	var branches []Branch
	b := o.executeBranch(context.Background(), "u1", "p1", "[DISCOVERY:] x", []string{}, &branches)
	if b.Trigger == "" {
		t.Error("expected trigger recorded")
	}
	if b.Result != "" {
		t.Errorf("expected empty result on failure, got %q", b.Result)
	}
}

// ============ execute.go: firstRoleName / reportProgress ============

func TestFirstRoleName_NoTeam(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	if got := o.firstRoleName(); got != "coder" {
		t.Errorf("expected coder fallback, got %q", got)
	}
}

func TestFirstRoleName_WithTeam(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	o.SetTeam(NewTeam(o))
	if got := o.firstRoleName(); got == "" {
		t.Error("expected non-empty role name")
	}
}

func TestReportProgress_NilCallback(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	o.reportProgress("execute", "test") // must not panic
}

func TestReportProgress_WithCallback(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	called := false
	o.OnProgress = func(phase, detail string) { called = true }
	o.reportProgress("execute", "test")
	if !called {
		t.Error("expected OnProgress to be called")
	}
}

// ============ execute.go: runLint / execCmd / getwd ============

func TestRunLint_NoProjectFiles(t *testing.T) {
	out, err := runLint(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error for empty dir, got %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
}

func TestRunLint_GoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)
	// golangci-lint likely not installed → error expected, but path must be detected
	out, err := runLint(dir)
	_ = out
	if err == nil && out == "" {
		t.Error("expected either lint output or error for go.mod dir")
	}
}

func TestExecCmd_NotFound(t *testing.T) {
	_, err := execCmd("openaide-nonexistent-cmd-xyz")
	if err == nil {
		t.Error("expected error for missing command")
	}
}

func TestGetwd(t *testing.T) {
	if got := getwd(); got == "" {
		t.Error("expected non-empty cwd")
	}
}

// ============ orchestrator.go: setters/getters ============

func TestOrchestrator_Accessors(t *testing.T) {
	te := &mockToolExecutor{}
	llm := &mockLLMProvider{}
	o := NewOrchestrator(&mockKernel{}, llm, te, &mockMemory{}, kernel.NewSessionStoreAdapter())

	if o.GetToolExecutor() != te {
		t.Error("GetToolExecutor mismatch")
	}
	if o.GetLLMProvider() != llm {
		t.Error("GetLLMProvider mismatch")
	}

	o.SetContextCompressor(nil) // nil-safe
	o.SetPermissionChecker(nil) // nil-safe
	o.SetPlanApprover(nil)      // nil-safe
	o.SetSubAgentTimeout(0)     // 0 → default
	o.SetContextCompressor(&mockCompressor{})
	o.SetPermissionChecker(&mockPermissionChecker{})
	o.SetPlanApprover(func(*Plan) bool { return true })
}

type mockCompressor struct{}

func (m *mockCompressor) Compress(ctx context.Context, messages []kernel.Message, maxTokens int) ([]kernel.Message, int, error) {
	return messages, 0, nil
}
func (m *mockCompressor) EstimateTokens(messages []kernel.Message) int { return len(messages) }

type mockPermissionChecker struct{}

func (m *mockPermissionChecker) Check(ctx context.Context, action, resource string, context map[string]interface{}) (bool, string) {
	return true, ""
}
func (m *mockPermissionChecker) GetLevel() kernel.PermissionLevel { return kernel.LevelStandard }

func TestOrchestrator_CleanupOldSessions(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	o.CreateSession(context.Background(), "p1", "u1")
	o.CleanupOldSessions(context.Background()) // must not panic
}

func TestOrchestrator_CleanupOldSessions_NilStore(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, nil)
	o.CleanupOldSessions(context.Background()) // must not panic
}

func TestOrchestrator_SearchMemory(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	msgs, score, err := o.SearchMemory(context.Background(), "query", 10)
	if err != nil {
		t.Fatalf("SearchMemory failed: %v", err)
	}
	if msgs != nil || score != 0 {
		t.Errorf("expected empty result, got %v %v", msgs, score)
	}
}

func TestOrchestrator_SearchMemory_NilMemory(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, nil, kernel.NewSessionStoreAdapter())
	if _, _, err := o.SearchMemory(context.Background(), "q", 10); err == nil {
		t.Error("expected error when memory not configured")
	}
}

func TestOrchestrator_CompressSession_NoCompressor(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	if err := o.CompressSession(context.Background(), "s1"); err == nil {
		t.Error("expected error when compressor not configured")
	}
}

func TestOrchestrator_UpdateSession(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	sess, _ := o.CreateSession(context.Background(), "p1", "u1")
	sess.Messages = []kernel.Message{{Role: "user", Content: "hi"}}
	if err := o.UpdateSession(context.Background(), sess); err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}
	got, _ := o.GetSession(context.Background(), sess.ID)
	if len(got.Messages) != 1 {
		t.Errorf("expected 1 message after update, got %d", len(got.Messages))
	}
}

func TestOrchestrator_UpdateSession_NilStore(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, nil)
	if err := o.UpdateSession(context.Background(), &kernel.Session{ID: "x"}); err == nil {
		t.Error("expected error when store not configured")
	}
}

// ============ orchestrator.go: PreviewPlan / DeepPlan ============

func TestOrchestrator_PreviewPlan(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	plan, err := o.PreviewPlan(context.Background(), "do something complex")
	if err != nil {
		t.Fatalf("PreviewPlan failed: %v", err)
	}
	if plan == nil || plan.Goal == "" {
		t.Error("expected non-nil plan")
	}
}

func TestOrchestrator_DeepPlan_Fallback(t *testing.T) {
	// mockLLMProvider returns "mock" → Research fails → error returned
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	if _, err := o.DeepPlan(context.Background(), "complex task"); err == nil {
		t.Error("expected error when research fails")
	}
}

func TestOrchestrator_DeepPlanFinalize_InvalidChoice(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	result := &DeepPlanResult{
		Proposals: &Proposals{Options: []Proposal{{Name: "A"}, {Name: "B"}}},
	}
	if _, err := o.DeepPlanFinalize(context.Background(), "task", result, 5); err == nil {
		t.Error("expected error for invalid choice index")
	}
	if _, err := o.DeepPlanFinalize(context.Background(), "task", result, -1); err == nil {
		t.Error("expected error for negative choice index")
	}
}

// ============ planner.go: SetToolExecutor / readOnlyToolDefs ============

func TestReadOnlyToolDefs(t *testing.T) {
	te := &mockToolExecutor{defs: []kernel.ToolDefinition{
		{Type: "function", Function: kernel.FunctionDef{Name: "read_file"}},
		{Type: "function", Function: kernel.FunctionDef{Name: "write_file"}},
		{Type: "function", Function: kernel.FunctionDef{Name: "git_log"}},
		{Type: "function", Function: kernel.FunctionDef{Name: "execute_command"}},
	}}
	p := NewPlanner(&mockLLMProvider{})
	p.SetToolExecutor(te)
	got := p.readOnlyToolDefs()
	if len(got) != 2 {
		t.Fatalf("expected 2 read-only tools, got %d", len(got))
	}
	names := map[string]bool{}
	for _, d := range got {
		names[d.Function.Name] = true
	}
	if !names["read_file"] || !names["git_log"] {
		t.Errorf("expected read_file and git_log, got %v", names)
	}
}

func TestReadOnlyToolDefs_NilTools(t *testing.T) {
	p := NewPlanner(&mockLLMProvider{})
	if got := p.readOnlyToolDefs(); got != nil {
		t.Errorf("expected nil for no tool executor, got %v", got)
	}
}

// ============ planner.go: Research ============

func TestPlanner_Research_Success(t *testing.T) {
	reportJSON := `{"findings":"clean arch","modules":"kernel","risks":"low","complexity":"low"}`
	llm := &scriptedLLM{responses: []*kernel.LLMResponse{
		{Content: reportJSON}, // round 0: no tool calls → parsed as report
	}}
	te := &mockToolExecutor{defs: []kernel.ToolDefinition{
		{Type: "function", Function: kernel.FunctionDef{Name: "read_file"}},
	}}
	p := NewPlanner(llm)
	p.SetToolExecutor(te)
	report, err := p.Research(context.Background(), "analyze kernel")
	if err != nil {
		t.Fatalf("Research failed: %v", err)
	}
	if report.Findings != "clean arch" {
		t.Errorf("expected findings, got %q", report.Findings)
	}
}

func TestPlanner_Research_LLMFails(t *testing.T) {
	p := NewPlanner(&errLLM{})
	if _, err := p.Research(context.Background(), "task"); err == nil {
		t.Error("expected error when LLM fails")
	}
}

// ============ planner.go: Propose ============

func TestPlanner_Propose_Success(t *testing.T) {
	proposalsJSON := `{"goal":"g","options":[{"name":"A","description":"d","reasoning":"r","pros":"p","cons":"c","risk":"low","effort":"1d"}]}`
	llm := &scriptedLLM{responses: []*kernel.LLMResponse{{Content: proposalsJSON}}}
	p := NewPlanner(llm)
	research := &ResearchReport{Findings: "f", Modules: "m", Risks: "r", Complexity: "low"}
	proposals, err := p.Propose(context.Background(), "task", research)
	if err != nil {
		t.Fatalf("Propose failed: %v", err)
	}
	if len(proposals.Options) != 1 || proposals.Options[0].Name != "A" {
		t.Errorf("unexpected proposals: %+v", proposals)
	}
}

func TestPlanner_Propose_LLMFails(t *testing.T) {
	p := NewPlanner(&errLLM{})
	if _, err := p.Propose(context.Background(), "task", &ResearchReport{Findings: "f"}); err == nil {
		t.Error("expected error when LLM fails")
	}
}

// ============ planner.go: PlanWithApproach ============

func TestPlanner_PlanWithApproach_Success(t *testing.T) {
	planJSON := `{"goal":"g","subtasks":[{"id":1,"title":"t","description":"d"}]}`
	llm := &scriptedLLM{responses: []*kernel.LLMResponse{{Content: planJSON}}}
	p := NewPlanner(llm)
	plan, err := p.PlanWithApproach(context.Background(), "task", &ResearchReport{Findings: "f"}, &Proposal{Name: "A"})
	if err != nil {
		t.Fatalf("PlanWithApproach failed: %v", err)
	}
	if len(plan.Subtasks) != 1 {
		t.Errorf("expected 1 subtask, got %d", len(plan.Subtasks))
	}
}

func TestPlanner_PlanWithApproach_LLMFails(t *testing.T) {
	p := NewPlanner(&errLLM{})
	if _, err := p.PlanWithApproach(context.Background(), "task", &ResearchReport{Findings: "f"}, &Proposal{Name: "A"}); err == nil {
		t.Error("expected error when LLM fails")
	}
}

// ============ planner.go: ExecutePlan (Orchestrator method) ============

func TestOrchestrator_ExecutePlan(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	resp, err := o.ExecutePlan(context.Background(), "u1", "p1", "do it", kernel.QueryOptions{})
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}
	if resp == nil || resp.Content == "" {
		t.Error("expected non-empty response")
	}
}

// ============ team.go: Delegate / DelegateAll ============

func TestTeam_Delegate_Success(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	o.SetTeam(team)
	resp, err := team.Delegate(context.Background(), "u1", "p1", "write a feature", kernel.QueryOptions{})
	if err != nil {
		t.Fatalf("Delegate failed: %v", err)
	}
	if resp == nil || resp.Content == "" {
		t.Error("expected non-empty response")
	}
}

func TestTeam_Delegate_RoleFallback(t *testing.T) {
	// ProcessQuery returns "mock response" (not a role name) → Delegate falls back
	// to the first available role. mockLLMProvider keeps RunSubAgent working.
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	o.SetTeam(team)
	resp, err := team.Delegate(context.Background(), "u1", "p1", "do task", kernel.QueryOptions{})
	if err != nil {
		t.Fatalf("Delegate fallback failed: %v", err)
	}
	if resp == nil || resp.Content == "" {
		t.Error("expected response from fallback")
	}
}

func TestTeam_Delegate_ProcessQueryError(t *testing.T) {
	// errLLM makes ProcessQuery succeed (default plan → mockKernel), but the
	// role sub-agent's ChatStream fails → error propagates. Exercises error path.
	o := NewOrchestrator(&mockKernel{}, &errLLM{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	o.SetTeam(team)
	_, err := team.Delegate(context.Background(), "u1", "p1", "do task", kernel.QueryOptions{})
	if err == nil {
		t.Error("expected error when sub-agent LLM fails")
	}
}

func TestTeam_DelegateAll(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	o.SetTeam(team)
	resp, err := team.DelegateAll(context.Background(), "u1", "p1", "implement", kernel.QueryOptions{})
	if err != nil {
		t.Fatalf("DelegateAll failed: %v", err)
	}
	if resp == nil || resp.Content == "" {
		t.Error("expected non-empty response")
	}
}

// ============ team.go: buildSingleGraph / executeGraph ============

func TestTeam_BuildSingleGraph(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	g := team.buildSingleGraph(&TeamRole{Name: "custom", Prompt: "work", Tools: []string{"read_file"}})
	if g == nil || len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.Nodes))
	}
	for _, n := range g.Nodes {
		if n.Name != "custom" {
			t.Errorf("expected custom node, got %q", n.Name)
		}
	}
}

func TestTeam_ExecuteGraph_Empty(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	o.SetTeam(team)
	resp, err := team.executeGraph(context.Background(), "query", kernel.QueryOptions{}, nil)
	if err != nil {
		t.Fatalf("executeGraph with nil graph failed: %v", err)
	}
	if resp == nil {
		t.Error("expected fallback response for empty graph")
	}
}

func TestTeam_ExecuteGraph_SingleNode(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	o.SetTeam(team)
	role := team.GetRole("coder")
	if role == nil {
		t.Fatal("coder role missing")
	}
	g := team.buildSingleGraph(role)
	resp, err := team.executeGraph(context.Background(), "query", kernel.QueryOptions{}, g)
	if err != nil {
		t.Fatalf("executeGraph failed: %v", err)
	}
	if resp == nil || resp.Content == "" {
		t.Error("expected content from graph execution")
	}
}

// ============ team.go: roleNames / FirstRole / RoleNames ============

func TestTeam_RoleNames(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	if got := team.RoleNames(); got == "" {
		t.Error("expected non-empty role names")
	}
	var nilTeam *Team
	if got := nilTeam.RoleNames(); got != "coder" {
		t.Errorf("nil team should return coder, got %q", got)
	}
}

func TestTeam_FirstRole(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	if team.FirstRole() == nil {
		t.Error("expected a first role")
	}
	var nilTeam *Team
	if nilTeam.FirstRole() != nil {
		t.Error("nil team should return nil")
	}
}

func TestTeam_GetRoleCoverage(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	if team.GetRole("coder") == nil {
		t.Error("expected coder role")
	}
	if team.GetRole("nonexistent") != nil {
		t.Error("expected nil for unknown role")
	}
}

// ============ team_roles.go: GenerateRoles / AddRole ============

func TestTeam_GenerateRoles_Success(t *testing.T) {
	rolesJSON := `{"auditor":{"name":"Security Auditor","description":"audits","prompt":"be careful","tools":["read_file"]}}`
	llm := &scriptedLLM{responses: []*kernel.LLMResponse{{Content: rolesJSON}}}
	o := NewOrchestrator(&mockKernel{}, llm, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	team.GenerateRoles(context.Background(), "audit security")
	if team.GetRole("auditor") == nil {
		t.Fatal("expected auditor role to be generated")
	}
	if team.GetRole("auditor").Name != "Security Auditor" {
		t.Errorf("unexpected role name: %q", team.GetRole("auditor").Name)
	}
}

func TestTeam_GenerateRoles_LLMFailure(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &errLLM{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	team.GenerateRoles(context.Background(), "task")
	// Falls back to defaults — analyst must exist
	if team.GetRole("analyst") == nil {
		t.Error("expected default analyst role after LLM failure")
	}
}

func TestTeam_GenerateRoles_ParseFailure(t *testing.T) {
	llm := &scriptedLLM{responses: []*kernel.LLMResponse{{Content: "not json"}}}
	o := NewOrchestrator(&mockKernel{}, llm, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	team.GenerateRoles(context.Background(), "task")
	if team.GetRole("analyst") == nil {
		t.Error("expected default roles after parse failure")
	}
}

func TestTeam_GenerateRoles_NilTeam(t *testing.T) {
	var nilTeam *Team
	nilTeam.GenerateRoles(context.Background(), "task") // must not panic
}

func TestTeam_AddRole(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	team := NewTeam(o)
	team.AddRole("custom", "Custom", "desc", "prompt", []string{"read_file"})
	if team.GetRole("custom") == nil {
		t.Error("expected custom role after AddRole")
	}
}

func TestTruncForLLM(t *testing.T) {
	if got := truncForLLM("short", 100); got != "short" {
		t.Errorf("short string should pass through, got %q", got)
	}
	long := strings.Repeat("x", 500)
	if got := truncForLLM(long, 100); len(got) != 103 {
		t.Errorf("expected 103 chars (100 + ...), got %d", len(got))
	}
}

// ============ subagent.go: firstTeamRole ============

func TestFirstTeamRole(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	if o.firstTeamRole() != nil {
		t.Error("expected nil when no team")
	}
	o.SetTeam(NewTeam(o))
	if o.firstTeamRole() == nil {
		t.Error("expected role when team set")
	}
}

// ============ tot.go: ExploreAlternatives / evaluateBranches ============

func TestExploreAlternatives_TooFew(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	if _, err := o.ExploreAlternatives(context.Background(), "u1", "p1", "task", []Approach{{Name: "A"}}); err == nil {
		t.Error("expected error for fewer than 2 approaches")
	}
}

func TestExploreAlternatives_Success(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	o.SetTeam(NewTeam(o))
	result, err := o.ExploreAlternatives(context.Background(), "u1", "p1", "task", []Approach{
		{Name: "A", Prompt: "approach a"},
		{Name: "B", Prompt: "approach b"},
	})
	if err != nil {
		t.Fatalf("ExploreAlternatives failed: %v", err)
	}
	if len(result.Branches) != 2 {
		t.Errorf("expected 2 branches, got %d", len(result.Branches))
	}
	if result.Best < 0 || result.Best > 1 {
		t.Errorf("expected best in [0,1], got %d", result.Best)
	}
}

func TestEvaluateBranches_Empty(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	best, rationale := o.evaluateBranches(context.Background(), "task", nil)
	if best != 0 || rationale != "" {
		t.Errorf("expected (0, empty), got (%d, %q)", best, rationale)
	}
}

func TestEvaluateBranches_LLMFailure(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &errLLM{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	branches := []BranchResult{{Approach: Approach{Name: "A"}, Findings: "f1"}}
	best, rationale := o.evaluateBranches(context.Background(), "task", branches)
	if best != 0 {
		t.Errorf("expected best 0 on failure, got %d", best)
	}
	if rationale == "" {
		t.Error("expected failure rationale")
	}
}

func TestEvaluateBranches_Parsed(t *testing.T) {
	llm := &scriptedLLM{responses: []*kernel.LLMResponse{{Content: "BEST=2 REASON=branch two wins"}}}
	o := NewOrchestrator(&mockKernel{}, llm, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	branches := []BranchResult{
		{Approach: Approach{Name: "A"}, Findings: "f1"},
		{Approach: Approach{Name: "B"}, Findings: "f2"},
	}
	best, rationale := o.evaluateBranches(context.Background(), "task", branches)
	if best != 1 {
		t.Errorf("expected best 1 (0-indexed), got %d", best)
	}
	if rationale != "branch two wins" {
		t.Errorf("expected rationale, got %q", rationale)
	}
}

func TestEvaluateBranches_OutOfRange(t *testing.T) {
	llm := &scriptedLLM{responses: []*kernel.LLMResponse{{Content: "BEST=9 REASON=oops"}}}
	o := NewOrchestrator(&mockKernel{}, llm, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	branches := []BranchResult{{Approach: Approach{Name: "A"}, Findings: "f1"}}
	best, _ := o.evaluateBranches(context.Background(), "task", branches)
	if best != 0 {
		t.Errorf("expected best clamped to 0, got %d", best)
	}
}

// ============ execute.go: ExecuteWithPlan / lintRepairLoop ============

func TestExecuteWithPlan_SingleSubtask(t *testing.T) {
	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	o.SetTeam(NewTeam(o))
	plan := &Plan{Goal: "g", Subtasks: []SubTask{{ID: 1, Title: "t", Description: "d"}}}
	resp, err := o.ExecuteWithPlan(context.Background(), "u1", "p1", "content", plan, kernel.QueryOptions{})
	if err != nil {
		t.Fatalf("ExecuteWithPlan failed: %v", err)
	}
	if resp == nil || resp.Content == "" {
		t.Error("expected non-empty response")
	}
}

func TestLintRepairLoop_NoProject(t *testing.T) {
	// chdir to a temp dir without project files → runLint returns empty → loop returns fast
	orig, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(orig)

	o := NewOrchestrator(&mockKernel{}, &mockLLMProvider{}, &mockToolExecutor{}, &mockMemory{}, kernel.NewSessionStoreAdapter())
	o.lintRepairLoop(context.Background(), "u1", "p1", 3) // must not hang or panic
}
