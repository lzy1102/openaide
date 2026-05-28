package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_New(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestManager_InstallUninstall(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	err := mgr.Install(`{"id":"test-plugin","name":"Test Plugin","version":"1.0"}`)
	if err != nil {
		t.Fatal(err)
	}

	plugins := mgr.List()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "Test Plugin" {
		t.Errorf("expected 'Test Plugin', got '%s'", plugins[0].Name)
	}

	// Uninstall
	mgr.Uninstall("test-plugin")
	if len(mgr.List()) != 0 {
		t.Error("expected empty plugin list after uninstall")
	}
}

func TestManager_EnableDisable(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	mgr.Install(`{"id":"p1","name":"P1"}`)
	if !mgr.List()[0].Enabled {
		t.Error("expected enabled by default")
	}

	mgr.Disable("p1")
	if mgr.List()[0].Enabled {
		t.Error("expected disabled")
	}

	mgr.Enable("p1")
	if !mgr.List()[0].Enabled {
		t.Error("expected enabled")
	}
}

func TestDiscoverClaudePlugins_Empty(t *testing.T) {
	dir := t.TempDir()
	plugins := DiscoverClaudePlugins(dir)
	if len(plugins) > 0 {
		t.Errorf("expected 0 plugins in empty dir, got %d", len(plugins))
	}
}

func TestDiscoverClaudePlugins_Valid(t *testing.T) {
	dir := t.TempDir()
	// Minimal Claude plugin structure
	pluginDir := filepath.Join(dir, "my-plugin")
	manifestDir := filepath.Join(pluginDir, ".claude-plugin")
	os.MkdirAll(manifestDir, 0755)
	os.WriteFile(filepath.Join(manifestDir, "plugin.json"),
		[]byte(`{"name":"my-plugin","version":"1.0","description":"test"}`), 0644)

	plugins := DiscoverClaudePlugins(dir)
	if len(plugins) < 1 {
		t.Fatalf("expected at least 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "my-plugin" {
		t.Errorf("expected 'my-plugin', got '%s'", plugins[0].Name)
	}
}
