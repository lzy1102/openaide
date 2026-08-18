package kernel

import (
	"strings"
)

// trimHistoryToBudget 按 token 预算截断历史:从旧到新累积 token,
// 超过预算时丢弃剩余旧消息。至少保留 2 条,保证对话上下文连贯。
func trimHistoryToBudget(history []Message, budget int) []Message {
	total := 0
	keep := 0
	for i, msg := range history {
		total += estimateTextTokens(msg.Content) + 4
		if total > budget && i >= 2 {
			break
		}
		keep = i + 1
	}
	if keep < len(history) {
		return history[:keep]
	}
	return history
}

// estimateMessagesTokens 统一的消息 token 估算(历史截断与压缩阈值共用)。
// 避免与压缩器内部估算口径不一致导致的预算判断偏差。
func estimateMessagesTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateTextTokens(msg.Content) + 4
		if len(msg.ToolCalls) > 0 {
			total += 20
		}
		if msg.ReasoningContent != "" {
			total += estimateTextTokens(msg.ReasoningContent)
		}
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
	if sampleLines == 0 {
		return charBasedTokenEstimate(text)
	}
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
