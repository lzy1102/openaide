package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"openaide/backend/internal/kernel"
)

// MemoryManager interface for MemGPT-style memory operations.
type MemoryManager interface {
	ArchiveConversation(ctx context.Context, sessionID, summary string, messages []kernel.Message, importance float64) error
	RetrieveArchive(ctx context.Context, query string, limit int) ([]kernel.Message, float64, error)
	StoreCoreFact(ctx context.Context, content string, importance float64)
	GetCoreFacts(ctx context.Context, query string, limit int) []string
}

var memoryManager MemoryManager

// SetMemoryManager injects the memory manager for manage_memory tool.
func SetMemoryManager(m MemoryManager) { memoryManager = m }

func memoryToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{{
		Type: "function",
		Function: kernel.FunctionDef{
			Name:        "manage_memory",
			Description: "Manage your memory: archive completed conversations, retrieve past knowledge, or store important facts. Use proactively to prevent context overflow and retain critical information.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"archive", "retrieve", "remember", "recall"},
						"description": "archive: save current conversation summary to long-term memory. retrieve: search past conversations. remember: store an important fact permanently. recall: get your core facts.",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "For archive: a 1-2 sentence summary of what was accomplished. For retrieve: search query for past conversations. For remember: the fact to store permanently.",
					},
					"importance": map[string]interface{}{
						"type":        "number",
						"description": "How important is this information? 0.0-1.0. Higher = more likely to be retrieved later. Default 0.5.",
					},
				},
				"required": []string{"action", "content"},
			},
		},
	}}
}

func handleManageMemory(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	if memoryManager == nil {
		return &kernel.ToolResult{Content: "Memory manager not available"}, nil
	}

	var args struct {
		Action     string  `json:"action"`
		Content    string  `json:"content"`
		Importance float64 `json:"importance"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Importance <= 0 {
		args.Importance = 0.5
	}

	switch args.Action {
	case "archive":
		// Archive current conversation summary
		memoryManager.ArchiveConversation(ctx, "global", args.Content, nil, args.Importance)
		return &kernel.ToolResult{Content: fmt.Sprintf("Archived: %s", args.Content)}, nil

	case "retrieve":
		msgs, score, _ := memoryManager.RetrieveArchive(ctx, args.Content, 5)
		if len(msgs) == 0 {
			return &kernel.ToolResult{Content: "No relevant archived conversations found."}, nil
		}
		result := fmt.Sprintf("Found %d archived conversations (best match: %.2f):\n", len(msgs), score)
		for i, m := range msgs {
			if i >= 3 {
				break
			}
			preview := m.Content
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			result += fmt.Sprintf("- %s\n", preview)
		}
		return &kernel.ToolResult{Content: result}, nil

	case "remember":
		memoryManager.StoreCoreFact(ctx, args.Content, args.Importance)
		return &kernel.ToolResult{Content: fmt.Sprintf("Remembered: %s", args.Content)}, nil

	case "recall":
		facts := memoryManager.GetCoreFacts(ctx, args.Content, 5)
		if len(facts) == 0 {
			return &kernel.ToolResult{Content: "No relevant core facts found."}, nil
		}
		result := "Core facts:\n"
		for _, f := range facts {
			result += fmt.Sprintf("- %s\n", f)
		}
		return &kernel.ToolResult{Content: result}, nil

	default:
		return &kernel.ToolResult{Error: fmt.Sprintf("Unknown action: %s. Use: archive, retrieve, remember, recall", args.Action)}, nil
	}
}
