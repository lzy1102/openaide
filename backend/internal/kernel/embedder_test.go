package kernel

import (
	"context"
	"math"
	"testing"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2, 3}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim-1.0) > 0.001 {
		t.Errorf("expected ~1.0, got %f", sim)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim-(-1.0)) > 0.001 {
		t.Errorf("expected ~-1.0, got %f", sim)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	sim := CosineSimilarity([]float32{}, []float32{1})
	if sim != 0 {
		t.Errorf("expected 0 for empty vector, got %f", sim)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	sim := CosineSimilarity([]float32{1, 2}, []float32{1, 2, 3})
	if sim != 0 {
		t.Errorf("expected 0 for different lengths, got %f", sim)
	}
}

func TestCosineSimilarity_ZeroVectors(t *testing.T) {
	sim := CosineSimilarity([]float32{0, 0, 0}, []float32{1, 2, 3})
	if sim != 0 {
		t.Errorf("expected 0 for zero vector, got %f", sim)
	}
}

func TestNoopEmbedder(t *testing.T) {
	e := NoopEmbedder{}
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil || len(vec) != 0 {
		t.Error("expected empty vector, no error")
	}
	if e.Dimension() != 0 {
		t.Errorf("expected dimension 0, got %d", e.Dimension())
	}
}

func TestEmbedderFunc(t *testing.T) {
	e := NewEmbedderFunc(
		func(ctx context.Context, text string) ([]float32, error) {
			return []float32{1.0, 2.0, 3.0}, nil
		},
		nil,
		3,
	)
	vec, err := e.Embed(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 {
		t.Errorf("expected 3 dims, got %d", len(vec))
	}
	batch, err := e.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil || len(batch) != 2 {
		t.Error("expected 2 batch results")
	}
}
