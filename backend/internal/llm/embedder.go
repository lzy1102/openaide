package llm

import (
	"context"
	"math"
)

// Embedder 文本向量化接口
// 所有需要语义搜索的组件（记忆、知识库）依赖此接口，而非具体实现
type Embedder interface {
	// Embed 将单段文本转为向量
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch 批量转换，实现方可利用 batching 优化
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimension 返回向量维度（用于索引初始化/兼容性检查）
	Dimension() int
}

// EmbedderFunc 将普通函数适配为 Embedder 接口
type EmbedderFunc struct {
	embedFn     func(ctx context.Context, text string) ([]float32, error)
	embedBatchFn func(ctx context.Context, texts []string) ([][]float32, error)
	dimension   int
}

func NewEmbedderFunc(
	embed func(ctx context.Context, text string) ([]float32, error),
	batch func(ctx context.Context, texts []string) ([][]float32, error),
	dim int,
) *EmbedderFunc {
	if batch == nil {
		batch = func(ctx context.Context, texts []string) ([][]float32, error) {
			result := make([][]float32, len(texts))
			for i, t := range texts {
				v, err := embed(ctx, t)
				if err != nil {
					return nil, err
				}
				result[i] = v
			}
			return result, nil
		}
	}
	return &EmbedderFunc{embedFn: embed, embedBatchFn: batch, dimension: dim}
}

func (f *EmbedderFunc) Embed(ctx context.Context, text string) ([]float32, error) {
	return f.embedFn(ctx, text)
}

func (f *EmbedderFunc) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return f.embedBatchFn(ctx, texts)
}

func (f *EmbedderFunc) Dimension() int { return f.dimension }

// NoopEmbedder 总是返回空向量和 nil error——用于 embedder 未配置时
// 调用方应检查维度是否为 0 并回退到文本搜索
type NoopEmbedder struct{}

func (NoopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, nil
}

func (NoopEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

func (NoopEmbedder) Dimension() int { return 0 }

// ============ 工具函数 ============

// CosineSimilarity 计算两个向量的余弦相似度
// 返回 [-1, 1]，值越大越相似
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		va := float64(a[i])
		vb := float64(b[i])
		dot += va * vb
		normA += va * va
		normB += vb * vb
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
