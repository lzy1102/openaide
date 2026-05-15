package kernel

import (
	"encoding/json"
	"strings"
)

// SimpleCompressor 简单上下文压缩器
// 基于消息数量的简单压缩，后续可替换为更智能的压缩策略
type SimpleCompressor struct{}

// Compress 压缩消息列表
func (c *SimpleCompressor) Compress(messages []Message, maxTokens int) ([]Message, int, error) {
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

// EstimateTokens 估算 Token 数（简单估算：中文字符按 1.5 token，英文按 0.75 token）
func (c *SimpleCompressor) EstimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		content := msg.Content
		for _, r := range content {
			if r > 127 {
				total += 2 // 中文字符约 2 token
			} else {
				total += 1 // 英文字符约 1 token
			}
		}
		// 消息格式开销
		total += 4
	}
	return total
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

// NoopCompressor 不压缩的压缩器（用于测试）
type NoopCompressor struct{}

func (c *NoopCompressor) Compress(messages []Message, maxTokens int) ([]Message, int, error) {
	return messages, 0, nil
}

func (c *NoopCompressor) EstimateTokens(messages []Message) int {
	return 0
}

// JSON 序列化辅助函数
func (m Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

func MessageFromJSON(data []byte) (*Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return &msg, err
}
