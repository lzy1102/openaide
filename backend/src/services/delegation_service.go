package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"openaide/backend/src/services/llm"
)

var (
	broadTaskPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(broad|large|cross.?cutting|end.?to.?end)\b`),
		regexp.MustCompile(`(?i)\bdebug|root.?cause|flaky|failure|regression|bug\b`),
		regexp.MustCompile(`(?i)\breview|audit|assess|validate\b`),
		regexp.MustCompile(`(?i)\bsearch|map|trace|find references|repo.?wide\b`),
		regexp.MustCompile(`(?i)\btest|coverage|verify|qa\b`),
		regexp.MustCompile(`(?i)\brefactor|cleanup|simplif(?:y|ication)\b`),
		regexp.MustCompile(`(?i)\bmigrat(?:e|ion)|upgrade|port\b`),
		regexp.MustCompile(`(?i)\binvestigat(?:e|ion)|analy[sz]e|diagnos(?:e|is)\b`),
	}

	narrowTaskPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\btypo\b`),
		regexp.MustCompile(`(?i)\bcopy\b`),
		regexp.MustCompile(`(?i)\bsingle.?file\b`),
		regexp.MustCompile(`(?i)\bone.?(line|word|sentence|file)\b`),
	}
)

type DelegationMode string

const (
	DelegationNone     DelegationMode = "none"
	DelegationOptional DelegationMode = "optional"
	DelegationAuto     DelegationMode = "auto"
)

type DelegationPlan struct {
	Mode                       DelegationMode
	MaxParallelSubtasks        int
	RequiredParallelProbe      bool
	SpawnBeforeSerialThreshold int
	ChildModelPolicy           string
	SubtaskCandidates          []string
	ChildReportFormat          string
	SkipAllowedReasonRequired  bool
}

type SubtaskResult struct {
	TaskName     string
	Findings     []string
	Model        string
	Duration     time.Duration
	ToolCalls    int
	Error        string
}

type DelegationService struct {
	modelSvc ModelCaller
}

func NewDelegationService(modelSvc ModelCaller) *DelegationService {
	return &DelegationService{modelSvc: modelSvc}
}

func (d *DelegationService) AnalyzeTask(subject, description string) *DelegationPlan {
	text := strings.Join([]string{subject, description}, "\n")

	if isNarrowTask(text) {
		return &DelegationPlan{
			Mode:               DelegationNone,
			MaxParallelSubtasks: 0,
		}
	}

	if isBroadTask(text) {
		return &DelegationPlan{
			Mode:                       DelegationAuto,
			MaxParallelSubtasks:        3,
			RequiredParallelProbe:      true,
			SpawnBeforeSerialThreshold: 3,
			ChildModelPolicy:           "standard",
			SubtaskCandidates:          roleAwareSubtaskCandidates(subject, description),
			ChildReportFormat:          "bullets",
			SkipAllowedReasonRequired:  true,
		}
	}

	return &DelegationPlan{
		Mode:                DelegationOptional,
		MaxParallelSubtasks: 2,
		ChildModelPolicy:    "standard",
	}
}

func (d *DelegationService) ExecuteSubtask(ctx context.Context, taskName, prompt, modelID string) (*SubtaskResult, error) {
	start := time.Now()

	systemPrompt := fmt.Sprintf(`You are a sub-agent performing a focused probe task.

Task: %s

Instructions:
- Focus ONLY on the assigned probe task
- Be thorough but concise
- Report findings as bullet points
- If you cannot complete the task, explain why
- Do NOT make any code changes
- Do NOT use write/modify tools

Provide your findings in this format:
## Findings
- [finding 1]
- [finding 2]
...

## Recommendations
- [recommendation 1]
...`, taskName)

	llmMessages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	resp, err := d.modelSvc.Chat(modelID, llmMessages, nil)
	if err != nil {
		return &SubtaskResult{
			TaskName: taskName,
			Error:    err.Error(),
			Duration: time.Since(start),
		}, err
	}

	content := ""
	if resp != nil && len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	findings := extractBulletPoints(content)

	return &SubtaskResult{
		TaskName:  taskName,
		Findings:  findings,
		Model:     modelID,
		Duration:  time.Since(start),
		ToolCalls: 0,
	}, nil
}

func (d *DelegationService) ExecuteParallelSubtasks(ctx context.Context, candidates []string, contextStr, modelID string) []*SubtaskResult {
	results := make([]*SubtaskResult, len(candidates))

	type indexedResult struct {
		index  int
		result *SubtaskResult
		err    error
	}

	ch := make(chan indexedResult, len(candidates))

	for i, candidate := range candidates {
		go func(idx int, taskName string) {
			prompt := fmt.Sprintf("Context:\n%s\n\nProbe task: %s", contextStr, taskName)
			result, err := d.ExecuteSubtask(ctx, taskName, prompt, modelID)
			ch <- indexedResult{index: idx, result: result, err: err}
		}(i, candidate)
	}

	for i := 0; i < len(candidates); i++ {
		ir := <-ch
		if ir.err != nil {
			results[ir.index] = &SubtaskResult{
				TaskName: candidates[ir.index],
				Error:    ir.err.Error(),
			}
		} else {
			results[ir.index] = ir.result
		}
	}

	return results
}

func (d *DelegationService) SynthesizeResults(results []*SubtaskResult) string {
	var sections []string

	for _, r := range results {
		if r == nil {
			continue
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("### %s", r.TaskName))
		if r.Error != "" {
			lines = append(lines, fmt.Sprintf("⚠ Error: %s", r.Error))
		} else {
			for _, f := range r.Findings {
				lines = append(lines, fmt.Sprintf("- %s", f))
			}
			if r.Duration > 0 {
				lines = append(lines, fmt.Sprintf("_Completed in %.1fs using %s_", r.Duration.Seconds(), r.Model))
			}
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

func isNarrowTask(text string) bool {
	for _, p := range narrowTaskPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

func isBroadTask(text string) bool {
	for _, p := range broadTaskPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

func roleAwareSubtaskCandidates(subject, description string) []string {
	text := strings.Join([]string{subject, description}, "\n")
	var candidates []string

	if regexp.MustCompile(`(?i)debug|root.?cause|flaky|failure|regression|bug|investigat`).MatchString(text) {
		candidates = append(candidates, "Debug/root-cause probe: trace likely failure paths and summarize evidence.")
	}
	if regexp.MustCompile(`(?i)search|map|find references|repo.?wide|investigat`).MatchString(text) {
		candidates = append(candidates, "Repository map probe: find relevant files, symbols, and ownership boundaries.")
	}
	if regexp.MustCompile(`(?i)review|audit|security|quality`).MatchString(text) {
		candidates = append(candidates, "Review probe: inspect risks, edge cases, and contract violations.")
	}
	if regexp.MustCompile(`(?i)test|coverage|verify|qa`).MatchString(text) {
		candidates = append(candidates, "Test probe: identify existing coverage and missing regression checks.")
	}
	if regexp.MustCompile(`(?i)refactor|cleanup|simplif|migrat|upgrade|port`).MatchString(text) {
		candidates = append(candidates, "Change-slice probe: isolate safe implementation slices and migration hazards.")
	}

	if len(candidates) == 0 {
		candidates = append(candidates, "Context probe: map the relevant code paths and summarize recommended next steps.")
	}

	if len(candidates) > 4 {
		candidates = candidates[:4]
	}

	return candidates
}

func extractBulletPoints(text string) []string {
	var points []string
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			point := strings.TrimPrefix(trimmed, "- ")
			point = strings.TrimPrefix(point, "* ")
			point = strings.TrimSpace(point)
			if point != "" {
				points = append(points, point)
			}
		}
	}
	if len(points) == 0 && text != "" {
		chunks := strings.Split(text, "\n\n")
		for _, chunk := range chunks {
			chunk = strings.TrimSpace(chunk)
			if chunk != "" && len(chunk) < 200 {
				points = append(points, chunk)
			}
		}
	}
	return points
}

func (d *DelegationService) findDelegationModel() string {
	if d.modelSvc == nil {
		return ""
	}
	models, err := d.modelSvc.ListModels()
	if err != nil || len(models) == 0 {
		return ""
	}
	for _, m := range models {
		name := strings.ToLower(m.Name)
		if strings.Contains(name, "mini") || strings.Contains(name, "flash") || strings.Contains(name, "fast") {
			return m.ID
		}
	}
	return models[0].ID
}
