package kernel

import "context"

// SimpleReflection 基础反思实现（LLMReflection 的降级兜底）
// 不做启发式评分，只返回中性结果——真正的评估由 LLMReflection 完成
type SimpleReflection struct{}

func NewSimpleReflection() *SimpleReflection {
	return &SimpleReflection{}
}

func (r *SimpleReflection) Reflect(ctx context.Context, sessionID string, execution ExecutionRecord) (*ReflectionResult, error) {
	return &ReflectionResult{
		Quality:     5,
		Issues:      []string{},
		Suggestions: []string{},
		Learned:     "LLM reflection unavailable — using neutral fallback",
	}, nil
}
