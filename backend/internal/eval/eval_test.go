package eval

import (
	"context"
	"strings"
	"testing"
	"time"

	"openaide/backend/internal/kernel"
)

// mockKernel returns controlled responses for testing the eval framework.
type mockKernel struct {
	response string
	tools    int
	tokens   int
	err      error
}

func (m *mockKernel) Process(ctx context.Context, query *kernel.Query) (*kernel.Response, error) {
	return &kernel.Response{
		Content:    m.response,
		ToolCalls:  m.tools,
		TokensUsed: m.tokens,
		Duration:   10 * time.Millisecond,
	}, m.err
}
func (m *mockKernel) ProcessStream(ctx context.Context, query *kernel.Query) (<-chan kernel.StreamChunk, error) {
	return nil, nil
}
func (m *mockKernel) GetState() kernel.KernelState            { return 0 }
func (m *mockKernel) Subscribe(handler kernel.EventHandler)   {}
func (m *mockKernel) Unsubscribe(handler kernel.EventHandler) {}
func (m *mockKernel) GetSlashCommands() map[string]string      { return nil }

// mockJudge returns controlled pass/fail for testing.
type mockJudge struct {
	pass   bool
	reason string
}

func (m *mockJudge) Judge(ctx context.Context, task Task, response string) (bool, string) {
	return m.pass, m.reason
}

func TestRunner_SinglePass(t *testing.T) {
	r := NewRunnerWithJudge(&mockKernel{
		response: "Binary search has O(log n) time complexity.",
		tools:    1,
		tokens:   100,
	}, &mockJudge{pass: true})

	task := Task{
		ID: "test", Name: "Test", Category: "coding", Difficulty: "easy",
		Query:        "What is binary search complexity?",
		EvalCriteria: "Response states O(log n) time complexity.",
	}

	result := r.runOne(context.Background(), task)
	if !result.Passed {
		t.Errorf("expected pass, got fail: %s", result.FailReason)
	}
	if result.ToolCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", result.ToolCalls)
	}
}

func TestRunner_SingleFail(t *testing.T) {
	r := NewRunnerWithJudge(&mockKernel{
		response: "I don't know the answer.",
		tools:    0,
		tokens:   50,
	}, &mockJudge{pass: false, reason: "response does not answer the question"})

	task := Task{
		ID: "test", Name: "Test", Category: "coding", Difficulty: "easy",
		Query:        "What is Go?",
		EvalCriteria: "Response describes Go as a programming language.",
	}

	result := r.runOne(context.Background(), task)
	if result.Passed {
		t.Error("expected fail, got pass")
	}
	if !strings.Contains(result.FailReason, "answer") {
		t.Errorf("expected reason in fail message, got: %s", result.FailReason)
	}
}

func TestRunner_RunTasks(t *testing.T) {
	k := &mockKernel{
		response: "Go uses goroutines for concurrency.",
		tools:    2,
		tokens:   150,
	}

	r := NewRunnerWithJudge(k, &mockJudge{pass: true})
	tasks := []Task{
		{ID: "t1", Name: "T1", Query: "q1", EvalCriteria: "ok"},
		{ID: "t2", Name: "T2", Query: "q2", EvalCriteria: "ok"},
		{ID: "t3", Name: "T3", Query: "q3", EvalCriteria: "ok"},
	}

	run := r.RunTasks(context.Background(), tasks)
	if run.Total != 3 {
		t.Errorf("expected 3 tasks, got %d", run.Total)
	}
	if run.Passed != 3 {
		t.Errorf("expected 3 passed, got %d", run.Passed)
	}
	if run.AvgTools != 2.0 {
		t.Errorf("expected avg 2.0 tools, got %.1f", run.AvgTools)
	}
}

func TestRunner_MixedResults(t *testing.T) {
	k := &mockKernel{
		response: "ok",
		tools:    1,
		tokens:   100,
	}

	// Judge that fails on even-numbered task indices via a custom runner
	mj := &mockJudge{pass: true}
	r := NewRunnerWithJudge(k, mj)
	tasks := []Task{
		{ID: "t1", Name: "T1", Query: "q1", EvalCriteria: "ok"},
		{ID: "t2", Name: "T2", Query: "q2", EvalCriteria: "ok"},
		{ID: "t3", Name: "T3", Query: "q3", EvalCriteria: "ok"},
	}

	// Test that scorecard works with mixed results
	mj.pass = false
	mj.reason = "task 1 failed"

	run := r.RunTasks(context.Background(), tasks)
	if run.Passed == 3 {
		t.Error("expected some failures")
	}
}

func TestCompare(t *testing.T) {
	before := &Run{
		ID: "before", Total: 3, Passed: 2,
		AvgTime: 100 * time.Millisecond, AvgTools: 3.0, AvgTokens: 200,
		Results: []Result{
			{Task: Task{ID: "t1", Name: "T1", Difficulty: "easy"}, Passed: true, Duration: 80 * time.Millisecond, ToolCalls: 2},
			{Task: Task{ID: "t2", Name: "T2", Difficulty: "medium"}, Passed: false, Duration: 120 * time.Millisecond, ToolCalls: 5},
			{Task: Task{ID: "t3", Name: "T3", Difficulty: "hard"}, Passed: true, Duration: 100 * time.Millisecond, ToolCalls: 2},
		},
	}
	after := &Run{
		ID: "after", Total: 3, Passed: 3,
		AvgTime: 80 * time.Millisecond, AvgTools: 2.0, AvgTokens: 150,
		Results: []Result{
			{Task: Task{ID: "t1", Name: "T1", Difficulty: "easy"}, Passed: true, Duration: 60 * time.Millisecond, ToolCalls: 1},
			{Task: Task{ID: "t2", Name: "T2", Difficulty: "medium"}, Passed: true, Duration: 90 * time.Millisecond, ToolCalls: 3},
			{Task: Task{ID: "t3", Name: "T3", Difficulty: "hard"}, Passed: true, Duration: 90 * time.Millisecond, ToolCalls: 2},
		},
	}

	report := Compare(before, after)
	if !strings.Contains(report, "FIXED") {
		t.Error("compare should detect FIXED tasks")
	}
	if !strings.Contains(report, "Pass Rate") {
		t.Error("compare should show pass rate")
	}
}

func TestScorecard(t *testing.T) {
	run := &Run{
		ID: "test", Total: 3, Passed: 2, AvgTime: 50 * time.Millisecond, AvgTools: 1.5, AvgTokens: 100,
		Results: []Result{
			{Task: Task{ID: "t1", Name: "T1", Difficulty: "easy"}, Passed: true},
			{Task: Task{ID: "t2", Name: "T2", Difficulty: "medium"}, Passed: false, FailReason: "response does not answer the question"},
			{Task: Task{ID: "t3", Name: "T3", Difficulty: "hard"}, Passed: true},
		},
	}
	card := run.Scorecard()
	if !strings.Contains(card, "2/3") || !strings.Contains(card, "answer") {
		t.Errorf("scorecard missing expected content: %s", card)
	}
}

func TestBuiltinTasks(t *testing.T) {
	tasks := BuiltinTasks()
	if len(tasks) < 5 {
		t.Errorf("expected at least 5 builtin tasks, got %d", len(tasks))
	}
	categories := map[string]int{}
	for _, task := range tasks {
		if task.Query == "" || task.ID == "" || task.Name == "" {
			t.Errorf("task %s has empty fields", task.ID)
		}
		if task.EvalCriteria == "" {
			t.Errorf("task %s missing EvalCriteria", task.ID)
		}
		categories[task.Category]++
	}
	if len(categories) < 3 {
		t.Error("expected at least 3 categories (coding, review, think, general)")
	}
}

func TestFullCapabilityTasks(t *testing.T) {
	tasks := FullCapabilityTasks()
	if len(tasks) < 5 {
		t.Errorf("expected at least 5 full capability tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.Query == "" || task.ID == "" || task.Name == "" {
			t.Errorf("task %s has empty fields", task.ID)
		}
		if task.EvalCriteria == "" {
			t.Errorf("task %s missing EvalCriteria", task.ID)
		}
	}
}

func TestQuickTasks(t *testing.T) {
	quick := QuickTasks()
	for _, task := range quick {
		if task.Difficulty != "easy" {
			t.Errorf("QuickTasks should only contain easy tasks, got %s: %s", task.ID, task.Difficulty)
		}
	}
	if len(quick) == 0 {
		t.Error("QuickTasks should not be empty")
	}
}

func TestMustNotContain(t *testing.T) {
	r := NewRunnerWithJudge(&mockKernel{
		response: "I don't know the answer, sorry.",
		tools:    0, tokens: 30,
	}, &mockJudge{pass: true})

	task := Task{
		ID: "test", Name: "Test", Query: "q",
		EvalCriteria:  "Any valid response.",
		MustNotContain: []string{"I don't know"},
	}

	result := r.runOne(context.Background(), task)
	if result.Passed {
		t.Error("expected fail due to must_not_contain")
	}
	if !strings.Contains(result.FailReason, "disqualifying") {
		t.Errorf("expected 'disqualifying' in fail reason, got: %s", result.FailReason)
	}
}

func TestMinToolCalls(t *testing.T) {
	r := NewRunnerWithJudge(&mockKernel{
		response: "ok", tools: 1, tokens: 40,
	}, &mockJudge{pass: true})

	task := Task{
		ID: "test", Name: "Test", Query: "q",
		EvalCriteria: "ok",
		MinToolCalls: 3,
	}

	result := r.runOne(context.Background(), task)
	if result.Passed {
		t.Error("expected fail due to insufficient tool calls")
	}
}

func TestLLMJudge_Truncate(t *testing.T) {
	// Tests truncation helper
	s := truncate("hello world", 5)
	if s != "hello...[truncated]" {
		t.Errorf("expected truncation, got %q", s)
	}

	// Short string should not truncate
	s2 := truncate("hi", 100)
	if s2 != "hi" {
		t.Errorf("expected no truncation, got %q", s2)
	}
}

func TestExtractJSON(t *testing.T) {
	s := extractJSON(`some prefix {"pass": true, "reason": "good"} suffix`)
	if !strings.Contains(s, `"pass"`) {
		t.Errorf("expected JSON in extracted string, got: %s", s)
	}

	s2 := extractJSON("no json here")
	if s2 != "no json here" {
		t.Errorf("expected unchanged for no JSON, got: %s", s2)
	}
}
