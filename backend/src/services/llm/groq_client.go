package llm

import (
	"context"
)

// GroqClient Groq 客户端
// Groq API 兼容 OpenAI 格式
type GroqClient struct {
	*OpenAICompatibleClient
}

// Groq 默认配置
const (
	GroqDefaultBaseURL = "https://api.groq.com/openai/v1"
	GroqDefaultModel   = "llama3-8b-8192"
)

// NewGroqClient 创建 Groq 客户端
func NewGroqClient(config *ClientConfig) (*GroqClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = GroqDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: GroqDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &GroqClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *GroqClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *GroqClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
