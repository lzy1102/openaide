package kernel

import (
	"context"
	"fmt"
	"strings"
)

// SimplePatternDetector 简单模式检测器
type SimplePatternDetector struct{}

// NewSimplePatternDetector 创建模式检测器
func NewSimplePatternDetector() *SimplePatternDetector {
	return &SimplePatternDetector{}
}

// Detect 检测消息中的模式
func (d *SimplePatternDetector) Detect(ctx context.Context, sessionID string, messages []Message) ([]Pattern, error) {
	var patterns []Pattern

	// 检测重复查询模式
	queryPatterns := d.detectRepeatedQueries(messages)
	patterns = append(patterns, queryPatterns...)

	// 检测工具使用模式
	toolPatterns := d.detectToolPatterns(messages)
	patterns = append(patterns, toolPatterns...)

	// 检测对话风格模式
	stylePatterns := d.detectStylePatterns(messages)
	patterns = append(patterns, stylePatterns...)

	// 检测错误模式
	errorPatterns := d.detectErrorPatterns(messages)
	patterns = append(patterns, errorPatterns...)

	return patterns, nil
}

func (d *SimplePatternDetector) detectRepeatedQueries(messages []Message) []Pattern {
	var patterns []Pattern
	queryCount := make(map[string]int)

	for _, msg := range messages {
		if msg.Role == "user" {
			// 简化查询用于统计（取前 20 个字符）
			simplified := simplifyQuery(msg.Content)
			queryCount[simplified]++
		}
	}

	for query, count := range queryCount {
		if count >= 3 {
			patterns = append(patterns, Pattern{
				Type:        "repeated_query",
				Description: fmt.Sprintf("用户重复询问相似问题: %s", query),
				Confidence:  min(0.9, float64(count)*0.1),
				Frequency:   count,
			})
		}
	}

	return patterns
}

func (d *SimplePatternDetector) detectToolPatterns(messages []Message) []Pattern {
	var patterns []Pattern
	toolUsage := make(map[string]int)
	toolSequences := make(map[string]int)

	var lastTool string
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				toolName := tc.Function.Name
				toolUsage[toolName]++

				if lastTool != "" {
					seq := lastTool + " -> " + toolName
					toolSequences[seq]++
				}
				lastTool = toolName
			}
		}
	}

	// 高频工具
	for tool, count := range toolUsage {
		if count >= 3 {
			patterns = append(patterns, Pattern{
				Type:        "frequent_tool",
				Description: fmt.Sprintf("频繁使用工具: %s", tool),
				Confidence:  min(0.8, float64(count)*0.05),
				Frequency:   count,
			})
		}
	}

	// 常见工具序列
	for seq, count := range toolSequences {
		if count >= 2 {
			patterns = append(patterns, Pattern{
				Type:        "tool_sequence",
				Description: fmt.Sprintf("常见工具序列: %s", seq),
				Confidence:  min(0.7, float64(count)*0.1),
				Frequency:   count,
			})
		}
	}

	return patterns
}

func (d *SimplePatternDetector) detectStylePatterns(messages []Message) []Pattern {
	var patterns []Pattern

	// 检测长回复模式
	longResponses := 0
	shortResponses := 0
	codeBlocks := 0

	for _, msg := range messages {
		if msg.Role == "assistant" {
			if len(msg.Content) > 500 {
				longResponses++
			} else if len(msg.Content) < 100 {
				shortResponses++
			}
			if strings.Contains(msg.Content, "```") {
				codeBlocks++
			}
		}
	}

	total := longResponses + shortResponses
	if total > 0 {
		if float64(longResponses)/float64(total) > 0.7 {
			patterns = append(patterns, Pattern{
				Type:        "response_style",
				Description: "助手倾向于提供详细的长回复",
				Confidence:  0.7,
				Frequency:   longResponses,
			})
		}
		if float64(shortResponses)/float64(total) > 0.7 {
			patterns = append(patterns, Pattern{
				Type:        "response_style",
				Description: "助手倾向于提供简短的回复",
				Confidence:  0.7,
				Frequency:   shortResponses,
			})
		}
	}

	if codeBlocks >= 3 {
		patterns = append(patterns, Pattern{
			Type:        "code_preference",
			Description: "对话中频繁涉及代码",
			Confidence:  min(0.8, float64(codeBlocks)*0.05),
			Frequency:   codeBlocks,
		})
	}

	return patterns
}

func (d *SimplePatternDetector) detectErrorPatterns(messages []Message) []Pattern {
	var patterns []Pattern
	errorCount := 0
	errorKeywords := []string{"error", "failed", "exception", "timeout", "错误", "失败"}

	for _, msg := range messages {
		if msg.Role == "tool" || msg.Role == "assistant" {
			lower := strings.ToLower(msg.Content)
			for _, kw := range errorKeywords {
				if strings.Contains(lower, kw) {
					errorCount++
					break
				}
			}
		}
	}

	if errorCount >= 3 {
		patterns = append(patterns, Pattern{
			Type:        "error_prone",
			Description: "对话中频繁出现错误，可能需要改进工具或提示",
			Confidence:  min(0.8, float64(errorCount)*0.05),
			Frequency:   errorCount,
		})
	}

	return patterns
}

func simplifyQuery(content string) string {
	// 取前 20 个字符作为简化查询
	if len(content) <= 20 {
		return strings.ToLower(strings.TrimSpace(content))
	}
	return strings.ToLower(strings.TrimSpace(content[:20]))
}
