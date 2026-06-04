package tools

import (
	"context"
	"encoding/json"

	"openaide/backend/internal/kernel"
)

// FeedbackRequested is set to true when the LLM calls request_feedback.
// The REPL reads this to decide whether to show the feedback prompt.
var FeedbackRequested bool

func feedbackToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{{
		Type: "function",
		Function: kernel.FunctionDef{
			Name:        "request_feedback",
			Description: "Call this at the END of a response when you believe user feedback would be valuable for learning. Use when: the task was complex (multiple steps), you made code changes, you're uncertain about the result quality, or the user seemed particularly satisfied or dissatisfied. Do NOT call after simple queries, greetings, or pure information lookups.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "Brief reason why you're requesting feedback (e.g. 'multi-step bug fix', 'uncertain about approach', 'complex refactor')",
					},
				},
				"required": []string{"reason"},
			},
		},
	}}
}

func handleRequestFeedback(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Reason string `json:"reason"`
	}
	json.Unmarshal([]byte(arguments), &args)
	FeedbackRequested = true
	return &kernel.ToolResult{Content: "feedback requested: " + args.Reason}, nil
}
