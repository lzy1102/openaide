package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// LLMReflection 基于 LLM 调用的反思实现。
// 无降级：如果 LLM 不可用，agent 本身已无法工作，不应假装能打分。
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
					"description": "Actionable suggestions for improvement",
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

// Reflect 对执行过程进行 LLM 驱动的反思
// 先尝试 LLM 分析（结构化输出），失败则回退到规则评估
func (r *LLMReflection) Reflect(ctx context.Context, sessionID string, execution ExecutionRecord) (*ReflectionResult, error) {
	// Build step-by-step execution trace for process supervision
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
4. Identify the WEAKEST decision (what to fix next time)
5. **Key Lesson for Next Time** — a 1-3 sentence directive. Be concrete.
6. **Infer user verdict from the conversation flow**: based on the FULL conversation, did the user seem satisfied? Look for: user moves to new topics (positive), user reports errors or asks for corrections (negative), user continues refining the same task (neutral). Output in 'learned' field as prefix: [good], [bad], or [neutral].`,
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
		"route":       "reasoning",
		"temperature": 0.3,
		"max_tokens":  1000,
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
		Quality    int      `json:"quality"`
		Issues     []string `json:"issues"`
		Suggestions []string `json:"suggestions"`
		Learned    string   `json:"learned"`
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
