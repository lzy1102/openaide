package knowledge

import (
	"context"
	"testing"
)

func TestActor_CreateAndStop(t *testing.T) {
	a, err := NewActor(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	a.Stop()
}

func TestActor_AddGet(t *testing.T) {
	a, _ := NewActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	id, err := a.AddKnowledge(ctx, "Test Title", "Test content for knowledge", "unit-test", []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("expected non-empty knowledge ID")
	}

	doc := a.Get(ctx, id)
	if doc == nil {
		t.Fatal("expected document, got nil")
	}
	if doc.Title != "Test Title" {
		t.Errorf("expected 'Test Title', got '%s'", doc.Title)
	}
}

func TestActor_AddSearchDelete(t *testing.T) {
	a, _ := NewActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	a.AddKnowledge(ctx, "Alpha", "First document", "test", []string{"tag1"})
	a.AddKnowledge(ctx, "Beta", "Second document", "test", []string{"tag2"})
	a.AddKnowledge(ctx, "Gamma", "Third document", "test", []string{"tag1", "tag2"})

	docs, err := a.Search(ctx, "Second", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) < 1 {
		t.Error("expected at least 1 search result")
	}

	// Delete first
	for _, d := range docs {
		a.Delete(ctx, d.ID)
	}

	// Verify deleted
	for _, d := range docs {
		if got := a.Get(ctx, d.ID); got != nil {
			t.Errorf("expected nil after delete for %s", d.ID)
		}
	}
}

func TestActor_Refine(t *testing.T) {
	a, _ := NewActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	// Refine without LLM should store directly
	id := a.Refine(ctx, "How do I deploy?", "Deploy using docker-compose up -d", "test-session")
	if id == "" {
		t.Error("expected non-empty refine result")
	}

	// Search should find it
	results, _ := a.Search(ctx, "deploy", 5)
	if len(results) == 0 {
		t.Error("expected to find refined knowledge")
	}
}

func TestActor_IncrementalAdd(t *testing.T) {
	a, _ := NewActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		a.AddKnowledge(ctx, "Doc", "Content", "test", nil)
	}
	// Should not crash — tests LRU eviction path
	results, _ := a.Search(ctx, "Content", 20)
	if len(results) < 1 {
		t.Error("expected results")
	}
}
