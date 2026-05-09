package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"openaide/backend/src/services/llm"
)

const (
	plannerPrompt = `You are a strategic planner. Given a task, create a detailed implementation plan.

Your plan must include:
1. Goal: Clear statement of what needs to be accomplished
2. Phases: Break the work into ordered phases
3. Steps: For each phase, list specific steps with:
   - What to do
   - Which tools to use (read_file, write_file, execute_code, search, web_search, etc.)
   - Expected outcome
4. Dependencies: Which steps depend on others
5. Risks: Potential issues and mitigations

Be specific and actionable. Avoid vague steps like "analyze the code" - instead say "read_file on main.go to understand the entry point".

Task: %s

Context:
%s

Respond in JSON format:
{
  "goal": "...",
  "phases": [
    {
      "name": "...",
      "steps": [
        {
          "action": "...",
          "tools": ["..."],
          "expected_outcome": "...",
          "depends_on": []
        }
      ]
    }
  ],
  "risks": [
    {"description": "...", "mitigation": "..."}
  ]
}`

	architectPrompt = `You are a software architect reviewing a plan. Your job is to:
1. Evaluate if the plan is technically sound
2. Identify missing steps or considerations
3. Suggest better approaches or tool choices
4. Check for potential issues the planner missed

Original task: %s

Proposed plan:
%s

Respond in JSON format:
{
  "score": 0.0-1.0,
  "issues": [
    {"severity": "critical|major|minor", "description": "...", "suggestion": "..."}
  ],
  "missing_steps": ["..."],
  "improved_approach": "..."
}`

	criticPrompt = `You are a critical reviewer. Your job is to find flaws and risks in the plan that others may have missed.

Focus on:
1. Security risks (data exfiltration, credential exposure, destructive operations)
2. Logical errors or incorrect assumptions
3. Edge cases not handled
4. Performance implications
5. Missing error handling

Original task: %s

Plan:
%s

Architect review:
%s

Respond in JSON format:
{
  "approved": true|false,
  "critical_issues": ["..."],
  "warnings": ["..."],
  "suggestions": ["..."],
  "overall_assessment": "safe|risky|dangerous"
}`
)

type ConsensusPhase struct {
	Name  string           `json:"name"`
	Steps []ConsensusStep  `json:"steps"`
}

type ConsensusStep struct {
	Action          string   `json:"action"`
	Tools           []string `json:"tools"`
	ExpectedOutcome string   `json:"expected_outcome"`
	DependsOn       []string `json:"depends_on"`
}

type ConsensusRisk struct {
	Description string `json:"description"`
	Mitigation  string `json:"mitigation"`
}

type ConsensusIssue struct {
	Severity   string `json:"severity"`
	Description string `json:"description"`
	Suggestion string `json:"suggestion"`
}

type ConsensusPlanResult struct {
	Goal           string           `json:"goal"`
	Phases         []ConsensusPhase `json:"phases"`
	Risks          []ConsensusRisk  `json:"risks"`
	ArchitectScore float64          `json:"architect_score"`
	CriticApproved bool             `json:"critic_approved"`
	Iterations     int              `json:"iterations"`
	ElapsedMs      int64            `json:"elapsed_ms"`
}

type architectReview struct {
	Score            float64          `json:"score"`
	Issues           []ConsensusIssue `json:"issues"`
	MissingSteps     []string         `json:"missing_steps"`
	ImprovedApproach string           `json:"improved_approach"`
}

type criticReview struct {
	Approved          bool     `json:"approved"`
	CriticalIssues    []string `json:"critical_issues"`
	Warnings          []string `json:"warnings"`
	Suggestions       []string `json:"suggestions"`
	OverallAssessment string   `json:"overall_assessment"`
}

type ConsensusPlanner struct {
	modelCaller   ModelCaller
	maxIterations int
}

func NewConsensusPlanner(modelCaller ModelCaller) *ConsensusPlanner {
	return &ConsensusPlanner{
		modelCaller:   modelCaller,
		maxIterations: 3,
	}
}

func (cp *ConsensusPlanner) Plan(ctx context.Context, task string, contextStr string) (*ConsensusPlanResult, error) {
	startTime := time.Now()

	var planResult *ConsensusPlanResult
	var archReview *architectReview
	var critReview *criticReview

	for iteration := 1; iteration <= cp.maxIterations; iteration++ {
		slog.Info("Consensus planning iteration", "component", "ConsensusPlanner", "iteration", iteration)

		planJSON, err := cp.runPlanner(ctx, task, contextStr, planResult, archReview, critReview)
		if err != nil {
			slog.Error("Planner failed", "component", "ConsensusPlanner", "error", err)
			if planResult == nil {
				return nil, fmt.Errorf("planner failed: %w", err)
			}
			break
		}

		planResult = planJSON
		planResult.Iterations = iteration

		archReview, err = cp.runArchitect(ctx, task, planResult)
		if err != nil {
			slog.Error("Architect review failed", "component", "ConsensusPlanner", "error", err)
			break
		}
		planResult.ArchitectScore = archReview.Score

		critReview, err = cp.runCritic(ctx, task, planResult, archReview)
		if err != nil {
			slog.Error("Critic review failed", "component", "ConsensusPlanner", "error", err)
			break
		}
		planResult.CriticApproved = critReview.Approved

		if critReview.Approved && archReview.Score >= 0.7 {
			slog.Info("Consensus reached", "component", "ConsensusPlanner",
				"iteration", iteration,
				"architect_score", archReview.Score,
				"critic_approved", critReview.Approved)
			break
		}

		slog.Info("Consensus not reached, refining plan", "component", "ConsensusPlanner",
			"iteration", iteration,
			"architect_score", archReview.Score,
			"critic_approved", critReview.Approved,
			"critical_issues", len(critReview.CriticalIssues))
	}

	if planResult != nil {
		planResult.ElapsedMs = time.Since(startTime).Milliseconds()
	}

	return planResult, nil
}

func (cp *ConsensusPlanner) runPlanner(ctx context.Context, task, contextStr string, prevPlan *ConsensusPlanResult, archReview *architectReview, critReview *criticReview) (*ConsensusPlanResult, error) {
	prompt := fmt.Sprintf(plannerPrompt, task, contextStr)

	if prevPlan != nil && archReview != nil {
		refinement := fmt.Sprintf("\n\n[Previous plan needs refinement]\nArchitect score: %.1f/1.0\n", archReview.Score)
		if len(archReview.Issues) > 0 {
			refinement += "Issues to fix:\n"
			for _, issue := range archReview.Issues {
				refinement += fmt.Sprintf("- [%s] %s → %s\n", issue.Severity, issue.Description, issue.Suggestion)
			}
		}
		if len(archReview.MissingSteps) > 0 {
			refinement += "Missing steps to add:\n"
			for _, step := range archReview.MissingSteps {
				refinement += fmt.Sprintf("- %s\n", step)
			}
		}
		if archReview.ImprovedApproach != "" {
			refinement += fmt.Sprintf("\nSuggested improved approach: %s\n", archReview.ImprovedApproach)
		}
		if critReview != nil {
			if len(critReview.CriticalIssues) > 0 {
				refinement += "\nCritical issues from security review:\n"
				for _, issue := range critReview.CriticalIssues {
					refinement += fmt.Sprintf("- %s\n", issue)
				}
			}
			if len(critReview.Suggestions) > 0 {
				refinement += "\nSuggestions:\n"
				for _, s := range critReview.Suggestions {
					refinement += fmt.Sprintf("- %s\n", s)
				}
			}
		}
		prompt += refinement
	}

	modelID := cp.findPlanningModel()
	resp, err := cp.modelCaller.Chat(modelID, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, map[string]interface{}{"max_tokens": float64(4000), "temperature": float64(0.3)})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from planner LLM")
	}

	content := resp.Choices[0].Message.Content
	content = extractConsensusJSON(content)

	var result ConsensusPlanResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse planner output: %w", err)
	}

	return &result, nil
}

func (cp *ConsensusPlanner) runArchitect(ctx context.Context, task string, plan *ConsensusPlanResult) (*architectReview, error) {
	planJSON, _ := json.MarshalIndent(plan, "", "  ")
	prompt := fmt.Sprintf(architectPrompt, task, string(planJSON))

	modelID := cp.findPlanningModel()
	resp, err := cp.modelCaller.Chat(modelID, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, map[string]interface{}{"max_tokens": float64(3000), "temperature": float64(0.2)})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from architect LLM")
	}

	content := resp.Choices[0].Message.Content
	content = extractConsensusJSON(content)

	var review architectReview
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		return &architectReview{Score: 0.5, Issues: []ConsensusIssue{{Severity: "major", Description: "Failed to parse architect review", Suggestion: "Proceed with caution"}}}, nil
	}

	return &review, nil
}

func (cp *ConsensusPlanner) runCritic(ctx context.Context, task string, plan *ConsensusPlanResult, archReview *architectReview) (*criticReview, error) {
	planJSON, _ := json.MarshalIndent(plan, "", "  ")
	archJSON, _ := json.MarshalIndent(archReview, "", "  ")
	prompt := fmt.Sprintf(criticPrompt, task, string(planJSON), string(archJSON))

	modelID := cp.findPlanningModel()
	resp, err := cp.modelCaller.Chat(modelID, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, map[string]interface{}{"max_tokens": float64(2000), "temperature": float64(0.1)})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from critic LLM")
	}

	content := resp.Choices[0].Message.Content
	content = extractConsensusJSON(content)

	var review criticReview
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		return &criticReview{Approved: false, CriticalIssues: []string{"Failed to parse critic review"}, OverallAssessment: "risky"}, nil
	}

	return &review, nil
}

func (cp *ConsensusPlanner) findPlanningModel() string {
	models, err := cp.modelCaller.ListModels()
	if err != nil || len(models) == 0 {
		return ""
	}

	for _, m := range models {
		for _, tag := range m.Tags {
			if strings.TrimSpace(tag) == "reasoning" || strings.TrimSpace(tag) == "planning" {
				return m.ID
			}
		}
	}

	for _, m := range models {
		if m.Status == "enabled" {
			return m.ID
		}
	}

	return models[0].ID
}

func extractConsensusJSON(content string) string {
	start := strings.Index(content, "{")
	if start < 0 {
		return content
	}

	depth := 0
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}

	return content[start:]
}
