package llm

import (
	"context"
)

// OllamaClient Ollama 本地客户端
// Ollama API 兼容 OpenAI 格式
type OllamaClient struct {
	*OpenAICompatibleClient
}

// Ollama 默认配置
const (
	OllamaDefaultBaseURL = "http://localhost:11434/v1"
	OllamaDefaultModel   = "llama2"
)

// NewOllamaClient 创建 Ollama 客户端
func NewOllamaClient(config *ClientConfig) (*OllamaClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = OllamaDefaultBaseURL
	}

	// Ollama 本地部署可能不需要 API Key
	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: OllamaDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &OllamaClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *OllamaClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *OllamaClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
