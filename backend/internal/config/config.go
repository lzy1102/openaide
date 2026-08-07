package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"openaide/backend/internal/llm"
)

// Config 应用配置
type Config struct {
	// 服务器配置
	Server ServerConfig `json:"server" yaml:"server"`

	// 全局语言偏好 ("zh" / "en")，REPL 和 Server 共用
	Lang string `json:"lang" yaml:"lang"`

	// LLM 配置
	LLM LLMConfig `json:"llm" yaml:"llm"`

	// 路由配置
	Router llm.RouterConfig `json:"router" yaml:"router"`

	// 记忆配置
	Memory MemoryConfig `json:"memory" yaml:"memory"`

	// 搜索配置
	Search SearchConfig `json:"search" yaml:"search"`

	// 内核配置
	Kernel KernelConfig `json:"kernel" yaml:"kernel"`

	// 代码索引配置(prompt 阶段注入相关代码)
	CodeIndex CodeIndexConfig `json:"codeindex" yaml:"codeindex"`

	// 外部检索配置(代码 + 记忆语义检索的后端)
	RAG RAGConfig `json:"rag" yaml:"rag"`

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

	// 嵌入模型（用于知识库/记忆语义搜索和模式蒸馏）
	EmbeddingModel string `json:"embedding_model,omitempty" yaml:"embedding_model,omitempty"`

	// DeepSeek 特有配置
	Thinking        *bool  `json:"thinking,omitempty" yaml:"thinking,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty" yaml:"reasoning_effort,omitempty"`
	JSONMode        bool   `json:"json_mode,omitempty" yaml:"json_mode,omitempty"`
	StrictTools     bool   `json:"strict_tools,omitempty" yaml:"strict_tools,omitempty"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	DataDir string `json:"data_dir" yaml:"data_dir"`
}

// SearchConfig 搜索配置
type SearchConfig struct {
	SearXNGURL string `json:"searxng_url" yaml:"searxng_url"` // SearXNG 实例地址
}

// KernelConfig 内核配置
type KernelConfig struct {
	MaxRounds         int    `json:"max_rounds" yaml:"max_rounds"`
	MaxTokens         int    `json:"max_tokens" yaml:"max_tokens"`
	MinRounds         int    `json:"min_rounds" yaml:"min_rounds"`
	MaxRoundsCap      int    `json:"max_rounds_cap" yaml:"max_rounds_cap"`
	SystemPrompt      string `json:"system_prompt" yaml:"system_prompt"`
	ReflectionEnabled *bool  `json:"reflection_enabled" yaml:"reflection_enabled"` // toggle reflection on/off (default true)
}

// CodeIndexConfig 代码索引配置。
// enabled 为 true(默认)时,启动时异步全量索引 CWD 项目,
// 并在 coding/debugging 任务的 prompt 阶段注入 top-K 相关代码 chunk。
// 检索完全外挂到 rag.Retriever(pgvector),未配置时返回空结果。
type CodeIndexConfig struct {
	Enabled   *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`       // 默认 true(nil = true)
	ChunkSize int   `json:"chunk_size,omitempty" yaml:"chunk_size,omitempty"` // 0 = 默认 1500
	MaxChunks int   `json:"max_chunks,omitempty" yaml:"max_chunks,omitempty"` // 0 = 默认 100
}

// EnabledOrDefault 判断是否启用代码索引,默认 true
func (c CodeIndexConfig) EnabledOrDefault() bool {
	return c.Enabled == nil || *c.Enabled
}

// RAGConfig 外部向量检索配置。
// 未配置 DSN 或后端不可达时,自动降级为 NoopRetriever(检索返回空结果)。
type RAGConfig struct {
	// PostgreSQL + pgvector 连接串,如 postgres://user:pass@host:5432/db
	DSN string `json:"dsn,omitempty" yaml:"dsn,omitempty"`

	// 外部 embedding API(OpenAI 兼容 /embeddings 端点)
	EmbeddingURL   string `json:"embedding_url,omitempty" yaml:"embedding_url,omitempty"`
	EmbeddingKey   string `json:"embedding_key,omitempty" yaml:"embedding_key,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty" yaml:"embedding_model,omitempty"` // 默认 text-embedding-3-small

	// 向量集合名(代码 / 记忆 / 归档 / 核心事实)
	Collection string `json:"collection,omitempty" yaml:"collection,omitempty"` // 默认 openaide_docs
}

// PlanningConfig 任务规划配置
type PlanningConfig struct {
	Enabled        bool `json:"enabled" yaml:"enabled"`                 // 启用规划（默认true）
	DeepTimeout    int  `json:"deep_timeout" yaml:"deep_timeout"`       // 深度分析超时秒（默认120）
	PreviewTimeout int  `json:"preview_timeout" yaml:"preview_timeout"` // 预览超时秒（默认15）
}

// StorageConfig 存储配置
type StorageConfig struct {
	DataDir      string `json:"data_dir" yaml:"data_dir"`
	SessionStore string `json:"session_store" yaml:"session_store"` // "file" (default), "sqlite", "memory"
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
	ID      string            `json:"id" yaml:"id"`
	Type    string            `json:"type,omitempty" yaml:"type,omitempty"`       // "stdio" (default) or "sse"
	Command string            `json:"command,omitempty" yaml:"command,omitempty"` // required for stdio
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`
	URL     string            `json:"url,omitempty" yaml:"url,omitempty"` // required for sse
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"` // env vars for stdio
}

// ChannelsConfig 渠道配置
type ChannelsConfig struct {
	Webhooks  []WebhookChannelConfig  `json:"webhooks" yaml:"webhooks"`
	Feishu    []FeishuChannelConfig   `json:"feishu" yaml:"feishu"`
	Telegram  []TelegramChannelConfig `json:"telegram" yaml:"telegram"`
	TaskQueue QueueConfig             `json:"task_queue" yaml:"task_queue"`
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
	Level   string `json:"level" yaml:"level"`
	Format  string `json:"format" yaml:"format"`
	Persist *bool  `json:"persist,omitempty" yaml:"persist,omitempty"` // 持久化 trace/event，默认 true（nil = true）
}

// PersistEnabled 判断是否启用持久化，nil 或 true 视为启用
func (l *LogConfig) PersistEnabled() bool {
	return l.Persist == nil || *l.Persist
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	return &Config{
		Server: ServerConfig{
			Host: "127.0.0.1", // 默认仅本机访问;局域网共享需显式改为 0.0.0.0
			Port: 8080,
			Mode: "direct",
		},
		LLM: LLMConfig{
			DefaultProvider: "",
			Providers:       []ProviderConfig{},
		},
		Memory: MemoryConfig{
			DataDir: home + "/.openaide/data/memory",
		},
		Search: SearchConfig{
			SearXNGURL: "http://localhost:8888",
		},
		Kernel: KernelConfig{
			MaxRounds:    50, // 安全上限，实际由上下文窗口驱动收敛
			MaxTokens:    200000,
			MinRounds:    8,
			MaxRoundsCap: 50,
		},
		Planning: PlanningConfig{
			Enabled:        true,
			DeepTimeout:    300,
			PreviewTimeout: 30,
		},
		Storage: StorageConfig{
			DataDir:      home + "/.openaide/data",
			SessionStore: "sqlite",
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
		if os.IsNotExist(err) {
			// 首次运行：生成示例配置
			return generateSampleConfig(path), nil
		}
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
	config.autoDetectMaxTokens()
	config.validate()
	return config, nil
}

// validate checks for common config mistakes and prints warnings.
func (c *Config) validate() {
	for i := range c.LLM.Providers {
		p := &c.LLM.Providers[i]
		if p.APIKey == "" {
			slog.Warn("Provider has no API key — requests will fail", "provider", p.Name)
		} else if strings.Contains(p.APIKey, "你的") || p.APIKey == "sk-your-key" {
			slog.Warn("Provider API key looks like a placeholder — replace with your real key", "provider", p.Name)
		}
		if p.Type == "openai" && p.BaseURL != "" {
			// coding plan detection
			if strings.Contains(p.BaseURL, "coding/paas") {
				slog.Info("Coding plan detected", "provider", p.Name)
			}
		}
	}
	if len(c.LLM.Providers) > 0 && c.LLM.DefaultProvider == "" {
		fmt.Fprintf(os.Stderr, "⚠  No default_provider set. Using first provider: %s\n", c.LLM.Providers[0].Name)
	}
}

// autoDetectMaxTokens 从模型名自动推断上下文大小
// 仅当用户未显式设置 max_tokens 且小于 200K 时生效（即使用了默认值）
func (c *Config) autoDetectMaxTokens() {
	// 已有 providers 配置 → 从 default_model 推断
	if len(c.LLM.Providers) > 0 {
		dm := strings.ToLower(c.LLM.Providers[0].DefaultModel)
		if d := guessContextSize(dm); d > 200000 && c.Kernel.MaxTokens <= 200000 {
			c.Kernel.MaxTokens = d - 20000
		}
		return
	}
	// 简单格式已在 normalize() 中处理
}

// generateSampleConfig 首次运行生成带注释的配置模板
func generateSampleConfig(path string) *Config {
	os.MkdirAll(filepath.Dir(path), 0755)

	sample := `# OpenAIDE 配置文件 — 编辑此文件后重新运行 openaide
# 文档: https://github.com/lzy1102/openaide

# 全局语言: zh (中文) / en (English)
lang: zh

# LLM 配置 — 最简单写法（系统自动识别提供商和 API 地址）
llm:
  api_key: sk-你的-api-key       # ← 改成你的 API Key
  model: deepseek-v4-pro         # 选一个: gpt-4o / claude-sonnet-4-6 / deepseek-v4-pro
  execution_model: deepseek-v4-flash  # 快速执行模型（可选）
  # base_url: https://api.deepseek.com/v1  # 中转站用户填自己的地址，不填自动识别

# 高级：双模型路由（Architect/Editor 模式）
# llm:
#   providers:
#     - name: deepseek
#       type: openai
#       base_url: https://api.deepseek.com/v1
#       api_key: sk-你的-api-key
#       default_model: deepseek-v4-pro
#       timeout: 300
#       thinking: true
#       reasoning_effort: max
#     - name: deepseek-flash
#       type: openai
#       base_url: https://api.deepseek.com/v1
#       api_key: sk-你的-api-key
#       default_model: deepseek-v4-flash
#       timeout: 120
#   model_routing:
#     reasoning: deepseek-v4-pro
#     execution: deepseek-v4-flash

log:
  level: info
  persist: false
`
	os.WriteFile(path, []byte(sample), 0644)

	// 返回一个有提示信息的默认配置
	cfg := DefaultConfig()
	cfg.LLM.Providers = nil // 清空，让用户看到提示后自己去编辑
	return cfg
}

// normalize 将扁平简化配置展开为完整内部格式
func (c *Config) normalize() {
	// 已有完整 providers → 跳过展开
	if len(c.LLM.Providers) > 0 {
		return
	}

	// Auto-detect provider identity from model name at config load time.
	// API type follows provider: openai-compatible for most, anthropic for Claude.
	model := strings.ToLower(c.LLM.Model)
	provider, baseURL := resolveProvider(model, c.LLM.BaseURL)
	providerType := "openai"
	if provider == "anthropic" {
		providerType = "anthropic"
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

	// 相对路径基于 CWD 解析为绝对路径
	if c.Storage.DataDir == "./data" || strings.HasPrefix(c.Storage.DataDir, "./") {
		if wd, err := os.Getwd(); err == nil {
			c.Storage.DataDir = filepath.Join(wd, c.Storage.DataDir[2:])
		}
	}
	if c.Memory.DataDir == "./data/memory" || strings.HasPrefix(c.Memory.DataDir, "./") {
		if wd, err := os.Getwd(); err == nil {
			c.Memory.DataDir = filepath.Join(wd, c.Memory.DataDir[2:])
		}
	}

	if home != "" {
		c.Memory.DataDir = expandPath(c.Memory.DataDir, home)
		c.Storage.DataDir = expandPath(c.Storage.DataDir, home)
	}
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

// resolveProvider determines provider identity from model name at config load time.
// This is a startup-time heuristic — not a runtime decision. Users should configure
// providers explicitly in config.yaml for reliable results.
func resolveProvider(model, explicitBaseURL string) (provider, baseURL string) {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "deepseek"):
		baseURL = "https://api.deepseek.com/v1"
		provider = "deepseek"
	case strings.Contains(m, "claude") || strings.Contains(m, "anthropic"):
		baseURL = "https://api.anthropic.com"
		provider = "anthropic"
	case strings.Contains(m, "gpt") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "o4"):
		baseURL = "https://api.openai.com/v1"
		provider = "openai"
	default:
		baseURL = "https://api.openai.com/v1"
		provider = "openai"
	}
	if explicitBaseURL != "" {
		baseURL = explicitBaseURL
	}
	return
}

func guessContextSize(model string) int {
	m := strings.ToLower(model)
	// Known model families with large context windows
	largeCtx := []string{"deepseek-v4", "deepseek-v3", "deepseek-r1", "gemini", "1m", "pro"}
	for _, prefix := range largeCtx {
		if strings.Contains(m, prefix) {
			return 1000000
		}
	}
	return 200000
}

// parseContextSize 解析 "1m" "200k" "128k" → token 数
func parseContextSize(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "")
	mult := 1
	if strings.HasSuffix(s, "k") {
		mult = 1000
		s = strings.TrimSuffix(s, "k")
	}
	if strings.HasSuffix(s, "m") {
		mult = 1000000
		s = strings.TrimSuffix(s, "m")
	}
	n := 0
	fmt.Sscanf(s, "%d", &n)
	if n > 0 {
		return n * mult
	}
	return 200000
}
