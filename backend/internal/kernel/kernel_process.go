package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Process 处理用户查询（同步）
func (k *AgentKernel) Process(ctx context.Context, query *Query) (*Response, error) {
	start := time.Now()

	if k.tracer != nil {
		ctx = k.tracer.StartSpan(ctx, query.SessionID, TraceSession, "process")
		defer k.tracer.EndSpan(ctx, nil, nil)
	}

	// 1. 发布查询接收事件
	k.publishEvent(Event{
		Type:      EventQueryReceived,
		Source:    "kernel",
		Data:      map[string]interface{}{"session_id": query.SessionID, "content": query.Content},
		Timestamp: time.Now(),
	})

	if k.tracer != nil {
		k.tracer.Record(ctx, &TraceEvent{
			Type:      TraceSession,
			Name:      "query_received",
			SessionID: query.SessionID,
			Input:     map[string]interface{}{"content": query.Content, "options": query.Options},
			Status:    TraceStatusOK,
		})
	}

	// 2. 获取或创建会话
	session, err := k.getOrCreateSession(ctx, query)
	if err != nil {
		if k.tracer != nil {
			k.tracer.EndSpan(ctx, nil, err)
		}
		return nil, fmt.Errorf("session error: %w", err)
	}

	// 3. 构建消息列表
	messages := k.buildMessages(ctx, session, query)
	// 4. Get tool definitions (skill-filtered if applicable)
	tools := k.getToolDefinitions(query.Content, query.Options)

	// Store query options for callback access during ReAct loop
	k.queryOptions = &query.Options

	// 5. ReAct 循环
	k.setState(StateThinking)
	totalToolCalls := 0
	toolErrors := 0
	totalTokens := 0

	maxRounds := k.determineMaxRounds(query.Content, len(session.Messages))
	slog.Debug("ReAct loop start", "query", query.Content[:min(80, len(query.Content))], "max_rounds", maxRounds, "tools", len(tools), "history_msgs", len(messages))
	for round := 0; round < maxRounds; round++ {
		// Prepare context: compress, snip old output, inject budget hints
		messages = k.prepareReActRound(messages, round, maxRounds)
		slog.Debug("ReAct round start", "round", round, "state", k.state, "msg_count", len(messages))
		// 调用 LLM（如果指定了模型，临时切换）
		if query.Options.ModelID != "" {
			k.llmProvider.SetModelID(query.Options.ModelID)
		}
		var llmCtx context.Context
		if k.tracer != nil {
			llmCtx = k.tracer.StartSpan(ctx, session.ID, TraceLLM, fmt.Sprintf("chat_round_%d", round))
		}
		llmResp, err := k.llmProvider.Chat(ctx, messages, tools, k.buildOptions(query.Options))
		if k.tracer != nil {
			output := map[string]interface{}{"model": llmResp.Model, "usage": llmResp.Usage}
			k.tracer.EndSpan(llmCtx, output, err)
		}
		if err != nil {
			k.setState(StateError)
			return nil, fmt.Errorf("llm error: %w", err)
		}

		if llmResp.Usage != nil {
			totalTokens += llmResp.Usage.TotalTokens
		}
		slog.Debug("ReAct LLM response", "round", round, "model", llmResp.Model, "content_len", len(llmResp.Content), "tool_calls", len(llmResp.ToolCalls), "reasoning_len", len(llmResp.ReasoningContent), "tokens", llmResp.Usage)

		// 添加 assistant 消息（包含 reasoning_content）
		messages = append(messages, Message{
			Role:             "assistant",
			Content:          llmResp.Content,
			ReasoningContent: llmResp.ReasoningContent,
			ToolCalls:        llmResp.ToolCalls,
		})

		// 无工具调用 -> 返回结果
		if len(llmResp.ToolCalls) == 0 {
			k.setState(StateResponding)

			// 保存到记忆
			k.saveToMemory(ctx, session.ID, messages)

			// 更新会话
			session.Messages = messages
			session.UpdatedAt = time.Now()
			ensureSessionTitle(session)
			if err := k.sessionStore.Update(ctx, session); err != nil {
		slog.Warn("session update failed", "error", err)
	}
			// Copy metadata before async goroutine
	titleCopy := make(map[string]interface{})
	for k, v := range session.Metadata {
		titleCopy[k] = v
	}
	session.Metadata = titleCopy
	go k.generateSessionTitle(session, query.Content)

			// 触发反思（如果启用）
			if k.reflection != nil {
				go k.doReflection(ctx, session.ID, query.Content, llmResp.Content, totalToolCalls, toolErrors)
			}

			k.setState(StateIdle)
			k.publishEvent(Event{
				Type:      EventResponseEnded,
				Source:    "kernel",
				Data:      map[string]interface{}{"session_id": session.ID, "tokens": totalTokens},
				Timestamp: time.Now(),
			})

			cacheHit, cacheMiss := 0, 0
			if llmResp.Usage != nil {
				cacheHit = llmResp.Usage.PromptCacheHitTokens
				cacheMiss = llmResp.Usage.PromptCacheMissTokens
			}
			return &Response{
				Content:    llmResp.Content,
				ToolCalls:  totalToolCalls,
				TokensUsed: totalTokens,
				CacheHit:   cacheHit,
				CacheMiss:  cacheMiss,
				Duration:   time.Since(start),
				Model:      llmResp.Model,
			}, nil
		}

		// 并行执行工具调用
		k.setState(StateToolCalling)
		type toolResult struct {
			id      string
			name    string
			content string
			err     string
		}

		type toolCallTask struct {
			ToolCall
			skip   bool
			reason string
		}
		tasks := make([]toolCallTask, len(llmResp.ToolCalls))
		for i, tc := range llmResp.ToolCalls {
			tasks[i] = toolCallTask{ToolCall: tc}
			if tc.Function.Name == "" {
				tasks[i].skip = true
				tasks[i].reason = "工具名称为空，已跳过"
			} else if tc.ID == "" {
				tc.ID = fmt.Sprintf("call_auto_%d_%d", round, i)
				tasks[i] = toolCallTask{ToolCall: tc}
			}
		}

		results := make([]toolResult, len(tasks))

		// Partition by concurrency safety
		type batch struct{ indices []int }
		var batches []batch
		current := batch{}
		for i, task := range tasks {
			if task.skip {
				results[i] = toolResult{id: task.ID, content: task.reason, err: task.reason}
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
					errStr := ""
					if r.Error != "" { errStr = r.Error; content = fmt.Sprintf("Error: %s", r.Error); toolErrors++ }
					results[idx] = toolResult{id: call.ID, name: call.Function.Name, content: content, err: errStr}
					k.publishEvent(Event{Type: EventToolCallEnded, Source: "kernel", Data: map[string]interface{}{"tool": call.Function.Name, "success": r.Error == "", "session_id": session.ID}, Timestamp: time.Now()})
				}(i, task.ToolCall)
			}
			// Check for cancellation while waiting for tool execution
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		return &Response{Content: "", Error: ctx.Err().Error()}, nil
	}
		}
		totalToolCalls += len(results)

		// 按原始顺序添加 tool 结果
		for _, r := range results {
			if r.id == "" {
				r.id = fmt.Sprintf("result_auto_%d", totalToolCalls)
			}
			messages = append(messages, Message{
				Role:       "tool",
				Content:    r.content,
				ToolCallID: r.id,
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
			if k.tracer != nil {
				k.tracer.Record(ctx, &TraceEvent{
					Type: TraceCheckpoint, Name: "checkpoint_save", SessionID: session.ID,
					Input: map[string]interface{}{"round": round + 1, "messages": len(messages)},
					Status: TraceStatusOK,
				})
			}
		}
	}

	// 超出最大轮次 → 合成最终回答（smolagents 风格）
	k.setState(StateIdle)
	slog.Debug("ReAct max rounds reached, synthesizing final answer", "rounds", maxRounds, "msgs", len(messages))
	messages = append(messages, Message{
		Role: "user",
		Content: "Max rounds reached. Based on all findings above, provide a complete summary. Do NOT call tools — output your final answer directly.",
	})
	resp, err := k.llmProvider.Chat(ctx, messages, nil, map[string]interface{}{"temperature": 0.3, "max_tokens": 4000, "route": "execution"})
	if err != nil {
		slog.Warn("Final synthesis failed", "error", err)
		lastMsg := messages[len(messages)-1]
		return &Response{Content: lastMsg.Content, ToolCalls: totalToolCalls, TokensUsed: totalTokens, Duration: time.Since(start), Model: k.llmProvider.GetModelID()}, nil
	}
	if resp.Usage != nil {
		totalTokens += resp.Usage.TotalTokens
	}
	return &Response{
		Content:    resp.Content,
		ToolCalls:  totalToolCalls,
		TokensUsed: totalTokens,
		CacheHit:   resp.Usage.PromptCacheHitTokens,
		CacheMiss:  resp.Usage.PromptCacheMissTokens,
		Duration:   time.Since(start),
		Model:      resp.Model,
	}, nil
}

func (k *AgentKernel) doReflection(ctx context.Context, sessionID, query, response string, toolCalls, toolErrors int) {
	if k.reflection == nil {
		return
	}

	record := ExecutionRecord{
		Query:     query,
		Response:  response,
		Success:   toolErrors == 0,
		ToolCalls: make([]ToolCall, 0),
	}

	result, err := k.reflection.Reflect(ctx, sessionID, record)
	if err != nil {
		slog.Warn("Reflection failed", "error", err)
		return
	}

	// 存储反思结果到会话 + 知识使用反馈
	if result != nil && k.sessionStore != nil {
		session, err := k.sessionStore.Get(ctx, sessionID)
		if err == nil && session != nil {
			if session.Metadata == nil {
				session.Metadata = make(map[string]interface{})
			}
			session.Metadata["reflection"] = result
			if err := k.sessionStore.Update(ctx, session); err != nil {
		slog.Warn("session update failed", "error", err)
	}

			// 反馈：本次使用的知识质量如何
			if k.knowledgeCollector != nil {
				if docIDsRaw, ok := session.Metadata["knowledge_doc_ids"]; ok {
					if docIDs, ok := docIDsRaw.([]string); ok && len(docIDs) > 0 {
						k.knowledgeCollector.RecordKnowledgeUsage(ctx, docIDs, float64(result.Quality)/10.0)
					}
				}
			}
		}
	}

	// 持久化学习洞察
	if k.learner != nil {
		if err := k.learner.Learn(ctx, record); err != nil {
			slog.Warn("Learner failed", "error", err)
		}
	}

	// 检测对话模式
	if k.patternDetector != nil && k.sessionStore != nil {
		session, err := k.sessionStore.Get(ctx, sessionID)
		if err == nil && session != nil {
			patterns, pErr := k.patternDetector.Detect(ctx, sessionID, session.Messages)
			if pErr == nil && len(patterns) > 0 {
				if session.Metadata == nil {
					session.Metadata = make(map[string]interface{})
				}
				session.Metadata["patterns"] = patterns
				if err := k.sessionStore.Update(ctx, session); err != nil {
		slog.Warn("session update failed", "error", err)
	}

				// 技能自动进化：从检测到的模式中创建新技能
				if k.skillEvolution != nil {
					var insights []Insight
					if l, ok := k.learner.(*SimpleLearner); ok {
						insights = l.GetAllInsights()
					}
				go k.skillEvolution.Evolve(ctx, patterns, insights)
				}
			}
		}
	}

	// 自动知识抽取：质量门控通过后存入知识库
	k.autoSaveKnowledge(ctx, sessionID, query, response, toolCalls)
}

// autoSaveKnowledge 自动知识抽取 — 质量门控通过后存入知识库
func (k *AgentKernel) autoSaveKnowledge(ctx context.Context, sessionID, query, response string, toolCalls int) {
	if k.knowledgeCollector == nil {
		return
	}

	// 构建质量评估快照
	toolSuccesses := toolCalls // 简化：有工具调用就计为成功尝试
	toolFailures := 0

	var reflectResult *ReflectionResult
	if session, err := k.sessionStore.Get(ctx, sessionID); err == nil && session != nil {
		if ref, ok := session.Metadata["reflection"]; ok {
			if r, ok := ref.(*ReflectionResult); ok {
				reflectResult = r
				if r.Quality < 5 {
					toolFailures = 1 // 低质量标记
					toolSuccesses = 0
				}
			}
		}
	}

	// 质量门控判断
	if k.qualityGate != nil {
		if !k.qualityGate.Pass(query, response, toolSuccesses, toolFailures, reflectResult) {
			return
		}
	}

	// 存入知识库
	title := query
	if len(title) > 80 {
		title = title[:80] + "..."
	}
	tags := []string{"auto", "session:" + sessionID}
	if reflectResult != nil && reflectResult.Quality >= 7 {
		tags = append(tags, "high-quality")
	}

	if _, err := k.knowledgeCollector.AddKnowledge(ctx, title, response, "auto-extract", tags); err != nil {
		slog.Debug("Auto knowledge save failed", "error", err)
	}
}
