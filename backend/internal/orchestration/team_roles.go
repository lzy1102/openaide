package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"openaide/backend/core"
)

// GenerateRoles asks the LLM to define custom roles for a task, then sets them on the Team.
// Falls back to defaultRoles() when the LLM is unavailable or fails.
// The LLM sees the task description, available tools, and outputs role definitions.
//
// Before: LLM picked from 4 hardcoded roles (analyst/coder/reviewer/executor)
// After:  LLM defines what roles are needed for THIS specific task
func (t *Team) GenerateRoles(ctx context.Context, task string) {
	if t == nil {
		return
	}

	llm := t.orchestrator.GetLLMProvider()
	if llm == nil {
		t.roles = defaultRoles()
		return
	}

	toolList := t.orchestrator.GetToolDefinitions()
	toolNames := make([]string, len(toolList))
	for i, td := range toolList {
		toolNames[i] = td.Function.Name
	}

	prompt := fmt.Sprintf(`Analyze this task and define the exact roles needed to complete it.

## Task
%s

## Available tools
%s

## Instructions
1. Think about what ROLES this task needs — don't just use "analyst/coder/reviewer".
   Examples: security-auditor, migration-engineer, api-designer,
   test-writer, performance-tuner, devops-deployer, documentation-writer
2. For each role, write a concise prompt describing HOW they work and what to AVOID
3. Assign ONLY the tools each role actually needs — minimal tool set
4. If the task is simple (one concern), define just ONE role

## Output format
Return a JSON object mapping role IDs to role definitions:
{
  "role-id": {
    "name": "Human-readable name",
    "description": "One sentence description",
    "prompt": "How to work + anti-patterns to avoid",
    "tools": ["tool1", "tool2"]
  }
}

Role IDs must be lowercase-kebab-case. Maximum 4 roles.
Reply with ONLY the JSON object, no markdown fences.`, truncForLLM(task, 800), strings.Join(toolNames, ", "))

	resp, err := llm.Chat(ctx, []kernel.Message{
		{Role: "user", Content: prompt},
	}, nil, map[string]interface{}{
		"max_tokens": 1000, "temperature": 0.2, "route": "execution", "no_thinking": true,
	})
	if err != nil {
		slog.Warn("Role generation LLM failed, using defaults", "error", err)
		t.roles = defaultRoles()
		return
	}

	body := strings.TrimSpace(resp.Content)
	body = strings.TrimPrefix(body, "```json")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)

	var raw map[string]struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Prompt      string   `json:"prompt"`
		Tools       []string `json:"tools"`
	}
	if err := json.Unmarshal([]byte(body), &raw); err != nil || len(raw) == 0 {
		slog.Warn("Role generation parse failed, using defaults", "error", err, "body", truncForLLM(body, 100))
		t.roles = defaultRoles()
		return
	}

	t.roles = make(map[string]*TeamRole, len(raw))
	for id, r := range raw {
		t.roles[id] = &TeamRole{
			Name:        r.Name,
			Description: r.Description,
			Prompt:      r.Prompt,
			Tools:       r.Tools,
		}
		slog.Info("LLM-defined role", "id", id, "name", r.Name, "tools", len(r.Tools))
	}
}

// AddRole adds or overwrites a single dynamic role.
func (t *Team) AddRole(id, name, description, prompt string, tools []string) {
	if t.roles == nil {
		t.roles = make(map[string]*TeamRole)
	}
	t.roles[id] = &TeamRole{
		Name:        name,
		Description: description,
		Prompt:      prompt,
		Tools:       tools,
	}
}

func truncForLLM(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
