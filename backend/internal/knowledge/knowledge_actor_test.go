package knowledge

import (
	"context"
	"testing"
	"time"
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

// ============ Composite scoring (Generative Agents 2023) ============

func TestCompositeScore(t *testing.T) {
	// High relevance, high importance → should score highest
	d1 := scoredDoc{score: 0.95, weight: 2.0}
	// High relevance, low importance
	d2 := scoredDoc{score: 0.95, weight: 0.5}
	// Low relevance, high importance
	d3 := scoredDoc{score: 0.5, weight: 2.0}

	s1 := d1.compositeScore()
	s2 := d2.compositeScore()
	s3 := d3.compositeScore()

	if s1 <= s2 {
		t.Errorf("high relevance+importance (%.3f) should beat high relevance only (%.3f)", s1, s2)
	}
	if s1 <= s3 {
		t.Errorf("high relevance+importance (%.3f) should beat importance only (%.3f)", s1, s3)
	}

	t.Logf("d1(relevance=0.95, weight=2.0) = %.3f", s1)
	t.Logf("d2(relevance=0.95, weight=0.5) = %.3f", s2)
	t.Logf("d3(relevance=0.50, weight=2.0) = %.3f", s3)
}

func TestDocumentWeight(t *testing.T) {
	a, err := NewActor(t.TempDir() + "/weight.db")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	ctx := context.Background()
	doc, _ := a.Add(ctx, "test", "content goes here", "test", nil)

	// Boost weight
	a.RecordKnowledgeUsage(ctx, []string{doc.ID}, 0.8)
	time.Sleep(10 * time.Millisecond) // wait for actor

	// Search should still work with the weight column
	results, _ := a.Search(ctx, "content", 5)
	if len(results) == 0 {
		t.Error("expected to find document after weight update")
	}
}
