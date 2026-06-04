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
func (m *mockKernel) GetState() kernel.KernelState                { return 0 }
func (m *mockKernel) Subscribe(handler kernel.EventHandler)       {}
func (m *mockKernel) Unsubscribe(handler kernel.EventHandler)     {}
func (m *mockKernel) GetSlashCommands() map[string]string          { return nil }

func TestRunner_SinglePass(t *testing.T) {
	r := NewRunner(&mockKernel{
		response: "Binary search has O(log n) time complexity.",
		tools:    1,
		tokens:   100,
	})

	task := Task{
		ID: "test", Name: "Test", Category: "coding", Difficulty: "easy",
		Query: "What is binary search complexity?",
		MustContain: []string{"O(log n)"},
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
	r := NewRunner(&mockKernel{
		response: "I don't know the answer.",
		tools:    0,
		tokens:   50,
	})

	task := Task{
		ID: "test", Name: "Test", Category: "coding", Difficulty: "easy",
		Query:       "What is Go?",
		MustContain: []string{"programming language"},
	}

	result := r.runOne(context.Background(), task)
	if result.Passed {
		t.Error("expected fail, got pass")
	}
	if !strings.Contains(result.FailReason, "missing") {
		t.Errorf("expected 'missing' in fail reason, got: %s", result.FailReason)
	}
}

func TestRunner_RunTasks(t *testing.T) {
	k := &mockKernel{
		response: "Go uses goroutines for concurrency. Channels communicate between goroutines.",
		tools:    2,
		tokens:   150,
	}

	r := NewRunner(k)
	tasks := []Task{
		{ID: "t1", Name: "T1", Query: "q1", MustContain: []string{"goroutines"}},
		{ID: "t2", Name: "T2", Query: "q2", MustContain: []string{"goroutines"}},
		{ID: "t3", Name: "T3", Query: "q3", MustContain: []string{"MISSING_KEYWORD"}},
	}

	run := r.RunTasks(context.Background(), tasks)
	if run.Total != 3 {
		t.Errorf("expected 3 tasks, got %d", run.Total)
	}
	if run.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", run.Passed)
	}
	if run.AvgTools != 2.0 {
		t.Errorf("expected avg 2.0 tools, got %.1f", run.AvgTools)
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
	t.Logf("\n%s", report)
}

func TestScorecard(t *testing.T) {
	run := &Run{
		ID: "test", Total: 3, Passed: 2, AvgTime: 50 * time.Millisecond, AvgTools: 1.5, AvgTokens: 100,
		Results: []Result{
			{Task: Task{ID: "t1", Name: "T1", Difficulty: "easy"}, Passed: true},
			{Task: Task{ID: "t2", Name: "T2", Difficulty: "medium"}, Passed: false, FailReason: "missing keyword"},
			{Task: Task{ID: "t3", Name: "T3", Difficulty: "hard"}, Passed: true},
		},
	}
	card := run.Scorecard()
	if !strings.Contains(card, "2/3") || !strings.Contains(card, "missing keyword") {
		t.Errorf("scorecard missing expected content: %s", card)
	}
	t.Logf("\n%s", card)
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
		categories[task.Category]++
	}
	t.Logf("Categories: %v", categories)
	if len(categories) < 3 {
		t.Error("expected at least 3 categories (coding, review, teaching, research, general)")
	}
}

func TestQuickTasks(t *testing.T) {
	quick := QuickTasks()
	for _, task := range quick {
		if task.Difficulty != "easy" {
			t.Errorf("QuickTasks should only contain easy tasks, got %s: %s", task.ID, task.Difficulty)
		}
	}
	t.Logf("Quick tasks: %d", len(quick))
}

func TestMustNotContain(t *testing.T) {
	r := NewRunner(&mockKernel{
		response: "I don't know the answer, sorry.",
		tools:    0, tokens: 30,
	})

	task := Task{
		ID: "test", Name: "Test", Query: "q",
		MustNotContain: []string{"I don't know"},
	}

	result := r.runOne(context.Background(), task)
	if result.Passed {
		t.Error("expected fail due to must_not_contain")
	}
	if !strings.Contains(result.FailReason, "forbidden") {
		t.Errorf("expected 'forbidden' in fail reason, got: %s", result.FailReason)
	}
}

func TestMinToolCalls(t *testing.T) {
	r := NewRunner(&mockKernel{
		response: "ok", tools: 1, tokens: 40,
	})

	task := Task{
		ID: "test", Name: "Test", Query: "q",
		MinToolCalls: 3,
	}

	result := r.runOne(context.Background(), task)
	if result.Passed {
		t.Error("expected fail due to insufficient tool calls")
	}
}
