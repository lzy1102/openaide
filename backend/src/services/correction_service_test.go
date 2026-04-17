package services

import (
	"context"
	"testing"
	"time"

	"openaide/backend/src/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB 设置测试数据库
func setupTestDBForCorrection(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// 自动迁移
	err = db.AutoMigrate(&models.Thought{}, &models.Correction{})
	assert.NoError(t, err)

	return db
}

// TestEvaluateOutputQuality 测试输出质量评估
func TestEvaluateOutputQuality(t *testing.T) {
	db := setupTestDBForCorrection(t)
	mockLLM := &MockLLMClient{}
	service := NewCorrectionService(db, mockLLM)

	ctx := context.Background()

	// 创建测试请求
	req := &OutputEvaluationRequest{
		OriginalQuery: "什么是人工智能？",
		Output:        "人工智能（AI）是计算机科学的一个分支，致力于创建能够执行通常需要人类智能的任务的系统。",
		ThoughtID:     "test-thought-id",
		UserID:        "test-user-id",
		Model:         "test-model",
	}

	// 执行评估
	resp, err := service.EvaluateOutputQuality(ctx, req)

	// 验证结果
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Greater(t, resp.QualityScore, 0.0)
	assert.Greater(t, resp.Confidence, 0.0)
	assert.NotNil(t, resp.EvaluationDetails)
}

// TestDetectErrors 测试错误检测
func TestDetectErrors(t *testing.T) {
	db := setupTestDBForCorrection(t)
	mockLLM := &MockLLMClient{}
	service := NewCorrectionService(db, mockLLM)

	ctx := context.Background()

	req := &ErrorDetectionRequest{
		Content:       "Python是一种编程语言，发布于2000年。",
		DetectionType: "all",
		Strictness:    3,
		OriginalQuery: "Python是什么时候发布的？",
		Model:         "test-model",
	}

	resp, err := service.DetectErrors(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.GreaterOrEqual(t, resp.TotalIssues, 0)
	assert.GreaterOrEqual(t, resp.CriticalIssues, 0)
	assert.NotEmpty(t, resp.DetectionSummary)
}

// TestGenerateCorrectionSuggestion 测试修正建议生成
func TestGenerateCorrectionSuggestion(t *testing.T) {
	db := setupTestDBForCorrection(t)
	mockLLM := &MockLLMClient{}
	service := NewCorrectionService(db, mockLLM)

	ctx := context.Background()

	// 先创建一个Thought记录用于关联
	thought := &models.Thought{
		ID:      "test-thought-id",
		Content: "原始思考内容",
		UserID:  "test-user-id",
		Status:  "published",
	}
	db.Create(thought)

	req := &CorrectionSuggestionRequest{
		OriginalContent: "Python发布于2000年。",
		Issues: []Issue{
			{
				IssueType:           "factual",
				Severity:            "high",
				Description:         "发布年份错误",
				SuggestedCorrection: "应改为1991年",
				Confidence:          0.95,
			},
		},
		ThoughtID:          "test-thought-id",
		UserID:             "test-user-id",
		CorrectionStrategy: "comprehensive",
		Model:              "test-model",
	}

	resp, err := service.GenerateCorrectionSuggestion(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotEmpty(t, resp.CorrectedContent)
	assert.GreaterOrEqual(t, len(resp.ChangesApplied), 0)
	assert.GreaterOrEqual(t, resp.EstimatedImprovement, 0.0)

	// 验证修正记录被创建
	var corrections []models.Correction
	db.Where("thought_id = ?", "test-thought-id").Find(&corrections)
	assert.Greater(t, len(corrections), 0)
}

// TestAutoCorrectWithValidation 测试自动修正与验证循环
func TestAutoCorrectWithValidation(t *testing.T) {
	db := setupTestDBForCorrection(t)
	mockLLM := &MockLLMClient{}
	service := NewCorrectionService(db, mockLLM)

	ctx := context.Background()

	// 创建Thought记录
	thought := &models.Thought{
		ID:      "test-thought-id",
		Content: "原始思考内容",
		UserID:  "test-user-id",
		Status:  "draft",
	}
	db.Create(thought)

	req := &AutoCorrectionRequest{
		Content:            "原始有问题的内容",
		OriginalQuery:      "测试查询",
		ThoughtID:          "test-thought-id",
		UserID:             "test-user-id",
		MaxRetries:         3,
		QualityThreshold:   80.0,
		CorrectionStrategy: "comprehensive",
		Model:              "test-model",
	}

	resp, err := service.AutoCorrectWithValidation(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.GreaterOrEqual(t, resp.FinalQualityScore, 0.0)
	assert.GreaterOrEqual(t, resp.TotalIterations, 0)
	assert.NotEmpty(t, resp.CorrectionSummary)
}

// TestGetCorrectionHistory 测试获取纠错历史
func TestGetCorrectionHistory(t *testing.T) {
	db := setupTestDBForCorrection(t)
	service := NewCorrectionService(db, nil)

	ctx := context.Background()

	// 创建测试数据
	thought := &models.Thought{
		ID:      "test-thought-id",
		Content: "测试思考内容",
		UserID:  "test-user-id",
		Status:  "published",
	}
	db.Create(thought)

	// 创建多个修正记录
	now := time.Now()
	corrections := []models.Correction{
		{
			ID:        "correction-1",
			ThoughtID: "test-thought-id",
			Content:   "修正1",
			UserID:    "test-user-id",
			Status:    "resolved",
			CreatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        "correction-2",
			ThoughtID: "test-thought-id",
			Content:   "修正2",
			UserID:    "test-user-id",
			Status:    "pending",
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        "correction-3",
			ThoughtID: "test-thought-id",
			Content:   "修正3",
			UserID:    "test-user-id",
			Status:    "resolved",
			CreatedAt: now,
		},
	}

	for _, c := range corrections {
		db.Create(&c)
	}

	req := &CorrectionHistoryRequest{
		ThoughtID: "test-thought-id",
		UserID:    "test-user-id",
		Limit:     10,
	}

	resp, err := service.GetCorrectionHistory(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, int64(3), resp.TotalCount)
	assert.Len(t, resp.HistoryItems, 3)
	assert.NotNil(t, resp.Summary)
	assert.Equal(t, int64(2), resp.Summary.ResolvedCount)
	assert.Equal(t, int64(1), resp.Summary.PendingCount)

	// 验证顺序（最新的在前）
	assert.Equal(t, "correction-3", resp.HistoryItems[0].Correction.ID)
	assert.Equal(t, "correction-1", resp.HistoryItems[2].Correction.ID)
}

// TestExtractJSON 测试JSON提取
func TestExtractJSON(t *testing.T) {
	db := setupTestDBForCorrection(t)
	service := NewCorrectionService(db, nil)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "纯JSON",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON在文本中",
			input:    `这是一个响应 {"key": "value"} 结束`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "嵌套JSON",
			input:    `{"outer": {"inner": "value"}}`,
			expected: `{"outer": {"inner": "value"}}`,
		},
		{
			name:     "JSON包含字符串中的括号",
			input:    `{"text": "这是一个 {括号} 内容"}`,
			expected: `{"text": "这是一个 {括号} 内容"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCountCriticalIssues 测试严重问题统计
func TestCountCriticalIssues(t *testing.T) {
	db := setupTestDBForCorrection(t)
	service := NewCorrectionService(db, nil)

	issues := []Issue{
		{Severity: "critical"},
		{Severity: "high"},
		{Severity: "critical"},
		{Severity: "low"},
	}

	count := service.countCriticalIssues(issues)
	assert.Equal(t, 2, count)
}

// BenchmarkEvaluateOutputQuality 性能测试
func BenchmarkEvaluateOutputQuality(b *testing.B) {
	db := setupTestDBForCorrection(&testing.T{})
	mockLLM := &MockLLMClient{}
	service := NewCorrectionService(db, mockLLM)

	ctx := context.Background()
	req := &OutputEvaluationRequest{
		OriginalQuery: "测试查询",
		Output:        "测试输出内容",
		UserID:        "test-user",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.EvaluateOutputQuality(ctx, req)
	}
}
