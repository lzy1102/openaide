package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/llm"
)

// Config 应用配置
type Config struct {
	// 服务器配置
	Server ServerConfig `json:"server" yaml:"server"`

	// LLM 配置
	LLM LLMConfig `json:"llm" yaml:"llm"`

	// 路由配置
	Router llm.RouterConfig `json:"router" yaml:"router"`

	// 记忆配置
	Memory MemoryConfig `json:"memory" yaml:"memory"`

	// 工具配置
	Tools ToolsConfig `json:"tools" yaml:"tools"`

	// 内核配置
	Kernel KernelConfig `json:"kernel" yaml:"kernel"`

	// 规划配置
	Planning PlanningConfig `json:"planning" yaml:"planning"`

	// 存储配置
	Storage StorageConfig `json:"storage" yaml:"storage"`

	// 浏览器配置
	Browser BrowserConfig `json:"browser" yaml:"browser"`

	// MCP 配置
	MCP MCPConfig `json:"mcp" yaml:"mcp"`

	// 渠道配置
	Channels ChannelsConfig `json:"channels" yaml:"channels"`

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
	DefaultProvider string           `json:"default_provider" yaml:"default_provider"`
	Providers       []ProviderConfig `json:"providers" yaml:"providers"`
	ModelRouting    ModelRoutingCfg  `json:"model_routing" yaml:"model_routing"`

	// 扁平格式（只需 api_key，其余都可选）
	APIKey         string `json:"api_key" yaml:"api_key"`                 // 必填
	Model          string `json:"model" yaml:"model"`                     // 模型名(从名字推断 API/provider/context)
	BaseURL        string `json:"base_url" yaml:"base_url"`               // API 地址
	ExecutionModel string `json:"execution_model" yaml:"execution_model"` // 子Agent 模型
	Context        string `json:"context" yaml:"context"`                 // "1m"/"200k"/"128k", 不配则从模型名猜
}

// ModelRoutingCfg 按能力分配模型
type ModelRoutingCfg struct {
	Reasoning string `json:"reasoning" yaml:"reasoning"` // analyst/coder/reviewer 使用的模型
	Execution string `json:"execution" yaml:"execution"` // executor/classifier 使用的模型
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
	DataDir string `json:"data_dir" yaml:"data_dir"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	DangerousTools []string `json:"dangerous_tools" yaml:"dangerous_tools"`
}

// KernelConfig 内核配置
type KernelConfig struct {
	MaxRounds    int    `json:"max_rounds" yaml:"max_rounds"`
	MaxTokens    int    `json:"max_tokens" yaml:"max_tokens"`
	MinRounds    int    `json:"min_rounds" yaml:"min_rounds"`       // 自适应轮次下限（默认5）
	MaxRoundsCap int    `json:"max_rounds_cap" yaml:"max_rounds_cap"` // 自适应轮次上限（默认30）
	SystemPrompt string `json:"system_prompt" yaml:"system_prompt"`
	UnsafeMode   *bool  `json:"unsafe_mode" yaml:"unsafe_mode"`     // 安全模式：true=跳过审批（默认true）
}

// PlanningConfig 任务规划配置
type PlanningConfig struct {
	Enabled        bool `json:"enabled" yaml:"enabled"`                 // 启用规划（默认true）
	DeepTimeout    int  `json:"deep_timeout" yaml:"deep_timeout"`       // 深度分析超时秒（默认120）
	PreviewTimeout int  `json:"preview_timeout" yaml:"preview_timeout"` // 预览超时秒（默认15）
	MaxProposals   int  `json:"max_proposals" yaml:"max_proposals"`     // 最大方案数（默认3）
}

// StorageConfig 存储配置
type StorageConfig struct {
	DataDir string `json:"data_dir" yaml:"data_dir"`
}

// BrowserConfig 浏览器配置
type BrowserConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// MCPConfig MCP 配置
type MCPConfig struct {
	Enabled bool             `json:"enabled" yaml:"enabled"`
	Servers []MCPServerEntry `json:"servers" yaml:"servers"`
}

// MCPServerEntry MCP 服务器配置
type MCPServerEntry struct {
	ID      string   `json:"id" yaml:"id"`
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args" yaml:"args"`
}

// ChannelsConfig 渠道配置
type ChannelsConfig struct {
	Webhooks []WebhookChannelConfig `json:"webhooks" yaml:"webhooks"`
	Feishu   []FeishuChannelConfig  `json:"feishu" yaml:"feishu"`
	Telegram []TelegramChannelConfig `json:"telegram" yaml:"telegram"`
	TaskQueue QueueConfig            `json:"task_queue" yaml:"task_queue"`
}

// WebhookChannelConfig Webhook渠道配置
type WebhookChannelConfig struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	SecretToken string `json:"secret_token" yaml:"secret_token"`
	CallbackURL string `json:"callback_url" yaml:"callback_url"`
}

// FeishuChannelConfig 飞书机器人配置
type FeishuChannelConfig struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	AppID       string `json:"app_id" yaml:"app_id"`
	AppSecret   string `json:"app_secret" yaml:"app_secret"`
	VerifyToken string `json:"verify_token" yaml:"verify_token"`
	AESKey      string `json:"aes_key" yaml:"aes_key"`
}

// TelegramChannelConfig Telegram机器人配置
type TelegramChannelConfig struct {
	ID    string `json:"id" yaml:"id"`
	Name  string `json:"name" yaml:"name"`
	Token string `json:"token" yaml:"token"`
}

// QueueConfig 任务队列配置
type QueueConfig struct {
	WorkerCount int `json:"worker_count" yaml:"worker_count"`
	QueueSize   int `json:"queue_size" yaml:"queue_size"`
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
		},
		Memory: MemoryConfig{
			DataDir: "./data/memory",
		},
		Tools: ToolsConfig{
			DangerousTools: []string{"execute_command", "write_file"},
		},
		Kernel: KernelConfig{
			MaxRounds:    30,
			MaxTokens:    200000,
			MinRounds:    5,
			MaxRoundsCap: 50,
		},
		Planning: PlanningConfig{
			Enabled:        true,
			DeepTimeout:    300,
			PreviewTimeout: 30,
			MaxProposals:   3,
		},
		Storage: StorageConfig{
			DataDir: "./data",
		},
		Browser: BrowserConfig{
			Enabled: false,
		},
		MCP: MCPConfig{
			Enabled: false,
			Servers: []MCPServerEntry{},
		},
		Channels: ChannelsConfig{
			Webhooks: []WebhookChannelConfig{},
			Feishu:   []FeishuChannelConfig{},
			Telegram: []TelegramChannelConfig{},
			TaskQueue: QueueConfig{
				WorkerCount: 4,
				QueueSize:   128,
			},
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

	config.normalize()
	config.resolvePaths()
	return config, nil
}

// normalize 将扁平简化配置展开为完整内部格式
func (c *Config) normalize() {
	// 已有完整 providers → 跳过展开
	if len(c.LLM.Providers) > 0 {
		return
	}

	// 从模型名自动推断一切
	model := strings.ToLower(c.LLM.Model)
	provider := "anthropic"
	baseURL := c.LLM.BaseURL

	if baseURL == "" {
		switch {
		case strings.Contains(model, "deepseek"):
			baseURL = "https://api.deepseek.com/anthropic"
			provider = "deepseek"
		case strings.Contains(model, "claude"):
			baseURL = "https://api.anthropic.com"
			provider = "anthropic"
		case strings.Contains(model, "gpt") || strings.Contains(model, "o1") || strings.Contains(model, "o3") || strings.Contains(model, "o4"):
			baseURL = "https://api.openai.com/v1"
			provider = "openai"
		default:
			baseURL = "https://api.openai.com/v1"
			provider = "openai"
		}
	}

	providerType := "anthropic"
	if strings.Contains(baseURL, "openai") || (strings.Contains(baseURL, "/v1") && !strings.Contains(baseURL, "anthropic")) {
		providerType = "openai"
	}

	c.LLM.DefaultProvider = provider
	c.LLM.Providers = []ProviderConfig{{
		Name: provider, Type: providerType, BaseURL: baseURL,
		APIKey: c.LLM.APIKey, DefaultModel: c.LLM.Model,
		Timeout: 300, Enabled: true,
	}}

	// model_routing
	if c.LLM.Model != "" && c.LLM.ModelRouting.Reasoning == "" {
		c.LLM.ModelRouting.Reasoning = c.LLM.Model
	}
	if c.LLM.ExecutionModel != "" && c.LLM.ModelRouting.Execution == "" {
		c.LLM.ModelRouting.Execution = c.LLM.ExecutionModel
	} else if c.LLM.ModelRouting.Execution == "" {
		c.LLM.ModelRouting.Execution = c.LLM.Model
	}

	// 上下文: context 字段 > 模型名推断 > 默认 200K; 统一留 20K
	if c.LLM.Context != "" {
		c.Kernel.MaxTokens = parseContextSize(c.LLM.Context) - 20000
	} else if c.LLM.Model != "" {
		c.Kernel.MaxTokens = guessContextSize(c.LLM.Model) - 20000
	}
}

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

// resolvePaths 将所有路径中的 ~ 展开为 home 目录
func (c *Config) resolvePaths() {
	home, _ := os.UserHomeDir()
	if home == "" {
		return
	}

	c.Memory.DataDir = expandPath(c.Memory.DataDir, home)
	c.Storage.DataDir = expandPath(c.Storage.DataDir, home)
}

func expandPath(p, home string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	if p == "~" {
		return home
	}
	return p
}

// guessContextSize 从模型名推断上下文大小
func guessContextSize(model string) int {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "gemini") || strings.Contains(m, "1m"):
		return 1000000
	case strings.Contains(m, "v4") || strings.Contains(m, "v3"):
		return 1000000
	case strings.Contains(m, "opus") || strings.Contains(m, "sonnet") || strings.Contains(m, "haiku"):
		return 200000
	case strings.Contains(m, "gpt-4") || strings.Contains(m, "gpt-5") || strings.Contains(m, "o"):
		return 128000
	default:
		return 200000
	}
}


// parseContextSize 解析 "1m" "200k" "128k" → token 数
func parseContextSize(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "")
	mult := 1
	if strings.HasSuffix(s, "k") { mult = 1000; s = strings.TrimSuffix(s, "k") }
	if strings.HasSuffix(s, "m") { mult = 1000000; s = strings.TrimSuffix(s, "m") }
	n := 0
	fmt.Sscanf(s, "%d", &n)
	if n > 0 { return n * mult }
	return 200000
}

