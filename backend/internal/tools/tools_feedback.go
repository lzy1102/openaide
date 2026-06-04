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
			Description: "Call this ONLY at the END of your final response, AFTER you have verified your work (tests pass, code builds, files read back correctly). Use when: you completed a multi-step task with code changes AND verified the result. Do NOT call: during intermediate steps, before verification, after simple queries, greetings, or pure information lookups. The user needs to see your complete verified result before giving feedback.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "What was accomplished and verified before requesting feedback (e.g. 'fixed login bug, tests pass', 'refactored auth module, build succeeds')",
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
