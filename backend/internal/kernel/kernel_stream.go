package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
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

	messages := k.buildMessages(ctx, session, query)

	tools := k.getToolDefinitions(query.Content, query.Options)

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

		k.queryOptions = &query.Options
	maxRounds := k.maxRounds
		if k.adaptiveRounds != nil {
			maxRounds = k.adaptiveRounds.Calculate(query.Content, len(session.Messages))
		}
		totalTokens := 0
		totalToolCalls := 0
		toolErrors := 0
		startTime := time.Now()

		slog.Debug("ReAct stream loop start", "query", query.Content[:min(80, len(query.Content))], "max_rounds", maxRounds, "tools", len(tools), "history_msgs", len(messages))
		for round := 0; round < maxRounds; round++ {
			slog.Debug("ReAct stream round", "round", round, "msg_count", len(messages))
			snipOldToolOutputs(messages)
		// 预算注入：过半后提醒 LLM 剩余轮次
		if round >= maxRounds/2 && round < maxRounds-1 {
			remaining := maxRounds - round
			messages = append(messages, Message{
				Role: "user", Content: fmt.Sprintf("[System] Used %d/%d rounds, %d remaining. Give your final answer now if you have enough information.", round, maxRounds, remaining),
			})
		} else if round >= maxRounds-1 {
			messages = append(messages, Message{
				Role: "user", Content: "[System] Final round — must give final answer. Do NOT call any tools.",
			})
		}
			// 检查上下文长度，必要时压缩
			if k.compressor != nil {
				tokenCount := k.compressor.EstimateTokens(messages)
				if tokenCount > k.maxTokens*9/10 {
					compressed, saved, err := k.compressor.Compress(messages, k.maxTokens)
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
				k.llmProvider.SetModelID(query.Options.ModelID)
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
					totalTokens = chunk.Usage.TotalTokens
				}
			}

			// 添加 assistant 消息
			messages = append(messages, Message{
				Role:             "assistant",
				Content:          fullContent.String(),
				ReasoningContent: reasoningContent.String(),
				ToolCalls:        lastToolCalls,
			})
			slog.Debug("ReAct stream LLM response", "round", round, "content_len", fullContent.Len(), "tool_calls", len(lastToolCalls), "reasoning_len", reasoningContent.Len(), "tokens", totalTokens)

			// 无工具调用 -> 返回结果
			if len(lastToolCalls) == 0 {
				k.saveToMemory(ctx, session.ID, messages)
				session.Messages = messages
				session.UpdatedAt = time.Now()
				ensureSessionTitle(session)
				if err := k.sessionStore.Update(ctx, session); err != nil {
		slog.Warn("session update failed", "error", err)
	}
				// Copy metadata to avoid concurrent map access in goroutine
	titleMeta := make(map[string]interface{})
	for k, v := range session.Metadata {
		titleMeta[k] = v
	}
	session.Metadata = titleMeta
	go k.generateSessionTitle(session, query.Content)

				if k.reflection != nil {
					go k.doReflection(ctx, session.ID, query.Content, fullContent.String(), totalToolCalls, toolErrors)
				}

				slog.Debug("ReAct stream complete", "rounds", round+1, "tokens", totalTokens, "tools", totalToolCalls, "model", k.llmProvider.GetModelID(), "duration", time.Since(startTime))
				k.setState(StateIdle)
				k.publishEvent(Event{
					Type:      EventResponseEnded,
					Source:    "kernel",
					Data:      map[string]interface{}{"session_id": session.ID, "tokens": totalTokens},
					Timestamp: time.Now(),
				})

				select {
				case resultChan <- StreamChunk{Type: ChunkTypeDone, Done: true, Usage: lastUsage}:
				case <-ctx.Done():
				}
				return
			}

			// === 工具调用轮次 ===
			k.setState(StateToolCalling)
			slog.Debug("ReAct stream executing tools", "round", round, "tool_count", len(lastToolCalls))

			type streamToolTask struct {
				ToolCall
				skip   bool
				reason string
			}
			tasks := make([]streamToolTask, len(lastToolCalls))
			for i, tc := range lastToolCalls {
				tasks[i] = streamToolTask{ToolCall: tc}
				if tc.Function.Name == "" {
					tasks[i].skip = true
					tasks[i].reason = "工具名称为空，已跳过"
				} else if tc.ID == "" {
					tc.ID = fmt.Sprintf("call_auto_%d_%d", round, i)
					tasks[i] = streamToolTask{ToolCall: tc}
				}
			}

			// 发送 tool_call 事件（跳过无效的）
			for _, task := range tasks {
				if task.skip {
					continue
				}
				select {
				case resultChan <- StreamChunk{
					Type:       ChunkTypeToolCall,
					ToolCallID: task.ID,
					ToolName:   task.Function.Name,
				}:
				case <-ctx.Done():
					return
				}
			}

			// 并行执行工具
			type toolResult struct {
				id    string
				name  string
				msg   Message
			}
			results := make([]toolResult, len(tasks))

			// 按并发安全性分组：安全工具可并行，不安全工具单独成组串行
			type batch struct{ indices []int }
			var batches []batch
			current := batch{}
			for i, task := range tasks {
				if task.skip {
					results[i] = toolResult{id: task.ID, msg: Message{Role: "tool", Content: task.reason, ToolCallID: task.ID}}
					continue
				}
				if !isParallelSafe(task.Function.Name) {
					if len(current.indices) > 0 { batches = append(batches, current); current = batch{} }
					batches = append(batches, batch{indices: []int{i}})
					continue
				}
				current.indices = append(current.indices, i)
			}
			if len(current.indices) > 0 { batches = append(batches, current) }

			for _, b := range batches {
				var wg sync.WaitGroup
				for _, i := range b.indices {
					task := tasks[i]
					k.publishEvent(Event{Type: EventToolCallStarted, Source: "kernel", Data: map[string]interface{}{"tool": task.Function.Name, "session_id": session.ID}, Timestamp: time.Now()})
					wg.Add(1)
					go func(idx int, call ToolCall) {
						defer wg.Done()
						var toolCtx context.Context
						if k.tracer != nil { toolCtx = k.tracer.StartSpan(ctx, session.ID, TraceTool, call.Function.Name) }
						r := k.executeTool(ctx, call, session.ID)
						if k.tracer != nil {
							var toolErr error
							if r.Error != "" { toolErr = fmt.Errorf("tool error: %s", r.Error) }
							k.tracer.EndSpan(toolCtx, map[string]interface{}{"tool": call.Function.Name, "content": r.Content}, toolErr)
						}
						content := fmt.Sprintf("%v", r.Content)
						if r.Error != "" { content = fmt.Sprintf("Error: %s", r.Error); toolErrors++ }
						results[idx] = toolResult{id: call.ID, name: call.Function.Name, msg: Message{Role: "tool", Content: content, ToolCallID: call.ID}}
						k.publishEvent(Event{Type: EventToolCallEnded, Source: "kernel", Data: map[string]interface{}{"tool": call.Function.Name, "success": r.Error == "", "session_id": session.ID}, Timestamp: time.Now()})
					}(i, task.ToolCall)
				}
				wg.Wait()
			}
			totalToolCalls += len(results)
			for _, r := range results {
				if r.msg.Content != "" && strings.HasPrefix(r.msg.Content, "Error:") {
					slog.Warn("Stream tool failed", "tool", r.name, "error", r.msg.Content[:min(200, len(r.msg.Content))])
				} else {
					slog.Debug("Stream tool done", "tool", r.name, "output_len", len(r.msg.Content))
				}
			}
			slog.Debug("ReAct stream round done", "round", round, "tools_executed", len(results))

			// 发送 tool_done 事件 + 添加到消息列表
			for _, r := range results {
				select {
				case resultChan <- StreamChunk{
					Type:       ChunkTypeToolDone,
					ToolCallID: r.id,
					ToolName:   r.name,
					ToolResult: &ToolResult{Content: r.msg.Content},
				}:
				case <-ctx.Done():
					return
				}
				messages = append(messages, r.msg)
			}

			// 发送进度事件
			select {
			case resultChan <- StreamChunk{
				Type:        ChunkTypeProgress,
				Round:       round + 1,
				TotalRounds: maxRounds,
			}:
			case <-ctx.Done():
				return
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
			Role: "user",
			Content: "Max rounds reached. Based on all findings above, provide a complete summary. Do NOT call tools — output your final answer directly.",
		})
		resp, err := k.llmProvider.Chat(ctx, messages, nil, map[string]interface{}{"temperature": 0.3, "max_tokens": 4000, "route": "execution"})
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
		messages = append(messages, Message{
			Role: "assistant", Content: resp.Content,
		})
		k.saveToMemory(ctx, session.ID, messages)
		session.Messages = messages
		session.UpdatedAt = time.Now()
		ensureSessionTitle(session)
		if err := k.sessionStore.Update(ctx, session); err != nil {
		slog.Warn("session update failed", "error", err)
	}

		if k.reflection != nil {
			go k.doReflection(ctx, session.ID, query.Content, resp.Content, totalToolCalls, toolErrors)
		}

		k.setState(StateIdle)
		resultChan <- StreamChunk{
			Type:  ChunkTypeDone,
			Done:  true,
			Usage: &TokenUsage{TotalTokens: totalTokens},
			Content: resp.Content,
		}
	}()

	return resultChan, nil
}
