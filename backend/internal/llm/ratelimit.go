package llm

import (
	"context"
	"time"

	"openaide/backend/internal/kernel/actor"
)

// RateLimiter global token bucket for LLM calls.
// Unlike the HTTP-layer limiter (deny with 429), this one BLOCKS until a
// token is available — agent-internal calls must not fail randomly,
// they just queue. CSP actor serializes bucket access, no locks.
type RateLimiter struct {
	actor  *actor.Actor
	bucket *tokenBucket
	stopCh chan struct{}
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
	rl := &RateLimiter{
		actor:  actor.NewActor(64),
		bucket: &tokenBucket{tokens: float64(capacity), lastFill: time.Now(), rate: float64(rate), capacity: float64(capacity)},
		stopCh: make(chan struct{}),
	}
	return rl
}

func (rl *RateLimiter) Shutdown() { close(rl.stopCh) }

// Wait blocks until a token is available or ctx is canceled.
// Refills the bucket by elapsed time on each call.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		ok := rl.tryTake()
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// tryTake attempts to consume one token. Returns false if bucket empty.
func (rl *RateLimiter) tryTake() bool {
	ch := make(chan bool, 1)
	rl.actor.Send(func() { ch <- rl.tryTakeLocked() })
	return <-ch
}

func (rl *RateLimiter) tryTakeLocked() bool {
	b := rl.bucket
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
