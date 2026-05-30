package kernel

import (
	"context"
	"strings"
)

// SimpleCompressor 简单上下文压缩器
// 基于消息数量的简单压缩，后续可替换为更智能的压缩策略
type SimpleCompressor struct{}

// Compress 压缩消息列表
func (c *SimpleCompressor) Compress(ctx context.Context, messages []Message, maxTokens int) ([]Message, int, error) {
	if len(messages) <= 2 {
		return messages, 0, nil
	}

	// 保留 system 和最近的用户消息，压缩中间的历史
	var result []Message
	var history []Message

	for _, msg := range messages {
		if msg.Role == "system" {
			result = append(result, msg)
		} else {
			history = append(history, msg)
		}
	}

	// 保留最近的 4 条消息（2 轮对话）
	keepCount := 4
	if len(history) > keepCount {
		// 压缩旧消息为摘要
		oldMessages := history[:len(history)-keepCount]
		recentMessages := history[len(history)-keepCount:]

		summary := c.summarize(oldMessages)
		if summary != "" {
			result = append(result, Message{
				Role:    "system",
				Content: "[历史对话摘要] " + summary,
			})
		}
		result = append(result, recentMessages...)
	} else {
		result = append(result, history...)
	}

	saved := c.EstimateTokens(messages) - c.EstimateTokens(result)
	return result, saved, nil
}

// EstimateTokens 估算 Token 数（采样估算：长文本抽样，短文本精确）
// 经验值：英文 ~4chars/token, 中文 ~2chars/token, 代码 ~3.5chars/token
func (c *SimpleCompressor) EstimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateTextTokens(msg.Content)
		total += 4 // message overhead
		if len(msg.ToolCalls) > 0 { total += 20 } // tool call overhead
		if msg.ReasoningContent != "" { total += estimateTextTokens(msg.ReasoningContent) }
	}
	return total
}

// estimateTextTokens 对文本做 token 估算：短文本逐字符，长文本抽样
func estimateTextTokens(text string) int {
	if len(text) < 500 {
		return charBasedTokenEstimate(text)
	}
	// 抽样：每 10 行抽样 1 行，用样本来估算全文
	lines := strings.Split(text, "\n")
	sampleLines := 0
	sampleTokens := 0
	for i, line := range lines {
		if i%10 == 0 {
			sampleTokens += charBasedTokenEstimate(line)
			sampleLines++
		}
	}
	if sampleLines == 0 { return charBasedTokenEstimate(text) }
	return sampleTokens * len(lines) / sampleLines
}

// charBasedTokenEstimate 基于字符的 token 粗略估算
func charBasedTokenEstimate(text string) int {
	ascii := 0
	cjk := 0
	for _, r := range text {
		if r > 0x2E80 { // CJK 范围
			cjk++
		} else if r > 127 {
			ascii++ // 其他非 ASCII
		} else {
			ascii++
		}
	}
	// ASCII ~4chars/token, CJK ~2chars/token
	return ascii/4 + cjk/2 + 1
}

// summarize 生成消息摘要
func (c *SimpleCompressor) summarize(messages []Message) string {
	var parts []string
	for _, msg := range messages {
		prefix := ""
		switch msg.Role {
		case "user":
			prefix = "用户"
		case "assistant":
			prefix = "助手"
		case "tool":
			prefix = "工具"
		}
		content := msg.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		parts = append(parts, prefix+": "+content)
	}

	result := strings.Join(parts, "; ")
	if len(result) > 500 {
		result = result[:500] + "..."
	}
	return result
}


