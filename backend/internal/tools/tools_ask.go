package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"openaide/backend/internal/kernel"
)

// ── User Clarification Tool ─────────────────────────────────
// Uses a buffered channel — CSP, zero locks.

var pendingQuestions = make(chan string, 32)

func askToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "ask_user",
				Description: "Ask the user a clarifying question when you need more information or the instructions are ambiguous. Only use when truly necessary — prefer making reasonable assumptions.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"question": map[string]interface{}{
							"type":        "string",
							"description": "The question to ask the user",
						},
						"options": map[string]interface{}{
							"type":        "array",
							"items":       map[string]string{"type": "string"},
							"description": "Optional preset options for the user to choose from",
						},
					},
					"required": []string{"question"},
				},
			},
		},
	}
}

func handleAskUser(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Question string   `json:"question"`
		Options  []string `json:"options,omitempty"`
	}
	json.Unmarshal([]byte(arguments), &args)

	select {
	case pendingQuestions <- args.Question:
	default:
	}

	msg := fmt.Sprintf("// ❓ Question for user: %s\n", args.Question)
	if len(args.Options) > 0 {
		msg += "// Options: "
		for i, o := range args.Options {
			msg += fmt.Sprintf("[%d] %s  ", i+1, o)
		}
		msg += "\n"
	}
	msg += "// The user will respond to this question. Wait for their answer before proceeding.\n"
	return &kernel.ToolResult{Content: msg}, nil
}

// GetPendingQuestions drains the channel and returns all pending questions.
func GetPendingQuestions() []string {
	var questions []string
	for {
		select {
		case q := <-pendingQuestions:
			questions = append(questions, q)
		default:
			return questions
		}
	}
}
