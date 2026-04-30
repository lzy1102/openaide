package storage

import (
	"context"
	"testing"
)

func TestMemoryVectorStore_CreateAndListCollections(t *testing.T) {
	store := NewMemoryVectorStore()

	err := store.CreateCollection("test_collection", 3)
	if err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	collections, err := store.ListCollections()
	if err != nil {
		t.Fatalf("failed to list collections: %v", err)
	}
	if len(collections) != 1 || collections[0] != "test_collection" {
		t.Fatalf("expected [test_collection], got %v", collections)
	}
}

func TestMemoryVectorStore_InsertAndSearch(t *testing.T) {
	store := NewMemoryVectorStore()
	ctx := context.Background()

	err := store.CreateCollection("test", 3)
	if err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	docs := []VectorDocument{
		{ID: "1", Content: "hello world", Embedding: []float32{1.0, 0.0, 0.0}, Metadata: map[string]interface{}{"tag": "greeting"}},
		{ID: "2", Content: "goodbye world", Embedding: []float32{0.0, 1.0, 0.0}, Metadata: map[string]interface{}{"tag": "farewell"}},
		{ID: "3", Content: "hello again", Embedding: []float32{0.9, 0.1, 0.0}, Metadata: map[string]interface{}{"tag": "greeting"}},
	}

	for _, doc := range docs {
		if err := store.Insert(ctx, "test", doc); err != nil {
			t.Fatalf("failed to insert doc %s: %v", doc.ID, err)
		}
	}

	results, err := store.Search(ctx, "test", []float32{1.0, 0.0, 0.0}, 2)
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Document.ID != "1" {
		t.Fatalf("expected first result to be doc 1, got %s", results[0].Document.ID)
	}
}

func TestMemoryVectorStore_Delete(t *testing.T) {
	store := NewMemoryVectorStore()
	ctx := context.Background()

	store.CreateCollection("test", 3)
	store.Insert(ctx, "test", VectorDocument{
		ID: "1", Embedding: []float32{1.0, 0.0, 0.0},
	})

	count, _ := store.Count(ctx, "test")
	if count != 1 {
		t.Fatalf("expected 1 doc, got %d", count)
	}

	store.Delete(ctx, "test", "1")
	count, _ = store.Count(ctx, "test")
	if count != 0 {
		t.Fatalf("expected 0 docs after delete, got %d", count)
	}
}

func TestMemoryVectorStore_DeleteCollection(t *testing.T) {
	store := NewMemoryVectorStore()

	store.CreateCollection("test", 3)
	store.DeleteCollection("test")

	collections, _ := store.ListCollections()
	if len(collections) != 0 {
		t.Fatalf("expected 0 collections after delete, got %d", len(collections))
	}
}

func TestMemoryVectorStore_GetDocument(t *testing.T) {
	store := NewMemoryVectorStore()
	ctx := context.Background()

	store.CreateCollection("test", 3)
	store.Insert(ctx, "test", VectorDocument{
		ID: "1", Content: "hello", Embedding: []float32{1.0, 0.0, 0.0},
	})

	doc, err := store.GetDocument(ctx, "test", "1")
	if err != nil {
		t.Fatalf("failed to get document: %v", err)
	}
	if doc.Content != "hello" {
		t.Fatalf("expected content 'hello', got '%s'", doc.Content)
	}
}

func TestMemoryVectorStore_SearchWithFilter(t *testing.T) {
	store := NewMemoryVectorStore()
	ctx := context.Background()

	store.CreateCollection("test", 3)
	store.Insert(ctx, "test", VectorDocument{
		ID: "1", Embedding: []float32{1.0, 0.0, 0.0}, Metadata: map[string]interface{}{"tag": "greeting"},
	})
	store.Insert(ctx, "test", VectorDocument{
		ID: "2", Embedding: []float32{0.9, 0.1, 0.0}, Metadata: map[string]interface{}{"tag": "farewell"},
	})

	results, err := store.SearchWithFilter(ctx, "test", []float32{1.0, 0.0, 0.0}, 10, map[string]interface{}{"tag": "greeting"})
	if err != nil {
		t.Fatalf("failed to search with filter: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(results))
	}
	if results[0].Document.ID != "1" {
		t.Fatalf("expected doc 1, got %s", results[0].Document.ID)
	}
}

func TestMemoryVectorStore_DuplicateCollection(t *testing.T) {
	store := NewMemoryVectorStore()

	store.CreateCollection("test", 3)
	err := store.CreateCollection("test", 3)
	if err == nil {
		t.Fatal("expected error for duplicate collection")
	}
}

func TestMemoryVectorStore_CollectionNotFound(t *testing.T) {
	store := NewMemoryVectorStore()
	ctx := context.Background()

	_, err := store.Search(ctx, "nonexistent", []float32{1.0, 0.0, 0.0}, 5)
	if err == nil {
		t.Fatal("expected error for nonexistent collection")
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		filter   map[string]interface{}
		expected bool
	}{
		{"empty filter", map[string]interface{}{"key": "val"}, map[string]interface{}{}, true},
		{"empty metadata", map[string]interface{}{}, map[string]interface{}{"key": "val"}, false},
		{"match", map[string]interface{}{"key": "val"}, map[string]interface{}{"key": "val"}, true},
		{"no match", map[string]interface{}{"key": "val"}, map[string]interface{}{"key": "other"}, false},
		{"missing key", map[string]interface{}{"key": "val"}, map[string]interface{}{"missing": "val"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesFilter(tt.metadata, tt.filter)
			if result != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
