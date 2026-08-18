package tools

import (
	"context"
	"strings"
	"testing"
)

func TestHandleTodoWrite(t *testing.T) {
	ctx := context.Background()
	result, err := handleTodoWrite(ctx, `{"todos":"[ ] Plan the feature\n[>] Implement core logic\n[x] Write tests"}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if !strings.Contains(result.Content.(string), "[x]") {
		t.Error("expected completed task marker")
	}
	if !strings.Contains(result.Content.(string), "[>]") {
		t.Error("expected in-progress task marker")
	}
	if !strings.Contains(result.Content.(string), "[ ]") {
		t.Error("expected pending task marker")
	}
}

func TestHandleTodoWrite_Empty(t *testing.T) {
	ctx := context.Background()
	result, _ := handleTodoWrite(ctx, `{"todos":""}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
}

func TestHandleTodoRead(t *testing.T) {
	ctx := context.Background()
	// Write first
	handleTodoWrite(ctx, `{"todos":"[x] Done task"}`)

	// Read back
	result, err := handleTodoRead(ctx, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content.(string), "[x]") {
		t.Error("expected to read back todo list")
	}
}

func TestHandleTodoRead_Empty(t *testing.T) {
	ctx := context.Background()
	// Clear todos for this session
	todoStore.Delete("default")

	result, _ := handleTodoRead(ctx, "{}")
	if !strings.Contains(result.Content.(string), "empty") {
		t.Error("expected empty todo message")
	}
}
