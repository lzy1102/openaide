package kernel

import (
	"sync"
	"testing"
	"time"
)

func TestActor_SendSync(t *testing.T) {
	a := NewActor(8)
	defer a.Stop()

	var result int
	a.Send(func() { result = 42 })
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestActor_SendAsync(t *testing.T) {
	a := NewActor(8)
	defer a.Stop()

	var result int
	done := make(chan struct{})
	a.SendAsync(func() {
		result = 42
		close(done)
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for async command")
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestActor_StopDrainsCommands(t *testing.T) {
	a := NewActor(8)

	var count int
	for i := 0; i < 10; i++ {
		a.SendAsync(func() { count++ })
	}
	a.Stop()
	if count != 10 {
		t.Errorf("expected all 10 commands to execute on stop, got %d", count)
	}
}

func TestActor_Concurrent(t *testing.T) {
	a := NewActor(64)
	defer a.Stop()

	var counter int
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.Send(func() { counter++ })
		}()
	}
	wg.Wait()
	if counter != 100 {
		t.Errorf("expected 100, got %d", counter)
	}
}

func TestActorStore_Basics(t *testing.T) {
	s := NewActorStore[string](8)
	defer s.Stop()

	s.Set("a", "alpha")
	s.Set("b", "beta")

	v, ok := s.Get("a")
	if !ok || v != "alpha" {
		t.Errorf("expected 'alpha', got '%s'", v)
	}

	if s.Len() != 2 {
		t.Errorf("expected len 2, got %d", s.Len())
	}

	s.Delete("a")
	if s.Len() != 1 {
		t.Errorf("expected len 1 after delete, got %d", s.Len())
	}

	keys := s.Keys()
	if len(keys) != 1 || keys[0] != "b" {
		t.Errorf("expected ['b'], got %v", keys)
	}
}

func TestActorStore_Values(t *testing.T) {
	s := NewActorStore[int](8)
	defer s.Stop()
	s.Set("x", 1)
	s.Set("y", 2)
	vals := s.Values()
	if len(vals) != 2 {
		t.Errorf("expected 2 values, got %d", len(vals))
	}
}

func TestActorStore_Concurrent(t *testing.T) {
	s := NewActorStore[int](32)
	defer s.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Set(string(rune('A'+n%26)), n)
		}(i)
	}
	wg.Wait()
	if s.Len() == 0 {
		t.Error("expected non-zero length after concurrent writes")
	}
}
