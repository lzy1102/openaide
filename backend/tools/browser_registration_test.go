package tools

import (
	"testing"
)

// Browser tools require a Chromium install (~500MB) and are opt-in.
// When disabled they must not appear in the tool schema at all, so the
// LLM never selects a tool that will fail at call time.
func TestBrowserToolsConditionalRegistration(t *testing.T) {
	browserGlobalEnabled = false
	t.Setenv("OPENAIDE_BROWSER", "")

	defs := BuiltinTools()
	for _, d := range defs {
		if d.Function.Name == "browser_navigate" || d.Function.Name == "browser_type" {
			t.Errorf("browser tool %q registered while disabled", d.Function.Name)
		}
	}

	r := NewRegistry()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	if r.HasTool("browser_navigate") {
		t.Error("browser_navigate handler registered while disabled")
	}
}

func TestBrowserToolsRegisteredWhenEnabled(t *testing.T) {
	browserGlobalEnabled = true
	defer func() { browserGlobalEnabled = false }()

	defs := BuiltinTools()
	browserCount := 0
	for _, d := range defs {
		switch d.Function.Name {
		case "browser_navigate", "browser_extract", "browser_screenshot",
			"browser_click", "browser_fill", "browser_click_at",
			"browser_scroll", "browser_type":
			browserCount++
		}
	}
	if browserCount != 8 {
		t.Errorf("found %d browser tools when enabled, want 8", browserCount)
	}

	r := NewRegistry()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	if !r.HasTool("browser_navigate") {
		t.Error("browser_navigate handler not registered while enabled")
	}
}
