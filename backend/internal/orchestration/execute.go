package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"openaide/backend/internal/kernel"
)

// Branch represents a diverged sub-task triggered by a discovery signal.
type Branch struct {
	Trigger string
	Parent  string
	Result  string
}

// ExecuteWithPlan runs a pre-made plan (from interactive select or DeepPlan).
func (o *Orchestrator) ExecuteWithPlan(ctx context.Context, userID, projectID, content string, plan *Plan, opts kernel.QueryOptions) (*kernel.Response, error) {
	opts.ForcePlan = true
	return o.executePlan(ctx, userID, projectID, content, plan, opts)
}

// executePlan orchestrates subtask execution: route → execute → verify → review.
// Supports branch-converge: sub-agents can signal discovery, triggering a parallel
// analysis branch whose results converge back into the main task line.
func (o *Orchestrator) executePlan(ctx context.Context, userID, projectID, content string, plan *Plan, opts kernel.QueryOptions) (*kernel.Response, error) {
	roleMap := o.routePipeline(ctx, plan)
	var results []string
	var branches []Branch
	var branchMu sync.Mutex
	totalTools := 0

	// Phase 1: Execute subtasks in dependency order
	groups := groupByDependency(plan.Subtasks)
	results = make([]string, len(plan.Subtasks))
	for _, group := range groups {
		g, gCtx := errgroup.WithContext(ctx)
		for _, st := range group {
			st := st // capture
			g.Go(func() error {
				idx := st.ID - 1
				if idx < 0 || idx >= len(results) {
					return fmt.Errorf("subtask ID %d out of range [1-%d]", st.ID, len(results))
				}
				roleName := roleMap[idx]
				if roleName == "" {
					roleName = "coder"
				}
				var deps []string
				for _, depID := range st.DependsOn {
					depIdx := depID - 1
					if depIdx >= 0 && depIdx < len(results) && results[depIdx] != "" {
						deps = append(deps, results[depIdx])
					}
				}
				task := fmt.Sprintf("Goal: %s\nStep: %s\nDetails: %s", plan.Goal, st.Title, st.Description)
				r, err := o.RunSubAgent(gCtx, userID, projectID, roleName, task, deps)
				if err != nil {
					return fmt.Errorf("subtask %d (%s): %w", st.ID, roleName, err)
				}
				results[idx] = r
				// Check for branch trigger
				if triggered, signal := detectBranchSignal(r); triggered {
					branch := o.executeBranch(gCtx, userID, projectID, signal, results, &branches)
					branchMu.Lock()
					branches = append(branches, branch)
					branchMu.Unlock()
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
	}

	// Phase 2: Verify — executor runs tests/lint
	execResult, err := o.RunSubAgent(ctx, userID, projectID, "executor",
		"Verify all subtask results work together. Run tests and linters. Report failures.",
		results)
	if err != nil {
		slog.Warn("Verification failed", "error", err)
	} else {
		results = append(results, execResult)
		totalTools++
	}

	// Phase 3: Review — reviewer checks overall quality
	reviewResult, err := o.RunSubAgent(ctx, userID, projectID, "reviewer",
		"Review the complete execution. Check correctness, style, and edge cases. Summarize final status.",
		results)
	if err != nil {
		slog.Warn("Review failed", "error", err)
	} else {
		// Self-correction loop: if reviewer finds issues, retry
		for retry := 0; retry < 2; retry++ {
			reviewUpper := strings.ToUpper(reviewResult)
			if strings.Contains(reviewUpper, "NEEDS_FIX") || strings.Contains(reviewUpper, "NEEDS FIX") || strings.Contains(reviewResult, "[需要返工]") {
				fixResult, ferr := o.RunSubAgent(ctx, userID, projectID, "coder",
					"Fix the issues found by the reviewer. Do NOT add features — only fix the issues listed:\n"+reviewResult,
					results)
				if ferr == nil {
					results = append(results, fixResult)
					reviewResult, _ = o.RunSubAgent(ctx, userID, projectID, "reviewer",
						"Re-review after the fix. Is it acceptable now?",
						results)
				}
			} else {
				break
			}
		}
	}

	// Build final response
	summary := "Execution complete.\n\n"
	summary += formatResults(results)
	if len(branches) > 0 {
		summary += fmt.Sprintf("\n[Branches] %d discoveries explored\n", len(branches))
	}

	return &kernel.Response{
		Content:   summary,
		ToolCalls: totalTools + len(plan.Subtasks),
	}, nil
}

// routePipeline assigns each subtask to the best role via a single LLM call.
func (o *Orchestrator) routePipeline(ctx context.Context, plan *Plan) map[int]string {
	result := make(map[int]string)
	if len(plan.Subtasks) == 0 {
		return result
	}
	if len(plan.Subtasks) == 1 {
		result[0] = "coder"
		return result
	}

	var prompt strings.Builder
	prompt.WriteString("Assign each subtask to the best role. Roles: analyst, coder, reviewer, executor.\n\n")
	prompt.WriteString(fmt.Sprintf("Goal: %s\n\n", plan.Goal))
	prompt.WriteString("Subtasks:\n")
	for _, st := range plan.Subtasks {
		prompt.WriteString(fmt.Sprintf("%d. %s - %s\n", st.ID, st.Title, st.Description))
	}
	prompt.WriteString("\nFormat: ID=role, comma separated. Example: 1=coder,2=reviewer,3=executor")

	messages := []kernel.Message{
		{Role: "system", Content: "You are a task router. Assign the best role to each subtask. Format: ID=role"},
		{Role: "user", Content: prompt.String()},
	}
	resp, err := o.llmGateway.Chat(ctx, messages, nil, map[string]interface{}{
		"max_tokens": 100, "temperature": 0, "route": "execution", "no_thinking": true,
	})
	if err != nil {
		for i := range plan.Subtasks {
			result[i] = "coder"
		}
		return result
	}

	for _, pair := range strings.Split(strings.TrimSpace(resp.Content), ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			id := 0
			fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &id)
			role := strings.TrimSpace(parts[1])
			if id > 0 && id <= len(plan.Subtasks) {
				result[id-1] = role
			}
		}
	}
	return result
}

// executeBranch runs a parallel analysis branch triggered by a discovery signal.
func (o *Orchestrator) executeBranch(ctx context.Context, userID, projectID, trigger string, mainResults []string, branches *[]Branch) Branch {
	branch := Branch{Trigger: trigger, Parent: "main"}
	slog.Info("Branch triggered", "trigger", trigger[:min(80, len(trigger))])
	analyzeResult, err := o.RunSubAgent(ctx, userID, projectID, "analyst",
		"Analyze this discovery and propose how to handle it:\n"+trigger, mainResults)
	if err != nil {
		return branch
	}
	branch.Result = analyzeResult
	return branch
}

// detectBranchSignal checks if a sub-agent result contains a discovery signal.
func detectBranchSignal(content string) (bool, string) {
	upper := strings.ToUpper(content)
	for _, marker := range []string{"[DISCOVERY:]", "[REPLAN:]", "DISCOVERY:", "REPLAN:"} {
		if idx := strings.Index(upper, marker); idx >= 0 {
			end := strings.Index(content[idx:], "\n")
			if end < 0 {
				end = len(content) - idx
			}
			signal := strings.TrimSpace(content[idx : idx+min(end, 300)])
			return true, signal
		}
	}
	return false, ""
}

func formatResults(results []string) string {
	var sb strings.Builder
	for i, r := range results {
		if r == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("## Result %d\n", i+1))
		if len(r) > 1000 {
			r = r[:1000] + "..."
		}
		sb.WriteString(r)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func truncateForLearning(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func minStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// lintRepairLoop runs the linter and feeds errors back to the coder for fixing.
func (o *Orchestrator) lintRepairLoop(ctx context.Context, userID, projectID string, maxRetries int) {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	cwd, _ := os.Getwd()
	var prevErrors []string
	for i := 0; i < maxRetries; i++ {
		output, err := runLint(cwd)
		if err == nil && output == "" {
			return
		}
		if err != nil {
			output = err.Error()
		}
		if output == "" || output == strings.Join(prevErrors, "\n") {
			return // no new errors or duplicate
		}
		prevErrors = append(prevErrors, output)
		slog.Info("Lint repair attempt", "attempt", i+1)
		o.RunSubAgent(ctx, userID, projectID, "coder",
			"Fix these lint errors. Only fix the lint issues — do NOT add features:\n"+output, nil)
	}
}

// runLint detects project language and runs the appropriate linter.
func runLint(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return execCmd("golangci-lint", "run", "--timeout=60s")
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		return execCmd("npx", "eslint", ".", "--ext=.js,.ts,.tsx", "--max-warnings=0")
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		return execCmd("ruff", "check", ".")
	}
	return "", nil
}

func execCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = getwd()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func getwd() string {
	cwd, _ := os.Getwd()
	return cwd
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
