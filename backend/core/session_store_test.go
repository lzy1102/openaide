package kernel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSessionStore_CreateAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewFileSessionStore failed: %v", err)
	}

	ctx := context.Background()
	session, err := store.Create(ctx, "proj1", "user1")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if session.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if session.ProjectID != "proj1" {
		t.Errorf("expected proj1, got %s", session.ProjectID)
	}

	got, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != session.ID {
		t.Error("session ID mismatch")
	}

	// Verify file on disk
	path := filepath.Join(dir, session.ID+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("session file not found on disk")
	}
}

func TestFileSessionStore_Update(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileSessionStore(dir)
	ctx := context.Background()

	session, _ := store.Create(ctx, "proj1", "user1")
	session.Messages = append(session.Messages, Message{Role: "user", Content: "hello"})

	if err := store.Update(ctx, session); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := store.Get(ctx, session.ID)
	if len(got.Messages) != 1 || got.Messages[0].Content != "hello" {
		t.Error("update did not persist messages")
	}
}

func TestFileSessionStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileSessionStore(dir)
	ctx := context.Background()

	session, _ := store.Create(ctx, "proj1", "user1")

	if err := store.Delete(ctx, session.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := store.Get(ctx, session.ID); err == nil {
		t.Error("expected error after delete")
	}

	path := filepath.Join(dir, session.ID+".json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("session file should be deleted")
	}
}

func TestFileSessionStore_Delete_NonExistent(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileSessionStore(dir)

	err := store.Delete(context.Background(), "nonexistent-id")
	if err != nil {
		t.Errorf("delete non-existent should succeed, got: %v", err)
	}
}

func TestFileSessionStore_List(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileSessionStore(dir)
	ctx := context.Background()

	store.Create(ctx, "proj1", "user1")
	store.Create(ctx, "proj1", "user1")
	store.Create(ctx, "proj2", "user1")

	t.Run("all sessions", func(t *testing.T) {
		sessions, err := store.List(ctx, "", "", 0, 0)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(sessions) != 3 {
			t.Errorf("expected 3 sessions, got %d", len(sessions))
		}
	})

	t.Run("filter by project", func(t *testing.T) {
		sessions, _ := store.List(ctx, "proj1", "", 0, 0)
		if len(sessions) != 2 {
			t.Errorf("expected 2 sessions for proj1, got %d", len(sessions))
		}
	})

	t.Run("filter by user", func(t *testing.T) {
		sessions, _ := store.List(ctx, "", "user1", 0, 0)
		if len(sessions) != 3 {
			t.Errorf("expected 3 sessions for user1, got %d", len(sessions))
		}
	})
}

func TestFileSessionStore_List_WithOffset(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileSessionStore(dir)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		store.Create(ctx, "proj1", "user1")
	}

	t.Run("limit", func(t *testing.T) {
		sessions, _ := store.List(ctx, "proj1", "user1", 3, 0)
		if len(sessions) != 3 {
			t.Errorf("expected 3, got %d", len(sessions))
		}
	})

	t.Run("offset", func(t *testing.T) {
		sessions, _ := store.List(ctx, "proj1", "user1", 3, 5)
		if len(sessions) != 3 {
			t.Errorf("expected 3, got %d", len(sessions))
		}
	})

	t.Run("offset beyond count", func(t *testing.T) {
		sessions, _ := store.List(ctx, "proj1", "user1", 10, 20)
		if len(sessions) != 0 {
			t.Errorf("expected 0, got %d", len(sessions))
		}
	})

	t.Run("limit zero returns all", func(t *testing.T) {
		sessions, _ := store.List(ctx, "proj1", "user1", 0, 0)
		if len(sessions) != 10 {
			t.Errorf("expected 10, got %d", len(sessions))
		}
	})
}

func TestFileSessionStore_Recover(t *testing.T) {
	dir := t.TempDir()

	store1, _ := NewFileSessionStore(dir)
	session, _ := store1.Create(context.Background(), "proj1", "user1")
	store1 = nil

	store2, err := NewFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewFileSessionStore failed: %v", err)
	}

	got, err := store2.Get(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	if got.ID != session.ID {
		t.Error("session ID mismatch after recovery")
	}
}

func TestSessionStoreAdapter_Delete(t *testing.T) {
	store := NewSessionStoreAdapter()
	ctx := context.Background()

	session, _ := store.Create(ctx, "proj1", "user1")

	if err := store.Delete(ctx, session.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.Get(ctx, session.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestSessionStoreAdapter_List_WithOffset(t *testing.T) {
	store := NewSessionStoreAdapter()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		store.Create(ctx, "proj1", "user1")
	}

	sessions, _ := store.List(ctx, "proj1", "user1", 2, 2)
	if len(sessions) != 2 {
		t.Errorf("expected 2, got %d", len(sessions))
	}
}
