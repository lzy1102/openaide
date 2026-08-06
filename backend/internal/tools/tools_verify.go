package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"openaide/backend/internal/kernel"
)

func verifyToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name: "verify_claim",
				Description: `Verify a claim about the codebase before reporting it as a finding. Call this BEFORE you report "X is missing" or "X is unsafe".
Grep the codebase for the pattern and checks LSP references. Returns a verdict: whether the claim is TRUE or FALSE.`,
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"claim":   map[string]interface{}{"type": "string", "description": "The claim to verify, e.g. 'Planner.Plan has no timeout'"},
						"file":    map[string]interface{}{"type": "string", "description": "The file being analyzed (optional)"},
						"pattern": map[string]interface{}{"type": "string", "description": "Pattern to grep for, e.g. 'WithTimeout', 'context.WithDeadline'"},
					},
					"required": []string{"claim"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "trace_callers",
				Description: "Find all callers of a function using grep. Use this to check if guards/validation exist in the call chain before claiming something is missing.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"function": map[string]interface{}{"type": "string", "description": "The function name to find callers for"},
						"file":     map[string]interface{}{"type": "string", "description": "The file containing the function (optional)"},
					},
					"required": []string{"function"},
				},
			},
		},
	}
}

func handleVerifyClaim(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		Claim   string `json:"claim"`
		File    string `json:"file"`
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	if a.Claim == "" {
		return &kernel.ToolResult{Error: "claim parameter required"}, nil
	}

	var results []string

	// Grep for the pattern
	if a.Pattern != "" {
		cwd, _ := os.Getwd()
		matches := grepCodebase(cwd, a.Pattern)
		if len(matches) > 0 {
			results = append(results, fmt.Sprintf("✅ Pattern '%s' found in %d locations:", a.Pattern, len(matches)))
			for i, m := range matches {
				if i >= 5 {
					results = append(results, fmt.Sprintf("  ... and %d more", len(matches)-5))
					break
				}
				results = append(results, fmt.Sprintf("  %s", m))
			}
		} else {
			results = append(results, fmt.Sprintf("❌ Pattern '%s' NOT found anywhere", a.Pattern))
		}
	}

	// LSP check
	if a.File != "" {
		c := clientForFile(a.File)
		if c != nil {
			locs, err := c.References(a.File, 1, 1)
			if err == nil && len(locs) > 0 {
				results = append(results, fmt.Sprintf("📎 LSP: %d references in %s", len(locs), filepath.Base(a.File)))
			}
		}
	}

	// Verdict
	results = append(results, "")
	switch {
	case len(results) > 1 && strings.HasPrefix(results[0], "✅"):
		results = append(results, fmt.Sprintf("VERDICT: '%s' is FALSE. Pattern exists. Do NOT report.", a.Claim))
	case len(results) > 0 && strings.HasPrefix(results[0], "❌"):
		results = append(results, fmt.Sprintf("VERDICT: '%s' may be TRUE. Report with [MEDIUM] confidence.", a.Claim))
	default:
		results = append(results, fmt.Sprintf("VERDICT: '%s' unverifiable. Mark [LOW] confidence.", a.Claim))
	}

	return &kernel.ToolResult{Content: strings.Join(results, "\n")}, nil
}

func handleTraceCallers(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		Function string `json:"function"`
		File     string `json:"file"`
	}
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if a.Function == "" {
		return &kernel.ToolResult{Error: "function parameter required"}, nil
	}

	cwd, _ := os.Getwd()
	matches := grepCodebase(cwd, a.Function)

	if len(matches) == 0 {
		return &kernel.ToolResult{Content: fmt.Sprintf("Function '%s' not found", a.Function)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found '%s' in %d locations:\n", a.Function, len(matches)))
	for i, m := range matches {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("... and %d more\n", len(matches)-20))
			break
		}
		sb.WriteString(fmt.Sprintf("  %s\n", m))
	}

	if a.File != "" {
		c := clientForFile(a.File)
		if c != nil {
			if locs, err := c.References(a.File, 1, 1); err == nil {
				sb.WriteString(fmt.Sprintf("\nLSP references: %d\n", len(locs)))
				for i, loc := range locs {
					if i >= 10 {
						break
					}
					sb.WriteString(fmt.Sprintf("  %s:%d\n", filepath.Base(loc.URI), loc.Range.Start.Line))
				}
			}
		}
	}

	return &kernel.ToolResult{Content: sb.String()}, nil
}

func grepCodebase(root, pattern string) []string {
	cmd := exec.Command("grep", "-rn", "--include=*.go", pattern, root)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil
	}
	var results []string
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			line = strings.TrimPrefix(line, root+"/")
			if len(line) > 120 {
				line = line[:120] + "..."
			}
			results = append(results, line)
		}
	}
	return results
}
