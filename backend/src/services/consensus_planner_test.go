package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsensusPlanResult_Structure(t *testing.T) {
	result := &ConsensusPlanResult{
		Goal: "Implement user auth",
		Phases: []ConsensusPhase{
			{
				Name: "Design",
				Steps: []ConsensusStep{
					{Action: "Design auth schema", Tools: []string{"read_file"}, ExpectedOutcome: "Schema defined"},
				},
			},
			{
				Name: "Implement",
				Steps: []ConsensusStep{
					{Action: "Implement JWT handler", Tools: []string{"write_file"}, ExpectedOutcome: "Handler created"},
				},
			},
		},
		Risks: []ConsensusRisk{
			{Description: "Token expiration handling", Mitigation: "Add refresh token flow"},
		},
		ArchitectScore: 0.85,
		Iterations:     2,
	}

	assert.Equal(t, "Implement user auth", result.Goal)
	assert.Equal(t, 2, len(result.Phases))
	assert.Equal(t, 1, len(result.Risks))
	assert.Equal(t, 0.85, result.ArchitectScore)
	assert.Equal(t, 2, result.Iterations)
}

func TestConsensusPlanResult_Empty(t *testing.T) {
	result := &ConsensusPlanResult{}
	assert.Empty(t, result.Phases)
	assert.Empty(t, result.Risks)
	assert.Equal(t, 0, result.Iterations)
}

func TestConsensusStep_Structure(t *testing.T) {
	step := ConsensusStep{
		Action:          "Read configuration file",
		Tools:           []string{"read_file"},
		ExpectedOutcome: "Config loaded",
		DependsOn:       []string{},
	}

	assert.Equal(t, "Read configuration file", step.Action)
	assert.Equal(t, []string{"read_file"}, step.Tools)
	assert.Equal(t, "Config loaded", step.ExpectedOutcome)
}

func TestConsensusRisk_Structure(t *testing.T) {
	risk := ConsensusRisk{
		Description: "Security vulnerability in token handling",
		Mitigation:  "Use secure token storage",
	}

	assert.Equal(t, "Security vulnerability in token handling", risk.Description)
	assert.Equal(t, "Use secure token storage", risk.Mitigation)
}

func TestConsensusPhase_Structure(t *testing.T) {
	phase := ConsensusPhase{
		Name: "Analysis",
		Steps: []ConsensusStep{
			{Action: "Analyze requirements", Tools: []string{"search"}, ExpectedOutcome: "Requirements understood"},
			{Action: "Review existing code", Tools: []string{"read_file"}, ExpectedOutcome: "Code reviewed"},
		},
	}

	assert.Equal(t, "Analysis", phase.Name)
	assert.Equal(t, 2, len(phase.Steps))
}
