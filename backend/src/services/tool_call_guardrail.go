package services

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

var (
	IdempotentTools = map[string]bool{
		"read_file": true, "search_files": true, "list_directory": true,
		"get_file_info": true, "search_code": true, "grep": true,
		"find": true, "cat": true, "ls": true, "head": true,
		"web_search": true, "web_fetch": true, "web_crawl": true,
		"code_search": true, "file_search": true,
	}

	MutatingTools = map[string]bool{
		"write_file": true, "edit_file": true, "create_file": true,
		"delete_file": true, "move_file": true,
		"execute_command": true, "run_command": true, "shell": true, "bash": true,
		"apply_patch": true, "terminal": true,
	}
)

type GuardrailConfig struct {
	WarningsEnabled            bool
	HardStopEnabled            bool
	ExactFailureWarnAfter      int
	ExactFailureBlockAfter     int
	SameToolFailureWarnAfter   int
	SameToolFailureHaltAfter   int
	NoProgressWarnAfter        int
	NoProgressBlockAfter       int
}

func DefaultGuardrailConfig() GuardrailConfig {
	return GuardrailConfig{
		WarningsEnabled:            true,
		HardStopEnabled:            false,
		ExactFailureWarnAfter:      2,
		ExactFailureBlockAfter:     5,
		SameToolFailureWarnAfter:   3,
		SameToolFailureHaltAfter:   8,
		NoProgressWarnAfter:        2,
		NoProgressBlockAfter:       5,
	}
}

type GuardrailAction string

const (
	GuardrailAllow GuardrailAction = "allow"
	GuardrailWarn  GuardrailAction = "warn"
	GuardrailBlock GuardrailAction = "block"
	GuardrailHalt  GuardrailAction = "halt"
)

type GuardrailDecision struct {
	Action   GuardrailAction
	Code     string
	Message  string
	ToolName string
	Count    int
}

func (d *GuardrailDecision) AllowsExecution() bool {
	return d.Action == GuardrailAllow || d.Action == GuardrailWarn
}

func (d *GuardrailDecision) ShouldHalt() bool {
	return d.Action == GuardrailBlock || d.Action == GuardrailHalt
}

type guardrailSignature struct {
	ToolName string
	ArgsHash string
}

func newGuardrailSignature(toolName string, args map[string]interface{}) guardrailSignature {
	canonical := canonicalArgs(args)
	h := sha256.Sum256([]byte(canonical))
	return guardrailSignature{
		ToolName: toolName,
		ArgsHash: fmt.Sprintf("%x", h[:8]),
	}
}

func canonicalArgs(args map[string]interface{}) string {
	if args == nil {
		return "{}"
	}
	b, _ := json.Marshal(args)
	return string(b)
}

type ToolCallGuardrail struct {
	mu     sync.Mutex
	config GuardrailConfig

	exactFailureCounts      map[guardrailSignature]int
	sameToolFailureCounts   map[string]int
	noProgress              map[guardrailSignature]noProgressEntry
	haltDecision            *GuardrailDecision
	roundIndex              int
	warnings                []string
}

type noProgressEntry struct {
	resultHash  string
	repeatCount int
}

func NewToolCallGuardrail(config GuardrailConfig) *ToolCallGuardrail {
	return &ToolCallGuardrail{
		config:                config,
		exactFailureCounts:    make(map[guardrailSignature]int),
		sameToolFailureCounts: make(map[string]int),
		noProgress:            make(map[guardrailSignature]noProgressEntry),
	}
}

func (g *ToolCallGuardrail) ResetForRound() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.exactFailureCounts = make(map[guardrailSignature]int)
	g.sameToolFailureCounts = make(map[string]int)
	g.noProgress = make(map[guardrailSignature]noProgressEntry)
	g.haltDecision = nil
	g.roundIndex++
	g.warnings = nil
}

func (g *ToolCallGuardrail) BeforeCall(toolName string, args map[string]interface{}) *GuardrailDecision {
	g.mu.Lock()
	defer g.mu.Unlock()

	sig := newGuardrailSignature(toolName, args)

	if g.haltDecision != nil {
		return &GuardrailDecision{
			Action:   GuardrailBlock,
			Code:     "halt_already_triggered",
			Message:  g.haltDecision.Message,
			ToolName: toolName,
		}
	}

	if !g.config.HardStopEnabled {
		return &GuardrailDecision{Action: GuardrailAllow, ToolName: toolName}
	}

	exactCount := g.exactFailureCounts[sig]
	if exactCount >= g.config.ExactFailureBlockAfter {
		decision := &GuardrailDecision{
			Action:   GuardrailBlock,
			Code:     "repeated_exact_failure_block",
			Message:  fmt.Sprintf("Blocked %s: same call failed %d times with identical args. Change strategy instead of retrying.", toolName, exactCount),
			ToolName: toolName,
			Count:    exactCount,
		}
		g.haltDecision = decision
		return decision
	}

	if g.isIdempotent(toolName) {
		if entry, ok := g.noProgress[sig]; ok {
			if entry.repeatCount >= g.config.NoProgressBlockAfter {
				decision := &GuardrailDecision{
					Action:   GuardrailBlock,
					Code:     "idempotent_no_progress_block",
					Message:  fmt.Sprintf("Blocked %s: this read-only call returned the same result %d times. Use the result already provided.", toolName, entry.repeatCount),
					ToolName: toolName,
					Count:    entry.repeatCount,
				}
				g.haltDecision = decision
				return decision
			}
		}
	}

	return &GuardrailDecision{Action: GuardrailAllow, ToolName: toolName}
}

func (g *ToolCallGuardrail) AfterCall(toolName string, args map[string]interface{}, result string, failed bool) *GuardrailDecision {
	g.mu.Lock()
	defer g.mu.Unlock()

	sig := newGuardrailSignature(toolName, args)

	if failed {
		exactCount := g.exactFailureCounts[sig] + 1
		g.exactFailureCounts[sig] = exactCount
		delete(g.noProgress, sig)

		sameCount := g.sameToolFailureCounts[toolName] + 1
		g.sameToolFailureCounts[toolName] = sameCount

		if g.config.HardStopEnabled && sameCount >= g.config.SameToolFailureHaltAfter {
			decision := &GuardrailDecision{
				Action:   GuardrailHalt,
				Code:     "same_tool_failure_halt",
				Message:  fmt.Sprintf("Stopped %s: it failed %d times this round. Choose a different approach.", toolName, sameCount),
				ToolName: toolName,
				Count:    sameCount,
			}
			g.haltDecision = decision
			return decision
		}

		if g.config.WarningsEnabled && exactCount >= g.config.ExactFailureWarnAfter {
			msg := fmt.Sprintf("%s failed %d times with identical args. This looks like a loop — change strategy.", toolName, exactCount)
			g.warnings = append(g.warnings, msg)
			return &GuardrailDecision{
				Action:   GuardrailWarn,
				Code:     "repeated_exact_failure_warning",
				Message:  msg,
				ToolName: toolName,
				Count:    exactCount,
			}
		}

		if g.config.WarningsEnabled && sameCount >= g.config.SameToolFailureWarnAfter {
			msg := fmt.Sprintf("%s has failed %d times this round. Change approach before retrying.", toolName, sameCount)
			g.warnings = append(g.warnings, msg)
			return &GuardrailDecision{
				Action:   GuardrailWarn,
				Code:     "same_tool_failure_warning",
				Message:  msg,
				ToolName: toolName,
				Count:    sameCount,
			}
		}

		return &GuardrailDecision{Action: GuardrailAllow, ToolName: toolName, Count: exactCount}
	}

	delete(g.exactFailureCounts, sig)
	delete(g.sameToolFailureCounts, toolName)

	if !g.isIdempotent(toolName) {
		delete(g.noProgress, sig)
		return &GuardrailDecision{Action: GuardrailAllow, ToolName: toolName}
	}

	resultHash := hashResult(result)
	if entry, ok := g.noProgress[sig]; ok {
		if entry.resultHash == resultHash {
			entry.repeatCount++
			g.noProgress[sig] = entry

			if g.config.WarningsEnabled && entry.repeatCount >= g.config.NoProgressWarnAfter {
				msg := fmt.Sprintf("%s returned the same result %d times. Use the result already provided or change the query.", toolName, entry.repeatCount)
				g.warnings = append(g.warnings, msg)
				return &GuardrailDecision{
					Action:   GuardrailWarn,
					Code:     "idempotent_no_progress_warning",
					Message:  msg,
					ToolName: toolName,
					Count:    entry.repeatCount,
				}
			}
		} else {
			g.noProgress[sig] = noProgressEntry{resultHash: resultHash, repeatCount: 1}
		}
	} else {
		g.noProgress[sig] = noProgressEntry{resultHash: resultHash, repeatCount: 1}
	}

	return &GuardrailDecision{Action: GuardrailAllow, ToolName: toolName}
}

func (g *ToolCallGuardrail) GetWarningsEnabled() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.config.WarningsEnabled
}

func (g *ToolCallGuardrail) GetHardStopEnabled() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.config.HardStopEnabled
}

func (g *ToolCallGuardrail) SetWarningsEnabled(v bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.config.WarningsEnabled = v
}

func (g *ToolCallGuardrail) SetHardStopEnabled(v bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.config.HardStopEnabled = v
}

func (g *ToolCallGuardrail) GetWarnings() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	result := make([]string, len(g.warnings))
	copy(result, g.warnings)
	return result
}

func (g *ToolCallGuardrail) HasHalted() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.haltDecision != nil
}

func (g *ToolCallGuardrail) SyntheticResult(decision *GuardrailDecision) string {
	result := map[string]interface{}{
		"error":     decision.Message,
		"guardrail": map[string]interface{}{
			"action":    string(decision.Action),
			"code":      decision.Code,
			"tool_name": decision.ToolName,
			"count":     decision.Count,
		},
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func AppendGuardrailGuidance(result string, decision *GuardrailDecision) string {
	if decision.Action != GuardrailWarn && decision.Action != GuardrailHalt {
		return result
	}
	if decision.Message == "" {
		return result
	}
	label := "Tool loop warning"
	if decision.Action == GuardrailHalt {
		label = "Tool loop hard stop"
	}
	return result + fmt.Sprintf("\n\n[%s: %s; count=%d; %s]", label, decision.Code, decision.Count, decision.Message)
}

func (g *ToolCallGuardrail) isIdempotent(toolName string) bool {
	if MutatingTools[toolName] {
		return false
	}
	return IdempotentTools[toolName]
}

func hashResult(result string) string {
	h := sha256.Sum256([]byte(result))
	return fmt.Sprintf("%x", h[:8])
}

func ClassifyToolFailure(toolName string, result string) bool {
	if result == "" {
		return false
	}

	if toolName == "execute_command" || toolName == "run_command" || toolName == "shell" || toolName == "bash" {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(result), &data); err == nil {
			if exitCode, ok := data["exit_code"].(float64); ok && exitCode != 0 {
				return true
			}
		}
	}

	lower := strings.ToLower(result[:guardrailMin(500, len(result))])
	if strings.Contains(lower, `"error"`) || strings.Contains(lower, `"failed"`) || strings.HasPrefix(lower, "error") {
		return true
	}

	return false
}

func guardrailMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
