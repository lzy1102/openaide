package llm

import (
	"testing"

	"openaide/backend/internal/kernel"
)

func TestPromptCache_SetGet(t *testing.T) {
	c := NewPromptCache("unused")
	defer c.Shutdown()

	msgs := []kernel.Message{{Role: "user", Content: "hello"}}
	resp := &kernel.LLMResponse{Content: "hi there", Model: "test"}

	c.Set(msgs, nil, "test-model", resp)

	got := c.Get(msgs, nil, "test-model")
	if got == nil {
		t.Fatal("expected cache hit")
	}
	if got.Content != "hi there" {
		t.Errorf("expected 'hi there', got '%s'", got.Content)
	}
}

func TestPromptCache_Miss(t *testing.T) {
	c := NewPromptCache("unused")
	defer c.Shutdown()

	got := c.Get([]kernel.Message{{Role: "user", Content: "x"}}, nil, "m")
	if got != nil {
		t.Error("expected cache miss")
	}
}

func TestPromptCache_DifferentKey(t *testing.T) {
	c := NewPromptCache("unused")
	defer c.Shutdown()

	c.Set([]kernel.Message{{Role: "user", Content: "a"}}, nil, "m", &kernel.LLMResponse{Content: "x"})
	got := c.Get([]kernel.Message{{Role: "user", Content: "b"}}, nil, "m")
	if got != nil {
		t.Error("expected miss for different messages")
	}
}

func TestPromptCache_Stats(t *testing.T) {
	c := NewPromptCache("unused")
	defer c.Shutdown()

	c.Set([]kernel.Message{{Role: "user", Content: "q"}}, nil, "m", &kernel.LLMResponse{Content: "a"})
	c.Get([]kernel.Message{{Role: "user", Content: "q"}}, nil, "m")     // hit
	c.Get([]kernel.Message{{Role: "user", Content: "other"}}, nil, "m") // miss

	stats := c.Stats()
	if stats["entries"] != 1 {
		t.Errorf("expected 1 entry, got %d", stats["entries"])
	}
	if stats["hits"] != 1 {
		t.Errorf("expected 1 hit, got %d", stats["hits"])
	}
	if stats["misses"] != 1 {
		t.Errorf("expected 1 miss, got %d", stats["misses"])
	}
}
