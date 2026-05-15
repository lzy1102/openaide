package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 应用配置
type Config struct {
	// 服务器配置
	Server ServerConfig `json:"server"`

	// LLM 配置
	LLM LLMConfig `json:"llm"`

	// 记忆配置
	Memory MemoryConfig `json:"memory"`

	// 工具配置
	Tools ToolsConfig `json:"tools"`

	// 内核配置
	Kernel KernelConfig `json:"kernel"`

	// 存储配置
	Storage StorageConfig `json:"storage"`

	// 日志配置
	Log LogConfig `json:"log"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Mode string `json:"mode"` // direct, server, tui
}

// LLMConfig LLM 配置
type LLMConfig struct {
	DefaultProvider string              `json:"default_provider"`
	Providers       []ProviderConfig    `json:"providers"`
	FallbackEnabled bool                `json:"fallback_enabled"`
}

// ProviderConfig 提供商配置
type ProviderConfig struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	BaseURL      string            `json:"base_url"`
	APIKey       string            `json:"api_key"`
	DefaultModel string            `json:"default_model"`
	Timeout      int               `json:"timeout"`
	Enabled      bool              `json:"enabled"`
	Headers      map[string]string `json:"headers,omitempty"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	DataDir      string  `json:"data_dir"`
	MaxItems     int     `json:"max_items"`
	CompressThreshold int `json:"compress_threshold"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	Enabled        []string `json:"enabled"`
	DangerousTools []string `json:"dangerous_tools"`
	MaxExecutionTime int    `json:"max_execution_time"`
}

// KernelConfig 内核配置
type KernelConfig struct {
	MaxRounds    int    `json:"max_rounds"`
	MaxTokens    int    `json:"max_tokens"`
	SystemPrompt string `json:"system_prompt"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	DataDir string `json:"data_dir"`
	IndexDir string `json:"index_dir"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "direct",
		},
		LLM: LLMConfig{
			DefaultProvider: "",
			Providers:       []ProviderConfig{},
			FallbackEnabled: true,
		},
		Memory: MemoryConfig{
			DataDir:           "./data/memory",
			MaxItems:          10000,
			CompressThreshold: 100,
		},
		Tools: ToolsConfig{
			Enabled:           []string{},
			DangerousTools:    []string{"execute_command", "write_file"},
			MaxExecutionTime:  30,
		},
		Kernel: KernelConfig{
			MaxRounds: 10,
			MaxTokens: 4000,
		},
		Storage: StorageConfig{
			DataDir:  "./data",
			IndexDir: "./data/index",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parse config failed: %w", err)
	}

	return config, nil
}

// Save 保存配置到文件
func (c *Config) Save(path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetProvider 获取指定名称的提供商配置
func (c *Config) GetProvider(name string) *ProviderConfig {
	for i := range c.LLM.Providers {
		if c.LLM.Providers[i].Name == name {
			return &c.LLM.Providers[i]
		}
	}
	return nil
}

// GetEnabledProviders 获取启用的提供商
func (c *Config) GetEnabledProviders() []ProviderConfig {
	var result []ProviderConfig
	for _, p := range c.LLM.Providers {
		if p.Enabled {
			result = append(result, p)
		}
	}
	return result
}

// IsToolDangerous 检查工具是否为危险工具
func (c *Config) IsToolDangerous(toolName string) bool {
	for _, t := range c.Tools.DangerousTools {
		if t == toolName {
			return true
		}
	}
	return false
}
