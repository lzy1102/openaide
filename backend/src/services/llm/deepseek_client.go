package llm

import (
	"context"
)

// DeepSeekClient DeepSeek 客户端
// DeepSeek API 兼容 OpenAI 格式
type DeepSeekClient struct {
	*OpenAICompatibleClient
}

// DeepSeek 默认配置
const (
	DeepSeekDefaultBaseURL = "https://api.deepseek.com"
	DeepSeekDefaultModel   = "deepseek-chat"
)

// NewDeepSeekClient 创建 DeepSeek 客户端
func NewDeepSeekClient(config *ClientConfig) (*DeepSeekClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = DeepSeekDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: DeepSeekDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &DeepSeekClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *DeepSeekClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *DeepSeekClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
