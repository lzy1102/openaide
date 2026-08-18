package plugin_test

import (
	"os"
	"path/filepath"
	"testing"

	"openaide/backend/core"
	"openaide/backend/internal/plugin"
)

func setupTestPlugin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "test-plugin")
	os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0755)
	os.MkdirAll(filepath.Join(pluginDir, "skills", "review"), 0755)
	os.MkdirAll(filepath.Join(pluginDir, "hooks"), 0755)

	os.WriteFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), []byte(
		`{"name":"test-plugin","version":"1.0.0","description":"Plugin for integration test"}`), 0644)

	os.WriteFile(filepath.Join(pluginDir, "skills", "review", "SKILL.md"), []byte(
		`---
name: Code Review
description: Review code changes for bugs and style issues
allowed-tools: ["Read", "Bash"]
---
# Code Review Skill
When reviewing code, check for bugs, style, and performance.
`), 0644)

	os.WriteFile(filepath.Join(pluginDir, "hooks", "hooks.json"), []byte(
		`{"hooks":[{"event":"PreToolUse","command":"echo $OPENAIDE_TOOL_NAME $OPENAIDE_SESSION_ID","tools":["Read"]}]}`), 0644)

	return dir
}

// TestPluginDiscovery verifies Claude plugins are discovered from disk.
func TestPluginDiscovery(t *testing.T) {
	dir := setupTestPlugin(t)

	plugins := plugin.DiscoverClaudePlugins(dir)
	if len(plugins) == 0 {
		t.Fatal("expected at least one plugin discovered")
	}

	p := plugins[0]
	if p.ID != "test-plugin" {
		t.Errorf("expected ID 'test-plugin', got %q", p.ID)
	}
	if p.SystemPrompt == "" {
		t.Error("expected non-empty SystemPrompt from skills")
	}
	t.Logf("Plugin: id=%s name=%s prompt_len=%d", p.ID, p.Name, len(p.SystemPrompt))
}

// TestSkillDiscovery verifies skills are parsed from Claude plugins.
func TestSkillDiscovery(t *testing.T) {
	dir := setupTestPlugin(t)

	skills := plugin.DiscoverClaudeSkills(dir)
	if len(skills) == 0 {
		t.Fatal("expected at least one skill discovered")
	}

	s := skills[0]
	if s.Name != "Code Review" {
		t.Errorf("expected name 'Code Review', got %q", s.Name)
	}
	if len(s.AllowedTools) == 0 {
		t.Error("expected non-empty AllowedTools")
	}
	if len(s.Keywords) == 0 {
		t.Error("expected auto-generated keywords")
	}
	if s.Prompt == "" {
		t.Error("expected non-empty Prompt body")
	}
	t.Logf("Skill: id=%s name=%s tools=%v keywords=%v", s.ID, s.Name, s.AllowedTools, s.Keywords)
}

// TestSkillStatsPreserved verifies that re-adding a Claude skill preserves usage stats.
func TestSkillStatsPreserved(t *testing.T) {
	dir := setupTestPlugin(t)
	skills := plugin.DiscoverClaudeSkills(dir)
	if len(skills) == 0 {
		t.Fatal("no skills found")
	}

	sa := kernel.NewSkillActor(nil) // no LLM for this test
	cs := skills[0]

	// First add
	sa.AddClaudeSkill(cs.ID, cs.Name, cs.Description, cs.Prompt, cs.Keywords, cs.AllowedTools, cs.Scripts)

	// Simulate accumulated stats through usage
	sa.RecordUsage(cs.ID, 8) // good quality
	sa.RecordUsage(cs.ID, 7) // good quality
	// Confidence should now be 0.6 + 2×0.05 = 0.7, UsageCount = 2
	exported := sa.ExportSkills()
	sk1 := exported[cs.ID]
	if sk1 == nil {
		t.Fatal("skill not found after first add")
	}
	t.Logf("After usage: confidence=%.2f usage=%d", sk1.Confidence, sk1.UsageCount)

	if sk1.UsageCount != 2 {
		t.Errorf("expected UsageCount=2, got %d", sk1.UsageCount)
	}

	// Now re-add (simulating hot-reload)
	sa.AddClaudeSkill(cs.ID, cs.Name, cs.Description, cs.Prompt, cs.Keywords, cs.AllowedTools, cs.Scripts)

	// Stats should be PRESERVED, not reset
	exported2 := sa.ExportSkills()
	sk2 := exported2[cs.ID]
	if sk2 == nil {
		t.Fatal("skill not found after re-add")
	}
	t.Logf("After re-add: confidence=%.2f usage=%d", sk2.Confidence, sk2.UsageCount)

	if sk2.UsageCount != 2 {
		t.Errorf("P0 REGRESSION: UsageCount was reset! got %d, want 2", sk2.UsageCount)
	}
	if sk2.Confidence < 0.65 {
		t.Errorf("P0 REGRESSION: Confidence was reset! got %.2f, want >= 0.65", sk2.Confidence)
	}
}

// TestHookEnvVars verifies hooks carry tool name and session ID.
func TestHookEnvVars(t *testing.T) {
	dir := setupTestPlugin(t)
	hooks := plugin.DiscoverClaudeHooks(dir)
	if len(hooks) == 0 {
		t.Fatal("expected at least one hook discovered")
	}

	h := hooks[0]
	if h.Event != "PreToolUse" {
		t.Errorf("expected event 'PreToolUse', got %q", h.Event)
	}
	if len(h.Tools) == 0 {
		t.Error("expected tool filter, got none")
	}
	if h.Command == "" {
		t.Error("expected hook command")
	}
	t.Logf("Hook: event=%s command=%s tools=%v", h.Event, h.Command, h.Tools)

	// Verify event mapping
	oevt := plugin.MapClaudeEvent(h.Event)
	if oevt == "" {
		t.Error("event mapping returned empty")
	}
	t.Logf("Mapped event: %s -> %s", h.Event, oevt)
}
