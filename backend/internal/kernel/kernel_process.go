package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
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
	tools := k.getToolDefinitions(ctx, query.Content, query.Options)

	// Store query options for callback access during ReAct loop

	// 5. ReAct 循环
	k.setState(StateThinking)
	var totalToolCalls atomic.Int32
	toolErrors := 0
	totalTokens := 0
	promptTokens := 0 // API-returned count for accurate compression

	_ = k.determineMaxRounds(ctx, query.Content, len(session.Messages))
	slog.Debug("ReAct loop start", "query", query.Content[:min(80, len(query.Content))], "tools", len(tools), "history_msgs", len(messages))
	for round := 0; ; round++ {
		// Safety net: 200 rounds is far beyond any reasonable task.
		// Only triggers if the LLM is stuck in a pathological loop.
		if round >= 200 {
			slog.Error("ReAct safety limit reached — LLM may be stuck", "round", round)
			break
		}
		// Prepare context: compress, snip old output, inject budget hints
		messages = k.prepareReActRound(ctx, messages, round, promptTokens, &query.Options)
		slog.Debug("ReAct round — about to call LLM", "round", round+1, "msgs", len(messages))
		// 调用 LLM（如果指定了模型，临时切换）
		if query.Options.ModelID != "" {
			if ms, ok := k.llmProvider.(ModelSwitcher); ok { ms.SetModelID(query.Options.ModelID) }
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
			promptTokens = llmResp.Usage.PromptTokens
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
				go k.doReflection(context.WithoutCancel(ctx), session.ID, query.Content, llmResp.Content, int(totalToolCalls.Load()), toolErrors)
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
				ToolCalls:  int(totalToolCalls.Load()),
				TokensUsed: totalTokens,
				CacheHit:   cacheHit,
				CacheMiss:  cacheMiss,
				Duration:   time.Since(start),
				Model:      llmResp.Model,
			}, nil
		}

		// 并行执行工具调用
		k.setState(StateToolCalling)
		execResults, batchErrors := k.executeToolBatch(ctx, llmResp.ToolCalls, session.ID, round, &query.Options)
		toolErrors += batchErrors
		totalToolCalls.Add(int32(len(execResults)))

		// 按原始顺序添加 tool 结果
		for _, r := range execResults {
			if r.ID == "" {
				r.ID = fmt.Sprintf("result_auto_%d", totalToolCalls.Load())
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
			if k.tracer != nil {
				k.tracer.Record(ctx, &TraceEvent{
					Type: TraceCheckpoint, Name: "checkpoint_save", SessionID: session.ID,
					Input: map[string]interface{}{"round": round + 1, "messages": len(messages)},
					Status: TraceStatusOK,
				})
			}
		}
	}
	// Should never reach here — loop exits when LLM returns no tool calls.
	var lastContent string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			lastContent = messages[i].Content
			break
		}
	}
	return &Response{Content: lastContent, ToolCalls: int(totalToolCalls.Load()), TokensUsed: totalTokens, Duration: time.Since(start), Model: k.llmProvider.GetModelID()}, nil
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
		Messages:  k.loadSessionMessages(ctx, sessionID),
		TaskType:  k.detectTaskType(ctx, query),
	}

	result, err := k.reflection.Reflect(ctx, sessionID, record)
	if err != nil {
		slog.Warn("Reflection failed", "error", err)
		return
	}

	// Infer user verdict from reflection — LLM analyzed the full conversation flow
	if result.Learned != "" && k.sessionStore != nil {
		verdict, cleanLearned := extractVerdictFromLearned(result.Learned)
		if verdict != "" {
			result.Learned = cleanLearned
			if session, err := k.sessionStore.Get(ctx, sessionID); err == nil && session != nil {
				if session.Metadata == nil { session.Metadata = make(map[string]interface{}) }
				session.Metadata["user_verdict"] = verdict
				k.sessionStore.Update(ctx, session)
			}
		}
	}

	// Skill feedback: record quality for the activated skill
	if k.skillActor != nil {
		k.skillActor.RecordLastUsage(result.Quality)
	}

	// Self-Rewarding: update evaluation criteria from reflection suggestions

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
				// Store "Key Lesson" in knowledge base for future retrieval (Reflexion pattern)
				if result.Learned != "" {
					k.knowledgeCollector.AddKnowledge(ctx, "lesson: "+query[:min(80, len(query))], result.Learned, "reflection", []string{"lesson", "reflection", "self-improvement"})
				}
						k.knowledgeCollector.RecordKnowledgeUsage(ctx, docIDs, float64(result.Quality)/10.0)
					}
				}
			}
		}
	}

	// 持久化学习洞察

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

				// Auto-extract skills from patterns (sync within doReflection goroutine)
				if k.skillActor != nil {
					k.extractSkillsFromPatterns(context.WithoutCancel(ctx), patterns)
				}
			}
		}
	}

	// 自动知识抽取：质量门控通过后存入知识库
	k.autoSaveKnowledge(ctx, sessionID, query, response, toolCalls, toolErrors)
}

// autoSaveKnowledge 自动知识抽取 — 质量门控通过后存入知识库
func (k *AgentKernel) autoSaveKnowledge(ctx context.Context, sessionID, query, response string, toolCalls, toolErrors int) {
	if k.knowledgeCollector == nil {
		return
	}

	toolSuccesses := toolCalls - toolErrors
	toolFailures := toolErrors

	var reflectResult *ReflectionResult
	var userVerdict string // "good", "bad", or empty
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
		// User feedback — the only ground truth signal
		if v, ok := session.Metadata["user_verdict"].(string); ok {
			userVerdict = v
		}
	}

	// Apply user verdict directly to knowledge weights
	if userVerdict != "" && k.knowledgeCollector != nil && reflectResult != nil {
		if session, err := k.sessionStore.Get(ctx, sessionID); err == nil && session != nil {
			if docIDsRaw, ok := session.Metadata["knowledge_doc_ids"]; ok {
				if docIDs, ok := docIDsRaw.([]string); ok && len(docIDs) > 0 {
					if userVerdict == "good" {
						k.knowledgeCollector.RecordKnowledgeUsage(ctx, docIDs, 0.9) // boost
					} else if userVerdict == "bad" {
						k.knowledgeCollector.RecordKnowledgeUsage(ctx, docIDs, 0.2) // decay
					}
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

	// 存入知识库 — 使用精炼管道（去重 + LLM 精炼 + 结构化存储）
	if refiner, ok := k.knowledgeCollector.(interface {
		Refine(ctx context.Context, query, response, sessionID string) string
	}); ok {
		refiner.Refine(ctx, query, response, sessionID)
	} else if _, err := k.knowledgeCollector.AddKnowledge(ctx, query, response, "auto-extract", []string{"session:" + sessionID}); err != nil {
		slog.Debug("Auto knowledge save failed", "error", err)
	}
}

// extractSkillsFromPatterns analyzes detected patterns and auto-creates skills
// for recurring successful patterns.
func (k *AgentKernel) extractSkillsFromPatterns(ctx context.Context, patterns []Pattern) {
	if k.skillActor == nil || !k.distillEnabled {
		return
	}
	// Get cluster examples for distillation
	var clusterExamples [][]clusterExample
	if sd, ok := k.patternDetector.(*SemanticPatternDetector); ok {
		clusterExamples = sd.GetDistillableExamples()
	}

	for i, p := range patterns {
		theme := strings.ToLower(strings.TrimSpace(p.Description))
		skillID := "auto-" + strings.ReplaceAll(strings.ReplaceAll(theme, " ", "-"), ":", "")
		if len(skillID) > 60 { skillID = skillID[:60] }
		skillName := capitalize(strings.TrimSpace(p.Description))
		if len(skillName) > 50 { skillName = skillName[:50] }

		keywords := tokenize(theme)
		if len(keywords) == 0 { keywords = strings.Fields(p.Type) }

		// Single LLM call: evaluate quality + distill if worth it
		if k.llmProvider != nil {
			skillID, skillName, desc, kw := skillID, skillName, p.Description, keywords
			go func() {
				distillCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				distilled := evaluateAndDistill(distillCtx, k.llmProvider, p, i, clusterExamples)
				if distilled == "" {
					slog.Debug("Cluster rejected by LLM", "theme", desc, "freq", p.Frequency)
					return
				}
				if sd, ok := k.patternDetector.(*SemanticPatternDetector); ok {
					sd.MarkDistilled(desc)
				}
				// Extract tool + file patterns from cluster
			ctx := extractSkillContext(clusterExamples[i])
			// Append file hints to the distilled prompt
			if len(ctx.Files) > 0 {
				distilled += "\n\n## Generated Scripts\nThese files are commonly created:\n"
				for _, f := range ctx.Files {
					distilled += fmt.Sprintf("- `%s`\n", f)
				}
			}
			k.skillActor.AddDistilledSkill(skillID, skillName, desc, distilled, kw, ctx.Tools)
				if k.knowledgeCollector != nil {
					k.knowledgeCollector.AddKnowledge(distillCtx,
						"pattern: "+skillName, distilled, "distillation",
						append(kw, "auto-distilled", "pattern"))
				}
				slog.Info("Skill distilled", "id", skillID, "name", skillName, "freq", p.Frequency)
			}()
		} else {
			simplePrompt := fmt.Sprintf("Auto-detected from %d executions sharing theme: %s", p.Frequency, p.Description)
			k.skillActor.AddSkill(skillID, skillName, p.Description, simplePrompt, keywords)
			slog.Info("Auto-extracted skill", "id", skillID, "name", skillName, "frequency", p.Frequency)
		}
	}
}


func capitalize(s string) string {
	if s == "" { return s }
	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}


// extractVerdictFromLearned parses the reflection's learned field for a verdict prefix.
// LLM is instructed to prefix with [good], [bad], or [neutral].
func extractVerdictFromLearned(learned string) (verdict string, clean string) {
	for _, v := range []string{"[good]", "[bad]", "[neutral]"} {
		if strings.HasPrefix(learned, v) {
			return v[1 : len(v)-1], strings.TrimSpace(learned[len(v):])
		}
	}
	return "", learned
}

// skillContext holds extracted patterns from a query cluster.
type skillContext struct {
	Tools []string // commonly used tools
	Files []string // commonly created/modified script files
}

// extractSkillContext scans cluster examples for tools and generated scripts.
func extractSkillContext(examples []clusterExample) skillContext {
	if len(examples) < 2 {
		return skillContext{}
	}
	var ctx skillContext

	// Count tools and files across examples
	toolCount := map[string]int{}
	fileCount := map[string]int{}
	for _, ex := range examples {
		lower := strings.ToLower(ex.response)
		for _, tool := range []string{"read_file", "search_files", "list_directory", "execute_command",
			"write_file", "diff_edit", "git_log", "git_diff", "git_status", "git_blame",
			"web_search", "web_fetch", "search_knowledge", "manage_memory"} {
			if strings.Contains(lower, tool) {
				toolCount[tool]++
			}
		}
		// Scan for reusable generated scripts — files that appear inside code blocks
		// or are explicitly marked as scripts/templates. Excludes regular coding files.
		inCodeBlock := false
		for _, line := range strings.Split(ex.response, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if !inCodeBlock { continue } // only look inside code blocks
			lower := strings.ToLower(line)
			for _, ext := range []string{".sh", ".py", ".rb", ".js"} {
				if strings.Contains(lower, ext) {
					for _, word := range strings.Fields(line) {
						if strings.HasSuffix(strings.Trim(word, "`'\".,;:!?()[]{}"), ext) {
							fileCount[strings.Trim(word, "`'\".,;:!?()[]{}")]++
						}
					}
				}
			}
		}
	}

	threshold := len(examples) / 2
	if threshold < 2 { threshold = 2 }
	for tool, count := range toolCount {
		if count >= threshold {
			ctx.Tools = append(ctx.Tools, tool)
		}
	}
	for file, count := range fileCount {
		if count >= threshold {
			ctx.Files = append(ctx.Files, file)
		}
	}
	return ctx
}
