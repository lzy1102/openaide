package kernel

import (
	"context"
	"strings"
	"testing"
)

func TestPlanner_NilReceiver(t *testing.T) {
	// nil planner should be safe — returns nil, no panic
	var p *Planner
	if plan := p.Plan(context.Background(), "test"); plan != nil {
		t.Errorf("nil planner should return nil, got %v", plan)
	}
}

func TestPlanner_NilLLM(t *testing.T) {
	p := &Planner{llm: nil}
	if plan := p.Plan(context.Background(), "test"); plan != nil {
		t.Errorf("planner with nil LLM should return nil, got %v", plan)
	}
}

func TestPlanner_ValidJSON(t *testing.T) {
	llm := &MockLLMProvider{
		responses: []LLMResponse{
			{Content: `{"sub_tasks":[{"goal":"read file","approach":"use read_file","expected_result":"file content returned"},{"goal":"edit file","approach":"use diff_edit","expected_result":"file modified"}]}`},
		},
	}
	p := NewPlanner(llm)
	plan := p.Plan(context.Background(), "refactor the auth module")
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}
	if len(plan.SubTasks) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(plan.SubTasks))
	}
	if plan.SubTasks[0].Goal != "read file" {
		t.Errorf("first subtask goal mismatch: %s", plan.SubTasks[0].Goal)
	}
}

func TestPlanner_MarkdownFencedJSON(t *testing.T) {
	llm := &MockLLMProvider{
		responses: []LLMResponse{
			{Content: "```json\n{\"sub_tasks\":[{\"goal\":\"step1\",\"approach\":\"a\",\"expected_result\":\"r\"}]}\n```"},
		},
	}
	p := NewPlanner(llm)
	plan := p.Plan(context.Background(), "complex task")
	if plan == nil {
		t.Fatal("expected non-nil plan from markdown-fenced JSON")
	}
	if len(plan.SubTasks) != 1 {
		t.Fatalf("expected 1 subtask, got %d", len(plan.SubTasks))
	}
}

func TestPlanner_EmptySubTasks(t *testing.T) {
	// LLM returns empty array → planner returns nil (simple task)
	llm := &MockLLMProvider{
		responses: []LLMResponse{
			{Content: `{"sub_tasks":[]}`},
		},
	}
	p := NewPlanner(llm)
	if plan := p.Plan(context.Background(), "hi"); plan != nil {
		t.Errorf("empty subtasks should return nil, got %v", plan)
	}
}

func TestPlanner_InvalidJSON(t *testing.T) {
	llm := &MockLLMProvider{
		responses: []LLMResponse{
			{Content: "not json at all"},
		},
	}
	p := NewPlanner(llm)
	if plan := p.Plan(context.Background(), "test"); plan != nil {
		t.Errorf("invalid JSON should return nil, got %v", plan)
	}
}

func TestPlanner_TruncatesToFive(t *testing.T) {
	// LLM returns 8 subtasks → planner truncates to 5
	llm := &MockLLMProvider{
		responses: []LLMResponse{
			{Content: `{"sub_tasks":[
				{"goal":"g1","approach":"a","expected_result":"r"},
				{"goal":"g2","approach":"a","expected_result":"r"},
				{"goal":"g3","approach":"a","expected_result":"r"},
				{"goal":"g4","approach":"a","expected_result":"r"},
				{"goal":"g5","approach":"a","expected_result":"r"},
				{"goal":"g6","approach":"a","expected_result":"r"},
				{"goal":"g7","approach":"a","expected_result":"r"},
				{"goal":"g8","approach":"a","expected_result":"r"}
			]}`},
		},
	}
	p := NewPlanner(llm)
	plan := p.Plan(context.Background(), "huge task")
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}
	if len(plan.SubTasks) != 5 {
		t.Fatalf("expected 5 subtasks (truncated), got %d", len(plan.SubTasks))
	}
}

func TestTaskPlan_ToSystemMessage(t *testing.T) {
	plan := &TaskPlan{
		SubTasks: []SubTask{
			{Goal: "read config", Approach: "use read_file", ExpectedResult: "config visible"},
			{Goal: "update port", Approach: "use diff_edit", ExpectedResult: "port changed"},
		},
	}
	msg := plan.ToSystemMessage()
	if msg.Role != "system" {
		t.Errorf("expected system role, got %s", msg.Role)
	}
	if msg.Content == "" {
		t.Error("expected non-empty content")
	}
	if !strings.Contains(msg.Content, "Step 1") || !strings.Contains(msg.Content, "Step 2") {
		t.Error("expected step numbering in content")
	}
	if !strings.Contains(msg.Content, "read config") {
		t.Error("expected first goal in content")
	}
}

func TestTaskPlan_ToSystemMessage_Empty(t *testing.T) {
	plan := &TaskPlan{SubTasks: nil}
	msg := plan.ToSystemMessage()
	if msg.Content != "" {
		t.Errorf("empty plan should produce empty message, got %s", msg.Content)
	}
}

func TestTaskPlan_ToSystemMessage_Nil(t *testing.T) {
	var plan *TaskPlan
	msg := plan.ToSystemMessage()
	if msg.Content != "" {
		t.Errorf("nil plan should produce empty message, got %s", msg.Content)
	}
}

func TestShouldPlan(t *testing.T) {
	tests := []struct {
		name     string
		analysis *QueryAnalysis
		want     bool
	}{
		{"nil analysis", nil, false},
		{"low complexity", &QueryAnalysis{Complexity: 5}, false},
		{"at threshold", &QueryAnalysis{Complexity: 15}, true},
		{"high complexity", &QueryAnalysis{Complexity: 30}, true},
		{"just below", &QueryAnalysis{Complexity: 14}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldPlan(tt.analysis); got != tt.want {
				t.Errorf("shouldPlan() = %v, want %v", got, tt.want)
			}
		})
	}
}
