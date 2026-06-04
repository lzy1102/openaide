package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"openaide/backend/internal/kernel"
)

// Task is a benchmark task that tests agent capabilities.
type Task struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"` // coding, review, research, teaching
	Query       string   `json:"query"`
	MustContain []string `json:"must_contain"` // keywords expected in response
	MustNotContain []string `json:"must_not_contain,omitempty"`
	MinToolCalls int     `json:"min_tool_calls,omitempty"` // 0 = no minimum
	Difficulty  string   `json:"difficulty"` // easy, medium, hard
}

// Result records a single task execution result.
type Result struct {
	Task       Task          `json:"task"`
	Passed     bool          `json:"passed"`
	Response   string        `json:"response"`
	Duration   time.Duration `json:"duration"`
	ToolCalls  int           `json:"tool_calls"`
	TokensUsed int           `json:"tokens_used"`
	FailReason string        `json:"fail_reason,omitempty"`
}

// Run is a snapshot of a full evaluation run.
type Run struct {
	ID        string        `json:"id"`
	Timestamp time.Time     `json:"timestamp"`
	Results   []Result      `json:"results"`
	Total     int           `json:"total"`
	Passed    int           `json:"passed"`
	AvgTime   time.Duration `json:"avg_time"`
	AvgTools  float64       `json:"avg_tools"`
	AvgTokens float64       `json:"avg_tokens"`
}

// Runner executes evaluation tasks against a kernel.
type Runner struct {
	kernel kernel.Kernel
}

// NewRunner creates an evaluation runner.
func NewRunner(k kernel.Kernel) *Runner {
	return &Runner{kernel: k}
}

// RunTasks executes all tasks and returns a Run report.
func (r *Runner) RunTasks(ctx context.Context, tasks []Task) *Run {
	run := &Run{
		ID:        fmt.Sprintf("eval_%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Total:     len(tasks),
	}

	var totalTime time.Duration
	var totalTools, totalTokens int
	for _, task := range tasks {
		result := r.runOne(ctx, task)
		run.Results = append(run.Results, result)
		if result.Passed {
			run.Passed++
		}
		totalTime += result.Duration
		totalTools += result.ToolCalls
		totalTokens += result.TokensUsed
	}

	if run.Total > 0 {
		run.AvgTime = totalTime / time.Duration(run.Total)
		run.AvgTools = float64(totalTools) / float64(run.Total)
		run.AvgTokens = float64(totalTokens) / float64(run.Total)
	}
	return run
}

func (r *Runner) runOne(ctx context.Context, task Task) Result {
	start := time.Now()
	resp, err := r.kernel.Process(ctx, &kernel.Query{
		Content: task.Query,
		Options: kernel.QueryOptions{MaxTokens: 2000},
	})
	duration := time.Since(start)

	result := Result{
		Task:     task,
		Response: resp.Content,
		Duration: duration,
	}
	if resp != nil {
		result.ToolCalls = resp.ToolCalls
		result.TokensUsed = resp.TokensUsed
	}

	if err != nil {
		result.FailReason = fmt.Sprintf("kernel error: %v", err)
		return result
	}

	// Check must_contain
	for _, keyword := range task.MustContain {
		if !strings.Contains(strings.ToLower(resp.Content), strings.ToLower(keyword)) {
			result.FailReason = fmt.Sprintf("missing required keyword: %q", keyword)
			return result
		}
	}

	// Check must_not_contain
	for _, keyword := range task.MustNotContain {
		if strings.Contains(strings.ToLower(resp.Content), strings.ToLower(keyword)) {
			result.FailReason = fmt.Sprintf("found forbidden keyword: %q", keyword)
			return result
		}
	}

	// Check min tool calls
	if task.MinToolCalls > 0 && result.ToolCalls < task.MinToolCalls {
		result.FailReason = fmt.Sprintf("expected >=%d tool calls, got %d", task.MinToolCalls, result.ToolCalls)
		return result
	}

	result.Passed = true
	return result
}

// Compare compares two runs and reports changes.
func Compare(before, after *Run) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Eval Comparison: %s → %s\n\n", before.ID, after.ID))
	sb.WriteString(fmt.Sprintf("| Metric | Before | After | Change |\n"))
	sb.WriteString(fmt.Sprintf("|--------|--------|-------|--------|\n"))
	sb.WriteString(fmt.Sprintf("| Pass Rate | %d/%d (%.0f%%) | %d/%d (%.0f%%) | %+d |\n",
		before.Passed, before.Total, pct(before.Passed, before.Total),
		after.Passed, after.Total, pct(after.Passed, after.Total),
		after.Passed-before.Passed))
	sb.WriteString(fmt.Sprintf("| Avg Time | %v | %v | %+v |\n",
		before.AvgTime.Round(time.Millisecond), after.AvgTime.Round(time.Millisecond),
		(after.AvgTime - before.AvgTime).Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| Avg Tools | %.1f | %.1f | %+.1f |\n",
		before.AvgTools, after.AvgTools, after.AvgTools-before.AvgTools))
	sb.WriteString(fmt.Sprintf("| Avg Tokens | %.0f | %.0f | %+.0f |\n",
		before.AvgTokens, after.AvgTokens, after.AvgTokens-before.AvgTokens))

	// Per-task comparison
	sb.WriteString("\n### Per-Task Breakdown\n\n")
	afterMap := make(map[string]Result)
	for _, r := range after.Results {
		afterMap[r.Task.ID] = r
	}
	for _, beforeR := range before.Results {
		afterR, ok := afterMap[beforeR.Task.ID]
		if !ok {
			continue
		}
		status := "✓"
		if !afterR.Passed {
			status = "✗"
		}
		if beforeR.Passed && !afterR.Passed {
			status = "⚠ REGRESSION"
		}
		if !beforeR.Passed && afterR.Passed {
			status = "↑ FIXED"
		}
		sb.WriteString(fmt.Sprintf("- **%s** (%s) %s: %v → %v, tools: %d→%d\n",
			afterR.Task.Name, afterR.Task.Difficulty, status,
			beforeR.Duration.Round(time.Millisecond), afterR.Duration.Round(time.Millisecond),
			beforeR.ToolCalls, afterR.ToolCalls))
	}
	return sb.String()
}

func pct(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(passed) / float64(total) * 100
}

// Scorecard returns a brief summary string.
func (r *Run) Scorecard() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Eval %s: %d/%d passed (%.0f%%), avg %v, %.1f tools, %.0f tokens\n",
		r.ID, r.Passed, r.Total, pct(r.Passed, r.Total),
		r.AvgTime.Round(time.Millisecond), r.AvgTools, r.AvgTokens))

	// Group by difficulty
	for _, diff := range []string{"easy", "medium", "hard"} {
		var passed, total int
		for _, r := range r.Results {
			if r.Task.Difficulty == diff {
				total++
				if r.Passed {
					passed++
				}
			}
		}
		if total > 0 {
			sb.WriteString(fmt.Sprintf("  %s: %d/%d\n", diff, passed, total))
		}
	}

	// Show failures
	for _, r := range r.Results {
		if !r.Passed {
			sb.WriteString(fmt.Sprintf("  ✗ %s: %s\n", r.Task.Name, r.FailReason))
		}
	}
	return sb.String()
}

// SortResultsByDuration sorts results for analysis.
func SortResultsByDuration(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Duration < results[j].Duration
	})
}

// SortResultsByToolCalls sorts results for analysis.
func SortResultsByToolCalls(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].ToolCalls < results[j].ToolCalls
	})
}
