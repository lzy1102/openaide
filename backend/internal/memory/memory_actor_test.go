package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"openaide/backend/internal/kernel"
)

func TestMemoryActor_SaveLoad(t *testing.T) {
	a, _ := NewMemoryActor(t.TempDir() + "/memory.db")
	defer a.Stop()
	ctx := context.Background()

	err := a.Save(ctx, "session1", []kernel.Message{
		{Role: "user", Content: "hello world"},
		{Role: "assistant", Content: "hi there"},
	})
	if err != nil {
		t.Fatal(err)
	}

	msgs, err := a.Load(ctx, "session1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	// Map iteration is random; check contains
	foundHello, foundHi := false, false
	for _, m := range msgs {
		if m.Content == "hello world" {
			foundHello = true
		}
		if m.Content == "hi there" {
			foundHi = true
		}
	}
	if !foundHello || !foundHi {
		t.Errorf("expected both messages, got %v", msgs)
	}
}

func TestMemoryActor_Search(t *testing.T) {
	a, _ := NewMemoryActor(t.TempDir() + "/memory.db")
	defer a.Stop()
	ctx := context.Background()

	a.Save(ctx, "s1", []kernel.Message{{Role: "user", Content: "deploy to production"}})
	a.Save(ctx, "s1", []kernel.Message{{Role: "user", Content: "fix typo in readme"}})
	a.Save(ctx, "s2", []kernel.Message{{Role: "user", Content: "refactor the kernel"}})

	results, score, err := a.Search(ctx, "deploy", 5)
	if err != nil {
		t.Fatal(err)
	}
	_ = score
	if len(results) < 1 {
		t.Error("expected at least 1 result for 'deploy'")
	}
}

func TestMemoryActor_Concurrent(t *testing.T) {
	a, _ := NewMemoryActor(t.TempDir() + "/memory.db")
	defer a.Stop()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a.Save(ctx, fmt.Sprintf("s%d", n%5), []kernel.Message{
				{Role: "user", Content: fmt.Sprintf("message %d", n)},
			})
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.Load(ctx, "s0", 20)
		}()
	}
	wg.Wait()
	// No deadlock/panic = pass
}

func TestMemoryActor_Compress(t *testing.T) {
	a, _ := NewMemoryActor(t.TempDir() + "/memory.db")
	defer a.Stop()
	ctx := context.Background()

	// Save 25 messages to same session
	for i := 0; i < 25; i++ {
		a.Save(ctx, "full-session", []kernel.Message{
			{Role: "user", Content: fmt.Sprintf("msg %d", i)},
		})
	}

	a.Compress(ctx, "full-session")
	msgs, _ := a.Load(ctx, "full-session", 50)
	if len(msgs) > 20 {
		t.Errorf("expected <=20 after compression, got %d", len(msgs))
	}
}

func TestMemoryActor_LoadLimit(t *testing.T) {
	a, _ := NewMemoryActor(t.TempDir() + "/memory.db")
	defer a.Stop()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		a.Save(ctx, "s", []kernel.Message{{Role: "user", Content: fmt.Sprintf("m%d", i)}})
	}

	msgs, _ := a.Load(ctx, "s", 3)
	if len(msgs) > 3 {
		t.Errorf("expected max 3 with limit, got %d", len(msgs))
	}
}
