package actor

import "sync"

// SafeMap is a generic concurrent map with type-safe access.
//
// Use when:
//   - Multiple goroutines read and occasionally write a shared map
//   - Type safety matters (unlike sync.Map which uses interface{})
//   - You need Range/Keys/Values iteration (sync.Map's Range is slow)
//
// Don't use when:
//   - Map is written once during init and only read after (use plain map)
//   - Key space is disjoint across goroutines (use sync.Map)
//   - You need lock-free reads under high write contention (use sync.Map)
//
// Wraps sync.RWMutex + map[K]V. Reads are concurrent, writes are exclusive.
type SafeMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// NewSafeMap creates a SafeMap with the given initial capacity.
func NewSafeMap[K comparable, V any](cap int) *SafeMap[K, V] {
	return &SafeMap[K, V]{m: make(map[K]V, cap)}
}

func (sm *SafeMap[K, V]) Load(k K) (V, bool) {
	sm.mu.RLock()
	v, ok := sm.m[k]
	sm.mu.RUnlock()
	return v, ok
}

func (sm *SafeMap[K, V]) Store(k K, v V) {
	sm.mu.Lock()
	sm.m[k] = v
	sm.mu.Unlock()
}

func (sm *SafeMap[K, V]) Delete(k K) {
	sm.mu.Lock()
	delete(sm.m, k)
	sm.mu.Unlock()
}

func (sm *SafeMap[K, V]) Len() int {
	sm.mu.RLock()
	n := len(sm.m)
	sm.mu.RUnlock()
	return n
}

func (sm *SafeMap[K, V]) Keys() []K {
	sm.mu.RLock()
	keys := make([]K, 0, len(sm.m))
	for k := range sm.m {
		keys = append(keys, k)
	}
	sm.mu.RUnlock()
	return keys
}

func (sm *SafeMap[K, V]) Values() []V {
	sm.mu.RLock()
	vals := make([]V, 0, len(sm.m))
	for _, v := range sm.m {
		vals = append(vals, v)
	}
	sm.mu.RUnlock()
	return vals
}

// Range calls fn for each entry. The callback runs under RLock — keep it fast.
func (sm *SafeMap[K, V]) Range(fn func(K, V) bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for k, v := range sm.m {
		if !fn(k, v) {
			return
		}
	}
}

// LoadOrStore returns the existing value if present, otherwise stores and returns the given value.
func (sm *SafeMap[K, V]) LoadOrStore(k K, v V) (V, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if existing, ok := sm.m[k]; ok {
		return existing, true
	}
	sm.m[k] = v
	return v, false
}
