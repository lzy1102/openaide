package tools

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHandleVerifyClaim(t *testing.T) {
	ctx := context.Background()

	t.Run("missing_claim", func(t *testing.T) {
		r, _ := handleVerifyClaim(ctx, `{}`)
		if !strings.Contains(r.Error, "claim parameter required") {
			t.Errorf("error = %q, want 'claim parameter required'", r.Error)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		r, _ := handleVerifyClaim(ctx, `{not json`)
		if r.Error == "" {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("no_pattern_no_file", func(t *testing.T) {
		r, _ := handleVerifyClaim(ctx, `{"claim":"Planner.Plan has no timeout"}`)
		if r.Error != "" {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		if !strings.Contains(r.Content.(string), "VERDICT") {
			t.Errorf("content = %q, want VERDICT line", r.Content)
		}
	})

	t.Run("pattern_found", func(t *testing.T) {
		r, _ := handleVerifyClaim(ctx, `{"claim":"x is missing","pattern":"func handleVerifyClaim"}`)
		if r.Error != "" {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		if !strings.Contains(r.Content.(string), "VERDICT") {
			t.Errorf("content = %q, want VERDICT line", r.Content)
		}
	})
}

func TestHandleTraceCallers(t *testing.T) {
	ctx := context.Background()

	t.Run("missing_function", func(t *testing.T) {
		r, _ := handleTraceCallers(ctx, `{}`)
		if !strings.Contains(r.Error, "function parameter required") {
			t.Errorf("error = %q, want 'function parameter required'", r.Error)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		r, _ := handleTraceCallers(ctx, `{not json`)
		if r.Error == "" {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		// Unique name so the test file itself cannot contain the full string.
		nonexistent := "DefinitelyNonexistentFunc" + strconv.Itoa(time.Now().Nanosecond())
		r, _ := handleTraceCallers(ctx, `{"function":"`+nonexistent+`"}`)
		if r.Error != "" {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		if !strings.Contains(r.Content.(string), "not found") {
			t.Errorf("content = %q, want 'not found'", r.Content)
		}
	})
}
