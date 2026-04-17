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

// AnthropicClient Anthropic (Claude) 客户端
type AnthropicClient struct {
	config     *ClientConfig
	httpClient *http.Client
}

// NewAnthropicClient 创建 Anthropic 客户端
func NewAnthropicClient(config *ClientConfig) (*AnthropicClient, error) {
	if config.APIKey == "" {
		return nil, ErrEmptyAPIKey
	}

	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = "https://api.anthropic.com/v1"
	}

	return &AnthropicClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}, nil
}

// Chat 发送聊天请求
func (c *AnthropicClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = "claude-3-sonnet-20240229"
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
		resp, err = c.sendRequest(ctx, "POST", "/messages", requestBody)
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

	var anthropicResp anthropicMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, &LLMError{Code: "decode_error", Message: "failed to decode response", Details: err.Error()}
	}

	// 转换为通用格式
	return c.convertResponse(&anthropicResp), nil
}

// ChatStream 发送流式聊天请求
func (c *AnthropicClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = "claude-3-sonnet-20240229"
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

		resp, err := c.sendRequest(ctx, "POST", "/messages", requestBody)
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
		var currentChunk *ChatStreamChunk

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

			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			// 转换为通用格式
			chunk := c.convertStreamChunk(&event, currentChunk)
			if chunk != nil {
				currentChunk = chunk
				chunkChan <- *chunk
			}
		}
	}()

	return chunkChan, nil
}

// anthropicMessageRequest Anthropic 消息请求格式
type anthropicMessageRequest struct {
	Model     string                 `json:"model"`
	MaxTokens int                    `json:"max_tokens"`
	Messages  []anthropicMessage     `json:"messages"`
	System    string                 `json:"system,omitempty"`
	Temperature float64              `json:"temperature,omitempty"`
	TopP      float64                `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	TopK      int                    `json:"top_k,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicMessageResponse Anthropic 消息响应格式
type anthropicMessageResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []anthropicContentBlock `json:"content"`
	Model   string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage   anthropicUsage `json:"usage"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// anthropicStreamEvent Anthropic 流式事件
type anthropicStreamEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index,omitempty"`
	ContentBlock *struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
		Thinking string `json:"thinking,omitempty"`
	} `json:"content_block,omitempty"`
	Delta        *struct {
		Type        string `json:"type,omitempty"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
		StopReason  string `json:"stop_reason,omitempty"`
	} `json:"delta,omitempty"`
	Message      *anthropicMessageResponse `json:"message,omitempty"`
	Usage        *anthropicUsage `json:"usage,omitempty"`
}

// buildChatRequest 构建聊天请求体
func (c *AnthropicClient) buildChatRequest(req *ChatRequest) ([]byte, error) {
	// 转换消息格式 (Anthropic 只支持 user 和 assistant 角色)
	messages := make([]anthropicMessage, 0)
	systemPrompt := req.System

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			// Anthropic 使用单独的 system 字段
			if systemPrompt == "" {
				systemPrompt = msg.Content
			} else {
				systemPrompt += "\n" + msg.Content
			}
		case "user", "assistant":
			messages = append(messages, anthropicMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// Anthropic 要求消息必须以 user 角色开始
	if len(messages) == 0 || messages[0].Role != "user" {
		return nil, &LLMError{Code: "invalid_messages", Message: "messages must start with user role"}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	anthropicReq := anthropicMessageRequest{
		Model:         req.Model,
		MaxTokens:     maxTokens,
		Messages:      messages,
		System:        systemPrompt,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: req.Stop,
	}

	return json.Marshal(anthropicReq)
}

// sendRequest 发送 HTTP 请求
func (c *AnthropicClient) sendRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := c.config.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	return c.httpClient.Do(req)
}

// handleErrorResponse 处理错误响应
func (c *AnthropicClient) handleErrorResponse(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)

	type anthropicError struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	var errResp anthropicError
	if err := json.Unmarshal(body, &errResp); err == nil {
		return &LLMError{
			Code:    errResp.Error.Type,
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

// convertResponse 转换 Anthropic 响应为通用格式
func (c *AnthropicClient) convertResponse(resp *anthropicMessageResponse) *ChatResponse {
	content := ""
	reasoningContent := ""
	for _, block := range resp.Content {
		if block.Type == "text" {
			content += block.Text
		} else if block.Type == "thinking" {
			reasoningContent += block.Thinking
		}
	}

	finishReason := "stop"
	if resp.StopReason == "max_tokens" {
		finishReason = "length"
	}

	return &ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:             "assistant",
					Content:          content,
					ReasoningContent: reasoningContent,
				},
				FinishReason: finishReason,
			},
		},
		Usage: &Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

// convertStreamChunk 转换流式数据块为通用格式
func (c *AnthropicClient) convertStreamChunk(event *anthropicStreamEvent, current *ChatStreamChunk) *ChatStreamChunk {
	switch event.Type {
	case "message_start":
		return &ChatStreamChunk{
			ID:      event.Message.ID,
			Model:   event.Message.Model,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Choices: []StreamChoice{{Index: 0}},
		}

	case "content_block_start":
		if current == nil {
			return nil
		}
		isThinking := event.ContentBlock != nil && event.ContentBlock.Type == "thinking"
		current.Choices = []StreamChoice{
			{
				Index: event.Index,
				Delta: MessageDelta{
					Role: "assistant",
				},
			},
		}
		if isThinking {
			current.Choices[0].Delta.ReasoningContent = " "
		}
		return current

	case "content_block_delta":
		if current == nil || event.Delta == nil {
			return nil
		}
		delta := MessageDelta{}
		if event.Delta.Type == "thinking_delta" {
			delta.ReasoningContent = event.Delta.Thinking
		} else {
			delta.Content = event.Delta.Text
		}
		current.Choices = []StreamChoice{
			{
				Index: event.Index,
				Delta: delta,
			},
		}
		return current

	case "message_delta":
		if current == nil {
			return nil
		}
		finishReason := "stop"
		if event.Delta != nil && event.Delta.StopReason == "max_tokens" {
			finishReason = "length"
		}
		current.Choices = []StreamChoice{
			{
				Index:        0,
				FinishReason: finishReason,
			},
		}
		if event.Usage != nil {
			current.Usage = &Usage{
				PromptTokens:     event.Usage.InputTokens,
				CompletionTokens: event.Usage.OutputTokens,
				TotalTokens:      event.Usage.InputTokens + event.Usage.OutputTokens,
			}
		}
		return current
	}

	return nil
}
