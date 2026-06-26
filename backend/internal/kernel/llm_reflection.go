package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// LLMReflection 基于 LLM 调用的反思实现。完全无状态——每次 Reflect 独立执行。
type LLMReflection struct {
	llm LLMProvider
}

// NewLLMReflection 创建基于 LLM 的反思器。
func NewLLMReflection(llm LLMProvider) *LLMReflection {
	return &LLMReflection{llm: llm}
}

// reflectionTool 定义用于获取结构化反思结果的 tool calling schema
var reflectionTool = ToolDefinition{
	Type: "function",
	Function: FunctionDef{
		Name:        "submit_evaluation",
		Description: "Submit your evaluation of the AI assistant's execution quality",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"quality": map[string]interface{}{
					"type":        "integer",
					"description": "Quality score 1-10. 10 = perfect, 1 = completely failed",
				},
				"issues": map[string]interface{}{
					"type":        "array",
					"description": "Specific issues found in the execution",
					"items":       map[string]interface{}{"type": "string"},
				},
				"suggestions": map[string]interface{}{
					"type":        "array",
					"description": "Behavioral rules for the next round: 'When X, always Y' format",
					"items":       map[string]interface{}{"type": "string"},
				},
				"learned": map[string]interface{}{
					"type":        "string",
					"description": "Key lesson learned that could be applied to future interactions",
				},
			},
			"required": []string{"quality", "issues", "suggestions", "learned"},
		},
	},
}

// Reflect 对执行过程进行 LLM 驱动的反思。完全无状态——每次独立。
func (r *LLMReflection) Reflect(ctx context.Context, sessionID string, execution ExecutionRecord) (*ReflectionResult, error) {
	var trace strings.Builder
	if len(execution.Messages) > 0 {
		trace.WriteString("## Step-by-Step Execution Trace\n\n")
		round := 0
		for i, msg := range execution.Messages {
			switch msg.Role {
			case "assistant":
				round++
				trace.WriteString(fmt.Sprintf("### Round %d — LLM Thought\n%s\n", round, truncStr(msg.Content, 200)))
				if len(msg.ToolCalls) > 0 {
					for _, tc := range msg.ToolCalls {
						trace.WriteString(fmt.Sprintf("  → Called: %s\n", tc.Function.Name))
					}
				}
			case "tool":
				trace.WriteString(fmt.Sprintf("  ← Result (%d): %s\n", i, truncStr(msg.Content, 150)))
			}
		}
		trace.WriteString("\n")
	}

	prompt := fmt.Sprintf(`Evaluate this AI assistant's execution STEP BY STEP. For each step, identify what went well and what could be improved.

## Query
%s

## Final Response
%s
%s
## Execution Summary
- Success: %v
- Error: %s
- Tool Calls: %d
- Duration: %dms
## Your Task
1. Rate overall quality (1-10)
2. Rate each step: was the tool choice correct? Was the right file read? Was the edit precise?
3. Identify the BEST decision in this execution (what to reinforce)
4. Identify the WEAKEST decision and output a SPECIFIC BEHAVIORAL RULE for the next round.
   Format: 'When [situation], always [action].' Example: 'When editing a file after running a
   command, always re-read the file first.' Not: 'better file handling needed.'
6. Infer user verdict from conversation: [good]/[bad]/[neutral]`,
		execution.Query, execution.Response, trace.String(),
		execution.Success, execution.Error,
		len(execution.ToolCalls), execution.Duration)

	messages := []Message{
		{
			Role:    "system",
			Content: "You are a process supervisor evaluating an AI agent's step-by-step execution. Identify specific steps that were strong or weak, not just overall quality.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	resp, err := r.llm.Chat(ctx, messages, []ToolDefinition{reflectionTool}, map[string]interface{}{
		"route":       "execution",
		"no_thinking": true,
		"temperature": 0.2,
		"max_tokens":  500,
	})
	if err != nil {
		slog.Warn("LLM reflection failed, skipping", "error", err)
		return nil, err
	}

	if len(resp.ToolCalls) == 0 {
		slog.Warn("LLM reflection returned no tool calls")
		return nil, fmt.Errorf("no tool calls in reflection response")
	}

	result, parseErr := parseReflectionResult(resp.ToolCalls[0].Function.Arguments)
	if parseErr != nil {
		slog.Warn("Failed to parse LLM reflection result", "error", parseErr)
		return nil, parseErr
	}

	return result, nil
}

func parseReflectionResult(jsonStr string) (*ReflectionResult, error) {
	var result struct {
		Quality     int      `json:"quality"`
		Issues      []string `json:"issues"`
		Suggestions []string `json:"suggestions"`
		Learned     string   `json:"learned"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("unmarshal reflection result: %w", err)
	}
	if result.Quality < 1 {
		result.Quality = 1
	}
	if result.Quality > 10 {
		result.Quality = 10
	}
	if result.Issues == nil {
		result.Issues = []string{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []string{}
	}
	return &ReflectionResult{
		Quality:     result.Quality,
		Issues:      result.Issues,
		Suggestions: result.Suggestions,
		Learned:     result.Learned,
	}, nil
}
