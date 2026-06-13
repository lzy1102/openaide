package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// QueryAnalysis is a holistic pre-execution analysis that replaces:
//   - detectTaskType (LLM call in buildSystemPrompt)
//   - SkillActor.DetectSkill (LLM call in InjectPrompt + GetTools)
//   - AdaptiveRounds.estimateWithLLM (LLM call in Calculate)
//
// One LLM call instead of 3-4 fragmented micro-judgments.
// The LLM sees the full picture — query, skills, tools — and makes
// a single coherent decision.
type QueryAnalysis struct {
	TaskType     string   `json:"task_type"`     // coding, review, think, general
	SkillID      string   `json:"skill_id"`      // matched skill ID, "" if none
	Complexity   int      `json:"complexity"`    // estimated max rounds
	Strategy     string   `json:"strategy"`      // ReAct strategy hint
	PostProcess  []string `json:"post_process"`  // LLM-decided: "reflect","knowledge","distill"
	AllowedTools []string `json:"allowed_tools"` // tools to keep, nil = all
	SkillPrompt  string   `json:"-"`             // resolved from matched skill (not in JSON)
}

// HasPostProcess checks if a post-processing step was requested by the LLM.
func (a *QueryAnalysis) HasPostProcess(step string) bool {
	if a == nil { return true } // nil = fallback, run everything
	for _, s := range a.PostProcess {
		if s == step { return true }
	}
	return false
}

const analyzeQueryPrompt = `You are a query analyzer for an AI coding agent. Analyze this query holistically.

## Query
%s

## Available Skills
%s

## Task
Return a JSON object with these fields:
- task_type: "coding" | "review" | "think" | "general"
- skill_id: matched skill ID from the list, or "" if none match
- complexity: estimated ReAct rounds needed (integer, 5-50)
- strategy: a ONE-SENTENCE strategy hint for the agent
- post_process: array of steps to run after the task. Choose from:
    "reflect" — worth reviewing what worked/didn't (complex or novel tasks)
    "knowledge" — worth saving facts to knowledge base (new discoveries)
    "distill" — worth extracting as reusable skill (recurring pattern)

## Rules
- Coding/bugfix: match relevant skill, complexity 10-30
- Review/audit: use review skill if present, complexity 10-25
- Think/explain/analyze: no skill needed, complexity 5-15
- General/chitchat/greetings: no skill, complexity 5, post_process: []
- Only include post_process steps that are GENUINELY valuable:
  * "hello"/"what is 2+2" → post_process: []
  * "fix the auth bug" → post_process: ["reflect"]
  * "review this PR" → post_process: ["reflect", "knowledge"]
  * recurring pattern you have seen → add "distill"
- Be conservative with skill_id: only match if genuinely relevant

Reply with ONLY the JSON object, no other text.`

func (k *AgentKernel) analyzeQuery(ctx context.Context, query string) *QueryAnalysis {
	if k.llmProvider == nil {
		return nil
	}
	start := time.Now()

	// Collect skill metadata (outside CSP actor)
	var skillInfos []struct {
		id, desc string
		tools    []string
	}
	if k.skillActor != nil {
		for _, s := range k.skillActor.ListEnabled() {
			skillInfos = append(skillInfos, struct {
				id, desc string
				tools    []string
			}{s.ID, s.Description, s.AllowedTools})
		}
	}

	// Build skill list for prompt
	var skillLines string
	if len(skillInfos) > 0 {
		var parts []string
		for _, s := range skillInfos {
			toolStr := strings.Join(s.tools, ", ")
			parts = append(parts, fmt.Sprintf("- %s: %s (tools: %s)", s.id, s.desc, toolStr))
		}
		skillLines = strings.Join(parts, "\n")
	} else {
		skillLines = "(none)"
	}

	prompt := fmt.Sprintf(analyzeQueryPrompt, truncStr(query, 500), skillLines)
	resp, err := k.llmProvider.Chat(ctx, []Message{
		{Role: "user", Content: prompt},
	}, nil, map[string]interface{}{
		"max_tokens": 300, "temperature": 0, "route": "execution", "no_thinking": true,
	})
	if err != nil {
		slog.Warn("Query analysis LLM failed, falling back", "error", err)
		return nil
	}

	// Strip markdown fences if present
	body := strings.TrimSpace(resp.Content)
	body = strings.TrimPrefix(body, "```json")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)

	// Try strict JSON first, then lenient single-keyword fallback
	var a QueryAnalysis
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		// LLM may have returned a bare keyword — treat as task type
		body = strings.ToLower(body)
		for _, cat := range []string{"coding", "review", "think", "general"} {
			if strings.Contains(body, cat) {
				a.TaskType = cat
				a.Complexity = 0
				slog.Debug("Query analysis: lenient task type only", "task", cat)
				return &a
			}
		}
		// Truly can't parse — fall back to individual LLM calls
		slog.Debug("Query analysis parse failed", "body", truncStr(body, 80))
		return nil
	}

	// Resolve skill prompt from matched skill
	if a.SkillID != "" && k.skillActor != nil {
		if s := k.skillActor.GetSkill(a.SkillID); s != nil {
			a.SkillPrompt = s.Prompt
			if len(s.AllowedTools) > 0 {
				a.AllowedTools = s.AllowedTools
			}
		}
	}

	slog.Debug("Query analyzed",
		"task", a.TaskType, "skill", a.SkillID,
		"complexity", a.Complexity, "post_process", a.PostProcess,
		"duration", time.Since(start))
	return &a
}
