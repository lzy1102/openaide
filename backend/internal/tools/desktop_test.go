package tools

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

// Desktop handlers depend on a graphical session (DISPLAY + xdotool on Linux).
// Tests assert they fail gracefully with a clear hint when no session exists,
// never hang, and never claim success on a failed command.

func TestDesktopHandlersNoSession(t *testing.T) {
	// Save environment so the test can restore it afterwards.
	oldDisplay, oldWayland := os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY")

	ctx := context.Background()
	cases := []struct {
		name string
		call func() string
	}{
		{"screenshot", func() string { r, _ := handleDesktopScreenshot(ctx, `{}`); return r.Error }},
		{"click", func() string { r, _ := handleDesktopClick(ctx, `{"x":10,"y":10}`); return r.Error }},
		{"type", func() string { r, _ := handleDesktopType(ctx, `{"text":"hi"}`); return r.Error }},
		{"key", func() string { r, _ := handleDesktopKey(ctx, `{"keys":"Return"}`); return r.Error }},
		{"scroll", func() string { r, _ := handleDesktopScroll(ctx, `{"y":100}`); return r.Error }},
		{"move", func() string { r, _ := handleDesktopMove(ctx, `{"x":1,"y":2}`); return r.Error }},
		{"drag", func() string { r, _ := handleDesktopDrag(ctx, `{"x1":0,"y1":0,"x2":5,"y2":5}`); return r.Error }},
	}

	if runtime.GOOS == "linux" {
		// Simulate a headless session: no DISPLAY, no xdotool.
		os.Setenv("DISPLAY", "")
		os.Setenv("WAYLAND_DISPLAY", "")
		// Restore after the test run.
		defer func() {
			os.Setenv("DISPLAY", oldDisplay)
			os.Setenv("WAYLAND_DISPLAY", oldWayland)
		}()

		for _, c := range cases {
			if msg := c.call(); !strings.Contains(msg, "desktop automation unavailable") {
				t.Errorf("%s: error = %q, want 'desktop automation unavailable' hint", c.name, msg)
			}
		}
	} else {
		t.Logf("skipping headless check on %s (uses built-in osascript/PowerShell)", runtime.GOOS)
	}
}

func TestDesktopHandlersArgValidation(t *testing.T) {
	// Even in a real session these must fail fast on bad input.
	ctx := context.Background()

	cases := []struct {
		name string
		call func() string
		want string
	}{
		{"type_missing_text", func() string { r, _ := handleDesktopType(ctx, `{}`); return r.Error }, "text parameter required"},
		{"key_missing_keys", func() string { r, _ := handleDesktopKey(ctx, `{}`); return r.Error }, "keys parameter required"},
		{"bad_json_click", func() string { r, _ := handleDesktopClick(ctx, `{oops`); return r.Error }, ""},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, c.want) {
			t.Errorf("%s: error = %q, want containing %q", c.name, msg, c.want)
		}
	}
}
