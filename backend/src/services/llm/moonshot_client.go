package llm

import (
	"context"
)

// MoonshotClient Moonshot (Kimi) 客户端
// Moonshot API 兼容 OpenAI 格式
type MoonshotClient struct {
	*OpenAICompatibleClient
}

// Moonshot 默认配置
const (
	MoonshotDefaultBaseURL = "https://api.moonshot.cn/v1"
	MoonshotDefaultModel   = "kimi-k2.5" // 最新旗舰模型
)

// NewMoonshotClient 创建 Moonshot 客户端
func NewMoonshotClient(config *ClientConfig) (*MoonshotClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = MoonshotDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: MoonshotDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &MoonshotClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *MoonshotClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *MoonshotClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
