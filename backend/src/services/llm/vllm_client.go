package llm

import (
	"context"
)

// VLLMClient vLLM 客户端
// vLLM API 兼容 OpenAI 格式
type VLLMClient struct {
	*OpenAICompatibleClient
}

// vLLM 默认配置
const (
	VLLMDefaultBaseURL = "http://localhost:8000/v1"
	VLLMDefaultModel   = ""
)

// NewVLLMClient 创建 vLLM 客户端
func NewVLLMClient(config *ClientConfig) (*VLLMClient, error) {
	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = VLLMDefaultBaseURL
	}

	// 创建 OpenAI 兼容客户端
	// vLLM 需要在请求中指定模型名称，不设置默认模型
	client, err := NewOpenAICompatibleClient(config, &OpenAICompatibleConfig{
		DefaultModel: VLLMDefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &VLLMClient{
		OpenAICompatibleClient: client,
	}, nil
}

// Chat 发送聊天请求
func (c *VLLMClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	return c.OpenAICompatibleClient.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (c *VLLMClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	return c.OpenAICompatibleClient.ChatStream(ctx, req)
}
