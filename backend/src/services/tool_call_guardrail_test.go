package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolCallGuardrail_BeforeCall_Allow(t *testing.T) {
	g := NewToolCallGuardrail(GuardrailConfig{
		WarningsEnabled:          true,
		HardStopEnabled:          true,
		ExactFailureWarnAfter:    2,
		ExactFailureBlockAfter:   5,
		SameToolFailureWarnAfter: 3,
		SameToolFailureHaltAfter: 8,
		NoProgressWarnAfter:      2,
		NoProgressBlockAfter:     5,
	})

	decision := g.BeforeCall("read_file", map[string]interface{}{"path": "/tmp/test.txt"})
	assert.Equal(t, GuardrailAllow, decision.Action)
	assert.True(t, decision.AllowsExecution())
}

func TestToolCallGuardrail_ExactFailureWarning(t *testing.T) {
	g := NewToolCallGuardrail(GuardrailConfig{
		WarningsEnabled:          true,
		HardStopEnabled:          true,
		ExactFailureWarnAfter:    2,
		ExactFailureBlockAfter:   5,
		SameToolFailureWarnAfter: 3,
		SameToolFailureHaltAfter: 8,
		NoProgressWarnAfter:      2,
		NoProgressBlockAfter:     5,
	})

	args := map[string]interface{}{"path": "/tmp/test.txt"}

	g.BeforeCall("read_file", args)
	g.AfterCall("read_file", args, "error: file not found", true)

	g.BeforeCall("read_file", args)
	after := g.AfterCall("read_file", args, "error: file not found", true)
	assert.Equal(t, GuardrailWarn, after.Action)
	assert.Equal(t, "repeated_exact_failure_warning", after.Code)
}

func TestToolCallGuardrail_ExactFailureBlockInBeforeCall(t *testing.T) {
	g := NewToolCallGuardrail(GuardrailConfig{
		WarningsEnabled:          true,
		HardStopEnabled:          true,
		ExactFailureWarnAfter:    2,
		ExactFailureBlockAfter:   3,
		SameToolFailureWarnAfter: 10,
		SameToolFailureHaltAfter: 20,
		NoProgressWarnAfter:      2,
		NoProgressBlockAfter:     5,
	})

	args := map[string]interface{}{"path": "/tmp/test.txt"}

	for i := 0; i < 3; i++ {
		g.BeforeCall("read_file", args)
		g.AfterCall("read_file", args, "error: file not found", true)
	}

	decision := g.BeforeCall("read_file", args)
	assert.Equal(t, GuardrailBlock, decision.Action)
	assert.Equal(t, "repeated_exact_failure_block", decision.Code)
	assert.False(t, decision.AllowsExecution())
}

func TestToolCallGuardrail_MutatingToolNoProgressCheck(t *testing.T) {
	g := NewToolCallGuardrail(GuardrailConfig{
		WarningsEnabled:          true,
		HardStopEnabled:          true,
		ExactFailureWarnAfter:    2,
		ExactFailureBlockAfter:   5,
		SameToolFailureWarnAfter: 3,
		SameToolFailureHaltAfter: 8,
		NoProgressWarnAfter:      2,
		NoProgressBlockAfter:     4,
	})

	args := map[string]interface{}{"path": "/tmp/test.txt", "content": "hello"}

	for i := 0; i < 5; i++ {
		g.BeforeCall("write_file", args)
		after := g.AfterCall("write_file", args, "ok", false)
		assert.Equal(t, GuardrailAllow, after.Action, "mutating tool should not trigger no-progress check")
	}
}

func TestToolCallGuardrail_HaltPreventsFurtherCalls(t *testing.T) {
	g := NewToolCallGuardrail(GuardrailConfig{
		WarningsEnabled:          true,
		HardStopEnabled:          true,
		ExactFailureWarnAfter:    2,
		ExactFailureBlockAfter:   2,
		SameToolFailureWarnAfter: 3,
		SameToolFailureHaltAfter: 8,
		NoProgressWarnAfter:      2,
		NoProgressBlockAfter:     5,
	})

	args := map[string]interface{}{"path": "/tmp/test.txt"}

	g.BeforeCall("read_file", args)
	g.AfterCall("read_file", args, "error", true)

	g.BeforeCall("read_file", args)
	g.AfterCall("read_file", args, "error", true)

	decision := g.BeforeCall("read_file", args)
	assert.Equal(t, GuardrailBlock, decision.Action)
	assert.False(t, decision.AllowsExecution())
}

func TestToolCallGuardrail_ResetForRound(t *testing.T) {
	g := NewToolCallGuardrail(GuardrailConfig{
		WarningsEnabled:          true,
		HardStopEnabled:          true,
		ExactFailureWarnAfter:    2,
		ExactFailureBlockAfter:   2,
		SameToolFailureWarnAfter: 3,
		SameToolFailureHaltAfter: 8,
		NoProgressWarnAfter:      2,
		NoProgressBlockAfter:     5,
	})

	args := map[string]interface{}{"path": "/tmp/test.txt"}
	g.BeforeCall("read_file", args)
	g.AfterCall("read_file", args, "error", true)

	g.BeforeCall("read_file", args)
	g.AfterCall("read_file", args, "error", true)

	g.BeforeCall("read_file", args)
	assert.True(t, g.HasHalted())

	g.ResetForRound()
	assert.False(t, g.HasHalted())
	assert.Empty(t, g.GetWarnings())

	decision := g.BeforeCall("read_file", args)
	assert.Equal(t, GuardrailAllow, decision.Action)
}

func TestToolCallGuardrail_SyntheticResult(t *testing.T) {
	g := NewToolCallGuardrail(DefaultGuardrailConfig())
	decision := &GuardrailDecision{
		Action:   GuardrailBlock,
		Code:     "repeated_exact_failure_block",
		Message:  "read_file failed 5 times with identical args",
		ToolName: "read_file",
		Count:    5,
	}

	result := g.SyntheticResult(decision)
	assert.Contains(t, result, "repeated_exact_failure_block")
	assert.Contains(t, result, "read_file")
	assert.Contains(t, result, "5")
}

func TestToolCallGuardrail_AppendGuardrailGuidance(t *testing.T) {
	result := "Tool execution error: file not found"
	decision := &GuardrailDecision{
		Action:  GuardrailWarn,
		Code:    "repeated_exact_failure_warning",
		Message: "read_file failed 2 times with identical args",
		Count:   2,
	}

	enhanced := AppendGuardrailGuidance(result, decision)
	assert.Contains(t, enhanced, "Tool loop warning")
	assert.Contains(t, enhanced, "repeated_exact_failure_warning")

	allowDecision := &GuardrailDecision{Action: GuardrailAllow}
	unchanged := AppendGuardrailGuidance(result, allowDecision)
	assert.Equal(t, result, unchanged)
}

func TestClassifyToolFailure(t *testing.T) {
	assert.True(t, ClassifyToolFailure("execute_command", `{"exit_code": 1, "stdout": "", "stderr": "failed"}`))
	assert.False(t, ClassifyToolFailure("execute_command", `{"exit_code": 0, "stdout": "ok"}`))
	assert.True(t, ClassifyToolFailure("read_file", `error: file not found`))
	assert.True(t, ClassifyToolFailure("read_file", `{"error": "permission denied"}`))
	assert.False(t, ClassifyToolFailure("read_file", `file content here`))
	assert.False(t, ClassifyToolFailure("read_file", ""))
}

func TestGuardrailConfig_Defaults(t *testing.T) {
	cfg := DefaultGuardrailConfig()
	assert.True(t, cfg.WarningsEnabled)
	assert.False(t, cfg.HardStopEnabled)
	assert.Equal(t, 2, cfg.ExactFailureWarnAfter)
	assert.Equal(t, 5, cfg.ExactFailureBlockAfter)
	assert.Equal(t, 3, cfg.SameToolFailureWarnAfter)
	assert.Equal(t, 8, cfg.SameToolFailureHaltAfter)
}

func TestGuardrailConfig_GetterSetter(t *testing.T) {
	g := NewToolCallGuardrail(DefaultGuardrailConfig())

	assert.True(t, g.GetWarningsEnabled())
	assert.False(t, g.GetHardStopEnabled())

	g.SetHardStopEnabled(true)
	assert.True(t, g.GetHardStopEnabled())

	g.SetWarningsEnabled(false)
	assert.False(t, g.GetWarningsEnabled())
}

func TestIdempotentAndMutatingToolClassification(t *testing.T) {
	assert.True(t, IdempotentTools["read_file"])
	assert.True(t, IdempotentTools["search_code"])
	assert.True(t, IdempotentTools["web_search"])
	assert.False(t, IdempotentTools["write_file"])
	assert.False(t, IdempotentTools["execute_command"])

	assert.True(t, MutatingTools["write_file"])
	assert.True(t, MutatingTools["execute_command"])
	assert.True(t, MutatingTools["delete_file"])
	assert.False(t, MutatingTools["read_file"])
	assert.False(t, MutatingTools["search_code"])
}
