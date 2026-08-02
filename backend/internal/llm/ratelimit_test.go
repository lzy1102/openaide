package llm

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterBurstCapacity(t *testing.T) {
	rl := NewRateLimiter(100, 5)
	defer rl.Shutdown()

	// Burst: first `capacity` calls succeed immediately
	for i := 0; i < 5; i++ {
		if err := rl.Wait(context.Background()); err != nil {
			t.Fatalf("call %d should succeed within burst: %v", i, err)
		}
	}

	// After burst, next call must block until refill (100/s → ~10ms per token)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("should refill within timeout: %v", err)
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	rl := NewRateLimiter(10, 1) // 10 tokens/s, burst 1
	defer rl.Shutdown()

	start := time.Now()
	// Consume 5 tokens sequentially: each needs ~100ms refill
	for i := 0; i < 5; i++ {
		if err := rl.Wait(context.Background()); err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 300*time.Millisecond {
		t.Fatalf("5 calls at 10/s should take ~400ms, took %v", elapsed)
	}
}

func TestRateLimiterContextCancel(t *testing.T) {
	rl := NewRateLimiter(1, 1) // 1 token/s, burst 1
	defer rl.Shutdown()

	// Drain the single token
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}

	// Next call blocks — cancel should return error promptly
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := rl.Wait(ctx); err != nil {
		if err != context.DeadlineExceeded {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	} else {
		t.Fatal("expected cancellation, got token")
	}
}

func TestRateLimiterDisabledByZero(t *testing.T) {
	g := NewGateway()
	g.SetRateLimiter(0, 0)
	if g.rateLimiter != nil {
		t.Fatal("rate<=0 should disable limiter")
	}
	g.SetRateLimiter(10, 100)
	if g.rateLimiter == nil {
		t.Fatal("positive rate should enable limiter")
	}
}
