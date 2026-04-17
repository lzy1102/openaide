package tui

import (
	"time"

	"github.com/google/uuid"
)

// Config 配置结构
type Config struct {
	API    APIConfig     `yaml:"api"`
	Chat   ChatSettings  `yaml:"chat"`
	Models []ModelConfig `yaml:"models"`
}

// APIConfig API 配置
type APIConfig struct {
	BaseURL    string `yaml:"base_url"`
	TimeoutSec int    `yaml:"timeout_sec"`
}

// ChatSettings 聊天配置
type ChatSettings struct {
	DefaultModel string `yaml:"default_model"`
	Stream       bool   `yaml:"stream"`
	ContextLimit int    `yaml:"context_limit"`
}

// ModelConfig 模型配置
type ModelConfig struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Provider    string `yaml:"provider"`
	Description string `yaml:"description"`
}

// Model 模型信息（对齐后端）
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Provider    string `json:"provider"`
	Version     string `json:"version"`
	Status      string `json:"status"`
}

// Dialogue 对话结构（对齐后端）
type Dialogue struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// Message 消息结构（对齐后端，使用 Sender）
type Message struct {
	ID         string `json:"id"`
	DialogueID string `json:"dialogue_id"`
	Sender     string `json:"sender"` // user, assistant
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	ID      string  `json:"id"`
	Message Message `json:"message"`
	Model   string  `json:"model"`
	Usage   Usage   `json:"usage,omitempty"`
}

// Usage 使用量统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SystemInfo 系统信息
type SystemInfo struct {
	APIURL    string
	Version   string
	TotalMsgs int
	StartTime time.Time
}

// ModelSelectResult 模型选择器返回值
type ModelSelectResult struct {
	Selected string // 选中的模型名，空=取消
	Changed  bool
}

// SettingsResult 设置向导返回值
type SettingsResult struct {
	Saved  bool
	Config *Config
}

// DashboardAction 仪表盘返回动作
type DashboardAction struct {
	Action      string // "chat", "select_model", "config", "exit"
	Model       string
	DialogueID  string
}

// GenerateID 生成唯一 ID
func GenerateID() string {
	return uuid.New().String()
}
