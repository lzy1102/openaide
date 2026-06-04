package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ── ReAct Loop Helpers ──────────────────────────────────────
// Extracted from Process and ProcessStream to eliminate duplication.
// Each method does ONE thing with a clear name.

// prepareReActRound handles context management before each ReAct iteration:
// 1. Compress context if exceeds 90% of token budget
// 2. Snips old tool outputs to avoid context bloat
// 3. Injects budget reminder when approaching round limit
func (k *AgentKernel) prepareReActRound(ctx context.Context, messages []Message, round, maxRounds int) []Message {
	// Compression: 90% threshold (Claude Code uses 92%)
	if k.compressor != nil {
		tokenCount := k.compressor.EstimateTokens(messages)
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

	snipOldToolOutputs(messages)

	// Budget injection: warn LLM about remaining rounds
	if round >= maxRounds/2 && round < maxRounds-1 {
		remaining := maxRounds - round
		messages = append(messages, Message{
			Role: "user", Content: fmt.Sprintf(
				"[System] Used %d/%d rounds, %d remaining. Give your final answer now if you have enough information.",
				round, maxRounds, remaining),
		})
	} else if round >= maxRounds-1 {
		// Check if user wants to continue beyond budget
	if k.queryOptions != nil && k.queryOptions.OnBudgetExhausted != nil {
		if k.queryOptions.OnBudgetExhausted(round, maxRounds) {
			return messages // Callback says continue without forced stop
		}
	}
	messages = append(messages, Message{
		Role: "user", Content: "[System] Final round — must give final answer. Do NOT call any tools.",
	})
	}
	return messages
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

// finalizeResponse performs all post-ReAct work shared by both paths:
// save memory, update session, generate title, run reflection, close tracer
func (k *AgentKernel) finalizeResponse(ctx context.Context, session *Session, query *Query, response string, toolCalls int) {
	// Saga: save memory → update session, with compensation
	RunSaga([]SagaStep{
		{
			Name: "save-memory",
			Execute: func() error {
				k.saveToMemory(ctx, session.ID, session.Messages)
				return nil
			},
			Compensate: func() error {
				// Best-effort: remove saved items (MemoryActor is eventually consistent)
				return nil
			},
		},
		{
			Name: "update-session",
			Execute: func() error {
				session.UpdatedAt = time.Now()
				ensureSessionTitle(session)
				return k.sessionStore.Update(ctx, session)
			},
			Compensate: nil, // session update is idempotent
		},
	})

	// Async: generate meaningful title and run reflection
	go k.generateSessionTitle(session, query.Content)
	if k.reflection != nil {
		go k.doReflection(ctx, session.ID, query.Content, response, toolCalls, 0)
	}
	go k.compressMemory(ctx, session.ID)
	// Periodically decay unused skills
	if k.skillActor != nil { go k.skillActor.DecayUnused() }
}

// executeToolBatch runs a group of tool calls concurrently (all are parallel-safe).
// Returns aggregated tool results, total count, and any per-tool errors (non-fatal).
func (k *AgentKernel) executeToolBatch(ctx context.Context, toolCalls []ToolCall, sessionID string) (results []string, count int) {
	results = make([]string, len(toolCalls))
	if len(toolCalls) == 0 {
		return results, 0
	}

	// Simple concurrent execution — all tools in this batch are parallel-safe
	type toolEntry struct {
		idx  int
		name string
		resp string
	}
	ch := make(chan toolEntry, len(toolCalls))
	for i, tc := range toolCalls {
		go func(i int, tc ToolCall) {
			res := k.executeTool(ctx, tc, sessionID)
			prefix := ""
			if res.Error != "" {
				prefix = "Error: "
			}
			content := fmt.Sprintf("%v", res.Content)
				ch <- toolEntry{i, tc.Function.Name, prefix + content}
		}(i, tc)
	}
	for range toolCalls {
		e := <-ch
		results[e.idx] = fmt.Sprintf("### %s\n%s", e.name, e.resp)
	}

	return results, len(toolCalls)
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

// injectMemoryContext appends reflection, learning insights, and knowledge
// context after the user query (dynamic tail — doesn't break prompt cache prefix).
func (k *AgentKernel) injectMemoryContext(ctx context.Context, messages []Message, session *Session, query *Query) []Message {
	// Previous-round reflection
	if session.Metadata != nil {
		if ref, ok := session.Metadata["reflection"]; ok {
			if r, ok := ref.(*ReflectionResult); ok && r != nil {
				hint := fmt.Sprintf("[Previous reflection] Quality: %d/10", r.Quality)
				if len(r.Issues) > 0 {
					hint += fmt.Sprintf(" | Issues: %s", strings.Join(r.Issues, "; "))
				}
				if len(r.Suggestions) > 0 {
					hint += fmt.Sprintf(" | Suggestions: %s", strings.Join(r.Suggestions, "; "))
				}
				messages = append(messages, Message{Role: "system", Content: hint})
			}
			delete(session.Metadata, "reflection")
		}
	}

	// Cross-session learning

	// Knowledge base context
	if k.knowledgeCollector != nil {
		kbCtx, docIDs, err := k.knowledgeCollector.InjectContext(ctx, query.Content, 500)
		if err == nil && kbCtx != "" {
			messages = append(messages, Message{
				Role: "system", Content: "[Knowledge] " + kbCtx,
			})
			if len(docIDs) > 0 {
				if session.Metadata == nil {
					session.Metadata = make(map[string]interface{})
				}
				session.Metadata["knowledge_doc_ids"] = docIDs
			}
		}
	}

	return messages
}

// determineMaxRounds calculates the ReAct loop limit (adaptive or config-based).
func (k *AgentKernel) determineMaxRounds(ctx context.Context, queryContent string, historyLen int) int {
	if k.adaptiveRounds != nil {
		return k.adaptiveRounds.Calculate(ctx, queryContent, historyLen)
	}
	return k.maxRounds
}

// compressMemory compresses old working memory items into short-term summaries.
// This prevents unlimited growth of per-message memory files.
func (k *AgentKernel) compressMemory(ctx context.Context, sessionID string) {
	if k.memory == nil { return }
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

