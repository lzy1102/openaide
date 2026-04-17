package llm

import (
	"strings"
	"testing"
)

func TestOpenAICompatibleClient_NoModel(t *testing.T) {
	// 测试当 model 为空时，构建的请求体不包含 model 字段
	config := &ClientConfig{
		APIKey:  "test-key",
		BaseURL: "https://openrouter.ai/openrouter/free",
		Model:   "", // 空模型
	}

	client, err := NewOpenAICompatibleClient(config, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// 构建请求
	req := &ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Model: "", // 空模型
	}

	// 构建请求体
	reqBody, err := client.buildChatRequest(req)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	reqStr := string(reqBody)
	if contains(reqStr, `"model":`) {
		t.Error("Request body should not contain 'model' field when model is empty")
	}

	if !contains(reqStr, `"messages":`) {
		t.Error("Request body should contain 'messages' field")
	}

	t.Logf("Request body (no model): %s", reqStr)
}

func TestOpenAICompatibleClient_WithModel(t *testing.T) {
	// 测试当 model 不为空时，构建的请求体包含 model 字段
	config := &ClientConfig{
		APIKey:  "test-key",
		BaseURL: "https://openrouter.ai/api/v1",
		Model:   "google/gemma-4-31b-it:free",
	}

	client, err := NewOpenAICompatibleClient(config, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// 构建请求
	req := &ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Model: "google/gemma-4-31b-it:free", // 显式指定模型
	}

	// 构建请求体
	reqBody, err := client.buildChatRequest(req)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	reqStr := string(reqBody)
	if !contains(reqStr, `"model":"google/gemma-4-31b-it:free"`) {
		t.Error("Request body should contain 'model' field when model is specified")
	}

	t.Logf("Request body (with model): %s", reqStr)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && strings.Contains(s, substr)
}
