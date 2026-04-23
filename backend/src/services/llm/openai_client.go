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

// OpenAIClient OpenAI 客户端
type OpenAIClient struct {
	config     *ClientConfig
	httpClient *http.Client
}

// NewOpenAIClient 创建 OpenAI 客户端
func NewOpenAIClient(config *ClientConfig) (*OpenAIClient, error) {
	if config.APIKey == "" {
		return nil, ErrEmptyAPIKey
	}

	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}

	return &OpenAIClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}, nil
}

// Chat 发送聊天请求
func (c *OpenAIClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = "gpt-3.5-turbo"
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
		resp, err = c.sendRequest(ctx, "POST", "/chat/completions", requestBody)
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
func (c *OpenAIClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = "gpt-3.5-turbo"
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

		resp, err := c.sendRequest(ctx, "POST", "/chat/completions", requestBody)
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
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
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
func (c *OpenAIClient) buildChatRequest(req *ChatRequest) ([]byte, error) {
	// OpenAI API 请求格式
	type openaiMessage struct {
		Role       string     `json:"role"`
		Content    string     `json:"content"`
		ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
		ToolCallID string     `json:"tool_call_id,omitempty"`
		Name       string     `json:"name,omitempty"`
	}

	type openaiToolDef struct {
		Type     string      `json:"type"`
		Function FunctionDef `json:"function"`
	}

	type openaiRequest struct {
		Model               string          `json:"model"`
		Messages            []openaiMessage `json:"messages"`
		Temperature         float64         `json:"temperature,omitempty"`
		MaxTokens           int             `json:"max_tokens,omitempty"`
		MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
		TopP                float64         `json:"top_p,omitempty"`
		Stop                []string        `json:"stop,omitempty"`
		PresencePenalty     float64         `json:"presence_penalty,omitempty"`
		FrequencyPenalty    float64         `json:"frequency_penalty,omitempty"`
		Tools               []openaiToolDef `json:"tools,omitempty"`
		ToolChoice          interface{}     `json:"tool_choice,omitempty"`
	}

	messages := make([]openaiMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openaiMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
		}
	}

	openaiReq := openaiRequest{
		Model:               req.Model,
		Messages:            messages,
		Temperature:         req.Temperature,
		MaxTokens:           req.MaxTokens,
		MaxCompletionTokens: req.MaxTokens,
		TopP:                req.TopP,
		Stop:                req.Stop,
		PresencePenalty:     req.PresencePenalty,
		FrequencyPenalty:    req.FrequencyPenalty,
		ToolChoice:          req.ToolChoice,
	}

	// 转换工具定义
	if len(req.Tools) > 0 {
		openaiReq.Tools = make([]openaiToolDef, len(req.Tools))
		for i, t := range req.Tools {
			openaiReq.Tools[i] = openaiToolDef{
				Type:     t.Type,
				Function: t.Function,
			}
		}
	}

	return json.Marshal(openaiReq)
}

// sendRequest 发送 HTTP 请求
func (c *OpenAIClient) sendRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := c.config.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	return c.httpClient.Do(req)
}

// handleErrorResponse 处理错误响应
func (c *OpenAIClient) handleErrorResponse(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)

	type openaiError struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	var errResp openaiError
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
