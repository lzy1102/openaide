package llm

import (
	"context"
	"io"
)

// LLMClient 统一的 LLM 客户端接口
type LLMClient interface {
	// Chat 发送聊天请求并返回响应
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// ChatStream 发送聊天请求并返回流式响应
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error)
}

// ChatRequest 聊天请求
type ChatRequest struct {
	// Messages 对话消息列表
	Messages []Message `json:"messages"`

	// Model 模型名称
	Model string `json:"model"`

	// Temperature 温度参数 (0.0 - 2.0)
	Temperature float64 `json:"temperature,omitempty"`

	// MaxTokens 最大生成 token 数
	MaxTokens int `json:"max_tokens,omitempty"`

	// TopP 采样参数
	TopP float64 `json:"top_p,omitempty"`

	// Stop 停止序列
	Stop []string `json:"stop,omitempty"`

	// PresencePenalty 存在惩罚 (-2.0 - 2.0)
	PresencePenalty float64 `json:"presence_penalty,omitempty"`

	// FrequencyPenalty 频率惩罚 (-2.0 - 2.0)
	FrequencyPenalty float64 `json:"frequency_penalty,omitempty"`

	// System 系统提示词 (部分 API 支持)
	System string `json:"system,omitempty"`

	// Tools 可用工具定义列表
	Tools []ToolDefinition `json:"tools,omitempty"`

	// ToolChoice 工具选择策略: "auto", "none", "required", 或指定工具
	ToolChoice interface{} `json:"tool_choice,omitempty"`
}

// Message 消息
type Message struct {
	// Role 角色: system, user, assistant, tool
	Role string `json:"role"`

	// Content 消息内容
	Content string `json:"content"`

	// ReasoningContent 推理/思考过程内容 (DeepSeek reasoning_content, etc.)
	ReasoningContent string `json:"reasoning_content,omitempty"`

	// Name 消息名称 (可选)
	Name string `json:"name,omitempty"`

	// ToolCalls assistant 发起的工具调用 (仅 assistant 角色)
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID tool 角色的消息，关联到哪个工具调用
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	// ID 响应 ID
	ID string `json:"id"`

	// Object 对象类型
	Object string `json:"object"`

	// Created 创建时间戳
	Created int64 `json:"created"`

	// Model 使用的模型
	Model string `json:"model"`

	// Choices 响应选项
	Choices []Choice `json:"choices"`

	// Usage token 使用情况
	Usage *Usage `json:"usage,omitempty"`

	// RawResponse 原始响应数据 (可选)
	RawResponse map[string]interface{} `json:"raw_response,omitempty"`
}

// Choice 响应选项
type Choice struct {
	// Index 选项索引
	Index int `json:"index"`

	// Message 消息内容
	Message Message `json:"message"`

	// FinishReason 结束原因
	FinishReason string `json:"finish_reason"`
}

// Usage token 使用情况
type Usage struct {
	// PromptTokens 提示词 token 数
	PromptTokens int `json:"prompt_tokens"`

	// CompletionTokens 完成 token 数
	CompletionTokens int `json:"completion_tokens"`

	// TotalTokens 总 token 数
	TotalTokens int `json:"total_tokens"`
}

// ChatStreamChunk 流式响应数据块
type ChatStreamChunk struct {
	// ID 数据块 ID
	ID string `json:"id"`

	// Object 对象类型
	Object string `json:"object"`

	// Created 创建时间戳
	Created int64 `json:"created"`

	// Model 使用的模型
	Model string `json:"model"`

	// Choices 响应选项
	Choices []StreamChoice `json:"choices"`

	// Usage token 使用情况 (仅在最后一个数据块中)
	Usage *Usage `json:"usage,omitempty"`

	// Error 错误信息
	Error error `json:"error,omitempty"`
}

// StreamChoice 流式响应选项
type StreamChoice struct {
	// Index 选项索引
	Index int `json:"index"`

	// Delta 增量内容
	Delta MessageDelta `json:"delta"`

	// FinishReason 结束原因
	FinishReason string `json:"finish_reason"`
}

// MessageDelta 消息增量
type MessageDelta struct {
	// Role 角色 (仅第一个数据块)
	Role string `json:"role,omitempty"`

	// Content 增量内容
	Content string `json:"content,omitempty"`

	// ReasoningContent 推理/思考过程增量内容 (DeepSeek reasoning_content, etc.)
	ReasoningContent string `json:"reasoning_content,omitempty"`

	// ToolCalls 工具调用增量 (流式)
	ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"`
}

// ProviderType LLM 提供商类型
type ProviderType string

const (
	// ProviderOpenAI OpenAI 提供商
	ProviderOpenAI ProviderType = "openai"

	// ProviderAnthropic Anthropic (Claude) 提供商
	ProviderAnthropic ProviderType = "anthropic"

	// ProviderGLM 智谱 AI 提供商
	ProviderGLM ProviderType = "glm"

	// 国内大模型
	// ProviderQwen 通义千问 (阿里云)
	ProviderQwen ProviderType = "qwen"

	// ProviderErnie 文心一言 (百度)
	ProviderErnie ProviderType = "ernie"

	// ProviderHunyuan 混元 (腾讯)
	ProviderHunyuan ProviderType = "hunyuan"

	// ProviderSpark 星火 (讯飞)
	ProviderSpark ProviderType = "spark"

	// ProviderMoonshot Moonshot (Kimi)
	ProviderMoonshot ProviderType = "moonshot"

	// ProviderBaichuan 百川
	ProviderBaichuan ProviderType = "baichuan"

	// ProviderMiniMax MiniMax
	ProviderMiniMax ProviderType = "minimax"

	// ProviderDeepSeek DeepSeek
	ProviderDeepSeek ProviderType = "deepseek"

	// 国际大模型
	// ProviderGemini Google Gemini
	ProviderGemini ProviderType = "gemini"

	// ProviderMistral Mistral AI
	ProviderMistral ProviderType = "mistral"

	// ProviderCohere Cohere
	ProviderCohere ProviderType = "cohere"

	// ProviderGroq Groq
	ProviderGroq ProviderType = "groq"

	// 本地模型
	// ProviderOllama Ollama
	ProviderOllama ProviderType = "ollama"

	// ProviderVLLM vLLM
	ProviderVLLM ProviderType = "vllm"
)

// 消息角色常量
const (
	// RoleSystem 系统角色
	RoleSystem = "system"

	// RoleUser 用户角色
	RoleUser = "user"

	// RoleAssistant 助手角色
	RoleAssistant = "assistant"

	// RoleTool 工具角色
	RoleTool = "tool"
)

// ClientConfig LLM 客户端配置
type ClientConfig struct {
	// Provider 提供商类型
	Provider ProviderType `json:"provider"`

	// APIKey API 密钥
	APIKey string `json:"api_key"`

	// BaseURL API 基础 URL
	BaseURL string `json:"base_url"`

	// Model 默认模型名称
	Model string `json:"model"`

	// Timeout 请求超时时间 (秒)
	Timeout int `json:"timeout"`

	// MaxRetries 最大重试次数
	MaxRetries int `json:"max_retries"`

	// RetryDelay 重试延迟 (毫秒)
	RetryDelay int `json:"retry_delay"`
}

// NewClient 创建 LLM 客户端
func NewClient(config *ClientConfig) (LLMClient, error) {
	// 设置默认值
	if config.Timeout == 0 {
		config.Timeout = 60
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1000
	}

	switch config.Provider {
	// 原有提供商
	case ProviderOpenAI:
		return NewOpenAIClient(config)
	case ProviderAnthropic:
		return NewAnthropicClient(config)
	case ProviderGLM:
		return NewGLMClient(config)

	// OpenAI 兼容的国内大模型
	case ProviderQwen:
		return NewQwenClient(config)
	case ProviderMoonshot:
		return NewMoonshotClient(config)
	case ProviderBaichuan:
		return NewBaichuanClient(config)
	case ProviderDeepSeek:
		return NewDeepSeekClient(config)
	case ProviderHunyuan:
		return NewHunyuanClient(config)
	case ProviderMiniMax:
		return NewMiniMaxClient(config)

	// OpenAI 兼容的国际大模型
	case ProviderMistral:
		return NewMistralClient(config)
	case ProviderGroq:
		return NewGroqClient(config)

	// OpenAI 兼容的本地模型
	case ProviderOllama:
		return NewOllamaClient(config)
	case ProviderVLLM:
		return NewVLLMClient(config)

	// 自定义 API 的模型
	case ProviderErnie:
		return NewErnieClient(config)
	case ProviderSpark:
		return NewSparkClient(config)
	case ProviderGemini:
		return NewGeminiClient(config)
	case ProviderCohere:
		return NewCohereClient(config)

	default:
		return nil, ErrUnsupportedProvider
	}
}

// 错误定义
var (
	// ErrUnsupportedProvider 不支持的提供商
	ErrUnsupportedProvider = &LLMError{Code: "unsupported_provider", Message: "unsupported LLM provider"}

	// ErrEmptyAPIKey API 密钥为空
	ErrEmptyAPIKey = &LLMError{Code: "empty_api_key", Message: "API key is required"}

	// ErrEmptyMessages 消息列表为空
	ErrEmptyMessages = &LLMError{Code: "empty_messages", Message: "messages cannot be empty"}

	// ErrInvalidResponse 无效响应
	ErrInvalidResponse = &LLMError{Code: "invalid_response", Message: "invalid API response"}

	// ErrRequestFailed 请求失败
	ErrRequestFailed = &LLMError{Code: "request_failed", Message: "request failed"}
)

// LLMError LLM 错误
type LLMError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *LLMError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// StreamReader 流式读取器接口
type StreamReader interface {
	// Recv 接收下一个数据块
	Recv() (*ChatStreamChunk, error)

	// Close 关闭流
	Close() error
}

// ioReaderCloser 将 io.ReadCloser 包装为 StreamReader
type ioReaderCloser struct {
	reader io.ReadCloser
	decoder func([]byte) (*ChatStreamChunk, error)
}

func (r *ioReaderCloser) Recv() (*ChatStreamChunk, error) {
	return nil, nil
}

func (r *ioReaderCloser) Close() error {
	return r.reader.Close()
}

// ToolDefinition 工具定义 (OpenAI function calling 格式)
type ToolDefinition struct {
	Type     string       `json:"type"`
	Function FunctionDef  `json:"function"`
}

// FunctionDef 函数定义
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall LLM 发起的工具调用
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用详情
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCallDelta 流式工具调用增量
type ToolCallDelta struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function *FunctionDelta `json:"function,omitempty"`
}

// FunctionDelta 流式函数调用增量
type FunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
