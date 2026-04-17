package services

import (
	"context"
	"testing"
)

// TestCosineSimilarity 测试余弦相似度计算
func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "相同向量",
			a:        []float64{1, 2, 3},
			b:        []float64{1, 2, 3},
			expected: 1.0,
		},
		{
			name:     "正交向量",
			a:        []float64{1, 0},
			b:        []float64{0, 1},
			expected: 0.0,
		},
		{
			name:     "零向量",
			a:        []float64{0, 0},
			b:        []float64{1, 1},
			expected: 0.0,
		},
		{
			name:     "不同长度",
			a:        []float64{1, 2},
			b:        []float64{1, 2, 3},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			// 允许一定的浮点误差
			if abs(result-tt.expected) > 0.001 {
				t.Errorf("CosineSimilarity() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestEuclideanDistance 测试欧几里得距离计算
func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "相同点",
			a:        []float64{1, 2},
			b:        []float64{1, 2},
			expected: 0.0,
		},
		{
			name:     "不同点",
			a:        []float64{0, 0},
			b:        []float64{3, 4},
			expected: 5.0,
		},
		{
			name:     "一维距离",
			a:        []float64{1},
			b:        []float64{4},
			expected: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EuclideanDistance(tt.a, tt.b)
			if abs(result-tt.expected) > 0.001 {
				t.Errorf("EuclideanDistance() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestDotProduct 测试点积计算
func TestDotProduct(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "常规点积",
			a:        []float64{1, 2, 3},
			b:        []float64{4, 5, 6},
			expected: 32.0, // 1*4 + 2*5 + 3*6 = 4 + 10 + 18 = 32
		},
		{
			name:     "零向量点积",
			a:        []float64{0, 0},
			b:        []float64{1, 1},
			expected: 0.0,
		},
		{
			name:     "负数点积",
			a:        []float64{-1, 2},
			b:        []float64{3, -4},
			expected: -11.0, // -1*3 + 2*-4 = -3 - 8 = -11
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DotProduct(tt.a, tt.b)
			if abs(result-tt.expected) > 0.001 {
				t.Errorf("DotProduct() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// MockEmbeddingService 模拟 Embedding 服务用于测试
type MockEmbeddingService struct {
	mockEmbeddings map[string][]float64
}

func NewMockEmbeddingService() *MockEmbeddingService {
	return &MockEmbeddingService{
		mockEmbeddings: make(map[string][]float64),
	}
}

func (m *MockEmbeddingService) SetMockEmbedding(text string, embedding []float64) {
	m.mockEmbeddings[text] = embedding
}

func (m *MockEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	if embedding, ok := m.mockEmbeddings[text]; ok {
		return embedding, nil
	}
	// 返回一个默认的 mock embedding
	return []float64{0.1, 0.2, 0.3}, nil
}

func (m *MockEmbeddingService) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
	for i, text := range texts {
		embedding, err := m.GenerateEmbedding(ctx, text)
		if err != nil {
			return nil, err
		}
		result[i] = embedding
	}
	return result, nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
