package llm

import (
	"context"
)

// BaichuanClient 百川客户端
// 百川 API 兼容 OpenAI 格式
type BaichuanClient struct {
	*OpenAICompatibleClient
}

// Baichuan 默认配置
const (
	BaichuanDefaultBaseURL = "https://api.baichuan-ai.com/v1"
	BaichuanDefaultModel   = "Baichuan2-Turbo"
)

// NewBaichuanClient 创建百川客户端
func NewBaichuanClient(config *ClientConfig) (*BaichuanClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = BaichuanDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: BaichuanDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &BaichuanClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *BaichuanClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *BaichuanClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
