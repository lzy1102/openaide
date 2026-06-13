package feedback

import (
	"context"
	"fmt"
	"math"
	"strings"

	"openaide/backend/internal/kernel"
)

// Verdict 用户反馈结论
type Verdict int

const (
	VerdictNone Verdict = iota
	VerdictGood
	VerdictBad
)

// Result 执行结果快照（用于质量评估）
type Result struct {
	Query         string
	Response      string
	ToolSuccesses int
	ToolFailures  int
	Reflection    *kernel.ReflectionResult
	UserVerdict   Verdict
}

// Gate 质量门控 — LLM-first, formula-fallback.
// When LLMReflection has already scored the execution (Quality 0-10),
// that judgment takes priority. Otherwise the LLM is asked directly.
// The 40/30/30 formula is only a last resort.
type Gate struct {
	llm   kernel.LLMProvider
}

// NewGate creates a quality gate.
func NewGate() *Gate { return &Gate{} }

// SetLLM injects an LLM provider for holistic quality assessment.
func (g *Gate) SetLLM(llm kernel.LLMProvider) { g.llm = llm }

// Pass implements kernel.QualityGate — LLM-first, formula-fallback.
func (g *Gate) Pass(query, response string, toolSuccesses, toolFailures int, reflection *kernel.ReflectionResult) bool {
	// LLM reflection already judged quality on a 0-10 scale — trust it
	if reflection != nil {
		return reflection.Quality >= 5
	}

	// No reflection: ask LLM to judge quality directly
	if g.llm != nil {
		return g.assessWithLLM(context.Background(), query, response, toolSuccesses, toolFailures)
	}

	// Fallback: formula (LLM unavailable)
	return g.formulaPass(query, response, toolSuccesses, toolFailures)
}

func (g *Gate) assessWithLLM(ctx context.Context, query, response string, toolSuccesses, toolFailures int) bool {
	prompt := fmt.Sprintf(`Evaluate whether this agent execution is worth saving to the knowledge base.

Query: %s
Response: %s
Tool calls: %d succeeded, %d failed

Reply with ONLY one word: "keep" or "skip".
Keep: the execution produced useful, novel, reusable knowledge.
Skip: the execution was mundane, incorrect, or already well-known.`,
		truncStr(query, 300), truncStr(response, 500), toolSuccesses, toolFailures)

	resp, err := g.llm.Chat(ctx, []kernel.Message{
		{Role: "user", Content: prompt},
	}, nil, map[string]interface{}{
		"max_tokens": 5, "temperature": 0, "route": "execution", "no_thinking": true,
	})
	if err != nil {
		return g.formulaPass(query, response, toolSuccesses, toolFailures)
	}
	return strings.Contains(strings.ToLower(resp.Content), "keep")
}

// formulaPass is the 40/30/30 formula — only used as fallback.
func (g *Gate) formulaPass(query, response string, toolSuccesses, toolFailures int) bool {
	toolScore := 0.5
	if total := toolSuccesses + toolFailures; total > 0 {
		toolScore = float64(toolSuccesses) / float64(total)
	}
	return math.Round((0.6*toolScore+0.4)*100)/100 >= 0.6
}

// QualityScore is kept for backward compatibility with tests.
func (r *Result) QualityScore() float64 {
	toolScore := 0.5
	if total := r.ToolSuccesses + r.ToolFailures; total > 0 {
		toolScore = float64(r.ToolSuccesses) / float64(total)
	}
	userScore := 0.5
	switch r.UserVerdict {
	case VerdictGood:
		userScore = 1.0
	case VerdictBad:
		userScore = 0.0
	}
	reflectScore := 0.5
	if r.Reflection != nil {
		reflectScore = float64(r.Reflection.Quality) / 10.0
	}
	return math.Round((0.4*toolScore+0.3*userScore+0.3*reflectScore)*100) / 100
}

func (r *Result) ShouldKeep() bool       { return r.QualityScore() > 0.6 }
func (r *Result) ShouldDiscard() bool    { return r.QualityScore() < 0.3 }
func (r *Result) NeedsConfirmation() bool { s := r.QualityScore(); return s >= 0.3 && s <= 0.6 }

func (g *Gate) ExtractKnowledge(r *Result) (title, content string, ok bool) {
	if !g.check(r) { return "", "", false }
	title = r.Query
	if len(title) > 80 { title = title[:80] + "..." }
	content = r.Response
	if r.Reflection != nil && len(r.Reflection.Learned) > 0 {
		content += "\n\n[经验] " + r.Reflection.Learned
	}
	return title, content, true
}

func (g *Gate) check(r *Result) bool { return r.QualityScore() >= 0.6 }

func truncStr(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}
