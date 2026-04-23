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

// GLMClient 智谱 AI (GLM) 客户端
type GLMClient struct {
	config     *ClientConfig
	httpClient *http.Client
}

// GLM 默认配置
const (
	GLMDefaultBaseURL = "https://open.bigmodel.cn/api/paas/v4"
	GLMDefaultModel   = "glm-5" // 最新旗舰模型
)

// NewGLMClient 创建 GLM 客户端
func NewGLMClient(config *ClientConfig) (*GLMClient, error) {
	if config.APIKey == "" {
		return nil, ErrEmptyAPIKey
	}

	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = GLMDefaultBaseURL
	}

	return &GLMClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}, nil
}

// Chat 发送聊天请求
func (c *GLMClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = "glm-4"
		}
	}

	// 构建请求体
	requestBody, err := c.buildChatRequest(req)
	if err != nil {
		return nil, err
	}

	// 生成 JWT token
	token, err := c.generateToken()
	if err != nil {
		return nil, &LLMError{Code: "token_error", Message: "failed to generate token", Details: err.Error()}
	}

	// 发送请求 (带重试)
	var resp *http.Response
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		resp, err = c.sendRequest(ctx, "POST", "/chat/completions", requestBody, token)
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
func (c *GLMClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = "glm-4"
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

	// 生成 JWT token
	token, err := c.generateToken()
	if err != nil {
		return nil, &LLMError{Code: "token_error", Message: "failed to generate token", Details: err.Error()}
	}

	// 创建响应通道
	chunkChan := make(chan ChatStreamChunk, 10)

	go func() {
		defer close(chunkChan)

		resp, err := c.sendRequest(ctx, "POST", "/chat/completions", requestBody, token)
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

			if !strings.HasPrefix(line, "data:") {
				continue
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

			chunkChan <- chunk
		}
	}()

	return chunkChan, nil
}

// buildChatRequest 构建聊天请求体
func (c *GLMClient) buildChatRequest(req *ChatRequest) ([]byte, error) {
	// GLM API 使用与 OpenAI 兼容的格式
	type glmMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type glmRequest struct {
		Model               string       `json:"model"`
		Messages            []glmMessage `json:"messages"`
		Temperature         float64      `json:"temperature,omitempty"`
		MaxTokens           int          `json:"max_tokens,omitempty"`
		MaxCompletionTokens int          `json:"max_completion_tokens,omitempty"`
		TopP                float64      `json:"top_p,omitempty"`
		Stop                []string     `json:"stop,omitempty"`
		PresencePenalty     float64      `json:"presence_penalty,omitempty"`
		FrequencyPenalty    float64      `json:"frequency_penalty,omitempty"`
	}

	messages := make([]glmMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = glmMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	glmReq := glmRequest{
		Model:               req.Model,
		Messages:            messages,
		Temperature:         req.Temperature,
		MaxTokens:           req.MaxTokens,
		MaxCompletionTokens: req.MaxTokens,
		TopP:                req.TopP,
		Stop:                req.Stop,
		PresencePenalty:  req.PresencePenalty,
		FrequencyPenalty: req.FrequencyPenalty,
	}

	return json.Marshal(glmReq)
}

// sendRequest 发送 HTTP 请求
func (c *GLMClient) sendRequest(ctx context.Context, method, path string, body []byte, token string) (*http.Response, error) {
	url := c.config.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	return c.httpClient.Do(req)
}

// handleErrorResponse 处理错误响应
func (c *GLMClient) handleErrorResponse(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)

	type glmError struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	var errResp glmError
	if err := json.Unmarshal(body, &errResp); err == nil {
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

// generateToken 生成 GLM API 认证 token
func (c *GLMClient) generateToken() (string, error) {
	// 智谱 API 直接使用 API Key 作为 Bearer token
	return c.config.APIKey, nil
}
