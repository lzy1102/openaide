package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatibleClient OpenAI 兼容客户端
// 可配置用于支持 OpenAI 兼容 API 的提供商
type OpenAICompatibleClient struct {
	config       *ClientConfig
	httpClient   *http.Client
	defaultModel string
	endpointPath string
}

// OpenAICompatibleConfig OpenAI 兼容客户端额外配置
type OpenAICompatibleConfig struct {
	DefaultModel string // 默认模型
	EndpointPath string // API 端点路径，默认 "/chat/completions"
}

// NewOpenAICompatibleClient 创建 OpenAI 兼容客户端
func NewOpenAICompatibleClient(config *ClientConfig, extra *OpenAICompatibleConfig) (*OpenAICompatibleClient, error) {
	if config.APIKey == "" && config.Provider != ProviderOllama {
		// Ollama 本地部署可能不需要 API Key
		return nil, ErrEmptyAPIKey
	}

	// 设置默认端点路径
	endpointPath := "/chat/completions"
	if extra != nil && extra.EndpointPath != "" {
		endpointPath = extra.EndpointPath
	}

	// 设置默认模型
	defaultModel := ""
	if extra != nil {
		defaultModel = extra.DefaultModel
	}

	return &OpenAICompatibleClient{
		config:       config,
		defaultModel: defaultModel,
		endpointPath: endpointPath,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}, nil
}

// Chat 发送聊天请求
func (c *OpenAICompatibleClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = c.defaultModel
		}
	}

	// 构建请求体
	requestBody, err := c.buildChatRequest(req)
	if err != nil {
		return nil, err
	}

	// 发送请求 (带重试)
	var resp *http.Response
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		resp, err = c.sendRequest(ctx, "POST", c.endpointPath, requestBody)
		if err == nil {
			break
		}
		if attempt < c.config.MaxRetries {
			time.Sleep(time.Duration(c.config.RetryDelay) * time.Millisecond)
		}
	}

	if err != nil {
		return nil, &LLMError{Code: "request_failed", Message: "failed to send request", Details: err.Error()}
	}
	defer resp.Body.Close()

	// 解析响应
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleErrorResponse(resp)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, &LLMError{Code: "decode_error", Message: "failed to decode response", Details: err.Error()}
	}

	return &chatResp, nil
}

// ChatStream 发送流式聊天请求
func (c *OpenAICompatibleClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = c.defaultModel
		}
	}

	// 构建请求体
	requestBody, err := c.buildChatRequest(req)
	if err != nil {
		return nil, err
	}

	// 设置流式参数
	var reqMap map[string]interface{}
	if err := json.Unmarshal(requestBody, &reqMap); err == nil {
		reqMap["stream"] = true
		requestBody, _ = json.Marshal(reqMap)
	}

	// 创建响应通道
	chunkChan := make(chan ChatStreamChunk, 10)

	go func() {
		defer close(chunkChan)

		resp, err := c.sendRequest(ctx, "POST", c.endpointPath, requestBody)
		if err != nil {
			chunkChan <- ChatStreamChunk{Error: &LLMError{Code: "request_failed", Message: "failed to send request", Details: err.Error()}}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			chunkChan <- ChatStreamChunk{Error: c.handleErrorResponse(resp)}
			return
		}

		// 读取流式响应
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					chunkChan <- ChatStreamChunk{Error: &LLMError{Code: "read_error", Message: "failed to read stream", Details: err.Error()}}
				}
				break
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				// 某些提供商可能使用 "data:" 而不是 "data: "
				if !strings.HasPrefix(line, "data:") {
					continue
				}
			}

			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			if data == "[DONE]" {
				break
			}

			var chunk ChatStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			for i := range chunk.Choices {
				delta := &chunk.Choices[i].Delta
				if delta.ReasoningContent == "" {
					var raw map[string]interface{}
					if err := json.Unmarshal([]byte(data), &raw); err == nil {
						if choices, ok := raw["choices"].([]interface{}); ok && i < len(choices) {
							if choice, ok := choices[i].(map[string]interface{}); ok {
								if delta2, ok := choice["delta"].(map[string]interface{}); ok {
									if rc, ok := delta2["reasoning_content"].(string); ok && rc != "" {
										delta.ReasoningContent = rc
									}
								}
							}
						}
					}
				}
			}

			chunkChan <- chunk
		}
	}()

	return chunkChan, nil
}

// buildChatRequest 构建聊天请求体
func (c *OpenAICompatibleClient) buildChatRequest(req *ChatRequest) ([]byte, error) {
	type openaiMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	// 使用 map 而不是结构体，以便条件性添加字段
	reqMap := map[string]interface{}{
		"messages": make([]openaiMessage, len(req.Messages)),
	}

	// 仅当模型非空时添加 model 字段
	if req.Model != "" {
		reqMap["model"] = req.Model
	}

	// 添加其他参数
	if req.Temperature > 0 {
		reqMap["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		reqMap["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		reqMap["top_p"] = req.TopP
	}
	if len(req.Stop) > 0 {
		reqMap["stop"] = req.Stop
	}
	if req.PresencePenalty != 0 {
		reqMap["presence_penalty"] = req.PresencePenalty
	}
	if req.FrequencyPenalty != 0 {
		reqMap["frequency_penalty"] = req.FrequencyPenalty
	}

	// 填充 messages
	messages := make([]openaiMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openaiMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	reqMap["messages"] = messages

	return json.Marshal(reqMap)
}

// sendRequest 发送 HTTP 请求
func (c *OpenAICompatibleClient) sendRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := c.config.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Ollama 可能不需要 Authorization header
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	return c.httpClient.Do(req)
}

// handleErrorResponse 处理错误响应
func (c *OpenAICompatibleClient) handleErrorResponse(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)

	type apiError struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	var errResp apiError
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return &LLMError{
			Code:    errResp.Error.Code,
			Message: errResp.Error.Message,
			Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	return &LLMError{
		Code:    "http_error",
		Message: string(body),
		Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

// GetDefaultModel 获取默认模型
func (c *OpenAICompatibleClient) GetDefaultModel() string {
	return c.defaultModel
}

// SetDefaultModel 设置默认模型
func (c *OpenAICompatibleClient) SetDefaultModel(model string) {
	c.defaultModel = model
}
