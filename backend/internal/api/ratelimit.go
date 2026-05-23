package api

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter token bucket 限流器
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	rate     int           // tokens per second
	capacity int           // max tokens
	stopCh   chan struct{}
}

type tokenBucket struct {
	tokens   float64
	lastFill time.Time
}

// NewRateLimiter 创建限流器
func NewRateLimiter(rate, capacity int) *RateLimiter {
	if rate <= 0 {
		rate = 10
	}
	if capacity <= 0 {
		capacity = 100
	}
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		rate:     rate,
		capacity: capacity,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) Shutdown() {
	close(rl.stopCh)
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.cleanup()
		}
	}
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: float64(rl.capacity), lastFill: time.Now()}
		rl.buckets[key] = b
	}

	// 补充token
	elapsed := time.Since(b.lastFill).Seconds()
	b.tokens += elapsed * float64(rl.rate)
	if b.tokens > float64(rl.capacity) {
		b.tokens = float64(rl.capacity)
	}
	b.lastFill = time.Now()

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-30 * time.Minute)
	for k, b := range rl.buckets {
		if b.lastFill.Before(cutoff) {
			delete(rl.buckets, k)
		}
	}
}

// Middleware 限流中间件 — 按IP限流
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 白名单
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}

		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
