package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"openaide/backend/internal/kernel"
)

// ── Agent Todo Management (Claude Code style) ───────────────
// Enables the agent to track progress during complex multi-step tasks.
// The todo list is per-session, stored in memory (not persisted across restarts).

type todoItem struct {
	ID       int    `json:"id"`
	Content  string `json:"content"`
	Status   string `json:"status"` // pending, in_progress, completed
}

var (
	todoStore = make(map[string][]todoItem) // sessionID → todos
	todoMu    sync.RWMutex
)

func todoToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "todo_write",
				Description: "Create and manage a structured task list. Use to track progress during complex multi-step tasks. Format: one task per line. Mark completed tasks by including them with status 'completed'.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"todos": map[string]interface{}{
							"type":        "string",
							"description": "Task list, one per line. Prefix with [x] for completed, [ ] for pending, [>] for in-progress.",
						},
					},
					"required": []string{"todos"},
				},
			},
		},
	}
}

func handleTodoWrite(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Todos string `json:"todos"`
	}
	json.Unmarshal([]byte(arguments), &args)

	// Extract session ID from context (best effort)
	sessionID := "default"
	if v := ctx.Value("session_id"); v != nil {
		if s, ok := v.(string); ok { sessionID = s }
	}

	var items []todoItem
	for i, line := range strings.Split(args.Todos, "\n") {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		status := "pending"
		if strings.HasPrefix(line, "[x]") || strings.HasPrefix(line, "[X]") {
			status = "completed"
			line = strings.TrimPrefix(strings.TrimPrefix(line, "[x]"), "[X]")
			line = strings.TrimSpace(line)
		} else if strings.HasPrefix(line, "[>]") {
			status = "in_progress"
			line = strings.TrimPrefix(line, "[>]")
			line = strings.TrimSpace(line)
		} else if strings.HasPrefix(line, "[ ]") {
			line = strings.TrimPrefix(line, "[ ]")
			line = strings.TrimSpace(line)
		}
		items = append(items, todoItem{ID: i + 1, Content: line, Status: status})
	}

	todoMu.Lock()
	todoStore[sessionID] = items
	todoMu.Unlock()

	var out strings.Builder
	out.WriteString("// Todo list:\n")
	for _, item := range items {
		icon := " "
		switch item.Status {
		case "completed": icon = "x"
		case "in_progress": icon = ">"
		}
		out.WriteString(fmt.Sprintf("  [%s] %d. %s\n", icon, item.ID, item.Content))
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

func handleTodoRead(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	sessionID := "default"
	if v := ctx.Value("session_id"); v != nil {
		if s, ok := v.(string); ok { sessionID = s }
	}

	todoMu.RLock()
	items := todoStore[sessionID]
	todoMu.RUnlock()

	if len(items) == 0 {
		return &kernel.ToolResult{Content: "// Todo list is empty. Use todo_write to create tasks."}, nil
	}

	var out strings.Builder
	out.WriteString("// Todo list:\n")
	for _, item := range items {
		icon := " "
		switch item.Status {
		case "completed": icon = "x"
		case "in_progress": icon = ">"
		}
		out.WriteString(fmt.Sprintf("  [%s] %d. %s\n", icon, item.ID, item.Content))
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}
