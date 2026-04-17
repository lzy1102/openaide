package llm

import (
	"context"
)

// MiniMaxClient MiniMax 客户端
// MiniMax API 兼容 OpenAI 格式
type MiniMaxClient struct {
	*OpenAICompatibleClient
}

// MiniMax 默认配置
const (
	MiniMaxDefaultBaseURL = "https://api.minimax.chat/v1"
	MiniMaxDefaultModel   = "abab6.5-chat"
)

// NewMiniMaxClient 创建 MiniMax 客户端
func NewMiniMaxClient(config *ClientConfig) (*MiniMaxClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = MiniMaxDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: MiniMaxDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &MiniMaxClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *MiniMaxClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *MiniMaxClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
