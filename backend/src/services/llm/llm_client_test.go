package llm

import (
	"context"
	"testing"
)

// TestNewClient 测试创建客户端
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *ClientConfig
		wantErr bool
	}{
		{
			name: "OpenAI client",
			config: &ClientConfig{
				Provider: ProviderOpenAI,
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "Anthropic client",
			config: &ClientConfig{
				Provider: ProviderAnthropic,
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "GLM client",
			config: &ClientConfig{
				Provider: ProviderGLM,
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "Empty API key",
			config: &ClientConfig{
				Provider: ProviderOpenAI,
				APIKey:   "",
			},
			wantErr: true,
		},
		{
			name: "Unsupported provider",
			config: &ClientConfig{
				Provider: ProviderType("unknown"),
				APIKey:   "test-key",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client")
			}
		})
	}
}

// TestLLMError 测试 LLM 错误
func TestLLMError(t *testing.T) {
	err := &LLMError{
		Code:    "test_error",
		Message: "Test error message",
		Details: "Test details",
	}

	expected := "Test error message: Test details"
	if err.Error() != expected {
		t.Errorf("LLMError.Error() = %v, want %v", err.Error(), expected)
	}

	errWithoutDetails := &LLMError{
		Code:    "test_error",
		Message: "Test error message",
	}

	expected2 := "Test error message"
	if errWithoutDetails.Error() != expected2 {
		t.Errorf("LLMError.Error() = %v, want %v", errWithoutDetails.Error(), expected2)
	}
}

// TestChatRequestValidation 测试聊天请求验证
func TestChatRequestValidation(t *testing.T) {
	// 注意: 这些测试需要有效的 API key 才能运行
	// 在 CI/CD 环境中应该跳过或使用 mock

	t.Skip("Skipping API tests - requires valid API keys")

	config := &ClientConfig{
		Provider: ProviderOpenAI,
		APIKey:   "sk-test-key",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// 测试空消息
	_, err = client.Chat(context.Background(), &ChatRequest{
		Messages: []Message{},
	})

	if err != ErrEmptyMessages {
		t.Errorf("Expected ErrEmptyMessages, got %v", err)
	}
}
