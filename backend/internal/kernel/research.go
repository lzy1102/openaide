package kernel

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// readOnlyToolNames 只读研究子循环允许的工具白名单。
// 子循环绝不能做写操作,只收集信息并回灌结论。
var readOnlyToolNames = []string{
	"read_file", "list_directory", "search_files", "search_symbols",
	"web_search", "web_fetch", "read_image",
	"git_status", "git_diff", "git_log", "git_blame",
}

// researchAgentPrompt 研究子循环的系统提示词。
// 仅要求收集事实结论,不生成代码、不做变更。
const researchAgentPrompt = `You are a read-only research sub-agent. Investigate the assigned question using only read/search tools.
Output a concise factual summary (facts, file paths, findings). Never modify anything. Never speculate — cite what you read.`

// runResearchSubAgent 运行一个只读研究子循环,返回提炼后的结论。
// 子循环拥有独立的消息上下文,中间工具调用不回流主上下文,
// 只把最终总结字符串返回给调用方,实现"隔离上下文 + 回灌结论"。
// 只允许 readOnlyResearchTools 内的工具;失败时返回已收集内容与错误。
func (k *AgentKernel) runResearchSubAgent(ctx context.Context, task string) (string, error) {
	readTools := k.toolExecutor.GetDefinitionsByNames(readOnlyToolNames)

	messages := []Message{
		{Role: "system", Content: researchAgentPrompt},
		{Role: "user", Content: task},
	}

	const maxResearchRounds = 2
	for round := 0; round < maxResearchRounds; round++ {
		var callTools []ToolDefinition
		finalRound := round >= maxResearchRounds-1
		if !finalRound {
			callTools = readTools
		} else {
			messages = append(messages, Message{
				Role: "user", Content: "[System] Final round — give your research summary now. Do NOT call tools.",
			})
		}

		stream, err := k.llmProvider.ChatStream(ctx, messages, callTools, map[string]interface{}{
			"route":       "reasoning",
			"max_tokens":  800,
			"temperature": 0.2,
		})
		if err != nil {
			return "", err
		}

		var contentBuf strings.Builder
		var toolCalls []ToolCall
		for chunk := range stream {
			if chunk.Content != "" {
				contentBuf.WriteString(chunk.Content)
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = make([]ToolCall, len(chunk.ToolCalls))
				copy(toolCalls, chunk.ToolCalls)
			}
			if chunk.Done {
				break
			}
		}

		messages = append(messages, Message{
			Role: "assistant", Content: contentBuf.String(), ToolCalls: toolCalls,
		})

		if len(toolCalls) == 0 {
			return contentBuf.String(), nil
		}

		for _, tc := range toolCalls {
			if tc.Function.Name == "" {
				continue
			}
			result, err := k.toolExecutor.Execute(ctx, tc, "")
			content := ""
			if err != nil {
				content = "Error: " + err.Error()
			} else if result != nil {
				if s, ok := result.Content.(string); ok {
					content = truncateToolResult(s)
				}
				if result.Error != "" {
					content = "Error: " + result.Error
				}
			}
			messages = append(messages, Message{
				Role: "tool", Content: content, ToolCallID: tc.ID,
			})
			slog.Debug("Research sub-loop tool", "tool", tc.Function.Name, "output_len", len(content))
		}
	}
	// 到达最大轮次,回退到最近一条 assistant 内容
	last := messages[len(messages)-1]
	if last.Role == "assistant" {
		return last.Content, nil
	}
	return "", nil
}

// researchSubagentPrompt 运行并行研究并把结论注入主上下文。
// 为计划的每个子任务派生一个独立只读子循环(并发),收集结论后合并为一条系统消息。
// 并行度限制为两路,避免一次研究消耗过多 LLM 配额。
func (k *AgentKernel) researchSubagentPrompt(ctx context.Context, plan *TaskPlan) string {
	if plan == nil || len(plan.SubTasks) == 0 {
		return ""
	}
	tasks := make([]string, 0, len(plan.SubTasks))
	for _, st := range plan.SubTasks {
		tasks = append(tasks, st.Goal+" — "+st.Approach)
	}

	results := make([]string, len(tasks))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 2)

	for i, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t string) {
			defer wg.Done()
			defer func() { <-sem }()
			rc, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			out, err := k.runResearchSubAgent(rc, t)
			if err != nil {
				slog.Warn("Research sub-agent failed", "task", truncStr(t, 60), "error", err)
				out = ""
			}
			results[i] = out
		}(i, t)
	}
	wg.Wait()

	var sb strings.Builder
	sb.WriteString("[Parallel Research Findings]\n")
	any := false
	for _, out := range results {
		if strings.TrimSpace(out) == "" {
			continue
		}
		any = true
		sb.WriteString(truncStr(out, 400))
		sb.WriteString("\n---\n")
	}
	if !any {
		return ""
	}
	slog.Info("Parallel research injected", "subtasks", len(tasks))
	return strings.TrimSpace(sb.String())
}