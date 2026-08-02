package kernel

import (
	"context"
	"fmt"
	"strings"
)

// AdaptiveRounds 根据任务复杂度动态调整最大轮次
type AdaptiveRounds struct {
	MinRounds int
	MaxRounds int
	llm       LLMProvider // 可选：LLM 辅助估计复杂度
}

// NewAdaptiveRounds 创建自适应轮次控制器
func NewAdaptiveRounds(min, max int) *AdaptiveRounds {
	if min <= 0 {
		min = 5
	}
	if max <= 0 {
		max = 30
	}
	return &AdaptiveRounds{MinRounds: min, MaxRounds: max}
}

// SetLLM 注入 LLM 提供商用于智能估计
func (a *AdaptiveRounds) SetLLM(llm LLMProvider) { a.llm = llm }

// Calculate returns the LLM's round estimate, clamped to [MinRounds, MaxRounds].
func (a *AdaptiveRounds) Calculate(ctx context.Context, query string, historyLength int) int {
	base := a.estimateWithLLM(ctx, query)
	if base < a.MinRounds {
		base = a.MinRounds
	}
	if base > a.MaxRounds {
		base = a.MaxRounds
	}
	return base
}

func (a *AdaptiveRounds) estimateWithLLM(ctx context.Context, query string) int {
	resp, err := a.llm.Chat(ctx, []Message{
		{Role: "system", Content: "Estimate how many reasoning rounds an AI agent needs for this task. Consider complexity, number of steps, and ambiguity. Reply with only an integer between 1 and 30. Simple queries need 1-3, complex multi-step tasks need 8-15."},
		{Role: "user", Content: query},
	}, nil, map[string]interface{}{"max_tokens": 10, "temperature": 0, "route": "execution", "no_thinking": true})
	if err != nil {
		return 0
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(resp.Content), "%d", &n)
	return n
}
