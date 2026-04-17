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

// GeminiClient Google Gemini 客户端
type GeminiClient struct {
	config     *ClientConfig
	httpClient *http.Client
}

// Gemini 默认配置
const (
	GeminiDefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	GeminiDefaultModel   = "gemini-pro"
)

// NewGeminiClient 创建 Gemini 客户端
func NewGeminiClient(config *ClientConfig) (*GeminiClient, error) {
	if config.APIKey == "" {
		return nil, ErrEmptyAPIKey
	}

	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = GeminiDefaultBaseURL
	}

	return &GeminiClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}, nil
}

// Chat 发送聊天请求
func (c *GeminiClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = GeminiDefaultModel
		}
	}

	// 构建请求体
	requestBody, err := c.buildChatRequest(req)
	if err != nil {
		return nil, err
	}

	// 构建请求 URL
	requestURL := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		c.config.BaseURL, req.Model, c.config.APIKey)

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
func (c *GeminiClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = GeminiDefaultModel
		}
	}

	// 构建请求体
	requestBody, err := c.buildChatRequest(req)
	if err != nil {
		return nil, err
	}

	// 构建请求 URL (streaming)
	requestURL := fmt.Sprintf("%s/models/%s:streamGenerateContent?key=%s&alt=sse",
		c.config.BaseURL, req.Model, c.config.APIKey)

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

		// 读取流式响应 (SSE 格式)
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

			if !strings.HasPrefix(line, "data:") && !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}

			var geminiResp geminiResponse
			if err := json.Unmarshal([]byte(data), &geminiResp); err != nil {
				continue
			}

			if geminiResp.Error != nil {
				chunkChan <- ChatStreamChunk{Error: &LLMError{Code: fmt.Sprintf("%d", geminiResp.Error.Code), Message: geminiResp.Error.Message}}
				break
			}

			if len(geminiResp.Candidates) > 0 {
				candidate := geminiResp.Candidates[0]
				content := ""
				if len(candidate.Content.Parts) > 0 {
					content = candidate.Content.Parts[0].Text
				}

				chunkChan <- ChatStreamChunk{
					ID:      "",
					Model:   req.Model,
					Created: time.Now().Unix(),
					Choices: []StreamChoice{
						{
							Index: 0,
							Delta: MessageDelta{
								Content: content,
							},
							FinishReason: candidate.FinishReason,
						},
					},
				}
			}
		}
	}()

	return chunkChan, nil
}

// geminiResponse Gemini API 响应结构
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// buildChatRequest 构建聊天请求体
func (c *GeminiClient) buildChatRequest(req *ChatRequest) ([]byte, error) {
	// 转换消息格式
	contents := make([]geminiContent, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role: role,
			Parts: []geminiPart{
				{Text: msg.Content},
			},
		})
	}

	geminiReq := struct {
		Contents         []geminiContent `json:"contents"`
		GenerationConfig struct {
			Temperature     float64 `json:"temperature,omitempty"`
			TopP            float64 `json:"topP,omitempty"`
			MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
		} `json:"generationConfig,omitempty"`
		SafetySettings []struct {
			Category  string `json:"category"`
			Threshold string `json:"threshold"`
		} `json:"safetySettings,omitempty"`
	}{
		Contents: contents,
		GenerationConfig: struct {
			Temperature     float64 `json:"temperature,omitempty"`
			TopP            float64 `json:"topP,omitempty"`
			MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
		}{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
		},
	}

	return json.Marshal(geminiReq)
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// parseChatResponse 解析聊天响应
func (c *GeminiClient) parseChatResponse(resp *http.Response, model string) (*ChatResponse, error) {
	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, &LLMError{Code: "decode_error", Message: "failed to decode response", Details: err.Error()}
	}

	if geminiResp.Error != nil {
		return nil, &LLMError{
			Code:    fmt.Sprintf("%d", geminiResp.Error.Code),
			Message: geminiResp.Error.Message,
		}
	}

	choices := make([]Choice, 0, len(geminiResp.Candidates))
	for i, candidate := range geminiResp.Candidates {
		content := ""
		if len(candidate.Content.Parts) > 0 {
			content = candidate.Content.Parts[0].Text
		}
		choices = append(choices, Choice{
			Index: i,
			Message: Message{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: candidate.FinishReason,
		})
	}

	usage := &Usage{}
	if geminiResp.UsageMetadata != nil {
		usage.PromptTokens = geminiResp.UsageMetadata.PromptTokenCount
		usage.CompletionTokens = geminiResp.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = geminiResp.UsageMetadata.TotalTokenCount
	}

	return &ChatResponse{
		ID:      "",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: choices,
		Usage:   usage,
	}, nil
}

// sendRequest 发送 HTTP 请求
func (c *GeminiClient) sendRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// handleErrorResponse 处理错误响应
func (c *GeminiClient) handleErrorResponse(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)

	type errorResponse struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}

	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return &LLMError{
			Code:    fmt.Sprintf("%d", errResp.Error.Code),
			Message: errResp.Error.Message,
			Details: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, errResp.Error.Status),
		}
	}

	return &LLMError{
		Code:    "http_error",
		Message: string(body),
		Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}
