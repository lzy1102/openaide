package actor

import (
	"sync"
	"testing"
)

func BenchmarkActor_Send(b *testing.B) {
	a := NewActor(64)
	defer a.Stop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Send(func() { _ = i + 1 })
	}
}

func BenchmarkActor_SendAsync(b *testing.B) {
	a := NewActor(256)
	defer a.Stop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.SendAsync(func() { _ = i + 1 })
	}
}

func BenchmarkSafeMap_Load(b *testing.B) {
	sm := NewSafeMap[string, int](64)
	sm.Store("key", 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sm.Load("key")
	}
}

func BenchmarkSafeMap_Store(b *testing.B) {
	sm := NewSafeMap[int, int](64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm.Store(i, i)
	}
}

func BenchmarkSyncMap_Load(b *testing.B) {
	var m sync.Map
	m.Store("key", 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Load("key")
	}
}

func BenchmarkSyncMap_Store(b *testing.B) {
	var m sync.Map
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Store(i, i)
	}
}

func BenchmarkRawRWMutex_Load(b *testing.B) {
	var mu sync.RWMutex
	m := map[string]int{"key": 42}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mu.RLock()
		_ = m["key"]
		mu.RUnlock()
	}
}
