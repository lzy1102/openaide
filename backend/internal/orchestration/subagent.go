package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"openaide/backend/internal/kernel"
)

// RunSubAgent creates an isolated session and runs a single role with restricted tools.
// Only the result summary is returned — intermediate tool calls stay in the sub-session.
func (o *Orchestrator) RunSubAgent(ctx context.Context, userID, projectID, roleName, task string, previousResults []string) (string, error) {
	role := o.getTeamRole(roleName)
	if role == nil {
		return "", fmt.Errorf("unknown role: %s", roleName)
	}

	// Build role-specific system prompt
	systemPrompt := fmt.Sprintf("%s\n\nTask: %s", role.Prompt, task)
	if len(previousResults) > 0 {
		systemPrompt += "\n\nPrevious results:\n" + strings.Join(previousResults, "\n---\n")
	}

	// Create isolated sub-session
	subUserID := fmt.Sprintf("%s-sub-%s-%d", userID, roleName, time.Now().UnixNano())
	session, err := o.sessions.Create(ctx, projectID, subUserID)
	if err != nil {
		return "", fmt.Errorf("create sub-session: %w", err)
	}

	// Build messages with role context
	messages := []kernel.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task},
	}

	// Use execution model for coder/executor, reasoning model for analyst/reviewer
	modelRoute := pickModel(roleName)
	slog.Info("Sub-agent started", "role", roleName, "model_route", modelRoute, "session", session.ID[:min(8, len(session.ID))])

	// Run ReAct loop in the sub-session
	resp, err := o.llmGateway.Chat(ctx, messages, o.toolExec.GetDefinitions(), map[string]interface{}{
		"route":        modelRoute,
		"max_tokens":   4000,
		"temperature":  0.3,
	})
	if err != nil {
		return "", fmt.Errorf("sub-agent %s: %w", roleName, err)
	}

	// Clean up sub-session
	o.sessions.Delete(ctx, session.ID)

	return resp.Content, nil
}

func (o *Orchestrator) getTeamRole(roleName string) *TeamRole {
	if o.team == nil {
		return nil
	}
	return o.team.GetRole(roleName)
}

func pickModel(roleName string) string {
	switch roleName {
	case "analyst", "reviewer":
		return "reasoning"
	case "coder", "executor", "synthesis":
		return "execution"
	default:
		return "execution"
	}
}

// groupByDependency groups subtasks so that independent tasks run in parallel
// and dependent tasks run sequentially.
func groupByDependency(subtasks []SubTask) [][]SubTask {
	if len(subtasks) == 0 {
		return nil
	}
	completed := make(map[int]bool)
	remaining := make(map[int]SubTask)
	for _, st := range subtasks {
		remaining[st.ID] = st
	}

	var groups [][]SubTask
	for len(remaining) > 0 {
		var group []SubTask
		for id, st := range remaining {
			ready := true
			for _, depID := range st.DependsOn {
				if !completed[depID] {
					ready = false
					break
				}
			}
			if ready {
				group = append(group, st)
				delete(remaining, id)
			}
		}
		if len(group) == 0 {
			// Cycle detected — break by adding all remaining
			for _, st := range remaining {
				group = append(group, st)
			}
			remaining = nil
		}
		groups = append(groups, group)
		for _, st := range group {
			completed[st.ID] = true
		}
	}
	return groups
}
