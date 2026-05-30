package compress

import (
	"context"
	"testing"

	"openaide/backend/internal/kernel"
)

func TestNovelCompressor_Compress(t *testing.T) {
	c := NewNovelCompressor()
	messages := []kernel.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "What is Go?"},
		{Role: "assistant", Content: "Go is a programming language created by Google."},
	}

	compressed, saved, err := c.Compress(context.Background(), messages, 10) // very low maxTokens to force compression
	if err != nil {
		t.Fatal(err)
	}
	_ = saved
	if len(compressed) == 0 {
		t.Error("expected non-empty compressed messages")
	}
	// System message should be preserved
	if compressed[0].Role != "system" {
		t.Error("system message should be preserved")
	}
}

func TestNovelCompressor_EstimateTokens(t *testing.T) {
	c := NewNovelCompressor()
	msgs := []kernel.Message{
		{Role: "user", Content: "Hello world, this is a test message with some content."},
	}
	tokens := c.EstimateTokens(msgs)
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}

func TestNovelCompressor_NoMessages(t *testing.T) {
	c := NewNovelCompressor()
	compressed, _, err := c.Compress(context.Background(), []kernel.Message{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) != 0 {
		t.Errorf("expected 0 messages, got %d", len(compressed))
	}
}
