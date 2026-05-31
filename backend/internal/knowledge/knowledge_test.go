package knowledge

import (
	"context"
	"testing"
)

func TestActor_Create(t *testing.T) {
	a, err := NewActor(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if a == nil {
		t.Fatal("expected non-nil actor")
	}
}

func TestActor_AddSearch(t *testing.T) {
	a, _ := NewActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	doc, err := a.Add(ctx, "Go Concurrency", "Goroutines are lightweight threads.", "docs", []string{"go"})
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID == "" {
		t.Error("expected non-empty document ID")
	}

	results, err := a.Search(ctx, "goroutines", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected search results")
	}
}

func TestActor_SearchNoMatch(t *testing.T) {
	a, _ := NewActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	a.Add(ctx, "Test", "Hello world", "test", nil)
	results, _ := a.Search(ctx, "xyzzynonexistent", 5)
	// Without embedder, all documents match with a base score
	if len(results) == 0 {
		t.Error("expected at least 1 result (no embedder = base score for all)")
	}
}

func TestActor_Delete(t *testing.T) {
	a, _ := NewActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	doc, _ := a.Add(ctx, "Temp", "To be deleted", "test", nil)
	a.Delete(ctx, doc.ID)

	got := a.Get(ctx, doc.ID)
	if got != nil {
		t.Error("document should be deleted")
	}
}

func TestActor_PersistReload(t *testing.T) {
	path := t.TempDir() + "/test.db"
	ctx := context.Background()

	a1, _ := NewActor(path)
	a1.Add(ctx, "Persist", "Content that survives restart", "test", nil)
	a1.Stop()

	a2, _ := NewActor(path)
	defer a2.Stop()
	results, _ := a2.Search(ctx, "survives", 5)
	if len(results) == 0 {
		t.Error("expected persisted data to survive restart")
	}
}
