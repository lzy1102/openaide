package services

import (
	"strings"
	"unicode/utf8"
)

// TokenEstimator Token估算器
// 基于不同模型的tokenizer特性提供相对准确的token估算
type TokenEstimator struct {
	modelCoefficients map[string]float64
	modelLimits       map[string]int
}

// NewTokenEstimator 创建Token估算器
func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{
		modelCoefficients: map[string]float64{
			"gpt-4":           0.25,
			"gpt-4-turbo":     0.25,
			"gpt-3.5-turbo":   0.25,
			"gpt-4o":          0.25,
			"gpt-4o-mini":     0.25,
			"gpt-5-mini":      0.25,
			"claude-3-opus":   0.27,
			"claude-3-sonnet": 0.27,
			"claude-3-haiku":  0.27,
			"deepseek-chat":   0.25,
			"deepseek-coder":  0.25,
			"qwen-turbo":      0.30,
			"qwen-plus":       0.30,
			"qwen-max":        0.30,
			"default":         0.30,
		},
		modelLimits: map[string]int{
			"gpt-4":           8192,
			"gpt-4-turbo":     128000,
			"gpt-3.5-turbo":   16385,
			"gpt-4o":          128000,
			"gpt-4o-mini":     128000,
			"gpt-5-mini":      128000,
			"claude-3-opus":   200000,
			"claude-3-sonnet": 200000,
			"claude-3-haiku":  200000,
			"deepseek-chat":   64000,
			"deepseek-coder":  64000,
			"qwen-turbo":      8192,
			"qwen-plus":       32000,
			"qwen-max":        32000,
		},
	}
}

// EstimateTokens 估算文本的token数量
// 使用基于字数的启发式算法，不同语言有不同的系数
func (e *TokenEstimator) EstimateTokens(text string, modelName string) int {
	if text == "" {
		return 0
	}

	// 获取模型系数
	coefficient := e.modelCoefficients["default"]
	if coef, ok := e.modelCoefficients[modelName]; ok {
		coefficient = coef
	}

	// 计算字符数
	charCount := utf8.RuneCountInString(text)

	// 估算token数 = 字符数 / 系数
	// 中文通常1个token ≈ 1-1.5个字符
	// 英文通常1个token ≈ 4个字符
	estimatedTokens := int(float64(charCount) / coefficient)

	// 添加基础开销（系统提示、格式等）
	overhead := e.estimateOverhead(text)

	return estimatedTokens + overhead
}

// EstimateMessagesTokens 估算消息列表的总token数
func (e *TokenEstimator) EstimateMessagesTokens(messages []map[string]string, modelName string) int {
	total := 0
	for _, msg := range messages {
		// 每条消息的基础开销（role字段等）
		total += 4

		if content, ok := msg["content"]; ok {
			total += e.EstimateTokens(content, modelName)
		}
	}
	// 对话格式开销
	total += 2
	return total
}

// GetModelLimit 获取模型的上下文长度限制
func (e *TokenEstimator) GetModelLimit(modelName string) int {
	if limit, ok := e.modelLimits[modelName]; ok {
		return limit
	}
	return 4096
}

// estimateOverhead 估算额外开销
func (e *TokenEstimator) estimateOverhead(text string) int {
	overhead := 3
	overhead += strings.Count(text, "```") * 2
	overhead += strings.Count(text, "\n- ") + strings.Count(text, "\n* ")
	overhead += strings.Count(text, "\n# ") * 2
	return overhead
}

// ShouldTruncate 判断是否需要截断上下文
func (e *TokenEstimator) ShouldTruncate(messages []map[string]string, modelName string) (bool, int) {
	totalTokens := e.EstimateMessagesTokens(messages, modelName)
	limit := e.GetModelLimit(modelName)

	// 保留20%的余量给响应
	safeLimit := int(float64(limit) * 0.8)

	return totalTokens > safeLimit, totalTokens
}

// TruncateMessages 智能截断消息列表
// 保留系统消息和最近的消息，截断中间的历史
func (e *TokenEstimator) TruncateMessages(messages []map[string]string, modelName string, maxTokens int) []map[string]string {
	if len(messages) == 0 {
		return messages
	}

	currentTokens := e.EstimateMessagesTokens(messages, modelName)
	if currentTokens <= maxTokens {
		return messages
	}

	var systemMsgs []map[string]string
	var normalMsgs []map[string]string

	for _, msg := range messages {
		if role, ok := msg["role"]; ok && role == "system" {
			systemMsgs = append(systemMsgs, msg)
		} else {
			normalMsgs = append(normalMsgs, msg)
		}
	}

	systemTokens := e.EstimateMessagesTokens(systemMsgs, modelName)
	availableTokens := maxTokens - systemTokens - 10

	if availableTokens <= 0 {
		return systemMsgs
	}

	var keptNormal []map[string]string
	remainingTokens := availableTokens
	for i := len(normalMsgs) - 1; i >= 0; i-- {
		msgTokens := e.EstimateMessagesTokens([]map[string]string{normalMsgs[i]}, modelName)
		if msgTokens <= remainingTokens {
			keptNormal = append([]map[string]string{normalMsgs[i]}, keptNormal...)
			remainingTokens -= msgTokens
		} else {
			break
		}
	}

	result := make([]map[string]string, 0, len(systemMsgs)+len(keptNormal))
	result = append(result, systemMsgs...)
	result = append(result, keptNormal...)

	return result
}
