package api

import (
	"net/http"
	"time"

	"openaide/backend/internal/kernel/actor"
)

// RateLimiter implements the token bucket algorithm for per-IP rate limiting.
// Uses a CSP actor to serialize access to buckets — no locks needed.
//
// How token bucket works:
//   - Each IP gets a bucket with 'capacity' tokens, filled at 'rate' tokens/sec.
//   - Each request consumes 1 token. If bucket is empty, request is denied.
//   - Bucket refills over time: tokens += elapsed_seconds * rate.
//   - Unused capacity is capped at 'capacity' to prevent burst abuse.
//
// Typical config: rate=20, capacity=200 (200 burst, 20/sec sustained).
type RateLimiter struct {
	actor    *actor.Actor
	buckets  map[string]*tokenBucket
	rate     int // tokens per second
	capacity int // max tokens per bucket
	stopCh   chan struct{}
}

type tokenBucket struct {
	tokens   float64
	lastFill time.Time
}

// NewRateLimiter creates a rate limiter. Defaults: 10 req/s, 100 burst.
func NewRateLimiter(rate, capacity int) *RateLimiter {
	if rate <= 0 {
		rate = 10
	}
	if capacity <= 0 {
		capacity = 100
	}
	rl := &RateLimiter{
		actor:    actor.NewActor(64),
		buckets:  make(map[string]*tokenBucket),
		rate:     rate,
		capacity: capacity,
		stopCh:   make(chan struct{}),
	}
	// Periodically purge inactive buckets to prevent memory leak
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) Shutdown() { close(rl.stopCh) }

// cleanupLoop removes buckets that haven't been accessed in 30 minutes.
// Runs every 10 minutes to bound memory from abandoned IPs.
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

// allow checks if a request from the given IP should be permitted.
// Returns true if a token was available, false if rate limited.
func (rl *RateLimiter) allow(key string) bool {
	ch := make(chan bool, 1)
	rl.actor.Send(func() { ch <- rl.allowLocked(key) })
	return <-ch
}

// allowLocked must be called from inside the actor goroutine.
func (rl *RateLimiter) allowLocked(key string) bool {
	b, ok := rl.buckets[key]
	if !ok {
		// First request from this IP — create a full bucket
		b = &tokenBucket{tokens: float64(rl.capacity), lastFill: time.Now()}
		rl.buckets[key] = b
	}
	// Refill: tokens accumulate proportionally to elapsed time
	elapsed := time.Since(b.lastFill).Seconds()
	b.tokens += elapsed * float64(rl.rate)
	// Cap at capacity to prevent unlimited burst
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

// Middleware returns an HTTP middleware that rate-limits by client IP.
// Health check endpoint is whitelisted (always passes).
// Uses X-Forwarded-For header if behind a reverse proxy.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health check always bypasses rate limiting
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		// Use X-Forwarded-For for clients behind reverse proxies
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
