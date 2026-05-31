package api

import (
	"net/http"
	"time"

	"openaide/backend/internal/kernel"
)

// RateLimiter token bucket 限流器 — CSP actor
type RateLimiter struct {
	actor    *kernel.Actor
	buckets  map[string]*tokenBucket
	rate     int
	capacity int
	stopCh   chan struct{}
}

type tokenBucket struct {
	tokens   float64
	lastFill time.Time
}

// NewRateLimiter 创建限流器
func NewRateLimiter(rate, capacity int) *RateLimiter {
	if rate <= 0 { rate = 10 }
	if capacity <= 0 { capacity = 100 }
	rl := &RateLimiter{
		actor:    kernel.NewActor(64),
		buckets:  make(map[string]*tokenBucket),
		rate:     rate,
		capacity: capacity,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) Shutdown() { close(rl.stopCh) }

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.actor.Send(func() { rl.cleanupLocked() })
		}
	}
}

func (rl *RateLimiter) allow(key string) bool {
	ch := make(chan bool, 1)
	rl.actor.Send(func() { ch <- rl.allowLocked(key) })
	return <-ch
}

func (rl *RateLimiter) allowLocked(key string) bool {
	b, ok := rl.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: float64(rl.capacity), lastFill: time.Now()}
		rl.buckets[key] = b
	}
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

func (rl *RateLimiter) cleanupLocked() {
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
