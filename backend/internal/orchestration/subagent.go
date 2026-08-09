package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"openaide/backend/internal/kernel"
)

// subAgentCoreRules are the essential L0 constraints injected into every sub-agent.
// These are the non-negotiable rules that prevent sub-agents from making common mistakes.
const subAgentCoreRules = `## Core Rules — ALWAYS Follow
- Read before write. Understand before change. Never guess file paths.
- If read_file returns "not found": search_files for the actual path.
- After any code change: read back the modified lines to verify correctness.
- Never add features the user didn't ask for. Stay focused on the assigned task.
- Never execute destructive commands without understanding their impact.
- When you need multiple files: read them in parallel, not one at a time.
- Prefer small, focused edits over large rewrites.
- Tool failed? Try an alternative approach. One failure is not a dead end.
- Never leave TODO or FIXME comments. Either do it or don't mention it.`

// SubAgentProgress is a callback for reporting sub-agent execution progress.
// roleName: which role is executing; round: current round; status: "thinking"/"executing"/"done"/"error"/"timeout".
type SubAgentProgress func(roleName string, round int, status string)

// subAgentTimeout 子代理每轮执行超时。防止单轮 LLM 挂起或工具卡死时无限阻塞。
// 总预算由轮数自然决定(maxRounds × subAgentTimeout),长任务不受固定总时限限制。
// 可通过 Orchestrator.SetSubAgentTimeout 覆盖(测试用短超时)。
const subAgentTimeout = 60 * time.Second

// RunSubAgent creates an isolated session and runs a single role with a mini ReAct loop.
// The sub-agent gets the role's allowed tools and can actually execute them.
// Only the final result summary is returned — intermediate tool calls stay in the sub-session.
func (o *Orchestrator) RunSubAgent(ctx context.Context, userID, projectID, roleName, task string, previousResults []string, onProgress SubAgentProgress) (string, error) {
	roundTimeout := o.subAgentTimeout
	if roundTimeout <= 0 {
		roundTimeout = subAgentTimeout
	}
	const maxRounds = 10
	// 外层总安全网:轮数 × 每轮超时(防整体失控),远大于任何单轮预算。
	sctx, cancel := context.WithTimeout(ctx, maxRounds*roundTimeout)
	defer cancel()

	role := o.getTeamRole(roleName)
	if role == nil {
		// Fallback: use first available role from Team
		role = o.firstTeamRole()
		if role == nil {
			return "", fmt.Errorf("no roles available")
		}
	}

	// Build system prompt with role + core rules + project context
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("## Role: %s\n%s\n\n", role.Name, role.Prompt))
	prompt.WriteString(subAgentCoreRules)
	if len(previousResults) > 0 {
		prompt.WriteString("\n\n## Previous Results\n")
		prompt.WriteString(strings.Join(previousResults, "\n---\n"))
	}

	// Create isolated sub-session
	subUserID := fmt.Sprintf("%s-sub-%s-%d", userID, roleName, time.Now().UnixNano())
	session, err := o.sessions.Create(ctx, projectID, subUserID)
	if err != nil {
		return "", fmt.Errorf("create sub-session: %w", err)
	}
	defer o.sessions.Delete(ctx, session.ID)

	// Filter tools to only the role's allowed tools
	tools := o.toolExec.GetDefinitionsByNames(role.Tools)

	messages := []kernel.Message{
		{Role: "system", Content: prompt.String()},
		{Role: "user", Content: task},
	}

	modelRoute := pickModel(roleName)
	slog.Info("Sub-agent started", "role", roleName, "model", modelRoute, "tools", len(tools))

	// Mini ReAct loop — sub-agents can think and act
	for round := 0; round < maxRounds; round++ {
		// Progress: thinking
		if onProgress != nil {
			onProgress(roleName, round, "thinking")
		}

		// Budget injection
		if round >= maxRounds/2 && round < maxRounds-1 {
			messages = append(messages, kernel.Message{
				Role: "user", Content: fmt.Sprintf("[System] Used %d/%d rounds, %d remaining. Give your final answer if you have enough information.", round, maxRounds, maxRounds-round),
			})
		} else if round >= maxRounds-1 {
			messages = append(messages, kernel.Message{
				Role: "user", Content: "[System] Final round — must give final answer. Do NOT call any tools.",
			})
		}

		var callTools []kernel.ToolDefinition
		if round < maxRounds-1 {
			callTools = tools
		}

		// 每轮独立超时:单轮 LLM 挂起在 roundTimeout 内被砍,不影响总轮数预算。
		roundCtx, roundCancel := context.WithTimeout(sctx, roundTimeout)

		// 流式调用:让 onProgress 回调能收到每轮的增量输出,
		// 替代之前同步 Chat() 的黑盒等待。
		stream, err := o.llmGateway.ChatStream(roundCtx, messages, callTools, map[string]interface{}{
			"route":       modelRoute,
			"max_tokens":  4000,
			"temperature": 0.3,
		})
		if err != nil {
			roundCancel()
			if roundCtx.Err() == context.DeadlineExceeded {
				if onProgress != nil {
					onProgress(roleName, round, "timeout")
				}
				return "", fmt.Errorf("sub-agent %s round %d timed out after %s: %w", roleName, round, roundTimeout, err)
			}
			if onProgress != nil {
				onProgress(roleName, round, "error")
			}
			return "", fmt.Errorf("sub-agent %s: %w", roleName, err)
		}

		// 消费流式 chunks,累积 content 和 tool_calls
		// OpenAI provider 在每个 chunk 中通过 ToolCalls 字段发送当前累积的完整 tool_calls 列表,
		// 我们只需要取最后一个非空的 ToolCalls 即可。
		var contentBuf strings.Builder
		var toolCalls []kernel.ToolCall
		for chunk := range stream {
			if chunk.Content != "" {
				contentBuf.WriteString(chunk.Content)
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = make([]kernel.ToolCall, len(chunk.ToolCalls))
				copy(toolCalls, chunk.ToolCalls)
			}
			if chunk.Done {
				break
			}
		}

		// 超时检查:流在 deadline 后结束或 roundCtx 已取消
		if roundCtx.Err() != nil {
			roundCancel()
			if onProgress != nil {
				onProgress(roleName, round, "timeout")
			}
			return "", fmt.Errorf("sub-agent %s round %d timed out after %s", roleName, round, roundTimeout)
		}
		roundCancel()

		messages = append(messages, kernel.Message{
			Role:      "assistant",
			Content:   contentBuf.String(),
			ToolCalls: toolCalls,
		})

		// No tool calls → return result
		if len(toolCalls) == 0 {
			if onProgress != nil {
				onProgress(roleName, round, "done")
			}
			return contentBuf.String(), nil
		}

		// Execute tools and feed results back
		for _, tc := range toolCalls {
			if tc.Function.Name == "" {
				continue
			}
			if onProgress != nil {
				onProgress(roleName, round, "executing:"+tc.Function.Name)
			}
			result, err := o.toolExec.Execute(sctx, tc, session.ID)
			content := fmt.Sprintf("%v", result.Content)
			if err != nil {
				content = fmt.Sprintf("Error: %v", err)
			} else if result.Error != "" {
				content = fmt.Sprintf("Error: %s", result.Error)
			}
			messages = append(messages, kernel.Message{
				Role:       "tool",
				Content:    content,
				ToolCallID: tc.ID,
			})
			slog.Debug("Sub-agent tool executed", "role", roleName, "tool", tc.Function.Name, "output_len", len(content))
		}
	}

	// Max rounds reached — synthesize final answer
	lastMsg := messages[len(messages)-1]
	if lastMsg.Role == "assistant" {
		return lastMsg.Content, nil
	}
	return "Max rounds reached without final answer.", nil
}

func (o *Orchestrator) getTeamRole(roleName string) *TeamRole {
	if o.team == nil {
		return nil
	}
	return o.team.GetRole(roleName)
}

func pickModel(roleName string) string {
	switch roleName {
	case "analyst", "reviewer":
		return "reasoning"
	case "coder", "executor", "synthesis":
		return "execution"
	default:
		return "execution"
	}
}

// groupByDependency groups subtasks so that independent tasks run in parallel
// and dependent tasks run sequentially.
func groupByDependency(subtasks []SubTask) [][]SubTask {
	if len(subtasks) == 0 {
		return nil
	}
	completed := make(map[int]bool)
	remaining := make(map[int]SubTask)
	for _, st := range subtasks {
		remaining[st.ID] = st
	}

	var groups [][]SubTask
	for len(remaining) > 0 {
		var group []SubTask
		for id, st := range remaining {
			ready := true
			for _, depID := range st.DependsOn {
				if !completed[depID] {
					ready = false
					break
				}
			}
			if ready {
				group = append(group, st)
				delete(remaining, id)
			}
		}
		if len(group) == 0 {
			for _, st := range remaining {
				group = append(group, st)
			}
			remaining = nil
		}
		groups = append(groups, group)
		for _, st := range group {
			completed[st.ID] = true
		}
	}
	return groups
}

// firstTeamRole returns the first available team role as fallback.
func (o *Orchestrator) firstTeamRole() *TeamRole {
	if o.team != nil {
		return o.team.FirstRole()
	}
	return nil
}
