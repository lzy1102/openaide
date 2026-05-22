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

	messages := k.buildMessages(session, query)

	tools := k.toolExecutor.GetDefinitions()
	if len(query.Options.ToolFilter) > 0 {
		tools = k.toolExecutor.GetDefinitionsByNames(query.Options.ToolFilter)
	} else if k.skillManager != nil {
		if skillTools := k.skillManager.GetTools(query.Content); len(skillTools) > 0 {
			tools = k.toolExecutor.GetDefinitionsByNames(skillTools)
		}
	}

	resultChan := make(chan StreamChunk, 100)

	go func() {
		defer close(resultChan)

		var traceCtx context.Context
		if k.tracer != nil {
			traceCtx = k.tracer.StartSpan(ctx, session.ID, TraceSession, "process_stream_loop")
			defer k.tracer.EndSpan(traceCtx, nil, nil)
		}

		maxRounds := k.maxRounds
		if k.adaptiveRounds != nil {
			maxRounds = k.adaptiveRounds.Calculate(query.Content, len(session.Messages))
		}
		totalTokens := 0
		totalToolCalls := 0

		for round := 0; round < maxRounds; round++ {
			// 检查上下文长度，必要时压缩
			if k.compressor != nil {
				tokenCount := k.compressor.EstimateTokens(messages)
				if tokenCount > k.maxTokens {
					compressed, saved, err := k.compressor.Compress(messages, k.maxTokens)
					if err == nil {
						messages = compressed
						slog.Debug("Context compressed", "saved_tokens", saved)
					}
				}
			}

			// 发送 thinking 事件
			k.setState(StateThinking)
			resultChan <- StreamChunk{
				Type:   ChunkTypeThinking,
				Round:  round,
				TotalRounds: maxRounds,
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
				resultChan <- StreamChunk{Type: ChunkTypeError, Error: err, Done: true}
				return
			}

			k.setState(StateResponding)
			var fullContent, reasoningContent strings.Builder
			var lastToolCalls []ToolCall
			var lastUsage *TokenUsage

			for chunk := range llmStream {
				if chunk.Error != nil {
					resultChan <- StreamChunk{Type: ChunkTypeError, Error: chunk.Error, Done: true}
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

			// 无工具调用 -> 返回结果
			if len(lastToolCalls) == 0 {
				k.saveToMemory(ctx, session.ID, messages)
				session.Messages = messages
				session.UpdatedAt = time.Now()
				ensureSessionTitle(session)
				k.sessionStore.Update(ctx, session)

				if k.reflection != nil {
					go k.doReflection(ctx, session.ID, query.Content, fullContent.String(), totalToolCalls)
				}

				k.setState(StateIdle)
				k.publishEvent(Event{
					Type:      EventResponseEnded,
					Source:    "kernel",
					Data:      map[string]interface{}{"session_id": session.ID, "tokens": totalTokens},
					Timestamp: time.Now(),
				})

				resultChan <- StreamChunk{
					Type:  ChunkTypeDone,
					Done:  true,
					Usage: lastUsage,
				}
				return
			}

			// === 工具调用轮次 ===
			k.setState(StateToolCalling)

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
			var wg sync.WaitGroup

			for i, task := range tasks {
				if task.skip {
					results[i] = toolResult{
						id:   task.ID,
						name: "",
						msg: Message{
							Role:       "tool",
							Content:    task.reason,
							ToolCallID: task.ID,
						},
					}
					continue
				}

				k.publishEvent(Event{
					Type:      EventToolCallStarted,
					Source:    "kernel",
					Data:      map[string]interface{}{"tool": task.Function.Name, "session_id": session.ID},
					Timestamp: time.Now(),
				})

				wg.Add(1)
				go func(idx int, call ToolCall) {
					defer wg.Done()
					var toolCtx context.Context
					if k.tracer != nil {
						toolCtx = k.tracer.StartSpan(ctx, session.ID, TraceTool, call.Function.Name)
					}
					r := k.executeTool(ctx, call, session.ID)
					if k.tracer != nil {
						var toolErr error
						if r.Error != "" {
							toolErr = fmt.Errorf("tool error: %s", r.Error)
						}
						k.tracer.EndSpan(toolCtx, map[string]interface{}{
							"tool":    call.Function.Name,
							"content": r.Content,
						}, toolErr)
					}
					content := fmt.Sprintf("%v", r.Content)
					if r.Error != "" {
						content = fmt.Sprintf("Error: %s", r.Error)
					}
					results[idx] = toolResult{
						id:   call.ID,
						name: call.Function.Name,
						msg: Message{
							Role:       "tool",
							Content:    content,
							ToolCallID: call.ID,
						},
					}
					k.publishEvent(Event{
						Type:      EventToolCallEnded,
						Source:    "kernel",
						Data:      map[string]interface{}{"tool": call.Function.Name, "success": r.Error == "", "session_id": session.ID},
						Timestamp: time.Now(),
					})
				}(i, task.ToolCall)
			}
			wg.Wait()
			totalToolCalls += len(results)

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
		}

		// 超出最大轮次
		k.setState(StateIdle)
		lastMsg := messages[len(messages)-1]
		resultChan <- StreamChunk{
			Type:  ChunkTypeDone,
			Done:  true,
			Usage: &TokenUsage{TotalTokens: totalTokens},
			Content: lastMsg.Content,
		}
	}()

	return resultChan, nil
}
