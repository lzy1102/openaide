package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"openaide/backend/internal/kernel"
)

// Approach is a named strategy for solving a task.
type Approach struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"` // guiding prompt for this approach
}

// BranchResult holds the result of exploring one approach.
type BranchResult struct {
	Approach   Approach `json:"approach"`
	Findings   string   `json:"findings"`
	ToolCalls  int      `json:"tool_calls"`
	Confidence float64  `json:"confidence"` // 0-1, set by evaluator
}

// ExploreResult holds the full tree-of-thoughts exploration result.
type ExploreResult struct {
	Branches []BranchResult `json:"branches"`
	Best     int            `json:"best"` // index of best branch
	Rationale string        `json:"rationale"`
}

// ExploreAlternatives explores multiple approaches to a task in parallel,
// evaluates each branch independently, and selects the best approach.
// This implements Tree of Thoughts (2023): branching at decision points.
func (o *Orchestrator) ExploreAlternatives(ctx context.Context, userID, projectID, task string, approaches []Approach) (*ExploreResult, error) {
	if len(approaches) < 2 {
		return nil, fmt.Errorf("need at least 2 approaches for tree-of-thoughts exploration")
	}
	if len(approaches) > 4 {
		approaches = approaches[:4]
	}

	slog.Info("Tree of Thoughts exploration started", "branches", len(approaches), "task", task[:min(80, len(task))])

	// Phase 1: Fork — run each approach in parallel as an analyst sub-agent
	branches := make([]BranchResult, len(approaches))
	type idxResult struct {
		idx  int
		res  string
		tools int
		err  error
	}
	ch := make(chan idxResult, len(approaches))

	for i, app := range approaches {
		go func(idx int, a Approach) {
			role := o.getTeamRole("analyst")
			if role == nil {
				ch <- idxResult{idx: idx, err: fmt.Errorf("analyst role not available")}
				return
			}
			// Build the exploration prompt
			guidance := fmt.Sprintf("## Task\n%s\n\n## Your Approach\n%s\n\nExplore this approach: read relevant files, analyze trade-offs, and report your findings. Keep it under 300 words.", task, a.Prompt)
			res, err := o.RunSubAgent(ctx, userID, projectID, "analyst", guidance, nil)
			if err != nil {
				ch <- idxResult{idx: idx, err: err}
				return
			}
			ch <- idxResult{idx: idx, res: res, tools: 0} // tool count tracked internally
		}(i, app)
	}

	for range approaches {
		r := <-ch
		if r.err != nil {
			slog.Warn("Tree branch failed", "branch", r.idx, "error", r.err)
			continue
		}
		branches[r.idx] = BranchResult{
			Approach:  approaches[r.idx],
			Findings:  r.res,
			ToolCalls: r.tools,
		}
	}

	// Phase 2: Evaluate — LLM compares branches and picks the best
	best, rationale := o.evaluateBranches(ctx, task, branches)
	for i := range branches {
		if i == best {
			branches[i].Confidence = 0.9
		} else {
			branches[i].Confidence = 0.3
		}
	}

	// Phase 3: Learn — record which approaches worked
	for i, b := range branches {
		tag := "tot-failed"
		if i == best { tag = "tot-best" }
		if o.mind != nil {
			o.mind.RecordExecution(task, b.Approach.Name, i == best, nil, nil, nil, 0, "")
		}
		_ = tag
	}

	return &ExploreResult{
		Branches:  branches,
		Best:      best,
		Rationale: rationale,
	}, nil
}

func (o *Orchestrator) evaluateBranches(ctx context.Context, task string, branches []BranchResult) (best int, rationale string) {
	if len(branches) == 0 { return 0, "" }

	var sb strings.Builder
	sb.WriteString("Evaluate the following exploration results for the task and select the best approach.\n\n")
	sb.WriteString(fmt.Sprintf("## Task\n%s\n\n", task))
	sb.WriteString("## Branch Results\n\n")
	for i, b := range branches {
		sb.WriteString(fmt.Sprintf("### Branch %d: %s\n%s\n\n", i+1, b.Approach.Name, b.Findings))
	}
	sb.WriteString("Reply with ONLY: BEST=<number> REASON=<one sentence why>")

	resp, err := o.llmGateway.Chat(ctx, []kernel.Message{
		{Role: "user", Content: sb.String()},
	}, nil, map[string]interface{}{"max_tokens": 100, "temperature": 0, "route": "execution", "no_thinking": true})
	if err != nil {
		return 0, "evaluation failed, defaulting to first branch"
	}

	// Parse: BEST=1 REASON=...
	content := resp.Content
	best = 0
	fmt.Sscanf(content, "BEST=%d", &best)
	best-- // convert to 0-indexed
	if best < 0 || best >= len(branches) { best = 0 }

	if idx := strings.Index(content, "REASON="); idx >= 0 {
		rationale = strings.TrimSpace(content[idx+7:])
	}
	return
}
