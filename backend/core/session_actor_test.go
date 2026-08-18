package kernel

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSessionActor_Create(t *testing.T) {
	a, err := NewSessionActor(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	ctx := context.Background()
	s, err := a.Create(ctx, "proj", "user")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if s.ProjectID != "proj" || s.UserID != "user" {
		t.Errorf("expected proj/user, got %s/%s", s.ProjectID, s.UserID)
	}
}

func TestSessionActor_GetSet(t *testing.T) {
	a, _ := NewSessionActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	s, _ := a.Create(ctx, "proj", "user")
	s.Messages = []Message{{Role: "user", Content: "hello"}}
	a.Update(ctx, s)

	got, err := a.Get(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "hello" {
		t.Errorf("expected 'hello', got %v", got.Messages)
	}
	if got.ID != s.ID {
		t.Errorf("expected ID %s, got %s", s.ID, got.ID)
	}
}

func TestSessionActor_List(t *testing.T) {
	a, _ := NewSessionActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a.Create(ctx, "proj", "user")
	}

	list, err := a.List(ctx, "proj", "user", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 5 {
		t.Errorf("expected 5 sessions, got %d", len(list))
	}

	// Pagination
	page, _ := a.List(ctx, "proj", "user", 2, 0)
	if len(page) != 2 {
		t.Errorf("expected 2 sessions (paginated), got %d", len(page))
	}
}

func TestSessionActor_Delete(t *testing.T) {
	a, _ := NewSessionActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	s, _ := a.Create(ctx, "proj", "user")
	a.Delete(ctx, s.ID)

	_, err := a.Get(ctx, s.ID)
	if err == nil {
		t.Error("expected error for deleted session")
	}
}

func TestSessionActor_Concurrent(t *testing.T) {
	a, _ := NewSessionActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// 20 concurrent writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s, err := a.Create(ctx, "proj", fmt.Sprintf("user%d", n%3))
			if err != nil {
				errors <- err
				return
			}
			s.Messages = []Message{{Role: "user", Content: fmt.Sprintf("msg%d", n)}}
			if err := a.Update(ctx, s); err != nil {
				errors <- err
			}
		}(i)
	}

	// 10 concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			list, err := a.List(ctx, "proj", "user0", 50, 0)
			if err != nil {
				errors <- err
			}
			_ = len(list)
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error("concurrent error:", err)
	}

	count := a.Count(ctx)
	if count < 20 {
		t.Errorf("expected at least 20 sessions, got %d", count)
	}
}

func TestSessionActor_Cleanup(t *testing.T) {
	a, _ := NewSessionActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	s, _ := a.Create(ctx, "proj", "user")
	// Manually set update time to 8 days ago
	a.super.Send(func() {
		a.db.ExecContext(ctx, `UPDATE sessions SET updated_at = ? WHERE id = ?`,
			time.Now().Add(-8*24*time.Hour).Format(time.RFC3339), s.ID)
	})

	n, err := a.CleanupOldSessions(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 cleaned up, got %d", n)
	}
	if a.Count(ctx) != 0 {
		t.Errorf("expected 0 remaining, got %d", a.Count(ctx))
	}
}

func TestSessionActor_Search(t *testing.T) {
	a, _ := NewSessionActor(t.TempDir() + "/test.db")
	defer a.Stop()
	ctx := context.Background()

	s, _ := a.Create(ctx, "proj", "user")
	s.Messages = []Message{{Role: "user", Content: "deploy the production server"}}
	a.Update(ctx, s)

	results, err := a.Search(ctx, "proj", "deploy", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}
	if results[0].ID != s.ID {
		t.Errorf("expected ID %s, got %s", s.ID, results[0].ID)
	}
}

func TestSessionActor_Persistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/sessions.db"

	// Create, write, close
	a1, _ := NewSessionActor(dbPath)
	ctx := context.Background()
	s, _ := a1.Create(ctx, "proj", "user")
	sid := s.ID
	a1.Stop()

	// Reopen, verify data survived
	a2, _ := NewSessionActor(dbPath)
	defer a2.Stop()
	got, err := a2.Get(ctx, sid)
	if err != nil {
		t.Fatal("session lost after restart:", err)
	}
	if got.ID != sid {
		t.Errorf("expected ID %s, got %s", sid, got.ID)
	}
}
