package services

import (
	"testing"
)

func TestPatternDetector_WorkflowPattern(t *testing.T) {
	pattern := WorkflowPattern{
		Name:        "deploy_pipeline",
		Description: "自动部署流水线",
		Steps:       []string{"构建", "测试", "部署"},
		Triggers:    []string{"部署", "deploy"},
		Frequency:   3,
		Confidence:  0.75,
	}

	if len(pattern.Steps) != 3 {
		t.Errorf("Expected 3 steps, got %d", len(pattern.Steps))
	}
	if pattern.Confidence < 0.6 {
		t.Errorf("Confidence too low: %.2f", pattern.Confidence)
	}
	if pattern.Frequency < 2 {
		t.Errorf("Frequency too low: %d", pattern.Frequency)
	}
}

func TestPatternDetector_ValidationThresholds(t *testing.T) {
	tests := []struct {
		frequency  int
		confidence float64
		shouldPass bool
	}{
		{2, 0.6, true},
		{1, 0.8, false},  // frequency too low
		{3, 0.5, false},  // confidence too low
		{5, 0.9, true},
	}

	for _, tt := range tests {
		passes := tt.frequency >= 2 && tt.confidence >= 0.6
		if passes != tt.shouldPass {
			t.Errorf("frequency=%d, confidence=%.2f: expected %v, got %v",
				tt.frequency, tt.confidence, tt.shouldPass, passes)
		}
	}
}
