package tools

import (
	"context"
	"strings"
	"testing"
)

func TestHandleAskUser(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	result, err := r.handleAskUser(ctx, `{"question":"Which approach do you prefer?","options":["Option A","Option B"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if !strings.Contains(result.Content.(string), "Which approach do you prefer?") {
		t.Error("expected question in output")
	}
	if !strings.Contains(result.Content.(string), "Option A") {
		t.Error("expected options in output")
	}
}

func TestHandleAskUser_NoOptions(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	result, _ := r.handleAskUser(ctx, `{"question":"What is your name?"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if !strings.Contains(result.Content.(string), "What is your name?") {
		t.Error("expected question text")
	}
}

func TestGetPendingQuestions(t *testing.T) {
	r := NewRegistry()
	r.GetPendingQuestions() // clear any leftovers

	ctx := context.Background()
	r.handleAskUser(ctx, `{"question":"Q1?"}`)
	r.handleAskUser(ctx, `{"question":"Q2?"}`)

	questions := r.GetPendingQuestions()
	if len(questions) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(questions))
	}
	if questions[0] != "Q1?" || questions[1] != "Q2?" {
		t.Error("question order mismatch")
	}

	remaining := r.GetPendingQuestions()
	if len(remaining) != 0 {
		t.Error("questions should be consumed after first GetPendingQuestions")
	}
}

func TestHandleAskUser_CrossPlatform(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	result, _ := r.handleAskUser(ctx, `{"question":"Test?"}`)
	if result.Error != "" {
		t.Error("ask_user should work cross-platform")
	}
}
