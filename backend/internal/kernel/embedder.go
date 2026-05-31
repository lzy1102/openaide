package kernel

import (
	"context"
	"math"
)

// Embedder is the canonical text vectorization interface.
//
// Defined in kernel (not llm) to break circular import:
//   - llm imports kernel (for Message, ToolDefinition, etc.)
//   - memory + knowledge import kernel (for Actor)
//   - If kernel imported llm → circular dependency
//
// llm.Embedder is a type alias for this interface (see llm/embedder.go).
//
// CosineSimilarity is also defined here for the same reason.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// NoopEmbedder returns empty vectors — used when no embedder is configured.
type NoopEmbedder struct{}

func (NoopEmbedder) Embed(_ context.Context, _ string) ([]float32, error) { return nil, nil }
func (NoopEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) { return nil, nil }
func (NoopEmbedder) Dimension() int { return 0 }

// EmbedderFunc wraps plain functions into an Embedder interface.
type EmbedderFunc struct {
	embedFn      func(ctx context.Context, text string) ([]float32, error)
	embedBatchFn func(ctx context.Context, texts []string) ([][]float32, error)
	dimension    int
}

// NewEmbedderFunc creates an EmbedderFunc. If batch is nil, it batches serially.
func NewEmbedderFunc(
	embedFn func(ctx context.Context, text string) ([]float32, error),
	batchFn func(ctx context.Context, texts []string) ([][]float32, error),
	dim int,
) *EmbedderFunc {
	if batchFn == nil {
		batchFn = func(ctx context.Context, texts []string) ([][]float32, error) {
			result := make([][]float32, len(texts))
			for i, t := range texts {
				v, err := embedFn(ctx, t)
				if err != nil {
					return nil, err
				}
				result[i] = v
			}
			return result, nil
		}
	}
	return &EmbedderFunc{embedFn: embedFn, embedBatchFn: batchFn, dimension: dim}
}

func (f *EmbedderFunc) Embed(ctx context.Context, text string) ([]float32, error) {
	return f.embedFn(ctx, text)
}

func (f *EmbedderFunc) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return f.embedBatchFn(ctx, texts)
}

func (f *EmbedderFunc) Dimension() int { return f.dimension }

// CosineSimilarity computes the cosine similarity between two vectors.
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
