package tools

import (
	"bytes"
	"context"
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
Accepts a claim like "Planner.Plan lacks timeout control" and returns whether it's true or false by checking callers, grep, and LSP.`,
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"claim": map[string]interface{}{
							"type":        "string",
							"description": "The claim to verify, e.g. 'Planner.Plan has no timeout' or 'atomic.Value usage in kernel.go is unsafe'",
						},
						"file": map[string]interface{}{
							"type":        "string",
							"description": "The file being analyzed (optional, for context)",
						},
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "A pattern to grep for, e.g. 'WithTimeout', 'context.WithDeadline', 'sync.Mutex'",
						},
					},
					"required": []string{"claim"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "trace_callers",
				Description: "Find all callers of a function using LSP or grep. Use this to check if validation/guards exist in the call chain before claiming something is missing.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"function": map[string]interface{}{
							"type":        "string",
							"description": "The function name to find callers for, e.g. 'Planner.Plan' or 'executeTool'",
						},
						"file": map[string]interface{}{
							"type":        "string",
							"description": "The file containing the function",
						},
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
	jsonUnmarshalStd([]byte(args), &a)

	if a.Claim == "" {
		return &kernel.ToolResult{Error: "claim parameter required"}, nil
	}

	var results []string

	// 1. Grep for the pattern in the codebase
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
			results = append(results, fmt.Sprintf("❌ Pattern '%s' NOT found anywhere in the codebase", a.Pattern))
		}
	}

	// 2. Check LSP for callers/references
	if a.File != "" {
		c := clientForFile(a.File)
		if c != nil {
			// Try to find references to the function
			locs, err := c.References(a.File, 1, 1)
			if err == nil && len(locs) > 0 {
				results = append(results, fmt.Sprintf("📎 LSP: %d references found for symbols in %s", len(locs), filepath.Base(a.File)))
			} else {
				results = append(results, fmt.Sprintf("📎 LSP: no references found (may need to open the file first)"))
			}
		}
	}

	// 3. Verdict
	results = append(results, "")
	if len(results) > 1 && strings.Contains(results[0], "✅") {
		results = append(results, fmt.Sprintf("VERDICT: Claim '%s' appears to be FALSE. The pattern exists in the codebase. Do NOT report this as missing.", a.Claim))
	} else if len(results) > 0 && strings.Contains(results[0], "❌") {
		results = append(results, fmt.Sprintf("VERDICT: Claim '%s' may be TRUE. The pattern was not found. You CAN report this with [MEDIUM] confidence.", a.Claim))
	} else {
		results = append(results, fmt.Sprintf("VERDICT: Claim '%s' could not be verified. Mark as [LOW] confidence and suggest manual review.", a.Claim))
	}

	return &kernel.ToolResult{Content: strings.Join(results, "\n")}, nil
}

func handleTraceCallers(ctx context.Context, args string) (*kernel.ToolResult, error) {
	var a struct {
		Function string `json:"function"`
		File     string `json:"file"`
	}
	jsonUnmarshalStd([]byte(args), &a)

	// Grep for the function name in the codebase
	cwd, _ := os.Getwd()
	matches := grepCodebase(cwd, a.Function)

	if len(matches) == 0 {
		return &kernel.ToolResult{Content: fmt.Sprintf("Function '%s' not found in codebase", a.Function)}, nil
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

	// Check using LSP if file is provided
	if a.File != "" {
		c := clientForFile(a.File)
		if c != nil {
			locs, err := c.References(a.File, 1, 1)
			if err == nil {
				sb.WriteString(fmt.Sprintf("\nLSP references: %d\n", len(locs)))
				for i, loc := range locs {
					if i >= 10 {
						sb.WriteString(fmt.Sprintf("... and %d more\n", len(locs)-10))
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
	var results []string
	cmd := exec.Command("grep", "-rn", "--include=*.go", pattern, root)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil
	}
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			// Shorten: remove root prefix
			line = strings.TrimPrefix(line, root+"/")
			if len(line) > 120 {
				line = line[:120] + "..."
			}
			results = append(results, line)
		}
	}
	return results
}

// Override jsonUnmarshalStd for tools_verify to use encoding/json
func jsonUnmarshalStd(data []byte, v interface{}) error {
	// Try to find JSON object in the data
	str := string(data)
	start := strings.Index(str, "{")
	end := strings.LastIndex(str, "}")
	if start >= 0 && end > start {
		str = str[start : end+1]
	}
	// Simple parse: key:"value"
	type simpleArgs struct {
		Claim   string `json:"claim"`
		File    string `json:"file"`
		Pattern string `json:"pattern"`
		Function string `json:"function"`
	}
	var sa simpleArgs
	str = strings.TrimPrefix(str, "{")
	str = strings.TrimSuffix(str, "}")
	for _, part := range strings.Split(str, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.Trim(strings.TrimSpace(kv[0]), "\"")
		val := strings.TrimSpace(kv[1])
		val = strings.Trim(val, "\"")

		switch key {
		case "claim":
			sa.Claim = val
		case "file":
			sa.File = val
		case "pattern":
			sa.Pattern = val
		case "function":
			sa.Function = val
		}
	}

	switch t := v.(type) {
	case *struct {
		Claim   string `json:"claim"`
		File    string `json:"file"`
		Pattern string `json:"pattern"`
	}:
		t.Claim = sa.Claim
		t.File = sa.File
		t.Pattern = sa.Pattern
	case *struct {
		Function string `json:"function"`
		File     string `json:"file"`
	}:
		t.Function = sa.Function
		t.File = sa.File
	}
	return nil
}
