package services

import (
	"testing"
)

func TestUserFeedbackCollector_AnalyzeSatisfaction(t *testing.T) {
	collector := &UserFeedbackCollector{}

	tests := []struct {
		content     string
		expectLow   bool
		expectHigh  bool
	}{
		{"好的，谢谢，非常满意", false, true},
		{"perfect, great job!", false, true},
		{"不对，错了，重新来", true, false},
		{"bad, not good, wrong", true, false},
		{"嗯，还行", false, false}, // neutral
		{"", false, false},         // empty
	}

	for _, tt := range tests {
		score := collector.analyzeSatisfaction(tt.content)
		if tt.expectLow && score >= 0.3 {
			t.Errorf("content='%s': expected low satisfaction, got %.2f", tt.content, score)
		}
		if tt.expectHigh && score < 0.6 {
			t.Errorf("content='%s': expected high satisfaction, got %.2f", tt.content, score)
		}
	}
}

func TestUserFeedbackCollector_NeutralScore(t *testing.T) {
	collector := &UserFeedbackCollector{}
	score := collector.analyzeSatisfaction("")
	if score != 0.5 {
		t.Errorf("Expected neutral score 0.5, got %.2f", score)
	}
}
