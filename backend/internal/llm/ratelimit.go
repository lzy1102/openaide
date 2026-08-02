package llm

import (
	"context"
	"sync"
	"time"
)

// RateLimiter global token bucket for LLM calls.
// Unlike the HTTP-layer limiter (deny with 429), this one BLOCKS until a
// token is available — agent-internal calls must not fail randomly,
// they just queue.
type RateLimiter struct {
	mu     sync.Mutex
	bucket tokenBucket
}

type tokenBucket struct {
	tokens   float64
	lastFill time.Time
	rate     float64 // tokens per second
	capacity float64
}

// NewRateLimiter creates a rate limiter. Defaults: 10 tokens/s, 100 burst.
func NewRateLimiter(rate, capacity int) *RateLimiter {
	if rate <= 0 {
		rate = 10
	}
	if capacity <= 0 {
		capacity = 100
	}
	return &RateLimiter{
		bucket: tokenBucket{tokens: float64(capacity), lastFill: time.Now(), rate: float64(rate), capacity: float64(capacity)},
	}
}

// Shutdown is kept for API compatibility (previously stopped an actor).
func (rl *RateLimiter) Shutdown() {}

// Wait blocks until a token is available or ctx is canceled.
// Refills the bucket by elapsed time on each call.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.tryTake() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (rl *RateLimiter) tryTake() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b := &rl.bucket
	elapsed := time.Since(b.lastFill).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastFill = time.Now()
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
