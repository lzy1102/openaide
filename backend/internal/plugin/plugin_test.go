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

func TestManager_Reload(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	// Initially empty
	if len(mgr.List()) != 0 {
		t.Fatal("expected 0 plugins initially")
	}

	// Register onLoad callback to verify it fires on hot reload
	var loadedPluginID string
	mgr.OnLoad(func(p *Plugin) {
		loadedPluginID = p.ID
	})

	// Create a Claude-format plugin AFTER manager is initialized (simulating hot-add)
	pluginDir := filepath.Join(dir, "hot-reloaded-plugin")
	manifestDir := filepath.Join(pluginDir, ".claude-plugin")
	os.MkdirAll(manifestDir, 0755)
	os.WriteFile(filepath.Join(manifestDir, "plugin.json"),
		[]byte(`{"name":"hot-reloaded-plugin","version":"1.0","description":"Hot reloaded!"}`), 0644)

	// Reload
	newIDs := mgr.Reload()
	if len(newIDs) != 1 {
		t.Fatalf("expected 1 new plugin, got %d: %v", len(newIDs), newIDs)
	}

	// Verify it's in the list
	plugins := mgr.List()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin after reload, got %d", len(plugins))
	}
	if plugins[0].Name != "hot-reloaded-plugin" {
		t.Errorf("expected 'hot-reloaded-plugin', got '%s'", plugins[0].Name)
	}

	// Verify onLoad callback fired
	if loadedPluginID != "hot-reloaded-plugin" {
		t.Errorf("expected onLoad to fire for 'hot-reloaded-plugin', got '%s'", loadedPluginID)
	}

	// Verify Reload is idempotent — calling again returns no new plugins
	again := mgr.Reload()
	if len(again) != 0 {
		t.Errorf("expected 0 new plugins on second reload, got %d", len(again))
	}
}

func TestManager_ReloadFromJSON(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	// Add a legacy JSON plugin file after init
	os.WriteFile(filepath.Join(dir, "legacy.json"),
		[]byte(`{"id":"legacy","name":"Legacy Plugin","version":"0.1"}`), 0644)

	newIDs := mgr.Reload()
	if len(newIDs) != 1 {
		t.Fatalf("expected 1 new legacy plugin, got %d", len(newIDs))
	}

	plugins := mgr.List()
	if len(plugins) != 1 {
		t.Fatal("expected 1 legacy plugin after reload")
	}
	if plugins[0].Name != "Legacy Plugin" {
		t.Errorf("expected 'Legacy Plugin', got '%s'", plugins[0].Name)
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
