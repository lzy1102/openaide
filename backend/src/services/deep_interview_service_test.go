package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeepInterviewService_ShouldInterview(t *testing.T) {
	svc := NewDeepInterviewService(nil)

	assert.True(t, svc.ShouldInterview("fix it", ""))
	assert.True(t, svc.ShouldInterview("help me with this", ""))
	assert.True(t, svc.ShouldInterview("refactor the code", ""))
	assert.True(t, svc.ShouldInterview("improve the performance", ""))
	assert.False(t, svc.ShouldInterview("Read the file /tmp/test.txt and show me the contents", ""))
	assert.False(t, svc.ShouldInterview("List all files in the current directory", ""))
}

func TestDeepInterviewService_ShouldInterview_WithShortQuery(t *testing.T) {
	svc := NewDeepInterviewService(nil)

	assert.True(t, svc.ShouldInterview("fix it", ""))
	assert.True(t, svc.ShouldInterview("help", ""))
}

func TestDeepInterviewService_ShouldInterview_WithDetailedQuery(t *testing.T) {
	svc := NewDeepInterviewService(nil)

	detailed := "I need to implement a REST API endpoint at /api/v1/users that supports GET, POST, PUT, DELETE methods with JWT authentication, input validation, and PostgreSQL database integration using the existing User Model"
	assert.False(t, svc.ShouldInterview(detailed, ""))
}

func TestInformationGap_Structure(t *testing.T) {
	gap := InformationGap{
		Topic:             "target framework",
		Question:          "Which web framework are you using?",
		Priority:          "critical",
		DefaultAssumption: "Using standard library",
		Answer:            "Gin framework",
		Answered:          true,
	}

	assert.Equal(t, "target framework", gap.Topic)
	assert.Equal(t, "critical", gap.Priority)
	assert.True(t, gap.Answered)
}

func TestInterviewAnalysis_Structure(t *testing.T) {
	analysis := InterviewAnalysis{
		Understood: []string{"User wants to build a web API"},
		Gaps: []InformationGap{
			{Topic: "framework", Question: "Which framework?", Priority: "critical"},
			{Topic: "database", Question: "Which database?", Priority: "important"},
		},
		ReadinessScore: 0.4,
		Recommendation: "Need more information about framework and database choice",
	}

	assert.Equal(t, 2, len(analysis.Gaps))
	assert.Less(t, analysis.ReadinessScore, 0.5)
}

func TestInterviewSynthesis_Structure(t *testing.T) {
	synthesis := InterviewSynthesis{
		CompleteContext: "User wants to build a REST API with Gin framework and PostgreSQL",
		RemainingGaps:  []string{},
		Confidence:     0.9,
		ReadyToProceed: true,
	}

	assert.True(t, synthesis.ReadyToProceed)
	assert.Empty(t, synthesis.RemainingGaps)
	assert.GreaterOrEqual(t, synthesis.Confidence, 0.8)
}
