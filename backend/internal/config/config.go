package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"openaide/backend/internal/kernel"
)

// Config 应用配置
type Config struct {
	// 服务器配置
	Server ServerConfig `json:"server" yaml:"server"`

	// LLM 配置
	LLM LLMConfig `json:"llm" yaml:"llm"`

	// 记忆配置
	Memory MemoryConfig `json:"memory" yaml:"memory"`

	// 工具配置
	Tools ToolsConfig `json:"tools" yaml:"tools"`

	// 内核配置
	Kernel KernelConfig `json:"kernel" yaml:"kernel"`

	// 存储配置
	Storage StorageConfig `json:"storage" yaml:"storage"`

	// 浏览器配置
	Browser BrowserConfig `json:"browser" yaml:"browser"`

	// 日志配置
	Log LogConfig `json:"log" yaml:"log"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
	Mode string `json:"mode" yaml:"mode"` // direct, server, tui
}

// LLMConfig LLM 配置
type LLMConfig struct {
	DefaultProvider string              `json:"default_provider" yaml:"default_provider"`
	Providers       []ProviderConfig    `json:"providers" yaml:"providers"`
	FallbackEnabled bool                `json:"fallback_enabled" yaml:"fallback_enabled"`
}

// ProviderConfig 提供商配置
type ProviderConfig struct {
	Name         string            `json:"name" yaml:"name"`
	Type         string            `json:"type" yaml:"type"`
	BaseURL      string            `json:"base_url" yaml:"base_url"`
	APIKey       string            `json:"api_key" yaml:"api_key"`
	DefaultModel string            `json:"default_model" yaml:"default_model"`
	Timeout      int               `json:"timeout" yaml:"timeout"`
	Enabled      bool              `json:"enabled" yaml:"enabled"`
	Headers      map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`

	// DeepSeek 特有配置
	Thinking        *kernel.ThinkingConfig `json:"thinking,omitempty" yaml:"thinking,omitempty"`
	ReasoningEffort string                 `json:"reasoning_effort,omitempty" yaml:"reasoning_effort,omitempty"`
	JSONMode        bool                   `json:"json_mode,omitempty" yaml:"json_mode,omitempty"`
	StrictTools     bool                   `json:"strict_tools,omitempty" yaml:"strict_tools,omitempty"`
}



// MemoryConfig 记忆配置
type MemoryConfig struct {
	DataDir      string  `json:"data_dir" yaml:"data_dir"`
	MaxItems     int     `json:"max_items" yaml:"max_items"`
	CompressThreshold int `json:"compress_threshold" yaml:"compress_threshold"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	Enabled        []string `json:"enabled" yaml:"enabled"`
	DangerousTools []string `json:"dangerous_tools" yaml:"dangerous_tools"`
	MaxExecutionTime int    `json:"max_execution_time" yaml:"max_execution_time"`
}

// KernelConfig 内核配置
type KernelConfig struct {
	MaxRounds    int    `json:"max_rounds" yaml:"max_rounds"`
	MaxTokens    int    `json:"max_tokens" yaml:"max_tokens"`
	SystemPrompt string `json:"system_prompt" yaml:"system_prompt"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	DataDir string `json:"data_dir" yaml:"data_dir"`
	IndexDir string `json:"index_dir" yaml:"index_dir"`
}

// BrowserConfig 浏览器配置
type BrowserConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `json:"level" yaml:"level"`
	Format string `json:"format" yaml:"format"`
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
		Browser: BrowserConfig{
			Enabled: false, // 默认关闭，需手动开启
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load 从文件加载配置（支持 JSON 和 YAML）
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	config := DefaultConfig()

	// 根据文件扩展名选择解析器
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("parse yaml config failed: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("parse json config failed: %w", err)
		}
	}

	return config, nil
}

// Save 保存配置到文件（自动根据扩展名选择 JSON 或 YAML）
func (c *Config) Save(path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var data []byte
	var err error

	// 根据文件扩展名选择序列化格式
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		data, err = yaml.Marshal(c)
		if err != nil {
			return fmt.Errorf("marshal yaml config failed: %w", err)
		}
	} else {
		data, err = json.MarshalIndent(c, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json config failed: %w", err)
		}
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
