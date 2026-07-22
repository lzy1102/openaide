package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ProcessStream 处理用户查询（流式 ReAct 循环）
func (k *AgentKernel) ProcessStream(ctx context.Context, query *Query) (<-chan StreamChunk, error) {
	if k.tracer != nil {
		ctx = k.tracer.StartSpan(ctx, query.SessionID, TraceSession, "process_stream")
	}

	k.publishEvent(Event{
		Type:      EventQueryReceived,
		Source:    "kernel",
		Data:      map[string]interface{}{"session_id": query.SessionID, "content": query.Content},
		Timestamp: time.Now(),
	})

	session, err := k.getOrCreateSession(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("session error: %w", err)
	}

	// Unified query analysis — one LLM call replaces detectTaskType + DetectSkill + estimateComplexity
	analysis := k.analyzeQuery(ctx, query.Content)
	if k.skillActor != nil && analysis != nil {
		k.skillActor.UsePreMatch(analysis.SkillID) // "" = no match -> skip
	}

	messages := k.buildMessages(ctx, session, query)

	tools := k.getToolDefinitions(ctx, query.Content, query.Options)

	resultChan := make(chan StreamChunk, 100)

	go func() {
		defer close(resultChan)
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in stream goroutine", "panic", r)
				select {
				case resultChan <- StreamChunk{Type: ChunkTypeError, Error: fmt.Errorf("internal error: %v", r), Done: true}:
				default:
				}
			}
		}()

		var traceCtx context.Context
		if k.tracer != nil {
			traceCtx = k.tracer.StartSpan(ctx, session.ID, TraceSession, "process_stream_loop")
			defer k.tracer.EndSpan(traceCtx, nil, nil)
		}

		maxRounds := k.maxRounds
		if analysis != nil && analysis.Complexity > 0 {
			maxRounds = analysis.Complexity
		} else if k.adaptiveRounds != nil {
			maxRounds = k.adaptiveRounds.Calculate(ctx, query.Content, len(session.Messages))
		}
		totalTokens := 0
	promptTokens := 0
		totalToolCalls := 0
		toolErrors := 0
		startTime := time.Now()
		filesModified := make(map[string]bool)
		uniqueTools := make(map[string]bool)
		verifyAttempts := 0

		slog.Info("ReAct stream: entering loop", "query", query.Content[:min(80, len(query.Content))], "max_rounds", maxRounds, "tools", len(tools), "history_msgs", len(messages))
		for round := 0; ; round++ {
			slog.Debug("ReAct stream round", "round", round, "msg_count", len(messages))
			// Safety net + self-aware budget injection (shared with sync path)
		if round >= 200 {
			slog.Error("ReAct stream safety limit reached", "round", round)
			break
		}
		messages = k.prepareReActRound(ctx, messages, round, promptTokens, &query.Options)
			// 检查上下文长度，必要时压缩
			if k.compressor != nil {
				tokenCount := k.compressor.EstimateTokens(messages)
				if tokenCount > k.maxTokens*9/10 {
					compressed, saved, err := k.compressor.Compress(ctx, messages, k.maxTokens)
					if err == nil {
						messages = compressed
						slog.Debug("Context compressed", "saved_tokens", saved)
					}
				}
			}

			// 发送 thinking 事件
			k.setState(StateThinking)
			select {
			case resultChan <- StreamChunk{Type: ChunkTypeThinking, Round: round, TotalRounds: maxRounds}:
			default:
			}

			if query.Options.ModelID != "" {
				if ms, ok := k.llmProvider.(ModelSwitcher); ok {
					ms.SetModelID(query.Options.ModelID)
				}
			}
			llmStream, err := k.llmProvider.ChatStream(ctx, messages, tools, k.buildOptions(query.Options))
			if err != nil {
				if k.tracer != nil {
					k.tracer.Record(ctx, &TraceEvent{
						Type: TraceError, Name: "llm_stream", SessionID: session.ID,
						Error: err.Error(), Status: TraceStatusError,
					})
				}
				k.setState(StateError)
				select {
				case resultChan <- StreamChunk{Type: ChunkTypeError, Error: err, Done: true}:
				case <-ctx.Done():
				}
				return
			}

			k.setState(StateResponding)
			var fullContent, reasoningContent strings.Builder
			var lastToolCalls []ToolCall
			var lastUsage *TokenUsage

			for chunk := range llmStream {
				if chunk.Error != nil {
					select {
					case resultChan <- StreamChunk{Type: ChunkTypeError, Error: chunk.Error, Done: true}:
					case <-ctx.Done():
					}
					k.setState(StateError)
					return
				}

				if chunk.Done {
					break
				}

				// 累积内容
				if chunk.Content != "" {
					fullContent.WriteString(chunk.Content)
					select {
					case resultChan <- StreamChunk{Type: ChunkTypeContent, Content: chunk.Content}:
					case <-ctx.Done():
						return
					}
				}

				// 推理内容 -> thinking 事件
				if chunk.ReasoningContent != "" {
					reasoningContent.WriteString(chunk.ReasoningContent)
					select {
					case resultChan <- StreamChunk{Type: ChunkTypeThinking, ReasoningContent: chunk.ReasoningContent}:
					case <-ctx.Done():
						return
					}
				}

				// 工具调用（累积，取最后一个完整块）
				if len(chunk.ToolCalls) > 0 {
					lastToolCalls = chunk.ToolCalls
				}

				if chunk.Usage != nil {
					lastUsage = chunk.Usage
					totalTokens += chunk.Usage.TotalTokens
					promptTokens = chunk.Usage.PromptTokens
				}
			}

			// 添加 assistant 消息
			messages = append(messages, buildFinalMessage(fullContent.String(), reasoningContent.String(), lastToolCalls))
			slog.Debug("ReAct stream LLM response", "round", round, "content_len", fullContent.Len(), "tool_calls", len(lastToolCalls), "reasoning_len", reasoningContent.Len(), "tokens", totalTokens)

			// 无工具调用 -> 返回结果
			if len(lastToolCalls) == 0 {
				if len(filesModified) > 0 && verifyAttempts < 3 {
					verifyAttempts++
					verifyMsg := runAutoVerify(ctx, ".")
					if verifyMsg != "" {
						messages = append(messages, Message{Role: "user", Content: verifyMsg})
						lastToolCalls = []ToolCall{}
						continue
					}
				}

			session.Messages = messages
			k.RecordTaskMetrics(TaskMetrics{
				SessionID:        session.ID,
				StartedAt:        startTime,
				EndedAt:          time.Now(),
				Duration:         float64(time.Since(startTime).Milliseconds()),
				TaskType:         queryTaskType(analysis),
				Complexity:       queryComplexity(analysis),
				Rounds:           round + 1,
				PromptTokens:     promptTokens,
				CompletionTokens: totalTokens - promptTokens,
				TotalTokens:      totalTokens,
				Model:            k.llmProvider.GetModelID(),
				ToolCalls:        totalToolCalls,
				ToolErrors:       toolErrors,
				UniqueTools:      len(uniqueTools),
				Success:          toolErrors == 0,
			})
			k.finalizeResponse(context.WithoutCancel(ctx), session, query, fullContent.String(), totalToolCalls, toolErrors, analysis)

				slog.Debug("ReAct stream complete", "rounds", round+1, "tokens", totalTokens, "tools", totalToolCalls, "model", k.llmProvider.GetModelID(), "duration", time.Since(startTime))
				k.setState(StateIdle)
				k.publishEvent(Event{
					Type:      EventResponseEnded,
					Source:    "kernel",
					Data:      map[string]interface{}{"session_id": session.ID, "tokens": totalTokens},
					Timestamp: time.Now(),
				})

				usage := lastUsage
				if usage == nil {
					usage = &TokenUsage{TotalTokens: totalTokens}
				}
				select {
				case resultChan <- StreamChunk{Type: ChunkTypeDone, Done: true, Usage: usage}:
				case <-ctx.Done():
				}
				return
			}

			// === 工具调用轮次 ===
			k.setState(StateToolCalling)
			slog.Debug("ReAct stream executing tools", "round", round, "tool_count", len(lastToolCalls))

			// Send tool_call events for stream (unique to stream path)
			for _, tc := range lastToolCalls {
				if tc.Function.Name == "" { continue }
				select {
				case resultChan <- StreamChunk{Type: ChunkTypeToolCall, ToolCallID: tc.ID, ToolName: tc.Function.Name, ToolArgs: truncateToolArgs(tc.Function.Arguments)}:
				case <-ctx.Done(): return
				}
			}

			// Execute tools (shared with sync path)
			execResults, batchErrors := k.executeToolBatch(ctx, lastToolCalls, session.ID, round, &query.Options)
			toolErrors += batchErrors
			totalToolCalls += len(execResults)

			for _, tc := range lastToolCalls {
				if tc.Function.Name == "write_file" || tc.Function.Name == "diff_edit" {
					if path := extractFilePath(tc.Function.Arguments); path != "" {
						filesModified[path] = true
					}
				}
				uniqueTools[tc.Function.Name] = true
			}

			// Send tool_done events + append results
			for _, r := range execResults {
				if r.ID == "" {
					r.ID = fmt.Sprintf("result_auto_%d", totalToolCalls)
				}
				select {
				case resultChan <- StreamChunk{Type: ChunkTypeToolDone, ToolCallID: r.ID, ToolName: r.Name}:
				case <-ctx.Done(): return
				}
				// 工具结果入 messages 前截断:防止单条大输出(read_file 整个文件)
				// 把上下文吃爆。截断后 LLM 仍能看到头尾,中间可用 read_file offset/limit 补读。
				messages = append(messages, Message{
					Role:       "tool",
					Content:    truncateToolResult(r.Content),
					ToolCallID: r.ID,
				})
			}

			// 每轮结束后保存检查点
			if k.checkpointer != nil {
				cp := &Checkpoint{
					SessionID: session.ID,
					Round:     round + 1,
					Messages:  messages,
				}
				if err := k.checkpointer.Save(ctx, session.ID, cp); err != nil {
					slog.Warn("Failed to save checkpoint", "round", round, "error", err)
				}
			}
			slog.Debug("ReAct stream iteration end", "round", round)
		}

		// 超出最大轮次 → 合成最终回答（smolagents 风格）
		k.setState(StateIdle)
		slog.Debug("ReAct stream max rounds reached, synthesizing", "rounds", maxRounds, "msgs", len(messages), "tokens", totalTokens, "tools", totalToolCalls)
		messages = append(messages, Message{
			Role:    "user",
			Content: "Max rounds reached. Based on all findings above, provide a complete summary. Do NOT call tools — output your final answer directly.",
		})
		resp, err := k.llmProvider.Chat(ctx, messages, nil, map[string]interface{}{"temperature": 0.3, "max_tokens": 4000, "route": "reasoning"})
		if err != nil {
			slog.Warn("Stream final synthesis failed", "error", err)
			lastMsg := messages[len(messages)-1]
			select {
			case resultChan <- StreamChunk{Type: ChunkTypeDone, Done: true, Usage: &TokenUsage{TotalTokens: totalTokens}, Content: lastMsg.Content}:
			case <-ctx.Done():
			}
			return
		}
		if resp.Usage != nil {
			totalTokens += resp.Usage.TotalTokens
		}
		slog.Debug("ReAct stream synthesis complete", "tokens", totalTokens, "tools", totalToolCalls, "model", resp.Model, "duration", time.Since(startTime))
		// 追加合成结果到消息历史
		messages = append(messages, buildFinalMessage(resp.Content, "", nil))
		session.Messages = messages
		k.RecordTaskMetrics(TaskMetrics{
			SessionID:        session.ID,
			StartedAt:        startTime,
			EndedAt:          time.Now(),
			Duration:         float64(time.Since(startTime).Milliseconds()),
			TaskType:         queryTaskType(analysis),
			Complexity:       queryComplexity(analysis),
			Rounds:           maxRounds,
			PromptTokens:     promptTokens,
			CompletionTokens: totalTokens - promptTokens,
			TotalTokens:      totalTokens,
			Model:            k.llmProvider.GetModelID(),
			ToolCalls:        totalToolCalls,
			ToolErrors:       toolErrors,
			UniqueTools:      len(uniqueTools),
			Success:          toolErrors == 0,
		})
		k.finalizeResponse(context.WithoutCancel(ctx), session, query, resp.Content, totalToolCalls, toolErrors, analysis)

		k.setState(StateIdle)
		resultChan <- StreamChunk{
			Type:    ChunkTypeDone,
			Done:    true,
			Usage:   &TokenUsage{TotalTokens: totalTokens},
			Content: resp.Content,
		}
	}()

	return resultChan, nil
}

func detectTestCommand(dir string) string {
	checks := []struct {
		file string
		cmd  string
	}{
		{"go.mod", "go test ./..."},
		{"package.json", "npm test"},
		{"Makefile", "make test"},
		{"pyproject.toml", "pytest"},
		{"Cargo.toml", "cargo test"},
		{"pom.xml", "mvn test"},
		{"build.gradle", "gradle test"},
	}
	for _, c := range checks {
		if _, err := os.Stat(dir + "/" + c.file); err == nil {
			return c.cmd
		}
	}
	return ""
}

func runAutoVerify(ctx context.Context, dir string) string {
	if dir == "" {
		dir = "."
	}
	testCmd := detectTestCommand(dir)
	if testCmd == "" {
		return ""
	}
	cmdParts := strings.Fields(testCmd)
	if len(cmdParts) == 0 {
		return ""
	}
	slog.Debug("Auto-verifying", "cmd", testCmd)
	var c *exec.Cmd
	if len(cmdParts) == 1 {
		c = exec.CommandContext(ctx, cmdParts[0])
	} else {
		c = exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	}
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		output := string(out)
		if len(output) > 2000 {
			output = output[len(output)-2000:]
		}
		return fmt.Sprintf("[Auto-Verification] Command '%s' FAILED:\n%s\n\nFix the issue and try again.", testCmd, output)
	}
	return ""
}

func truncateToolArgs(args string) string {
	if len(args) <= 120 {
		return args
	}
	return args[:117] + "..."
}

func extractFilePath(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	return args.Path
}

func queryTaskType(a *QueryAnalysis) string {
	if a == nil {
		return "general"
	}
	return a.TaskType
}

func queryComplexity(a *QueryAnalysis) string {
	if a == nil {
		return "medium"
	}
	switch {
	case a.Complexity <= 3:
		return "low"
	case a.Complexity <= 8:
		return "medium"
	default:
		return "high"
	}
}
