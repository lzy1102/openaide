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

// CohereClient Cohere 客户端
type CohereClient struct {
	config     *ClientConfig
	httpClient *http.Client
}

// Cohere 默认配置
const (
	CohereDefaultBaseURL = "https://api.cohere.ai/v1"
	CohereDefaultModel   = "command"
)

// NewCohereClient 创建 Cohere 客户端
func NewCohereClient(config *ClientConfig) (*CohereClient, error) {
	if config.APIKey == "" {
		return nil, ErrEmptyAPIKey
	}

	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = CohereDefaultBaseURL
	}

	return &CohereClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}, nil
}

// Chat 发送聊天请求
func (c *CohereClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = CohereDefaultModel
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
		resp, err = c.sendRequest(ctx, "POST", "/chat", requestBody)
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
func (c *CohereClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = CohereDefaultModel
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

		resp, err := c.sendRequest(ctx, "POST", "/chat", requestBody)
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

			var event cohereStreamEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}

			switch event.EventType {
			case "text-generation":
				chunkChan <- ChatStreamChunk{
					ID:      "",
					Model:   req.Model,
					Created: time.Now().Unix(),
					Choices: []StreamChoice{
						{
							Index: 0,
							Delta: MessageDelta{
								Content: event.Text,
							},
						},
					},
				}
			case "stream-end":
				chunkChan <- ChatStreamChunk{
					ID:      "",
					Model:   req.Model,
					Created: time.Now().Unix(),
					Choices: []StreamChoice{
						{
							Index:        0,
							FinishReason: "stop",
						},
					},
					Usage: &Usage{
						PromptTokens:     event.Response.Meta.Tokens.InputTokens,
						CompletionTokens: event.Response.Meta.Tokens.OutputTokens,
						TotalTokens:      event.Response.Meta.Tokens.InputTokens + event.Response.Meta.Tokens.OutputTokens,
					},
				}
			case "stream-start":
				// 忽略开始事件
			}
		}
	}()

	return chunkChan, nil
}

// cohereStreamEvent Cohere 流式事件结构
type cohereStreamEvent struct {
	EventType string `json:"event_type"`
	Text      string `json:"text"`
	Response  struct {
		Meta struct {
			Tokens struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"tokens"`
		} `json:"meta"`
	} `json:"response"`
}

// buildChatRequest 构建聊天请求体
func (c *CohereClient) buildChatRequest(req *ChatRequest) ([]byte, error) {
	// Cohere 使用 chat_history + message 格式
	// 最后一条用户消息作为 message，其他作为 chat_history
	var message string
	var chatHistory []struct {
		Role    string `json:"role"`
		Message string `json:"message"`
	}

	for i, msg := range req.Messages {
		if i == len(req.Messages)-1 && msg.Role == "user" {
			message = msg.Content
		} else {
			role := msg.Role
			if role == "assistant" {
				role = "CHATBOT"
			} else if role == "user" {
				role = "USER"
			} else if role == "system" {
				role = "SYSTEM"
			}
			chatHistory = append(chatHistory, struct {
				Role    string `json:"role"`
				Message string `json:"message"`
			}{
				Role:    role,
				Message: msg.Content,
			})
		}
	}

	cohereReq := struct {
		Model          string  `json:"model"`
		Message        string  `json:"message"`
		ChatHistory    []struct {
			Role    string `json:"role"`
			Message string `json:"message"`
		} `json:"chat_history,omitempty"`
		Temperature float64 `json:"temperature,omitempty"`
		MaxTokens   int     `json:"max_tokens,omitempty"`
		Preamble    string  `json:"preamble,omitempty"`
		Stream      bool    `json:"stream,omitempty"`
	}{
		Model:        req.Model,
		Message:      message,
		ChatHistory:  chatHistory,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		Preamble:     req.System,
	}

	return json.Marshal(cohereReq)
}

// parseChatResponse 解析聊天响应
func (c *CohereClient) parseChatResponse(resp *http.Response, model string) (*ChatResponse, error) {
	type cohereResponse struct {
		Text          string `json:"text"`
		GenerationID  string `json:"generation_id"`
		ResponseID    string `json:"response_id"`
		FinishReason  string `json:"finish_reason"`
		Meta          struct {
			Tokens struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"tokens"`
		} `json:"meta"`
		Message string `json:"message,omitempty"`
	}

	var cohereResp cohereResponse
	if err := json.NewDecoder(resp.Body).Decode(&cohereResp); err != nil {
		return nil, &LLMError{Code: "decode_error", Message: "failed to decode response", Details: err.Error()}
	}

	if cohereResp.Message != "" {
		return nil, &LLMError{Code: "api_error", Message: cohereResp.Message}
	}

	return &ChatResponse{
		ID:      cohereResp.ResponseID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: cohereResp.Text,
				},
				FinishReason: cohereResp.FinishReason,
			},
		},
		Usage: &Usage{
			PromptTokens:     cohereResp.Meta.Tokens.InputTokens,
			CompletionTokens: cohereResp.Meta.Tokens.OutputTokens,
			TotalTokens:      cohereResp.Meta.Tokens.InputTokens + cohereResp.Meta.Tokens.OutputTokens,
		},
	}, nil
}

// sendRequest 发送 HTTP 请求
func (c *CohereClient) sendRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
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
func (c *CohereClient) handleErrorResponse(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)

	type errorResponse struct {
		Message string `json:"message"`
	}

	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
		return &LLMError{
			Code:    "api_error",
			Message: errResp.Message,
			Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	return &LLMError{
		Code:    "http_error",
		Message: string(body),
		Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}
