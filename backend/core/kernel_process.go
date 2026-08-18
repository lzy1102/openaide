package kernel

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Process handles a user query synchronously by delegating to ProcessStream
// and collecting all stream chunks into a single Response.
// This eliminates ~200 lines of duplicated ReAct loop logic.
func (k *AgentKernel) Process(ctx context.Context, query *Query) (*Response, error) {
	start := time.Now()

	stream, err := k.ProcessStream(ctx, query)
	if err != nil {
		return nil, err
	}

	var content strings.Builder
	toolCalls := 0
	totalTokens := 0

	for chunk := range stream {
		switch chunk.Type {
		case ChunkTypeContent:
			content.WriteString(chunk.Content)
		case ChunkTypeToolCall:
			toolCalls++
			content.Reset() // tool round, reset for final answer
		case ChunkTypeDone:
			if chunk.Usage != nil {
				totalTokens = chunk.Usage.TotalTokens
			}
			if chunk.Content != "" {
				content.WriteString(chunk.Content)
			}
		case ChunkTypeError:
			if chunk.Error != nil {
				return nil, chunk.Error
			}
		}
	}

	model := ""
	if k.llmProvider != nil {
		model = k.llmProvider.GetModelID()
	}
	return &Response{
		Content:    content.String(),
		ToolCalls:  toolCalls,
		TokensUsed: totalTokens,
		Duration:   time.Since(start),
		Model:      model,
	}, nil
}

func (k *AgentKernel) doReflection(ctx context.Context, sessionID, query, response string, toolCalls, toolErrors int, analysis *QueryAnalysis) {
	if k.reflection == nil {
		return
	}

	record := ExecutionRecord{
		Query:     query,
		Response:  response,
		Success:   toolErrors == 0,
		ToolCalls: make([]ToolCall, 0),
		Messages:  k.loadSessionMessages(ctx, sessionID),
		TaskType:  queryTaskType(analysis),
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
				if session.Metadata == nil {
					session.Metadata = make(map[string]interface{})
				}
				session.Metadata["user_verdict"] = verdict
				k.sessionStore.Update(ctx, session)
			}
		}
	}

	// Skill feedback: record quality for the activated skill
	if k.skillActor != nil {
		k.skillActor.RecordLastUsage(result.Quality)
	}

	// Store reflection result to session
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
		}
	}
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

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
