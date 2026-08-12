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

	messages := k.buildMessages(ctx, session, query, analysis)

	// 复杂任务分解:complexity >= 阈值时,生成子任务计划注入消息
	// 引导 agent 按步骤执行 + 每步自我验证。
	var activePlan *TaskPlan
	planDone := 0
	if shouldPlan(analysis) && k.planner != nil {
		if plan := k.planner.Plan(ctx, query.Content); plan != nil {
			if query.Options.OnPlanApproved != nil {
				if !query.Options.OnPlanApproved(plan) {
					slog.Info("Plan rejected by user, continuing without plan")
					plan = nil
				}
			}
			if plan != nil {
				if planMsg := plan.ToSystemMessage(); planMsg.Content != "" {
					messages = append(messages, planMsg)
					activePlan = plan
				}
				// 计划批准后并行研究子任务,把隔离上下文的研究结论注入主循环
				if query.Options.ParallelResearch {
					if researchMsg := k.researchSubagentPrompt(ctx, plan); researchMsg != "" {
						messages = append(messages, Message{Role: "system", Content: researchMsg})
					}
				}
			}
		}
	}

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
		stuckDetector := NewStuckDetector()
		reflectRetries := 0

		slog.Info("ReAct stream: entering loop", "query", query.Content[:min(80, len(query.Content))], "max_rounds", maxRounds, "tools", len(tools), "history_msgs", len(messages))
		for round := 0; ; round++ {
			slog.Debug("ReAct stream round", "round", round, "msg_count", len(messages))
			// 绝对安全网：防 runaway 循环（90% 压缩/预算提示已由 prepareReActRound 完成）
			if round >= maxRounds*2 {
				slog.Error("ReAct stream safety limit reached", "round", round)
				break
			}
			messages = k.prepareReActRound(ctx, messages, round, promptTokens, &query.Options)

			// 方向检查:每 directionCheckInterval 轮检测是否偏离原始需求。
			// 检测到偏离时注入重聚焦提示,防止长任务跑偏。
			if round > 0 && round%directionCheckInterval == 0 {
				if pivot := k.checkDirection(ctx, query.Content, messages); pivot != "" {
					messages = append(messages, Message{Role: "system", Content: pivot})
					slog.Info("Direction pivot injected", "round", round)
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
			// 停滞 watchdog:无 token 输出超时则中断本轮并注入恢复提示
			roundCtx, roundCancel := context.WithCancel(ctx)
			llmStream, err := k.llmProvider.ChatStream(roundCtx, messages, tools, k.buildOptions(query.Options))
			if err != nil {
				roundCancel()
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

			stallTimeout := k.stallTimeoutValue()
			stalled := false

			streamCh := llmStream
		receiveLoop:
			for streamCh != nil {
				select {
				case chunk, ok := <-streamCh:
					if !ok {
						break receiveLoop
					}
					if chunk.Error != nil {
						roundCancel()
						select {
						case resultChan <- StreamChunk{Type: ChunkTypeError, Error: chunk.Error, Done: true}:
						case <-ctx.Done():
						}
						k.setState(StateError)
						return
					}

					if chunk.Done {
						break receiveLoop
					}

					// 累积内容
					if chunk.Content != "" {
						fullContent.WriteString(chunk.Content)
						select {
						case resultChan <- StreamChunk{Type: ChunkTypeContent, Content: chunk.Content}:
						case <-ctx.Done():
							roundCancel()
							return
						}
					}

					// 推理内容 -> thinking 事件
					if chunk.ReasoningContent != "" {
						reasoningContent.WriteString(chunk.ReasoningContent)
						select {
						case resultChan <- StreamChunk{Type: ChunkTypeThinking, ReasoningContent: chunk.ReasoningContent}:
						case <-ctx.Done():
							roundCancel()
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
				case <-time.After(stallTimeout):
					stalled = true
					roundCancel()
					break receiveLoop
				}
			}
			roundCancel()

			if stalled {
				messages = append(messages, Message{
					Role:    "user",
					Content: "[System Notice] The LLM stream produced no output for " + stallTimeout.String() + ". If you have a pending analysis, output it now; otherwise respond with a concise status update.",
				})
				slog.Warn("LLM stream stalled, injecting recovery notice", "round", round, "timeout", stallTimeout)
				continue
			}

			// 添加 assistant 消息
			messages = append(messages, buildFinalMessage(fullContent.String(), reasoningContent.String(), lastToolCalls))
			slog.Debug("ReAct stream LLM response", "round", round, "content_len", fullContent.Len(), "tool_calls", len(lastToolCalls), "reasoning_len", reasoningContent.Len(), "tokens", totalTokens)

			// 无工具调用 -> 返回结果
			if len(lastToolCalls) == 0 {
				// LLM 无内容返回(限流/静默失败):不要保存空 assistant 或标记成功。
				// 发错误 chunk 让调用方感知,避免界面显示"完成"但无回复。
				if fullContent.Len() == 0 && reasoningContent.Len() == 0 && toolErrors == 0 {
					roundCancel()
					err := fmt.Errorf("LLM returned no response (rate limit or empty reply)")
					k.setState(StateError)
					slog.Warn("LLM empty response, treating as failure", "session", session.ID[:min(8, len(session.ID))], "round", round)
					select {
					case resultChan <- StreamChunk{Type: ChunkTypeError, Error: err, Done: true}:
					case <-ctx.Done():
					}
					return
				}
				// Reflexion 回流：有工具错误且反思器可用时，同步反思并把教训注入重试
				if toolErrors > 0 && k.reflection != nil && reflectRetries < maxReflectRetries {
					reflectRetries++
					record := ExecutionRecord{
						Query:     query.Content,
						Response:  fullContent.String(),
						Success:   false,
						Error:     fmt.Sprintf("%d tool errors", toolErrors),
						Duration:  int64(time.Since(startTime).Milliseconds()),
						Messages:  messages,
						TaskType:  queryTaskType(analysis),
						ToolCalls: lastToolCalls,
					}
					result, rErr := k.reflection.Reflect(ctx, session.ID, record)
					if rErr == nil && result != nil {
						if lesson := reflectionLessonMessage(result); lesson != "" {
							messages = append(messages, Message{Role: "system", Content: lesson})
							slog.Info("Reflection retry injected",
								"round", round, "retry", reflectRetries,
								"quality", result.Quality, "tool_errors", toolErrors)
							continue
						}
					}
				}

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
				if tc.Function.Name == "" {
					continue
				}
				select {
				case resultChan <- StreamChunk{Type: ChunkTypeToolCall, ToolCallID: tc.ID, ToolName: tc.Function.Name, ToolArgs: truncateToolArgs(tc.Function.Arguments)}:
				case <-ctx.Done():
					return
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
			// 同时记录到 StuckDetector 用于自我纠错检测
			argsByID := make(map[string]string, len(lastToolCalls))
			for _, tc := range lastToolCalls {
				argsByID[tc.ID] = tc.Function.Arguments
			}
			for _, r := range execResults {
				if r.ID == "" {
					r.ID = fmt.Sprintf("result_auto_%d", totalToolCalls)
				}
				select {
				case resultChan <- StreamChunk{Type: ChunkTypeToolDone, ToolCallID: r.ID, ToolName: r.Name}:
				case <-ctx.Done():
					return
				}
				// 工具结果入 messages 前截断:防止单条大输出(read_file 整个文件)
				// 把上下文吃爆。截断后 LLM 仍能看到头尾,中间可用 read_file offset/limit 补读。
				messages = append(messages, Message{
					Role:       "tool",
					Content:    truncateToolResult(r.Content),
					ToolCallID: r.ID,
				})
				// 记录到 StuckDetector:工具名 + 参数 + 错误(空=成功)
				stuckDetector.RecordResult(r.Name, argsByID[r.ID], r.Error, round)
			}

			// 自我纠错:检测到停滞时注入 pivot 消息,强制换策略
			if stuck, reason := stuckDetector.IsStuck(round); stuck {
				pivotMsg := stuckDetector.PivotMessage(reason)
				messages = append(messages, Message{Role: "system", Content: pivotMsg})
				slog.Info("Stuck detected, injecting pivot",
					"round", round, "reason", reason,
					"pivot_count", stuckDetector.PivotCount())
			} else if stuckDetector.PivotLimitReached() && k.checkpointer != nil {
				// pivot 上限已到:从检查点重定向,要求基于已保存进度重新规划。
				// 每轮只注入一次(round 唯一),避免重复。
				messages = append(messages, Message{Role: "system", Content: "[Recovery] You have hit the stuck-detection limit. Review your recent progress, restate what you have verified so far, and pick a DIFFERENT approach for the remaining goal. Do not repeat failed actions."})
				slog.Info("Stuck pivot limit reached, injecting recovery redirect", "round", round)
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

			// 计划进度回写:每 3 轮注入一次,避免进度消息随轮数线性膨胀。
			// 有工具调用且无错误视为推进一个子步骤,提示下一步待办。
			if activePlan != nil && round%3 == 0 && len(execResults) > 0 &&
				batchErrors == 0 && planDone < len(activePlan.SubTasks) {
				planDone++
				if msg := activePlan.ProgressMessage(planDone, len(activePlan.SubTasks)); msg.Content != "" {
					messages = append(messages, msg)
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

// maxReflectRetries 限制单次查询最多反思重试次数。
// 每次重试会注入反思教训并重新进入 ReAct 循环，避免无限自我纠错。
const maxReflectRetries = 2

// reflectionLessonMessage 把反思结果格式化为注入消息。
// 只提取具体的行为规则(suggestions)和关键教训(learned)，跳过空结果。
func reflectionLessonMessage(result *ReflectionResult) string {
	var lessons []string
	for _, s := range result.Suggestions {
		if strings.TrimSpace(s) != "" {
			lessons = append(lessons, "- "+strings.TrimSpace(s))
		}
	}
	if strings.TrimSpace(result.Learned) != "" {
		lessons = append(lessons, "- "+strings.TrimSpace(result.Learned))
	}
	if len(lessons) == 0 {
		return ""
	}
	return "[Reflection from previous attempt]\n" +
		"Some tool calls failed. Apply these lessons on the retry:\n" +
		strings.Join(lessons, "\n")
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
