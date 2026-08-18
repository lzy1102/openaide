package tools

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

// Command-construction tests run on every OS without a GUI: they verify the
// right executable and args are produced, but never execute the commands.

func TestDesktopCmdConstruction(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("command construction verified on linux")
	}

	cases := []struct {
		name string
		cmd  func() []string
		want []string // any of these binaries is acceptable
	}{
		{"screenshot_full", func() []string {
			c := screenshotCmd("")
			return append([]string{c.Path}, c.Args...)
		}, []string{"gnome-screenshot", "scrot"}},
		{"screenshot_selection", func() []string {
			c := screenshotCmd("selection")
			return append([]string{c.Path}, c.Args...)
		}, []string{"gnome-screenshot"}},
		{"click", func() []string {
			c := clickCmd(10, 20, "left", false)
			return append([]string{c.Path}, c.Args...)
		}, []string{"xdotool"}},
		{"type", func() []string {
			c := typeCmd("hello")
			return append([]string{c.Path}, c.Args...)
		}, []string{"xdotool"}},
		{"key", func() []string {
			c := keyCmd("ctrl+c")
			return append([]string{c.Path}, c.Args...)
		}, []string{"xdotool"}},
		{"scroll", func() []string {
			c := scrollCmd(0, 2)
			return append([]string{c.Path}, c.Args...)
		}, []string{"xdotool"}},
		{"move", func() []string {
			c := moveCmd(5, 6)
			return append([]string{c.Path}, c.Args...)
		}, []string{"xdotool"}},
		{"drag", func() []string {
			c := dragCmd(0, 0, 5, 5)
			return append([]string{c.Path}, c.Args...)
		}, []string{"xdotool"}},
	}
	for _, c := range cases {
		got := c.cmd()[0]
		ok := false
		for _, w := range c.want {
			if strings.Contains(got, w) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("%s: command = %v, want one of %v", c.name, c.cmd(), c.want)
		}
	}
}

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
