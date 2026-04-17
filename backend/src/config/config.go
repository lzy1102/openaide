package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config 根配置结构
type Config struct {
	Models    []ModelConfig  `json:"models"`
	Feishu    FeishuConfig   `json:"feishu"`
	Voice     VoiceConfig    `json:"voice"`
	Sandbox   SandboxConfig  `json:"sandbox"`
	Email     EmailConfig    `json:"email"`
	Embedding EmbeddingConfig `json:"embedding"` // 嵌入服务配置（可选）
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

	// 优先级: 环境变量 > 当前目录 > 可执行文件目录 > 用户主目录
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

	// 当前目录
	cwd, _ := os.Getwd()
	if p := resolveConfigPath(filepath.Join(cwd, ".openaide")); p != "" {
		configPath = p
		return configPath
	}

	// 可执行文件目录
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
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

	// 默认: 用户主目录下的配置
	if homeDir != "" {
		configPath = filepath.Join(homeDir, ".openaide", "config.json")
	} else {
		configPath = filepath.Join(cwd, ".openaide")
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
			// 配置文件不存在，返回默认配置
			return &Config{Models: []ModelConfig{}}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
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

// GetExampleConfig 获取示例配置
func GetExampleConfig() *Config {
	return &Config{
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
