package kernel

import (
	"context"
	"fmt"
	"strings"
)

// SimpleReflection 简单反思实现
type SimpleReflection struct{}

// NewSimpleReflection 创建简单反思器
func NewSimpleReflection() *SimpleReflection {
	return &SimpleReflection{}
}

// Reflect 对执行过程进行反思
func (r *SimpleReflection) Reflect(ctx context.Context, sessionID string, execution ExecutionRecord) (*ReflectionResult, error) {
	result := &ReflectionResult{
		Quality:     5,
		Issues:      []string{},
		Suggestions: []string{},
		Learned:     "",
	}

	// 基于执行结果评估质量
	if execution.Success {
		result.Quality = 8
	} else {
		result.Quality = 3
		result.Issues = append(result.Issues, fmt.Sprintf("执行失败: %s", execution.Error))
	}

	// 检查工具调用数量
	if len(execution.ToolCalls) > 5 {
		result.Issues = append(result.Issues, "工具调用次数过多，可能存在效率问题")
		result.Suggestions = append(result.Suggestions, "尝试合并工具调用或减少不必要的调用")
		result.Quality -= 1
	}

	// 检查响应长度
	if len(execution.Response) < 50 {
		result.Issues = append(result.Issues, "响应过短，可能没有充分回答问题")
		result.Suggestions = append(result.Suggestions, "提供更详细的解释和示例")
		result.Quality -= 1
	}

	// 检查是否包含错误关键词
	errorKeywords := []string{"error", "failed", "unable", "cannot", "不知道", "错误", "失败"}
	lowerResp := strings.ToLower(execution.Response)
	for _, kw := range errorKeywords {
		if strings.Contains(lowerResp, kw) {
			result.Issues = append(result.Issues, fmt.Sprintf("响应包含负面关键词: %s", kw))
			result.Quality -= 1
			break
		}
	}

	// 生成学习经验
	if result.Quality >= 7 {
		result.Learned = "本次执行效果良好，保持了有效的工具使用和清晰的响应"
	} else if result.Quality >= 4 {
		result.Learned = "本次执行有改进空间，需要优化工具调用策略和响应质量"
	} else {
		result.Learned = "本次执行效果不佳，需要重新审视问题分解和解决方案"
	}

	// 确保质量在 1-10 范围内
	if result.Quality < 1 {
		result.Quality = 1
	}
	if result.Quality > 10 {
		result.Quality = 10
	}

	return result, nil
}
