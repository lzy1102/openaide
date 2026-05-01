package storage

import (
	"testing"
	"time"
)

func TestMemoryCache_BasicOperations(t *testing.T) {
	cache := NewMemoryCache(5*time.Minute, 10*time.Minute)

	cache.Set("key1", "value1", 0)
	val, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if val != "value1" {
		t.Fatalf("expected value1, got %v", val)
	}

	cache.Delete("key1")
	_, ok = cache.Get("key1")
	if ok {
		t.Fatal("expected key1 to be deleted")
	}
}

func TestMemoryCache_Expiration(t *testing.T) {
	cache := NewMemoryCache(5*time.Minute, 10*time.Minute)

	cache.Set("key1", "value1", 100*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	_, ok := cache.Get("key1")
	if ok {
		t.Fatal("expected key1 to be expired")
	}
}

func TestMemoryCache_Flush(t *testing.T) {
	cache := NewMemoryCache(5*time.Minute, 10*time.Minute)

	cache.Set("key1", "value1", 0)
	cache.Set("key2", "value2", 0)
	cache.Flush()

	if cache.ItemCount() != 0 {
		t.Fatalf("expected 0 items after flush, got %d", cache.ItemCount())
	}
}

func TestMemoryCache_ItemCount(t *testing.T) {
	cache := NewMemoryCache(5*time.Minute, 10*time.Minute)

	cache.Set("key1", "value1", 0)
	cache.Set("key2", "value2", 0)

	if cache.ItemCount() != 2 {
		t.Fatalf("expected 2 items, got %d", cache.ItemCount())
	}
}

func TestNewCacheProvider_Memory(t *testing.T) {
	provider := NewCacheProvider(CacheConfig{
		Type:              "memory",
		DefaultExpiration: 5 * time.Minute,
		CleanupInterval:   10 * time.Minute,
	})
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNewCacheProvider_Default(t *testing.T) {
	provider := NewCacheProvider(CacheConfig{
		Type: "",
	})
	if provider == nil {
		t.Fatal("expected non-nil provider for empty type")
	}
}

func TestNewCacheProvider_UnsupportedType(t *testing.T) {
	provider := NewCacheProvider(CacheConfig{
		Type: "unknown",
	})
	if provider == nil {
		t.Fatal("expected fallback to memory for unknown type")
	}
}

func TestNewCacheProvider_RedisFallback(t *testing.T) {
	provider := NewCacheProvider(CacheConfig{
		Type:       "redis",
		RedisAddr:  "localhost:6379",
	})
	if provider == nil {
		t.Fatal("expected fallback to memory for unimplemented redis")
	}
}

func TestCacheService_BasicOperations(t *testing.T) {
	provider := NewMemoryCache(5*time.Minute, 10*time.Minute)
	svc := NewCacheServiceWithProvider(provider)

	svc.Set("key1", "value1", 0)
	val, ok := svc.Get("key1")
	if !ok || val != "value1" {
		t.Fatalf("expected value1, got %v, ok=%v", val, ok)
	}

	svc.Delete("key1")
	_, ok = svc.Get("key1")
	if ok {
		t.Fatal("expected key1 to be deleted")
	}
}
