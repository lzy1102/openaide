package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDelegationService_AnalyzeTask_Narrow(t *testing.T) {
	svc := NewDelegationService(nil)

	tests := []struct {
		subject string
		mode    DelegationMode
	}{
		{"Fix typo in README", DelegationNone},
		{"Copy this file to backup", DelegationNone},
		{"Update single-file configuration", DelegationNone},
	}

	for _, tt := range tests {
		plan := svc.AnalyzeTask(tt.subject, "")
		assert.Equal(t, tt.mode, plan.Mode, "subject: %s", tt.subject)
		assert.Equal(t, 0, plan.MaxParallelSubtasks)
	}
}

func TestDelegationService_AnalyzeTask_Broad(t *testing.T) {
	svc := NewDelegationService(nil)

	tests := []struct {
		subject string
		mode    DelegationMode
	}{
		{"Debug the flaky test in CI", DelegationAuto},
		{"Review the entire authentication module for security issues", DelegationAuto},
		{"Investigate the root cause of the regression", DelegationAuto},
		{"Refactor the database layer", DelegationAuto},
		{"Migrate from REST to GraphQL", DelegationAuto},
	}

	for _, tt := range tests {
		plan := svc.AnalyzeTask(tt.subject, "")
		assert.Equal(t, tt.mode, plan.Mode, "subject: %s", tt.subject)
		assert.Greater(t, plan.MaxParallelSubtasks, 0)
	}
}

func TestDelegationService_AnalyzeTask_Medium(t *testing.T) {
	svc := NewDelegationService(nil)

	plan := svc.AnalyzeTask("Add a new API endpoint for user registration", "")
	assert.Equal(t, DelegationOptional, plan.Mode)
	assert.Equal(t, 2, plan.MaxParallelSubtasks)
}

func TestDelegationService_RoleAwareSubtaskCandidates(t *testing.T) {
	tests := []struct {
		subject   string
		wantProbe string
	}{
		{"Debug the failing test", "Debug"},
		{"Search for all usages of this function", "Repository map"},
		{"Review the code for security issues", "Review"},
		{"Add test coverage for the auth module", "Test"},
		{"Refactor the legacy codebase", "Change-slice"},
	}

	for _, tt := range tests {
		candidates := roleAwareSubtaskCandidates(tt.subject, "")
		assert.Greater(t, len(candidates), 0, "subject: %s", tt.subject)
		found := false
		for _, c := range candidates {
			if containsStr(c, tt.wantProbe) {
				found = true
				break
			}
		}
		assert.True(t, found, "subject: %s, expected probe containing '%s', got %v", tt.subject, tt.wantProbe, candidates)
	}
}

func TestDelegationService_SynthesizeResults(t *testing.T) {
	svc := NewDelegationService(nil)

	results := []*SubtaskResult{
		{
			TaskName: "Debug probe",
			Findings: []string{"Null pointer in auth handler", "Missing error check"},
			Model:    "gpt-4",
		},
		{
			TaskName: "Repo map probe",
			Findings: []string{"3 files affected", "No tests for auth module"},
			Model:    "gpt-4",
		},
	}

	synthesized := svc.SynthesizeResults(results)
	assert.Contains(t, synthesized, "Debug probe")
	assert.Contains(t, synthesized, "Repo map probe")
	assert.Contains(t, synthesized, "Null pointer in auth handler")
}

func TestDelegationService_SynthesizeResults_WithError(t *testing.T) {
	svc := NewDelegationService(nil)

	results := []*SubtaskResult{
		{
			TaskName: "Debug probe",
			Error:    "model unavailable",
		},
	}

	synthesized := svc.SynthesizeResults(results)
	assert.Contains(t, synthesized, "Error")
	assert.Contains(t, synthesized, "model unavailable")
}

func TestExtractBulletPoints(t *testing.T) {
	text := `## Findings
- First finding
- Second finding
* Third finding with asterisk

## Recommendations
- Do this
- Do that`

	points := extractBulletPoints(text)
	assert.Equal(t, 5, len(points))
	assert.Equal(t, "First finding", points[0])
	assert.Equal(t, "Third finding with asterisk", points[2])
}

func TestExtractBulletPoints_Fallback(t *testing.T) {
	text := "No bullet points here, just plain text paragraphs.\n\nAnother paragraph."

	points := extractBulletPoints(text)
	assert.Greater(t, len(points), 0)
}
