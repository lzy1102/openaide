package services

import (
	"strings"
	"unicode/utf8"
)

// TokenEstimator Token估算器
// 基于不同模型的tokenizer特性提供相对准确的token估算
type TokenEstimator struct {
	// 模型特定的估算系数
	modelCoefficients map[string]float64
}

// NewTokenEstimator 创建Token估算器
func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{
		modelCoefficients: map[string]float64{
			// OpenAI GPT系列 (cl100k_base tokenizer)
			"gpt-4":           0.25,
			"gpt-4-turbo":     0.25,
			"gpt-3.5-turbo":   0.25,
			"gpt-4o":          0.25,
			"gpt-4o-mini":     0.25,

			// Anthropic Claude系列
			"claude-3-opus":   0.27,
			"claude-3-sonnet": 0.27,
			"claude-3-haiku":  0.27,

			// DeepSeek
			"deepseek-chat":  0.25,
			"deepseek-coder": 0.25,

			// Qwen
			"qwen-turbo": 0.30,
			"qwen-plus":  0.30,
			"qwen-max":   0.30,

			// 默认系数
			"default": 0.30,
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

// EstimateLLMMessagesTokens 估算LLM消息列表的token数
func (e *TokenEstimator) EstimateLLMMessagesTokens(messages []interface{}, modelName string) int {
	total := 0
	for _, msg := range messages {
		total += 4 // 每条消息基础开销
		// 这里简化处理，实际应该根据消息结构提取content
	}
	total += 2
	return total
}

// estimateOverhead 估算额外开销
func (e *TokenEstimator) estimateOverhead(text string) int {
	overhead := 3 // 基础开销

	// 代码块增加开销
	codeBlocks := strings.Count(text, "```")
	overhead += codeBlocks * 2

	// 列表增加开销
	listItems := strings.Count(text, "\n- ") + strings.Count(text, "\n* ")
	overhead += listItems

	// 标题增加开销
	headers := strings.Count(text, "\n# ")
	overhead += headers * 2

	return overhead
}

// GetModelLimit 获取模型的上下文长度限制
func (e *TokenEstimator) GetModelLimit(modelName string) int {
	limits := map[string]int{
		"gpt-4":           8192,
		"gpt-4-turbo":     128000,
		"gpt-3.5-turbo":   16385,
		"gpt-4o":          128000,
		"gpt-4o-mini":     128000,
		"claude-3-opus":   200000,
		"claude-3-sonnet": 200000,
		"claude-3-haiku":  200000,
		"deepseek-chat":   64000,
		"deepseek-coder":  64000,
		"qwen-turbo":      8192,
		"qwen-plus":       32000,
		"qwen-max":        32000,
	}

	if limit, ok := limits[modelName]; ok {
		return limit
	}
	return 4096 // 默认限制
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

	// 估算当前token数
	currentTokens := e.EstimateMessagesTokens(messages, modelName)
	if currentTokens <= maxTokens {
		return messages
	}

	// 分离系统消息和普通消息
	var systemMsgs []map[string]string
	var normalMsgs []map[string]string

	for _, msg := range messages {
		if role, ok := msg["role"]; ok && role == "system" {
			systemMsgs = append(systemMsgs, msg)
		} else {
			normalMsgs = append(normalMsgs, msg)
		}
	}

	// 计算系统消息的token数
	systemTokens := e.EstimateMessagesTokens(systemMsgs, modelName)

	// 普通消息可用的token数
	availableTokens := maxTokens - systemTokens - 10 // 保留10个token的余量

	if availableTokens <= 0 {
		// 如果系统消息就超限了，只保留系统消息
		return systemMsgs
	}

	// 保留最近的普通消息，直到达到限制
	var result []map[string]string
	result = append(result, systemMsgs...)

	// 从后往前添加消息（保留最近的）
	remainingTokens := availableTokens
	for i := len(normalMsgs) - 1; i >= 0; i-- {
		msgTokens := e.EstimateMessagesTokens([]map[string]string{normalMsgs[i]}, modelName)
		if msgTokens <= remainingTokens {
			result = append([]map[string]string{normalMsgs[i]}, result[1:]...) // 插入系统消息之后
			result = append([]map[string]string{systemMsgs[0]}, result...)     // 确保系统消息在最前面
			remainingTokens -= msgTokens
		} else {
			break
		}
	}

	return result
}
