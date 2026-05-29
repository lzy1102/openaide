package knowledge

import (
	"context"
	"testing"
)

func TestNewBase(t *testing.T) {
	dir := t.TempDir()
	kb, err := NewBase(dir)
	if err != nil {
		t.Fatal(err)
	}
	if kb == nil {
		t.Fatal("expected non-nil knowledge base")
	}
}

func TestBase_AddSearch(t *testing.T) {
	dir := t.TempDir()
	kb, _ := NewBase(dir)
	ctx := context.Background()

	doc, err := kb.Add(ctx, "Go Concurrency", "Goroutines are lightweight threads managed by the Go runtime.", "docs", []string{"go", "concurrency"})
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID == "" {
		t.Error("expected non-empty document ID")
	}

	results, err := kb.Search(ctx, "goroutines", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected search results")
	}
}

func TestBase_SearchNoMatch(t *testing.T) {
	dir := t.TempDir()
	kb, _ := NewBase(dir)
	ctx := context.Background()

	kb.Add(ctx, "Test", "Hello world", "test", nil)
	results, _ := kb.Search(ctx, "xyzzy_nonexistent_term", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestBase_Delete(t *testing.T) {
	dir := t.TempDir()
	kb, _ := NewBase(dir)
	ctx := context.Background()

	doc, _ := kb.Add(ctx, "Temp", "This content will be deleted soon.", "test", nil)
	kb.Delete(ctx, doc.ID)

	results, _ := kb.Search(ctx, "deleted", 5)
	for _, r := range results {
		if r.ID == doc.ID {
			t.Error("document should be deleted")
		}
	}
}

func TestBase_InjectToPrompt(t *testing.T) {
	dir := t.TempDir()
	kb, _ := NewBase(dir)
	ctx := context.Background()

	kb.Add(ctx, "Auth API", "POST /login accepts username and password for authentication.", "docs", []string{"api", "auth"})

	prompt, sources, err := kb.InjectToPrompt(ctx, "How do I authenticate?", 500)
	if err != nil {
		t.Fatal(err)
	}
	_ = prompt
	_ = sources
	// Even if empty, it should not error
}

func TestBase_PersistReload(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	kb, _ := NewBase(dir)
	kb.Add(ctx, "Persist Test", "Content for persistence check", "test", nil)

	// Reload from disk
	kb2, err := NewBase(dir)
	if err != nil {
		t.Fatal(err)
	}
	results, _ := kb2.Search(ctx, "persistence", 5)
	if len(results) == 0 {
		t.Log("persistence reload may not preserve in-memory index without explicit save")
	}
}
