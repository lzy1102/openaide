package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ── ReAct Loop Helpers ──────────────────────────────────────
// Extracted from Process and ProcessStream to eliminate duplication.
// Each method does ONE thing with a clear name.

// prepareReActRound handles context management before each ReAct iteration:
// 1. Compress context if exceeds 90% of token budget
// 2. Snips old tool outputs to avoid context bloat
// 3. Injects budget reminder when approaching round limit
func (k *AgentKernel) prepareReActRound(ctx context.Context, messages []Message, round int, promptTokens int, opts *QueryOptions) []Message {
	// Compression: 90% threshold (Claude Code uses 92%)
	if k.compressor != nil && k.maxTokens > 0 {
		tokenCount := estimateMessagesTokens(messages)
		if promptTokens > tokenCount {
			tokenCount = promptTokens
		}
		if tokenCount > k.maxTokens*9/10 {
			compressed, saved, err := k.compressor.Compress(ctx, messages, k.maxTokens)
			if err == nil {
				messages = compressed
				messages = append(messages, Message{
					Role: "system", Content: "[System] Context was compressed to stay within token budget. Earlier details have been summarized. Focus on the current task and most recent messages.",
				})
				slog.Debug("Context compressed", "saved_tokens", saved)
			}
		}
	}

	// 动态裁剪旧工具输出:根据上下文压力调整裁剪强度
	// 70% 以下:宽松(keepFull=4, head/tail=500)
	// 70-85%:中等(keepFull=2, head/tail=300)
	// 85%+:激进(keepFull=1, head/tail=200)
	snipOldToolOutputsDynamic(messages, k.estimateContextPressure(promptTokens))

	// Budget injection — show the LLM its own tool usage pattern so it can self-regulate
	if round >= 10 {
		toolCounts := countToolCalls(messages)
		hint := buildBudgetHint(round, toolCounts)
		messages = append(messages, Message{Role: "user", Content: hint})
		// Still allow the exhaustion callback for external control
		if round >= 50 && opts != nil && opts.OnBudgetExhausted != nil {
			if opts.OnBudgetExhausted(round, 50) {
				return messages
			}
		}
	}
	return messages
}

// countToolCalls extracts tool usage stats from message history.
func countToolCalls(messages []Message) map[string]int {
	counts := map[string]int{}
	for _, msg := range messages {
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				counts[tc.Function.Name]++
			}
		}
	}
	return counts
}

// buildBudgetHint creates a context-aware hint based on the LLM's actual tool usage.
func buildBudgetHint(round int, toolCounts map[string]int) string {
	// Summarize what tools were used
	totalCalls := 0
	for _, n := range toolCounts {
		totalCalls += n
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[System] Round %d. You've made %d tool calls so far.", round, totalCalls))

	// Show top tools used
	if len(toolCounts) > 0 {
		type kv struct {
			k string
			v int
		}
		var sorted []kv
		for k, v := range toolCounts {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
		sb.WriteString(" Tools used: ")
		for i, t := range sorted {
			if i >= 5 {
				break
			}
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s×%d", t.k, t.v))
		}
		sb.WriteString(".")
	}

	// Escalating urgency
	if round >= 50 {
		sb.WriteString(" You MUST give your final answer now. Do NOT call any more tools.")
	} else if round >= 20 {
		sb.WriteString(" Stop exploring. If you have enough information, give your final answer. Only call tools if absolutely necessary.")
	} else {
		sb.WriteString(" Begin wrapping up. Focus on key findings. Avoid further exploration unless essential.")
	}
	return sb.String()
}

// toolExecTask is a prepared tool call ready for execution.
type toolExecTask struct {
	ToolCall
	skip   bool
	reason string
}

// toolExecResult is the result of executing a single tool call.
type toolExecResult struct {
	ID      string
	Name    string
	Content string
	Error   string
}

// executeToolBatch prepares, partitions, and executes tool calls concurrently.
// Returns results for appending to messages and the count of tool errors.
// Shared by both sync (Process) and stream (ProcessStream) paths.
func (k *AgentKernel) executeToolBatch(ctx context.Context, toolCalls []ToolCall, sessionID string, round int, opts *QueryOptions) (results []toolExecResult, toolErrors int) {
	var toolErrCount atomic.Int32
	// 1. Validate and prepare tasks
	tasks := make([]toolExecTask, len(toolCalls))
	for i, tc := range toolCalls {
		tasks[i] = toolExecTask{ToolCall: tc}
		if tc.Function.Name == "" {
			tasks[i].skip = true
			tasks[i].reason = "tool name empty, skipped"
		} else if tc.ID == "" {
			tc.ID = fmt.Sprintf("call_auto_%d_%d", round, i)
			tasks[i] = toolExecTask{ToolCall: tc}
		}
	}

	results = make([]toolExecResult, len(tasks))

	// 2. Partition by concurrency safety: parallel-safe tools run together, others sequential
	type batch struct{ indices []int }
	var batches []batch
	current := batch{}
	for i, task := range tasks {
		if task.skip {
			results[i] = toolExecResult{ID: task.ID, Content: task.reason, Error: task.reason}
			continue
		}
		if !isParallelSafe(task.Function.Name) {
			if len(current.indices) > 0 {
				batches = append(batches, current)
				current = batch{}
			}
			batches = append(batches, batch{indices: []int{i}})
			continue
		}
		current.indices = append(current.indices, i)
	}
	if len(current.indices) > 0 {
		batches = append(batches, current)
	}

	// 3. Execute batches (parallel within batch, sequential between batches)
	for _, b := range batches {
		var wg sync.WaitGroup
		for _, i := range b.indices {
			task := tasks[i]
			k.publishEvent(Event{Type: EventToolCallStarted, Source: "kernel", Data: map[string]interface{}{"tool": task.Function.Name, "session_id": sessionID}, Timestamp: time.Now()})
			wg.Add(1)
			go func(idx int, call ToolCall) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						slog.Error("Tool goroutine panicked", "tool", call.Function.Name, "panic", r)
						results[idx] = toolExecResult{ID: call.ID, Name: call.Function.Name, Content: fmt.Sprintf("Error: panic: %v", r), Error: fmt.Sprintf("panic: %v", r)}
					}
				}()

				// 自动重试:IsRetryable=true 的工具临时失败时重试,
				// 避免可恢复的临时错误(文件锁冲突、网络抖动)浪费一轮 ReAct 往返。
				const maxRetries = 2
				var r *ToolResult
				for attempt := 0; attempt <= maxRetries; attempt++ {
					r = k.executeTool(ctx, call, sessionID, opts)
					if r.Error == "" || !r.IsRetryable || attempt == maxRetries {
						break
					}
					slog.Debug("Tool retryable error, retrying",
						"tool", call.Function.Name, "attempt", attempt+1,
						"error", r.Error, "error_code", r.ErrorCode)
					time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
				}

				content := fmt.Sprintf("%v", r.Content)
				errStr := ""
				if r.Error != "" {
					errStr = r.Error
					content = fmt.Sprintf("Error: %s", r.Error)
					toolErrCount.Add(1)
				}
				results[idx] = toolExecResult{ID: call.ID, Name: call.Function.Name, Content: content, Error: errStr}
				k.publishEvent(Event{Type: EventToolCallEnded, Source: "kernel", Data: map[string]interface{}{"tool": call.Function.Name, "success": r.Error == "", "session_id": sessionID}, Timestamp: time.Now()})
			}(i, task.ToolCall)
		}
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-ctx.Done():
			wg.Wait() // drain in-flight goroutines before returning results
			toolErrors = int(toolErrCount.Load())
			return results, toolErrors
		}
	}
	toolErrors = int(toolErrCount.Load())
	return results, toolErrors
}

// getToolDefinitions returns the tool set for this query,
// optionally filtered by skill or explicit tool filter
func (k *AgentKernel) getToolDefinitions(ctx context.Context, queryContent string, opts QueryOptions) []ToolDefinition {
	tools := k.toolExecutor.GetDefinitions()
	if len(opts.ToolFilter) > 0 {
		return k.toolExecutor.GetDefinitionsByNames(opts.ToolFilter)
	}
	if k.skillActor != nil {
		if skillTools := k.skillActor.GetTools(ctx, queryContent); len(skillTools) > 0 {
			return k.toolExecutor.GetDefinitionsByNames(skillTools)
		}
	}
	return tools
}

// finalizeResponse performs all post-ReAct work shared by sync and stream paths:
// save memory, update session, generate title, run reflection.
func (k *AgentKernel) finalizeResponse(ctx context.Context, session *Session, query *Query, response string, toolCalls, toolErrors int, analysis *QueryAnalysis) {
	RunSaga([]SagaStep{
		{
			Name: "save-memory",
			Execute: func() error {
				k.saveToMemory(ctx, session.ID, session.Messages)
				return nil
			},
			Compensate: func() error { return nil },
		},
		{
			Name: "update-session",
			Execute: func() error {
				session.UpdatedAt = time.Now()
				ensureSessionTitle(session)
				return k.sessionStore.Update(ctx, session)
			},
			Compensate: nil,
		},
	})

	go k.generateSessionTitle(session.ID, query.Content)
	if k.reflection != nil {
		go k.doReflection(ctx, session.ID, query.Content, response, toolCalls, toolErrors, analysis)
	}
	go k.compressMemory(ctx, session.ID)
}

// buildFinalMessage constructs the assistant message with optional reasoning and tool calls.
func buildFinalMessage(content, reasoning string, toolCalls []ToolCall) Message {
	return Message{
		Role:             "assistant",
		Content:          content,
		ReasoningContent: reasoning,
		ToolCalls:        toolCalls,
	}
}

// buildSynthesisPrompt constructs the max-rounds-exceeded synthesis message.
func buildSynthesisPrompt(messages []Message) []Message {
	return append(messages, Message{
		Role: "user",
		Content: "Max rounds reached. Based on all findings above, provide a complete summary. " +
			"Do NOT call tools — output your final answer directly.",
	})
}

// partitionToolCalls splits tool calls into consecutive safe/unsafe batches.
// Safe tools within a contiguous block run concurrently; unsafe tools run alone.
func partitionToolCalls(toolCalls []ToolCall) [][]ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	var batches [][]ToolCall
	var current []ToolCall
	for _, tc := range toolCalls {
		if isParallelSafe(tc.Function.Name) {
			current = append(current, tc)
		} else {
			if len(current) > 0 {
				batches = append(batches, current)
			}
			batches = append(batches, []ToolCall{tc})
			current = nil
		}
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// determineMaxRounds calculates the ReAct loop limit (adaptive or config-based).
// Uses cached result from unified query analysis when available.
func (k *AgentKernel) determineMaxRounds(ctx context.Context, queryContent string, historyLen int, analysis *QueryAnalysis) int {
	if analysis != nil && analysis.Complexity > 0 {
		return analysis.Complexity
	}
	if k.adaptiveRounds != nil {
		return k.adaptiveRounds.Calculate(ctx, queryContent, historyLen)
	}
	return k.maxRounds
}

// compressMemory compresses old working memory items into short-term summaries.
// This prevents unlimited growth of per-message memory files.
func (k *AgentKernel) compressMemory(ctx context.Context, sessionID string) {
	if k.memory == nil {
		return
	}
	// Type-assert to access the Compress method (not in the interface)
	type memoryCompressor interface {
		Compress(ctx context.Context, sessionID string) error
	}
	if mc, ok := k.memory.(memoryCompressor); ok {
		if err := mc.Compress(ctx, sessionID); err != nil {
			slog.Debug("memory compression skipped", "session", sessionID[:8], "error", err)
		}
	}
}
