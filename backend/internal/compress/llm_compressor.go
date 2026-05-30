package compress

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"openaide/backend/internal/kernel"
)

// LLMCompressor 基于 LLM 的语义上下文压缩器
// 用 LLM 生成有意义的摘要，而非简单的文本截断拼接。
// 实现了 kernel.ContextCompressor 接口，可替换 NovelCompressor。
type LLMCompressor struct {
	llm      kernel.LLMProvider
	fallback kernel.ContextCompressor // LLM 不可用时的降级压缩器
}

// NewLLMCompressor 创建基于 LLM 的压缩器
// llm: 用于生成摘要的 LLM 提供商
// fallback: LLM 调用失败时的降级压缩器（通常为 NovelCompressor）
func NewLLMCompressor(llm kernel.LLMProvider, fallback kernel.ContextCompressor) *LLMCompressor {
	return &LLMCompressor{llm: llm, fallback: fallback}
}

// Compress 压缩消息列表
// 策略：
// 1. 保留所有 system 消息
// 2. 保留最近 keepCount 条非 system 消息
// 3. 将较旧的消息交给 LLM 生成语义摘要
// 4. 摘要作为 system 消息插入
func (c *LLMCompressor) Compress(ctx context.Context, messages []kernel.Message, maxTokens int) ([]kernel.Message, int, error) {
	if len(messages) <= 4 {
		return messages, 0, nil
	}

	saved := c.fallback.EstimateTokens(messages)

	var result []kernel.Message
	var history []kernel.Message

	// 分离 system 和历史消息
	for _, msg := range messages {
		if msg.Role == "system" {
			result = append(result, msg)
		} else {
			history = append(history, msg)
		}
	}

	// 保留最近 keepCount 条
	const keepCount = 4
	if len(history) <= keepCount {
		return messages, 0, nil
	}

	oldMessages := history[:len(history)-keepCount]
	recentMessages := history[len(history)-keepCount:]

	// 用 LLM 生成摘要
	summary, err := c.generateSummary(ctx, oldMessages)
	if err != nil {
		// LLM 失败 → fallback
		compressed, fbSaved, fbErr := c.fallback.Compress(ctx, messages, maxTokens)
		if fbErr != nil {
			return messages, 0, fbErr
		}
		return compressed, fbSaved, nil
	}

	if summary != "" {
		result = append(result, kernel.Message{
			Role:    "system",
			Content: fmt.Sprintf("[前文摘要] %s", summary),
		})
	}

	// 提取未决问题（从旧消息中检测未回答的用户问题）
	pending := extractPendingQuestions(oldMessages)
	if pending != "" {
		result = append(result, kernel.Message{
			Role:    "system",
			Content: fmt.Sprintf("[待解决问题] %s", pending),
		})
	}

	result = append(result, recentMessages...)

	if saved <= 0 {
		saved = c.fallback.EstimateTokens(messages) - c.fallback.EstimateTokens(result)
	}
	if saved < 0 {
		saved = 0
	}

	return result, saved, nil
}

// EstimateTokens 估算 token 数量（委托给 fallback 的估算方法）
func (c *LLMCompressor) EstimateTokens(messages []kernel.Message) int {
	return c.fallback.EstimateTokens(messages)
}

// generateSummary 用 LLM 生成历史消息的语义摘要
func (c *LLMCompressor) generateSummary(ctx context.Context, messages []kernel.Message) (string, error) {
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
		default:
			prefix = msg.Role
		}
		content := msg.Content
		if len([]rune(content)) > 200 {
			rs := []rune(content)
			content = string(rs[:200]) + "..."
		}
		parts = append(parts, prefix+": "+content)
	}

	input := fmt.Sprintf(`Compress the conversation below into a structured summary. The summary replaces the full history in the LLM's context window, so you must preserve everything needed for future reasoning.

## Priority order (must keep, from most important)

1. **User requests & intents** — What did the user ask for? Include ALL explicit requests, even if already completed (the LLM may need to refer back).
2. **Decisions & agreements** — What was decided? "Use PostgreSQL instead of MySQL", "Agreed on 3-phase plan", "User prefers concise answers".
3. **Technical facts & findings** — File paths read, error messages seen, command outputs, code patterns identified, test results.
4. **Current task state** — What is the in-progress task? Which step are we on? What remains to be done?
5. **Tool call results** — Only keep results that changed state or returned important data. Skip trivial outputs (ls results, small reads with no findings).

## What to discard (waste, not worth keeping)

- Greetings, politeness, acknowledgments ("got it", "thanks", "will do")
- Boilerplate tool output (long ls listings, large file reads with no relevant content)
- Redundant confirmations (asking "shall I proceed?" after user already said yes)
- Failed attempts that were already corrected (keep only the final successful approach)

## Output format

Output a plain text summary structured as:

[用户意图] What the user ultimately wants to achieve
[关键事实] File paths, error messages, data, decisions (as bullet list if multiple)
[当前状态] Current step, what's done, what remains
[注意事项] Any user preferences, constraints, or pitfalls to avoid

Keep the total under 200 words. Write in the same language the user used (Chinese/English).

## Conversation
%s

## Summary`, strings.Join(parts, "\n"))

	resp, err := c.llm.Chat(ctx, []kernel.Message{
		{Role: "system", Content: `You are a context compression expert for an AI coding agent. Your summaries replace the full conversation history in the agent's context window. The agent must be able to continue working from your summary alone.

Rules:
- Preserve: user intents, decisions, technical facts, current task state
- Discard: greetings, boilerplate output, redundant confirmations, failed attempts
- Be specific: "read main.go:42-58, found login handler with SQL injection risk" not "read some code"
- Match the user's language (Chinese or English)
- Under 200 words`},
		{Role: "user", Content: input},
	}, nil, map[string]interface{}{
		"max_tokens":  400,
		"temperature": 0.2,
	})
	if err != nil {
		slog.Debug("LLM compression failed", "error", err)
		return "", err
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", fmt.Errorf("empty summary from LLM")
	}

	return summary, nil
}

// extractPendingQuestions 从历史消息中提取未解决的疑问
func extractPendingQuestions(messages []kernel.Message) string {
	// 检查最后几条 user 消息是否都得到了回答
	var lastUserContent string
	var answered bool

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "user" && lastUserContent == "" {
			lastUserContent = msg.Content
		}
		if msg.Role == "assistant" && lastUserContent != "" {
			answered = true
			break
		}
		if msg.Role == "user" && lastUserContent != "" {
			break
		}
	}

	if lastUserContent != "" && !answered {
		if len([]rune(lastUserContent)) > 100 {
			rs := []rune(lastUserContent)
			lastUserContent = string(rs[:100]) + "..."
		}
		return lastUserContent
	}

	return ""
}
