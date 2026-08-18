package tools

import (
	"context"
	"strings"
	"testing"
)

// Handler tests run without a real browser: when disabled they must fail
// gracefully with a clear message instead of panicking or hanging.

func TestBrowserHandlersDisabled(t *testing.T) {
	browserGlobalEnabled = false
	t.Setenv("OPENAIDE_BROWSER", "")

	ctx := context.Background()
	cases := []struct {
		name string
		call func() string
	}{
		{"navigate", func() string { r, _ := handleBrowserNavigate(ctx, `{"url":"https://example.com"}`); return r.Error }},
		{"extract", func() string { r, _ := handleBrowserExtract(ctx, `{}`); return r.Error }},
		{"screenshot", func() string { r, _ := handleBrowserScreenshot(ctx, `{}`); return r.Error }},
		{"click", func() string { r, _ := handleBrowserClick(ctx, `{"selector":"#btn"}`); return r.Error }},
		{"fill", func() string { r, _ := handleBrowserFill(ctx, `{"selector":"#in","value":"x"}`); return r.Error }},
		{"click_at", func() string { r, _ := handleBrowserClickAt(ctx, `{"x":10,"y":20}`); return r.Error }},
		{"scroll", func() string { r, _ := handleBrowserScroll(ctx, `{"y":300}`); return r.Error }},
		{"type", func() string { r, _ := handleBrowserType(ctx, `{"text":"Enter"}`); return r.Error }},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, "browser disabled") {
			t.Errorf("%s: error = %q, want 'browser disabled' hint", c.name, msg)
		}
	}
}

func TestBrowserHandlersArgValidation(t *testing.T) {
	browserGlobalEnabled = false
	t.Setenv("OPENAIDE_BROWSER", "")
	ctx := context.Background()

	cases := []struct {
		name string
		call func() string
		want string
	}{
		{"navigate_missing_url", func() string { r, _ := handleBrowserNavigate(ctx, `{}`); return r.Error }, "url is required"},
		{"click_missing_selector", func() string { r, _ := handleBrowserClick(ctx, `{}`); return r.Error }, "selector is required"},
		{"fill_missing_args", func() string { r, _ := handleBrowserFill(ctx, `{"selector":"#in"}`); return r.Error }, "selector and value are required"},
		{"bad_json", func() string { r, _ := handleBrowserNavigate(ctx, `{not json`); return r.Error }, ""},
	}
	for _, c := range cases {
		if msg := c.call(); !strings.Contains(msg, c.want) {
			t.Errorf("%s: error = %q, want containing %q", c.name, msg, c.want)
		}
	}
}
