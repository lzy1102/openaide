package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Mode != "direct" {
		t.Errorf("Expected mode direct, got %s", cfg.Server.Mode)
	}
	if cfg.Kernel.MaxRounds != 10 {
		t.Errorf("Expected max rounds 10, got %d", cfg.Kernel.MaxRounds)
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.Server.Port = 9090
	cfg.Kernel.MaxRounds = 20

	// 保存
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Config file not created: %v", err)
	}

	// 加载
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Server.Port != 9090 {
		t.Errorf("Expected port 9090, got %d", loaded.Server.Port)
	}
	if loaded.Kernel.MaxRounds != 20 {
		t.Errorf("Expected max rounds 20, got %d", loaded.Kernel.MaxRounds)
	}
}

func TestConfig_GetProvider(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Providers = []ProviderConfig{
		{Name: "openai", Enabled: true},
		{Name: "anthropic", Enabled: false},
	}

	provider := cfg.GetProvider("openai")
	if provider == nil {
		t.Fatal("Expected to find openai provider")
	}
	if !provider.Enabled {
		t.Error("Expected openai provider to be enabled")
	}

	notFound := cfg.GetProvider("nonexistent")
	if notFound != nil {
		t.Error("Expected nil for nonexistent provider")
	}
}

func TestConfig_GetEnabledProviders(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Providers = []ProviderConfig{
		{Name: "openai", Enabled: true},
		{Name: "anthropic", Enabled: false},
		{Name: "local", Enabled: true},
	}

	enabled := cfg.GetEnabledProviders()
	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled providers, got %d", len(enabled))
	}
}
