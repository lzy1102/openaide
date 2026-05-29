package tools

import (
	"context"
	"strings"
	"testing"
)

func TestHandleAskUser(t *testing.T) {
	ctx := context.Background()
	result, err := handleAskUser(ctx, `{"question":"Which approach do you prefer?","options":["Option A","Option B"]}`)
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
	result, _ := handleAskUser(ctx, `{"question":"What is your name?"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if !strings.Contains(result.Content.(string), "What is your name?") {
		t.Error("expected question text")
	}
}

func TestGetPendingQuestions(t *testing.T) {
	// Clear any leftover questions from previous tests
	GetPendingQuestions()

	ctx := context.Background()
	handleAskUser(ctx, `{"question":"Q1?"}`)
	handleAskUser(ctx, `{"question":"Q2?"}`)

	questions := GetPendingQuestions()
	if len(questions) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(questions))
	}
	if questions[0] != "Q1?" || questions[1] != "Q2?" {
		t.Error("question order mismatch")
	}

	// Second call should return empty (questions are consumed)
	remaining := GetPendingQuestions()
	if len(remaining) != 0 {
		t.Error("questions should be consumed after first GetPendingQuestions")
	}
}

func TestHandleAskUser_CrossPlatform(t *testing.T) {
	// ask_user is pure Go (in-memory), works on all platforms
	ctx := context.Background()
	result, _ := handleAskUser(ctx, `{"question":"Test?"}`)
	if result.Error != "" {
		t.Error("ask_user should work cross-platform")
	}
}
