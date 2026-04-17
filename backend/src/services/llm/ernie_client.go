package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErnieClient 文心一言 (百度 ERNIE) 客户端
type ErnieClient struct {
	config     *ClientConfig
	httpClient *http.Client
}

// Ernie 默认配置
const (
	ErnieDefaultBaseURL    = "https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop/chat"
	ErnieDefaultModel      = "ernie-4.0-8k"
	ErnieAccessTokenURL    = "https://aip.baidubce.com/oauth/2.0/token"
)

// 模型名称到 API 路径的映射
var ernieModelPaths = map[string]string{
	"ernie-4.0-8k":        "completions_pro",
	"ernie-4.0":           "completions_pro",
	"ernie-3.5-8k":        "completions",
	"ernie-3.5":           "completions",
	"ernie-speed-8k":      "ernie_speed",
	"ernie-speed":         "ernie_speed",
	"ernie-lite-8k":       "eb-instant",
	"ernie-lite":          "eb-instant",
	"ernie-tiny-8k":       "ernie-tiny",
	"ernie-tiny":          "ernie-tiny",
	"ernie-char-8k":       "ernie-char",
	"ernie-char":          "ernie-char",
}

// NewErnieClient 创建文心一言客户端
func NewErnieClient(config *ClientConfig) (*ErnieClient, error) {
	if config.APIKey == "" {
		return nil, ErrEmptyAPIKey
	}

	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = ErnieDefaultBaseURL
	}

	return &ErnieClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}, nil
}

// Chat 发送聊天请求
func (c *ErnieClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = ErnieDefaultModel
		}
	}

	// 获取 Access Token
	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, &LLMError{Code: "auth_error", Message: "failed to get access token", Details: err.Error()}
	}

	// 构建请求体
	requestBody, err := c.buildChatRequest(req)
	if err != nil {
		return nil, err
	}

	// 获取模型 API 路径
	modelPath, ok := ernieModelPaths[req.Model]
	if !ok {
		modelPath = "completions_pro" // 默认使用 ERNIE 4.0
	}

	// 构建请求 URL
	requestURL := fmt.Sprintf("%s/%s?access_token=%s", c.config.BaseURL, modelPath, accessToken)

	// 发送请求 (带重试)
	var resp *http.Response
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		resp, err = c.sendRequest(ctx, "POST", requestURL, requestBody)
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

	return c.parseChatResponse(resp, req.Model)
}

// ChatStream 发送流式聊天请求
func (c *ErnieClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = ErnieDefaultModel
		}
	}

	// 获取 Access Token
	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, &LLMError{Code: "auth_error", Message: "failed to get access token", Details: err.Error()}
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

	// 获取模型 API 路径
	modelPath, ok := ernieModelPaths[req.Model]
	if !ok {
		modelPath = "completions_pro"
	}

	// 构建请求 URL
	requestURL := fmt.Sprintf("%s/%s?access_token=%s", c.config.BaseURL, modelPath, accessToken)

	// 创建响应通道
	chunkChan := make(chan ChatStreamChunk, 10)

	go func() {
		defer close(chunkChan)

		resp, err := c.sendRequest(ctx, "POST", requestURL, requestBody)
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

			var chunk ernieStreamChunk
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				continue
			}

			if chunk.ErrorMsg != "" {
				chunkChan <- ChatStreamChunk{Error: &LLMError{Code: chunk.ErrorCode, Message: chunk.ErrorMsg}}
				break
			}

			chunkChan <- ChatStreamChunk{
				ID:      chunk.ID,
				Model:   req.Model,
				Created: chunk.Created,
				Choices: []StreamChoice{
					{
						Index: 0,
						Delta: MessageDelta{
							Content: chunk.Result,
						},
						FinishReason: func() string {
							if chunk.IsEnd == 1 {
								return "stop"
							}
							return ""
						}(),
					},
				},
			}

			if chunk.IsEnd == 1 {
				break
			}
		}
	}()

	return chunkChan, nil
}

// ernieStreamChunk 文心一言流式响应结构
type ernieStreamChunk struct {
	ID        string `json:"id"`
	Created   int64  `json:"created"`
	Result    string `json:"result"`
	IsEnd     int    `json:"is_end"`
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`
}

// buildChatRequest 构建聊天请求体
func (c *ErnieClient) buildChatRequest(req *ChatRequest) ([]byte, error) {
	// 文心一言使用不同的消息格式
	type ernieMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type ernieRequest struct {
		Messages    []ernieMessage `json:"messages"`
		Temperature float64        `json:"temperature,omitempty"`
		TopP        float64        `json:"top_p,omitempty"`
		MaxOutput   int            `json:"max_output_tokens,omitempty"`
		Stream      bool           `json:"stream,omitempty"`
	}

	// 转换消息格式
	messages := make([]ernieMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = ernieMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	ernieReq := ernieRequest{
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxOutput:   req.MaxTokens,
	}

	return json.Marshal(ernieReq)
}

// parseChatResponse 解析聊天响应
func (c *ErnieClient) parseChatResponse(resp *http.Response, model string) (*ChatResponse, error) {
	type ernieResponse struct {
		ID               string `json:"id"`
		Object           string `json:"object"`
		Created          int64  `json:"created"`
		Result           string `json:"result"`
		IsTruncated      bool   `json:"is_truncated"`
		NeedClearHistory bool   `json:"need_clear_history"`
		Usage            struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		ErrorCode string `json:"error_code,omitempty"`
		ErrorMsg  string `json:"error_msg,omitempty"`
	}

	var ernieResp ernieResponse
	if err := json.NewDecoder(resp.Body).Decode(&ernieResp); err != nil {
		return nil, &LLMError{Code: "decode_error", Message: "failed to decode response", Details: err.Error()}
	}

	if ernieResp.ErrorCode != "" {
		return nil, &LLMError{Code: ernieResp.ErrorCode, Message: ernieResp.ErrorMsg}
	}

	return &ChatResponse{
		ID:      ernieResp.ID,
		Object:  "chat.completion",
		Created: ernieResp.Created,
		Model:   model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: ernieResp.Result,
				},
				FinishReason: "stop",
			},
		},
		Usage: &Usage{
			PromptTokens:     ernieResp.Usage.PromptTokens,
			CompletionTokens: ernieResp.Usage.CompletionTokens,
			TotalTokens:      ernieResp.Usage.TotalTokens,
		},
	}, nil
}

// getAccessToken 获取 Access Token
func (c *ErnieClient) getAccessToken(ctx context.Context) (string, error) {
	// 解析 API Key: 格式为 "apiKey:secretKey"
	parts := strings.Split(c.config.APIKey, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid API key format, expected 'apiKey:secretKey'")
	}
	apiKey := parts[0]
	secretKey := parts[1]

	// 构建请求 URL
	tokenURL := fmt.Sprintf("%s?grant_type=client_credentials&client_id=%s&client_secret=%s",
		ErnieAccessTokenURL, url.QueryEscape(apiKey), url.QueryEscape(secretKey))

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error,omitempty"`
		ErrorDesc   string `json:"error_description,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return tokenResp.AccessToken, nil
}

// sendRequest 发送 HTTP 请求
func (c *ErnieClient) sendRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// handleErrorResponse 处理错误响应
func (c *ErnieClient) handleErrorResponse(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)

	type ernieError struct {
		ErrorCode string `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}

	var errResp ernieError
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.ErrorMsg != "" {
		return &LLMError{
			Code:    errResp.ErrorCode,
			Message: errResp.ErrorMsg,
			Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	return &LLMError{
		Code:    "http_error",
		Message: string(body),
		Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}
