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
	// Generate custom roles for this task via LLM (falls back to defaults)
	if o.team != nil {
		o.team.GenerateRoles(ctx, content)
	}
	roleMap := o.routePipeline(ctx, plan)
	var results []string
	var branches []Branch
	var branchMu sync.Mutex
	totalTools := 0

	// progress 把子 agent 的状态转成 OnProgress 调用,
	// 让外层(repl 进度条)能看到每个子任务的实时进展。
	// 之前传 nil 导致进度条永远停在 0%,用户以为"失联"。
	makeProgress := func(subtaskTitle string) SubAgentProgress {
		return func(roleName string, round int, status string) {
			o.reportProgress("execute", fmt.Sprintf("[%s] %s: %s (round %d)", roleName, subtaskTitle, status, round))
		}
	}

	// Phase 1: Execute subtasks in dependency order
	groups := groupByDependency(plan.Subtasks)
	results = make([]string, len(plan.Subtasks))
	for gi, group := range groups {
		o.reportProgress("execute", fmt.Sprintf("Phase 1: group %d/%d (%d tasks)", gi+1, len(groups), len(group)))
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
					roleName = o.firstRoleName()
				}
				var deps []string
				for _, depID := range st.DependsOn {
					depIdx := depID - 1
					if depIdx >= 0 && depIdx < len(results) && results[depIdx] != "" {
						deps = append(deps, results[depIdx])
					}
				}
				task := fmt.Sprintf("Goal: %s\nStep: %s\nDetails: %s", plan.Goal, st.Title, st.Description)
				r, err := o.RunSubAgent(gCtx, userID, projectID, roleName, task, deps, makeProgress(st.Title))
				if err != nil {
					// 子代理失败(超时/错误):标记失败而非终止整批,
					// 后续子任务仍可基于已成功的结果继续。
					results[idx] = fmt.Sprintf("[subtask %d failed: %v]", st.ID, err)
					o.reportProgress("execute", fmt.Sprintf("✗ subtask %d failed (%s): %v", st.ID, st.Title, err))
					slog.Warn("Subtask sub-agent failed", "subtask", st.ID, "role", roleName, "error", err)
					return nil
				}
				results[idx] = r
				o.reportProgress("execute", fmt.Sprintf("✓ subtask %d done (%s)", st.ID, st.Title))
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
	o.reportProgress("verify", "Phase 2: verifying results (tests/lint)")
	execResult, err := o.RunSubAgent(ctx, userID, projectID, o.firstRoleName(),
		"Verify all subtask results work together. Run tests and linters. Report failures.",
		results, makeProgress("verify"))
	if err != nil {
		slog.Warn("Verification failed", "error", err)
	} else {
		results = append(results, execResult)
		totalTools++
	}
	o.reportProgress("verify", "Phase 2 complete")

	// Phase 3: Review — reviewer checks overall quality
	o.reportProgress("review", "Phase 3: reviewing quality")
	reviewResult, err := o.RunSubAgent(ctx, userID, projectID, o.firstRoleName(),
		"Review the complete execution. Check correctness, style, and edge cases. Summarize final status.",
		results, makeProgress("review"))
	if err != nil {
		slog.Warn("Review failed", "error", err)
	} else {
		// Self-correction loop: if reviewer finds issues, retry
		for retry := 0; retry < 2; retry++ {
			reviewUpper := strings.ToUpper(reviewResult)
			if strings.Contains(reviewUpper, "NEEDS_FIX") || strings.Contains(reviewUpper, "NEEDS FIX") || strings.Contains(reviewResult, "[需要返工]") {
				o.reportProgress("review", fmt.Sprintf("Phase 3: fixing issues (attempt %d)", retry+1))
				fixResult, ferr := o.RunSubAgent(ctx, userID, projectID, o.firstRoleName(),
					"Fix the issues found by the reviewer. Do NOT add features — only fix the issues listed:\n"+reviewResult,
					results, makeProgress("fix"))
				if ferr == nil {
					results = append(results, fixResult)
					reviewResult, _ = o.RunSubAgent(ctx, userID, projectID, o.firstRoleName(),
						"Re-review after the fix. Is it acceptable now?",
						results, makeProgress("re-review"))
				} else {
					// 修复失败时 break,避免对同一 reviewResult 反复尝试(浪费时间)
					slog.Warn("Fix attempt failed, stopping retry loop", "attempt", retry+1, "error", ferr)
					break
				}
			} else {
				break
			}
		}
	}
	o.reportProgress("review", "Phase 3 complete")

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
		result[0] = o.firstRoleName()
		return result
	}

	var prompt strings.Builder
	prompt.WriteString("Assign each subtask to the best role.\n\n")
	if o.team != nil {
		prompt.WriteString(fmt.Sprintf("Available roles: %s\n\n", o.team.RoleNames()))
	}
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
			result[i] = o.firstRoleName()
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
	analyzeResult, err := o.RunSubAgent(ctx, userID, projectID, o.firstRoleName(),
		"Analyze this discovery and propose how to handle it:\n"+trigger, mainResults, nil)
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
		o.RunSubAgent(ctx, userID, projectID, o.firstRoleName(),
			"Fix these lint errors. Only fix the lint issues — do NOT add features:\n"+output, nil, nil)
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

// firstRoleName returns the first available role name, or "coder" as last resort.
func (o *Orchestrator) firstRoleName() string {
	if o.team != nil {
		if role := o.team.FirstRole(); role != nil {
			return role.Name
		}
	}
	return "coder"
}

// reportProgress 安全地调用 OnProgress 回调(如果已设置)。
// 这是子任务执行状态外泄的唯一通道 —— 之前完全缺失,
// 导致 repl 的进度条永远停在 0%,用户看不到任何进展,以为"失联"。
func (o *Orchestrator) reportProgress(phase, detail string) {
	if o.OnProgress != nil {
		o.OnProgress(phase, detail)
	}
}
