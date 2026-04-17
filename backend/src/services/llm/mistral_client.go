package llm

import (
	"context"
)

// MistralClient Mistral AI 客户端
// Mistral API 兼容 OpenAI 格式
type MistralClient struct {
	*OpenAICompatibleClient
}

// Mistral 默认配置
const (
	MistralDefaultBaseURL = "https://api.mistral.ai/v1"
	MistralDefaultModel   = "mistral-small-latest"
)

// NewMistralClient 创建 Mistral 客户端
func NewMistralClient(config *ClientConfig) (*MistralClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = MistralDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: MistralDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &MistralClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *MistralClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *MistralClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
