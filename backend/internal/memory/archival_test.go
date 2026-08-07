package memory

import (
	"context"
	"testing"
	"time"

	"openaide/backend/internal/kernel"
)

func TestArchiveConversation(t *testing.T) {
	a, err := NewMemoryActor(t.TempDir() + "/archive.db")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	ctx := context.Background()
	msgs := []kernel.Message{
		{Role: "user", Content: "fix login bug"},
		{Role: "assistant", Content: "fixed in auth/service.go by updating token validation"},
	}
	err = a.ArchiveConversation(ctx, "s1", "Fixed login bug in auth/service.go", msgs, 0.8)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRetrieveArchive(t *testing.T) {
	a, err := NewMemoryActor(t.TempDir() + "/retrieve.db")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	ctx := context.Background()

	// Archive two conversations
	a.ArchiveConversation(ctx, "s1", "Fixed login bug in auth", []kernel.Message{
		{Role: "user", Content: "login broken"},
		{Role: "assistant", Content: "fixed token validation"},
	}, 0.9)

	a.ArchiveConversation(ctx, "s2", "Added unit tests for handler", []kernel.Message{
		{Role: "user", Content: "add tests"},
		{Role: "assistant", Content: "created handler_test.go"},
	}, 0.6)

	time.Sleep(50 * time.Millisecond)

	// Retrieve should find login-related archive
	msgs, score, err := a.RetrieveArchive(ctx, "login bug", 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Retrieved %d messages, best score: %.3f", len(msgs), score)

	// Retrieve with no matching query should still work
	msgs2, _, err := a.RetrieveArchive(ctx, "zzz_nonexistent_zzz", 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("No-match retrieve: %d messages", len(msgs2))
}

func TestStoreCoreFact(t *testing.T) {
	a, err := NewMemoryActor(t.TempDir() + "/facts.db")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	a.SetRetriever(newMemRetriever())

	ctx := context.Background()
	a.StoreCoreFact(ctx, "Token validation happens in middleware/token.go, not in the login handler", 0.9)
	a.StoreCoreFact(ctx, "Use bcrypt.CompareHashAndPassword, never compare password strings directly", 1.0)

	time.Sleep(50 * time.Millisecond)

	facts := a.GetCoreFacts(ctx, "token", 3)
	if len(facts) == 0 {
		t.Error("expected core facts")
	}
	t.Logf("Core facts: %v", facts)
}

func TestMemoryManager_Integration(t *testing.T) {
	a, err := NewMemoryActor(t.TempDir() + "/integ.db")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	a.SetRetriever(newMemRetriever())

	ctx := context.Background()

	// Simulate a complete MemGPT-style workflow:
	// 1. Store core fact
	a.StoreCoreFact(ctx, "Auth module: service/auth.go, middleware/token.go, handlers/login.go", 0.8)

	// 2. Archive a completed conversation
	a.ArchiveConversation(ctx, "s1", "Resolved token expiration issue", []kernel.Message{
		{Role: "user", Content: "token expired error"},
		{Role: "assistant", Content: "token expiry check was using wrong timezone"},
	}, 0.7)

	time.Sleep(50 * time.Millisecond)

	// 3. Recall core facts
	facts := a.GetCoreFacts(ctx, "auth", 5)
	if len(facts) == 0 {
		t.Error("expected auth facts")
	}

	// 4. Retrieve archive
	msgs, _, err := a.RetrieveArchive(ctx, "token expiration", 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Integration test: %d facts, %d archived messages", len(facts), len(msgs))
}
