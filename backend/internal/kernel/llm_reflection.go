package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// LLMReflection 基于 LLM 调用的反思实现
// 将执行记录发给 LLM 做结构化分析，比 SimpleReflection 的规则评估更准确
type LLMReflection struct {
	llm      LLMProvider
	fallback *SimpleReflection // LLM 不可用时的降级
}

// NewLLMReflection 创建基于 LLM 的反思器
// llm: 用于分析的 LLM 提供商（通常是 Gateway，带故障转移）
// fallback: LLM 调用失败时的规则评估兜底
func NewLLMReflection(llm LLMProvider, fallback *SimpleReflection) *LLMReflection {
	return &LLMReflection{llm: llm, fallback: fallback}
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
	prompt := fmt.Sprintf(`Evaluate this AI assistant's execution quality.

## Query
%s

## Response
%s

## Execution Details
- Success: %v
- Error: %s
- Tool Calls: %d
- Duration: %dms

Analyze the execution and provide structured feedback. Be specific and actionable.`,
		execution.Query, execution.Response, execution.Success, execution.Error,
		len(execution.ToolCalls), execution.Duration)

	messages := []Message{
		{
			Role:    "system",
			Content: "You are an AI execution quality analyst. Evaluate the assistant's performance and provide structured feedback.",
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
		slog.Debug("LLM reflection failed, using fallback", "error", err)
		return r.fallback.Reflect(ctx, sessionID, execution)
	}

	if len(resp.ToolCalls) == 0 {
		slog.Debug("LLM reflection returned no tool calls, using fallback")
		return r.fallback.Reflect(ctx, sessionID, execution)
	}

	result, parseErr := parseReflectionResult(resp.ToolCalls[0].Function.Arguments)
	if parseErr != nil {
		slog.Debug("Failed to parse LLM reflection result, using fallback", "error", parseErr)
		return r.fallback.Reflect(ctx, sessionID, execution)
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
