package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// directionCheckInterval 每隔多少轮执行一次方向检查。
const directionCheckInterval = 5

// progressStallThreshold 连续无实质产出的轮数阈值。
// 达到该阈值时注入收敛提示,防止 agent 空转烧 token。
const progressStallThreshold = 5

// directionCheckPrompt 让 LLM 判断 agent 是否仍在朝原始需求前进。
// 输入: 原始查询 + 最近工具调用活动;输出: 严格 on_track 或 off_track。
const directionCheckPrompt = `You are monitoring an AI agent working on a user request.

## Original request
%s

## Recent agent activity (last %d rounds)
%s

## Task
Is the agent still working toward the original request, or has it drifted off course?

Reply with EXACTLY one line:
- "on_track" if the agent is still making progress toward the request
- "off_track <reason>" if the agent has drifted (e.g. exploring irrelevant areas, repeated dead ends, lost the goal)

No other text.`

// checkDirection 检查最近几轮的工具活动是否仍朝原始需求前进。
// 返回非空字符串表示检测到偏离,内容为注入给模型的重聚焦提示。
// LLM 失败或无法解析时返回空串(不阻断主流程)。
func (k *AgentKernel) checkDirection(ctx context.Context, query string, messages []Message) string {
	if k.llmProvider == nil {
		return ""
	}
	activity := recentActivitySummary(messages, 8)
	if activity == "" {
		return ""
	}

	prompt := fmt.Sprintf(directionCheckPrompt, truncStr(query, 300), 8, activity)
	resp, err := k.llmProvider.Chat(ctx, []Message{
		{Role: "user", Content: prompt},
	}, nil, map[string]interface{}{
		"max_tokens": 80, "temperature": 0, "route": "execution", "no_thinking": true,
	})
	if err != nil {
		slog.Debug("Direction check LLM failed, skipping", "error", err)
		return ""
	}

	body := strings.TrimSpace(resp.Content)
	if strings.HasPrefix(body, "off_track") {
		reason := strings.TrimSpace(strings.TrimPrefix(body, "off_track"))
		return fmt.Sprintf("[Direction] You appear to have drifted from the original request: %s. Re-focus on the original goal: %s", reason, truncStr(query, 200))
	}
	return ""
}

// recentActivitySummary 提取最近 limit 轮的工具调用活动摘要(每轮一行)。
func recentActivitySummary(messages []Message, limit int) string {
	var lines []string
	for i := len(messages) - 1; i >= 0 && len(lines) < limit; i-- {
		m := messages[i]
		switch m.Role {
		case "assistant":
			if len(m.ToolCalls) > 0 {
				names := make([]string, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					names = append(names, tc.Function.Name)
				}
				lines = append(lines, "assistant called: "+strings.Join(names, ", "))
			} else if m.Content != "" {
				lines = append(lines, "assistant: "+truncStr(m.Content, 120))
			}
		case "tool":
			lines = append(lines, "tool result: "+truncStr(m.Content, 120))
		case "user":
			lines = append(lines, "user: "+truncStr(m.Content, 120))
		}
	}
	// 逆序恢复时间顺序
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return strings.Join(lines, "\n")
}
