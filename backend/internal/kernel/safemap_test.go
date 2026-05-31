package kernel

import (
	"sync"
	"testing"
)

func TestSafeMap_LoadStore(t *testing.T) {
	m := NewSafeMap[string, int](8)
	m.Store("a", 1)
	v, ok := m.Load("a")
	if !ok || v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestSafeMap_Delete(t *testing.T) {
	m := NewSafeMap[string, int](8)
	m.Store("a", 1)
	m.Delete("a")
	_, ok := m.Load("a")
	if ok {
		t.Error("expected key to be deleted")
	}
}

func TestSafeMap_Len(t *testing.T) {
	m := NewSafeMap[string, int](8)
	if m.Len() != 0 {
		t.Error("expected 0")
	}
	m.Store("a", 1)
	m.Store("b", 2)
	if m.Len() != 2 {
		t.Errorf("expected 2, got %d", m.Len())
	}
}

func TestSafeMap_Keys(t *testing.T) {
	m := NewSafeMap[string, int](8)
	m.Store("a", 1)
	m.Store("b", 2)
	keys := m.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestSafeMap_Range(t *testing.T) {
	m := NewSafeMap[string, int](8)
	m.Store("a", 1)
	m.Store("b", 2)
	sum := 0
	m.Range(func(k string, v int) bool {
		sum += v
		return true
	})
	if sum != 3 {
		t.Errorf("expected sum 3, got %d", sum)
	}
}

func TestSafeMap_RangeEarlyExit(t *testing.T) {
	m := NewSafeMap[string, int](8)
	m.Store("a", 1)
	m.Store("b", 2)
	m.Store("c", 3)
	count := 0
	m.Range(func(k string, v int) bool {
		count++
		return count < 2 // stop after 2
	})
	if count != 2 {
		t.Errorf("expected 2 iterations, got %d", count)
	}
}

func TestSafeMap_LoadOrStore(t *testing.T) {
	m := NewSafeMap[string, int](8)
	v, loaded := m.LoadOrStore("a", 1)
	if loaded || v != 1 {
		t.Error("expected not loaded and value 1")
	}
	v, loaded = m.LoadOrStore("a", 2)
	if !loaded || v != 1 {
		t.Error("expected loaded and original value 1")
	}
}

func TestSafeMap_Concurrent(t *testing.T) {
	m := NewSafeMap[int, int](32)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.Store(n, n*2)
		}(i)
	}
	wg.Wait()
	if m.Len() != 100 {
		t.Errorf("expected 100 entries, got %d", m.Len())
	}
}
