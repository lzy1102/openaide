package compress

import (
	"context"
	"fmt"
	"strings"

	"openaide/backend/core"
)

// NovelCompressor 小说式上下文压缩器
type NovelCompressor struct {
	maxSummaryLength int
}

// NewNovelCompressor 创建小说式压缩器
func NewNovelCompressor() *NovelCompressor {
	return &NovelCompressor{
		maxSummaryLength: 500,
	}
}

// Compress 压缩消息列表
func (c *NovelCompressor) Compress(ctx context.Context, messages []kernel.Message, maxTokens int) ([]kernel.Message, int, error) {
	if len(messages) <= 4 {
		return messages, 0, nil
	}

	var result []kernel.Message
	var history []kernel.Message

	for _, msg := range messages {
		if msg.Role == "system" {
			result = append(result, msg)
		} else {
			history = append(history, msg)
		}
	}

	// 保留最近的 4 条消息
	keepCount := 4
	if len(history) > keepCount {
		oldMessages := history[:len(history)-keepCount]
		recentMessages := history[len(history)-keepCount:]

		// 生成"章节摘要"
		summary := c.generateChapterSummary(oldMessages)
		if summary != "" {
			result = append(result, kernel.Message{
				Role:    "system",
				Content: fmt.Sprintf("[前文摘要 - %s]", summary),
			})
		}

		// 添加"悬念钩子"
		hook := c.generateCliffhanger(oldMessages)
		if hook != "" {
			result = append(result, kernel.Message{
				Role:    "system",
				Content: fmt.Sprintf("[待解决问题] %s", hook),
			})
		}

		result = append(result, recentMessages...)
	} else {
		result = append(result, history...)
	}

	saved := c.estimateTokens(messages) - c.estimateTokens(result)
	if saved < 0 {
		saved = 0
	}

	// Enforce maxTokens budget: trim oldest history messages if result exceeds limit
	if maxTokens > 0 && c.estimateTokens(result) > maxTokens {
		// Separate system prefix from recent messages to trim only the history tail
		var sysMsgs []kernel.Message
		var histMsgs []kernel.Message
		for _, msg := range result {
			if msg.Role == "system" {
				sysMsgs = append(sysMsgs, msg)
			} else {
				histMsgs = append(histMsgs, msg)
			}
		}
		sysTokens := c.estimateTokens(sysMsgs)
		budget := maxTokens - sysTokens
		if budget < 0 {
			budget = 0
		}
		// Drop oldest non-system messages until within budget
		for len(histMsgs) > 0 && c.estimateTokens(histMsgs) > budget {
			histMsgs = histMsgs[1:]
		}
		result = append(sysMsgs, histMsgs...)
		saved = c.estimateTokens(messages) - c.estimateTokens(result)
		if saved < 0 {
			saved = 0
		}
	}

	return result, saved, nil
}

// EstimateTokens 估算 Token 数
func (c *NovelCompressor) EstimateTokens(messages []kernel.Message) int {
	return c.estimateTokens(messages)
}

func (c *NovelCompressor) estimateTokens(messages []kernel.Message) int {
	total := 0
	for _, msg := range messages {
		content := msg.Content
		for _, r := range content {
			if r > 127 {
				total += 2
			} else {
				total += 1
			}
		}
		total += 4
	}
	return total
}

// generateChapterSummary 生成章节摘要
func (c *NovelCompressor) generateChapterSummary(messages []kernel.Message) string {
	var parts []string

	// 按角色分组统计
	userQueries := 0
	assistantResponses := 0
	toolCalls := 0

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			userQueries++
		case "assistant":
			assistantResponses++
			toolCalls += len(msg.ToolCalls)
		}
	}

	parts = append(parts, fmt.Sprintf("共 %d 轮对话", userQueries))

	if toolCalls > 0 {
		parts = append(parts, fmt.Sprintf("使用了 %d 次工具", toolCalls))
	}

	// 提取关键主题（简单实现）
	topics := c.extractTopics(messages)
	if len(topics) > 0 {
		parts = append(parts, fmt.Sprintf("涉及主题: %s", strings.Join(topics, ", ")))
	}

	summary := strings.Join(parts, "，")
	if len([]rune(summary)) > c.maxSummaryLength {
		rs := []rune(summary)
		summary = string(rs[:c.maxSummaryLength]) + "..."
	}

	return summary
}

// generateCliffhanger 生成悬念钩子
func (c *NovelCompressor) generateCliffhanger(messages []kernel.Message) string {
	// 找到最后未解决的问题
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "user" {
			// 检查是否有后续 assistant 回复
			hasResponse := false
			for j := i + 1; j < len(messages); j++ {
				if messages[j].Role == "assistant" {
					hasResponse = true
					break
				}
			}
			if !hasResponse {
				content := msg.Content
				if len([]rune(content)) > 100 {
					rs := []rune(content)
					content = string(rs[:100]) + "..."
				}
				return content
			}
		}
	}

	// 检查是否有失败的工具调用
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "tool" && strings.HasPrefix(strings.TrimSpace(msg.Content), "Error:") {
			return "存在未解决的工具调用错误"
		}
	}

	return ""
}

// extractTopics 提取主题
func (c *NovelCompressor) extractTopics(messages []kernel.Message) []string {
	// Extract frequent meaningful words (no hardcoded term list)
	keywords := map[string]int{}

	for _, msg := range messages {
		for _, word := range strings.Fields(strings.ToLower(msg.Content)) {
			word = strings.Trim(word, ".,;:!?()[]{}\"")
			if len(word) > 3 {
				keywords[word]++
			}
		}
	}

	// 取前 3 个高频词
	var topics []string
	type kv struct {
		key   string
		value int
	}
	var pairs []kv
	for k, v := range keywords {
		pairs = append(pairs, kv{k, v})
	}

	// 简单排序
	for i := 0; i < len(pairs) && i < 3; i++ {
		maxIdx := i
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].value > pairs[maxIdx].value {
				maxIdx = j
			}
		}
		pairs[i], pairs[maxIdx] = pairs[maxIdx], pairs[i]
		topics = append(topics, pairs[i].key)
	}

	return topics
}

// Ensure NovelCompressor implements ContextCompressor
var _ kernel.ContextCompressor = (*NovelCompressor)(nil)
