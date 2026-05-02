package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// StorageConfig 存储配置
type StorageConfig struct {
	Cache      CacheConfig       `json:"cache"`
	DB         DBConfig          `json:"db"`
	VectorStore VectorStoreConfig `json:"vector_store"` // 向量存储配置
}

// VectorStoreConfig 向量存储配置
type VectorStoreConfig struct {
	Type    string `json:"type"`     // "hnsw", "memory", "pinecone", "weaviate", "milvus", "qdrant", "chroma"
	DataDir string `json:"data_dir"` // 数据目录（HNSW 使用）

	// 外部向量数据库配置
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Cloud     string `json:"cloud,omitempty"`  // Pinecone cloud
	Region    string `json:"region,omitempty"` // Pinecone region
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Type              string `json:"type"`                // "memory", "ledis", "redis"
	DefaultExpiration int    `json:"default_expiration"`  // 默认过期时间（秒）
	CleanupInterval   int    `json:"cleanup_interval"`    // 清理间隔（秒）
	DataDir           string `json:"data_dir,omitempty"`  // LedisDB 数据目录
	RedisAddr         string `json:"redis_addr,omitempty"`
	RedisPassword     string `json:"redis_password,omitempty"`
	RedisDB           int    `json:"redis_db,omitempty"`
}

// DBConfig 数据库配置
type DBConfig struct {
	Type string `json:"type"` // "sqlite", "postgres", "mysql"
	URI  string `json:"uri"`  // 连接 URI/DSN（优先使用）
	// 以下为分开配置（当 URI 为空时使用）
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
	SSLMode  string `json:"ssl_mode,omitempty"` // postgres only
}

// ServerConfig 服务配置
type ServerConfig struct {
	Port      int  `json:"port,omitempty"`       // 监听端口，默认 19375
	LocalMode bool `json:"local_mode,omitempty"` // 本地模式，免认证
}

// Config 根配置结构
type Config struct {
	HomeDir         string          `json:"home_dir,omitempty"` // 数据主目录，默认 ~/.openaide
	Server          ServerConfig    `json:"server"`             // 服务配置
	Models          []ModelConfig   `json:"models"`
	Feishu          FeishuConfig    `json:"feishu"`
	Voice           VoiceConfig     `json:"voice"`
	Sandbox         SandboxConfig   `json:"sandbox"`
	Email           EmailConfig     `json:"email"`
	Embedding       EmbeddingConfig `json:"embedding"` // 嵌入服务配置（可选）
	Context         ContextConfig   `json:"context"`   // 上下文引擎配置 (Hermes Agent)
	ActivityTimeout string          `json:"activity_timeout"` // 基于活动超时时间 (Hermes Agent)
	Storage         StorageConfig   `json:"storage"`          // 存储配置（缓存+数据库）
}

// ContextConfig 上下文引擎配置 (Hermes Agent)
type ContextConfig struct {
	CompressionEnabled bool   `json:"compression_enabled"`
	CompressionMode    string `json:"compression_mode"`
	MaxTokens          int    `json:"max_tokens"`
	KeepLastN          int    `json:"keep_last_n"`
	PreserveToolCalls  bool   `json:"preserve_tool_calls"`
	FallbackToSummary  bool   `json:"fallback_to_summary"`
}

// EmbeddingConfig 嵌入服务配置
type EmbeddingConfig struct {
	Enabled  bool   `json:"enabled"`   // 是否启用语义搜索
	Provider string `json:"provider"`  // 提供商: openai, ollama
	APIKey   string `json:"api_key"`   // API Key
	Model    string `json:"model"`     // 模型名称
	BaseURL  string `json:"base_url"`  // 自定义 BaseURL（可选）
}

// EmailConfig 邮件服务配置
type EmailConfig struct {
	Enabled  bool   `json:"enabled"`
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	UseTLS   bool   `json:"use_tls"`
}

// VoiceConfig 语音服务配置
type VoiceConfig struct {
	Enabled     bool   `json:"enabled"`
	WhisperAPI  string `json:"whisper_api"`
	WhisperKey  string `json:"whisper_key"`
	TTSAPI      string `json:"tts_api"`
	TTSKey      string `json:"tts_key"`
	TTSVoice    string `json:"tts_voice"`
	DefaultLang string `json:"default_lang"`
}

// SandboxConfig 沙箱配置
type SandboxConfig struct {
	Enabled     bool   `json:"enabled"`
	DockerImage string `json:"docker_image"`
	Timeout     int    `json:"timeout"`
	MaxMemoryMB int    `json:"max_memory_mb"`
}

// FeishuConfig 飞书机器人配置
type FeishuConfig struct {
	Enabled        bool   `json:"enabled"`
	AppID          string `json:"app_id"`
	AppSecret      string `json:"app_secret"`
	DefaultModel   string `json:"default_model"`
	SystemPrompt   string `json:"system_prompt"`
	StreamInterval int    `json:"stream_interval"`
}

// ModelConfig 模型配置
type ModelConfig struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Type        string                 `json:"type"`
	Provider    string                 `json:"provider"`
	Version     string                 `json:"version,omitempty"`
	APIKey      string                 `json:"api_key,omitempty"`
	BaseURL     string                 `json:"base_url,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Status      string                 `json:"status"`
}

var (
	config     *Config
	configOnce sync.Once
	configPath string
)

// GetConfigPath 获取配置文件路径
func GetConfigPath() string {
	if configPath != "" {
		return configPath
	}

	// 优先级: 环境变量 > /app/config.json(Docker) > 当前目录 > 可执行文件目录 > 用户主目录
	if p := os.Getenv("OPENAIDE_CONFIG"); p != "" {
		configPath = p
		return configPath
	}

	resolveConfigPath := func(p string) string {
		info, err := os.Stat(p)
		if err != nil {
			return ""
		}
		if info.IsDir() {
			cfgInDir := filepath.Join(p, "config.json")
			if _, err := os.Stat(cfgInDir); err == nil {
				return cfgInDir
			}
			return ""
		}
		return p
	}

	// Docker 容器内默认路径
	if p := resolveConfigPath("/app/config.json"); p != "" {
		configPath = p
		return configPath
	}

	// OPENAIDE_HOME 目录下
	if home := os.Getenv("OPENAIDE_HOME"); home != "" {
		if p := resolveConfigPath(filepath.Join(home, "config.json")); p != "" {
			configPath = p
			return configPath
		}
	}

	// 当前目录
	cwd, _ := os.Getwd()
	if p := resolveConfigPath(filepath.Join(cwd, "config.json")); p != "" {
		configPath = p
		return configPath
	}
	if p := resolveConfigPath(filepath.Join(cwd, ".openaide")); p != "" {
		configPath = p
		return configPath
	}

	// 可执行文件目录
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	if p := resolveConfigPath(filepath.Join(execDir, "config.json")); p != "" {
		configPath = p
		return configPath
	}
	if p := resolveConfigPath(filepath.Join(execDir, ".openaide")); p != "" {
		configPath = p
		return configPath
	}

	// 用户主目录
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		if p := resolveConfigPath(filepath.Join(homeDir, ".openaide")); p != "" {
			configPath = p
			return configPath
		}
	}

	// 默认: OPENAIDE_HOME 或用户主目录下
	if home := os.Getenv("OPENAIDE_HOME"); home != "" {
		configPath = filepath.Join(home, "config.json")
	} else if homeDir != "" {
		configPath = filepath.Join(homeDir, ".openaide", "config.json")
	} else {
		configPath = filepath.Join(cwd, "config.json")
	}
	return configPath
}

// Load 加载配置文件
func Load() (*Config, error) {
	var err error
	configOnce.Do(func() {
		config, err = loadConfig()
	})
	return config, err
}

func loadConfig() (*Config, error) {
	path := GetConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				Models: []ModelConfig{},
				Context: ContextConfig{
					CompressionEnabled: true,
					CompressionMode:    "balanced",
					MaxTokens:          8000,
					KeepLastN:          4,
					PreserveToolCalls:  true,
					FallbackToSummary:  true,
				},
				ActivityTimeout: "30m",
			}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 默认值
	if cfg.Context.CompressionMode == "" {
		cfg.Context.CompressionMode = "balanced"
	}
	if cfg.Context.MaxTokens == 0 {
		cfg.Context.MaxTokens = 8000
	}
	if cfg.Context.KeepLastN == 0 {
		cfg.Context.KeepLastN = 4
	}
	if cfg.ActivityTimeout == "" {
		cfg.ActivityTimeout = "30m"
	}

	return &cfg, nil
}

// Save 保存配置文件
func Save(cfg *Config) error {
	path := GetConfigPath()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Validate 验证配置合法性
func (c *Config) Validate() error {
	if c.HomeDir == "" {
		return fmt.Errorf("home_dir cannot be empty")
	}

	// 验证存储配置
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("storage config invalid: %w", err)
	}

	// 验证模型配置
	if len(c.Models) == 0 {
		return fmt.Errorf("at least one model must be configured")
	}
	for i, model := range c.Models {
		if model.Name == "" {
			return fmt.Errorf("model[%d].name cannot be empty", i)
		}
		if model.APIKey == "" {
			return fmt.Errorf("model[%d].api_key cannot be empty", i)
		}
	}

	return nil
}

// Validate 验证存储配置
func (s *StorageConfig) Validate() error {
	// 验证数据库配置
	validDBTypes := map[string]bool{"sqlite": true, "postgres": true, "mysql": true}
	if !validDBTypes[s.DB.Type] {
		return fmt.Errorf("invalid db.type: %s, must be one of: sqlite, postgres, mysql", s.DB.Type)
	}
	if s.DB.Type == "sqlite" && s.DB.URI == "" {
		return fmt.Errorf("db.uri cannot be empty when using sqlite")
	}

	// 验证缓存配置
	validCacheTypes := map[string]bool{"memory": true, "ledis": true, "redis": true}
	if !validCacheTypes[s.Cache.Type] {
		return fmt.Errorf("invalid cache.type: %s, must be one of: memory, ledis, redis", s.Cache.Type)
	}

	// 验证向量存储配置
	validVectorTypes := map[string]bool{"hnsw": true, "memory": true, "pinecone": true, "weaviate": true, "milvus": true, "qdrant": true, "chroma": true}
	if !validVectorTypes[s.VectorStore.Type] {
		return fmt.Errorf("invalid vector_store.type: %s", s.VectorStore.Type)
	}

	return nil
}

// GetExampleConfig 获取示例配置
func GetExampleConfig() *Config {
	return &Config{
		HomeDir: "/opt/openaide", // 服务器部署默认使用 /opt/openaide
		Storage: StorageConfig{
			Cache: CacheConfig{
			Type:              "ledis", // 使用 LedisDB 缓存（兼容 Redis 协议，可平滑迁移）
			DefaultExpiration: 3600,
			CleanupInterval:   600,
			DataDir:           "/opt/openaide/data/ledis",
		},
			DB: DBConfig{
				Type: "sqlite",
				URI:  "/opt/openaide/data/db/openaide.db",
			},
			VectorStore: VectorStoreConfig{
				Type:    "hnsw",
				DataDir: "/opt/openaide/data/vectors",
			},
		},
		Models: []ModelConfig{
			{
				Name:        "gpt-4",
				Description: "OpenAI GPT-4 model",
				Type:        "llm",
				Provider:    "openai",
				Version:     "2024-01-01",
				APIKey:      "sk-your-openai-api-key-here",
				BaseURL:     "https://api.openai.com/v1",
				Config: map[string]interface{}{
					"timeout":     60,
					"max_retries": 3,
					"retry_delay": 1000,
				},
				Status: "enabled",
			},
			{
				Name:        "deepseek-chat",
				Description: "DeepSeek Chat 模型",
				Type:        "llm",
				Provider:    "deepseek",
				APIKey:      "your-deepseek-api-key-here",
				BaseURL:     "https://api.deepseek.com",
				Config: map[string]interface{}{
					"model":   "deepseek-chat",
					"timeout": 60,
				},
				Status: "enabled",
			},
			{
				Name:        "qwen-turbo",
				Description: "阿里云通义千问 Turbo",
				Type:        "llm",
				Provider:    "qwen",
				APIKey:      "your-dashscope-api-key-here",
				BaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
				Config: map[string]interface{}{
					"model":   "qwen-turbo",
					"timeout": 60,
				},
				Status: "enabled",
			},
			{
				Name:        "ollama-llama2",
				Description: "Ollama 本地 Llama2 模型 (无需 API Key)",
				Type:        "llm",
				Provider:    "ollama",
				BaseURL:     "http://localhost:11434/v1",
				Config: map[string]interface{}{
					"model":   "llama2",
					"timeout": 120,
				},
				Status: "enabled",
			},
			{
				Name:        "glm-5",
				Description: "智谱 GLM-5 旗舰模型",
				Type:        "llm",
				Provider:    "glm",
				APIKey:      "your-glm-api-key-id.secret",
				BaseURL:     "https://open.bigmodel.cn/api/paas/v4",
				Config: map[string]interface{}{
					"model":   "glm-5",
					"timeout": 60,
				},
				Status: "enabled",
			},
		},
	}
}

// CreateExampleConfig 创建示例配置文件
func CreateExampleConfig() error {
	path := GetConfigPath()

	// 如果已存在，不覆盖
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	cfg := GetExampleConfig()
	return Save(cfg)
}
