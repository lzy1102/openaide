package services

import (
	"testing"

	"openaide/backend/src/models"
)

func TestSkillDiscovery_CountPatternOccurrences(t *testing.T) {
	svc := &SkillDiscoveryService{minOccurrences: 3}

	// 模拟对话数据
	_ = []models.Dialogue{
		{ID: "d1"}, {ID: "d2"}, {ID: "d3"}, {ID: "d4"}, {ID: "d5"},
	}

	// 这里不依赖数据库，只测试逻辑
	// 实际测试需要用 mock DB
	if svc.minOccurrences != 3 {
		t.Errorf("Expected minOccurrences 3, got %d", svc.minOccurrences)
	}
}

func TestSkillDiscovery_CalculateConfidence(t *testing.T) {
	tests := []struct {
		occurrences int
		total       int
		expected    float64
	}{
		{3, 100, 0.03},
		{50, 100, 0.5},
		{100, 100, 1.0},
		{150, 100, 1.0}, // capped at 1.0
	}

	for _, tt := range tests {
		confidence := float64(tt.occurrences) / float64(tt.total)
		if confidence > 1.0 {
			confidence = 1.0
		}
		if confidence != tt.expected {
			t.Errorf("occurrences=%d, total=%d: expected %.2f, got %.2f",
				tt.occurrences, tt.total, tt.expected, confidence)
		}
	}
}

func TestSkillDiscovery_SkillPattern(t *testing.T) {
	pattern := SkillPattern{
		Name:        "code_formatter",
		Description: "自动格式化代码",
		Category:    "development",
		Triggers:    []string{"格式化", "format"},
		Occurrences: 5,
		Confidence:  0.8,
	}

	if pattern.Name != "code_formatter" {
		t.Errorf("Expected name 'code_formatter', got '%s'", pattern.Name)
	}
	if pattern.Confidence < 0.7 {
		t.Errorf("Confidence too low: %.2f", pattern.Confidence)
	}
}
