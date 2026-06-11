package kernel

import (
	"context"
	"fmt"
	"log/slog"
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
	k.cachedAnalysis = k.analyzeQuery(ctx, query.Content)
	if k.cachedAnalysis != nil && k.cachedAnalysis.SkillID != "" && k.skillActor != nil {
		k.skillActor.UsePreMatch(k.cachedAnalysis.SkillID)
	}
	defer func() { k.cachedAnalysis = nil }()

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
		if k.cachedAnalysis != nil && k.cachedAnalysis.Complexity > 0 {
			maxRounds = k.cachedAnalysis.Complexity
		} else if k.adaptiveRounds != nil {
			maxRounds = k.adaptiveRounds.Calculate(ctx, query.Content, len(session.Messages))
		}
		totalTokens := 0
	promptTokens := 0
		totalToolCalls := 0
		toolErrors := 0
		startTime := time.Now()

		slog.Info("ReAct stream: entering loop", "query", query.Content[:min(80, len(query.Content))], "max_rounds", maxRounds, "tools", len(tools), "history_msgs", len(messages))
		for round := 0; ; round++ {
			slog.Debug("ReAct stream round", "round", round, "msg_count", len(messages))
			snipOldToolOutputs(messages)
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
					go k.doReflection(context.WithoutCancel(ctx), session.ID, query.Content, fullContent.String(), totalToolCalls, toolErrors)
				}

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
				case resultChan <- StreamChunk{Type: ChunkTypeToolCall, ToolCallID: tc.ID, ToolName: tc.Function.Name}:
				case <-ctx.Done(): return
				}
			}

			// Execute tools (shared with sync path)
			execResults, batchErrors := k.executeToolBatch(ctx, lastToolCalls, session.ID, round, &query.Options)
			toolErrors += batchErrors
			totalToolCalls += len(execResults)

			// Send tool_done events + append results
			for _, r := range execResults {
				if r.ID == "" {
					r.ID = fmt.Sprintf("result_auto_%d", totalToolCalls)
				}
				select {
				case resultChan <- StreamChunk{Type: ChunkTypeToolDone, ToolCallID: r.ID, ToolName: r.Name}:
				case <-ctx.Done(): return
				}
				messages = append(messages, Message{
					Role:       "tool",
					Content:    r.Content,
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
			go k.doReflection(context.WithoutCancel(ctx), session.ID, query.Content, resp.Content, totalToolCalls, toolErrors)
		}

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
