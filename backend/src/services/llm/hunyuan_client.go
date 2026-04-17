package llm

import (
	"context"
)

// HunyuanClient 腾讯混元客户端
// 腾讯混元 API 兼容 OpenAI 格式
type HunyuanClient struct {
	*OpenAICompatibleClient
}

// Hunyuan 默认配置
const (
	HunyuanDefaultBaseURL = "https://api.hunyuan.cloud.tencent.com/v1"
	HunyuanDefaultModel   = "hunyuan-lite"
)

// NewHunyuanClient 创建腾讯混元客户端
func NewHunyuanClient(config *ClientConfig) (*HunyuanClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = HunyuanDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: HunyuanDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &HunyuanClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *HunyuanClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *HunyuanClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
