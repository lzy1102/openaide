package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(100, 200)
	defer rl.Shutdown()

	// Should allow initial burst
	for i := 0; i < 200; i++ {
		if !rl.allow("test-ip") {
			t.Fatalf("expected allow at %d", i)
		}
	}
	// Should deny after bucket exhausted
	if rl.allow("test-ip") {
		t.Error("expected deny after bucket exhausted")
	}
}

func TestRateLimiter_HealthBypass(t *testing.T) {
	rl := NewRateLimiter(1, 1) // very restrictive
	defer rl.Shutdown()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// Health should always pass
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("health should bypass rate limit, got %d", w.Code)
	}

	// First normal request consumes the only token
	req2 := httptest.NewRequest("GET", "/api/v1/chat", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	// Second normal request should hit limit
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, httptest.NewRequest("GET", "/api/v1/chat", nil))
	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on second request, got %d", w3.Code)
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter(1000, 10000) // very permissive
	defer rl.Shutdown()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.allow("concurrent-test")
		}()
	}
	wg.Wait()
	// No deadlock = pass
}
