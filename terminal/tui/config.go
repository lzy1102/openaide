package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadConfig 加载配置
func LoadConfig(explicitPath string) (*Config, string, error) {
	defaultCfg := &Config{
		API: APIConfig{
			BaseURL:    "http://localhost:19375/api",
			TimeoutSec: 180,
		},
		Chat: ChatSettings{
			DefaultModel: "auto",
			Stream:       true,
			ContextLimit: 10,
		},
		Models: []ModelConfig{},
	}

	configPath := explicitPath
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return defaultCfg, "", nil
		}
		configPath = filepath.Join(homeDir, ".openaide", "config.yaml")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultCfg, configPath, nil
		}
		return defaultCfg, configPath, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, defaultCfg); err != nil {
		return defaultCfg, configPath, fmt.Errorf("parsing config: %w", err)
	}

	return defaultCfg, configPath, nil
}

// SaveConfig 保存配置
func SaveConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// InitConfig 初始化默认配置文件
func InitConfig(explicitPath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".openaide")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	configPath := explicitPath
	if configPath == "" {
		configPath = filepath.Join(configDir, "config.yaml")
	}

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Configuration file already exists: %s\n", configPath)
		return nil
	}

	defaultCfg := &Config{
		API: APIConfig{
			BaseURL:    "http://localhost:19375/api",
			TimeoutSec: 180,
		},
		Chat: ChatSettings{
			DefaultModel: "auto",
			Stream:       true,
			ContextLimit: 10,
		},
		Models: []ModelConfig{},
	}

	if err := SaveConfig(configPath, defaultCfg); err != nil {
		return err
	}

	fmt.Printf("Configuration file created: %s\n", configPath)
	fmt.Println("Edit this file to customize your settings.")
	return nil
}

// GetAPIURL 获取 API 地址
func GetAPIURL(cfg *Config, flagAPI string) string {
	if flagAPI != "" {
		return flagAPI
	}
	if cfg != nil && cfg.API.BaseURL != "" {
		return cfg.API.BaseURL
	}
	return "http://localhost:19375/api"
}

// GetModel 获取模型
func GetModel(cfg *Config, flagModel string, apiURL string) string {
	if flagModel != "" {
		return flagModel
	}
	if cfg != nil && cfg.Chat.DefaultModel != "" && cfg.Chat.DefaultModel != "auto" {
		return cfg.Chat.DefaultModel
	}

	if apiURL != "" {
		models, err := FetchModels(apiURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to fetch models from %s: %v\n", apiURL, err)
		} else {
			for _, m := range models {
				if m.Status == "enabled" {
					return m.Name
				}
			}
			if len(models) > 0 {
				fmt.Fprintf(os.Stderr, "Warning: No enabled model found on server\n")
			}
		}
	}

	return ""
}

// GetStream 获取流式设置
func GetStream(cfg *Config, flagStream bool) bool {
	if flagStream {
		return true
	}
	if cfg != nil {
		return cfg.Chat.Stream
	}
	return false
}

// GetContextLimit 获取上下文限制
func GetContextLimit(cfg *Config, flagContext int) int {
	if flagContext > 0 {
		return flagContext
	}
	if cfg != nil && cfg.Chat.ContextLimit > 0 {
		return cfg.Chat.ContextLimit
	}
	return 10
}

// GetTimeout 获取超时时间
func GetTimeout(cfg *Config) int {
	if cfg != nil && cfg.API.TimeoutSec > 0 {
		return cfg.API.TimeoutSec
	}
	return 180
}
