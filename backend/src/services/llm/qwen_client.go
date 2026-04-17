package llm

import (
	"context"
)

// QwenClient 通义千问客户端
// 通义千问 API 兼容 OpenAI 格式
type QwenClient struct {
	*OpenAICompatibleClient
}

// Qwen 默认配置
const (
	QwenDefaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	QwenDefaultModel   = "qwen-turbo"
)

// NewQwenClient 创建通义千问客户端
func NewQwenClient(config *ClientConfig) (*QwenClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = QwenDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: QwenDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &QwenClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *QwenClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *QwenClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
