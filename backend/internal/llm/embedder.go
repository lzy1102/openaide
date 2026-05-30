package llm

import (
	"openaide/backend/internal/kernel"
)

// Embedder is an alias for kernel.Embedder — canonical definition lives there
// to avoid circular import issues (memory, knowledge, etc.).
type Embedder = kernel.Embedder

// NoopEmbedder is an alias for kernel.NoopEmbedder.
type NoopEmbedder = kernel.NoopEmbedder

// CosineSimilarity is an alias for kernel.CosineSimilarity.
var CosineSimilarity = kernel.CosineSimilarity

// EmbedderFunc wraps plain functions into an Embedder.
type EmbedderFunc = kernel.EmbedderFunc

// NewEmbedderFunc creates an EmbedderFunc.
var NewEmbedderFunc = kernel.NewEmbedderFunc
