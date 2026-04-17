package llm

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SparkClient 讯飞星火客户端
type SparkClient struct {
	config     *ClientConfig
	httpClient *http.Client
}

// Spark 默认配置
const (
	SparkDefaultBaseURL = "https://spark-api-open.xf-yun.com/v1/chat/completions"
	SparkDefaultModel   = "generalv3.5"
)

// Spark 模型版本映射
var sparkModelVersions = map[string]string{
	"spark-4.0-ultra":  "4.0Ultra",
	"generalv4":        "generalv4",
	"spark-3.5-max":    "generalv3.5",
	"generalv3.5":      "generalv3.5",
	"spark-3.0":        "generalv3",
	"generalv3":        "generalv3",
	"spark-2.0":        "generalv2",
	"generalv2":        "generalv2",
	"spark-1.5":        "general",
	"general":          "general",
	"spark-lite":       "lite",
	"lite":             "lite",
}

// NewSparkClient 创建讯飞星火客户端
func NewSparkClient(config *ClientConfig) (*SparkClient, error) {
	if config.APIKey == "" {
		return nil, ErrEmptyAPIKey
	}

	// 设置默认 BaseURL
	if config.BaseURL == "" {
		config.BaseURL = SparkDefaultBaseURL
	}

	return &SparkClient{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}, nil
}

// Chat 发送聊天请求
func (c *SparkClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = SparkDefaultModel
		}
	}

	// 构建请求体
	requestBody, err := c.buildChatRequest(req)
	if err != nil {
		return nil, err
	}

	// 生成授权 URL
	authURL, err := c.generateAuthURL()
	if err != nil {
		return nil, &LLMError{Code: "auth_error", Message: "failed to generate auth URL", Details: err.Error()}
	}

	// 发送请求 (带重试)
	var resp *http.Response
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		httpReq, _ := http.NewRequestWithContext(ctx, "POST", authURL, strings.NewReader(string(requestBody)))
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err = c.httpClient.Do(httpReq)
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
func (c *SparkClient) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error) {
	if len(req.Messages) == 0 {
		return nil, ErrEmptyMessages
	}

	// 设置默认模型
	if req.Model == "" {
		req.Model = c.config.Model
		if req.Model == "" {
			req.Model = SparkDefaultModel
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

	// 生成授权 URL
	authURL, err := c.generateAuthURL()
	if err != nil {
		return nil, &LLMError{Code: "auth_error", Message: "failed to generate auth URL", Details: err.Error()}
	}

	// 创建响应通道
	chunkChan := make(chan ChatStreamChunk, 10)

	go func() {
		defer close(chunkChan)

		httpReq, err := http.NewRequestWithContext(ctx, "POST", authURL, strings.NewReader(string(requestBody)))
		if err != nil {
			chunkChan <- ChatStreamChunk{Error: &LLMError{Code: "request_failed", Message: "failed to create request", Details: err.Error()}}
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
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
			if data == "[DONE]" {
				break
			}

			// 解析 SSE 数据
			var sparkResp sparkResponse
			if err := json.Unmarshal([]byte(data), &sparkResp); err != nil {
				continue
			}

			if sparkResp.Code != 0 {
				chunkChan <- ChatStreamChunk{Error: &LLMError{Code: fmt.Sprintf("%d", sparkResp.Code), Message: sparkResp.Message}}
				break
			}

			if len(sparkResp.Choices) > 0 {
				chunkChan <- ChatStreamChunk{
					ID:      sparkResp.ID,
					Model:   req.Model,
					Created: sparkResp.Created,
					Choices: []StreamChoice{
						{
							Index: sparkResp.Choices[0].Index,
							Delta: MessageDelta{
								Content: sparkResp.Choices[0].Delta.Content,
							},
							FinishReason: sparkResp.Choices[0].FinishReason,
						},
					},
				}
			}
		}
	}()

	return chunkChan, nil
}

// sparkResponse 星火 API 响应结构
type sparkResponse struct {
	ID      string `json:"id"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int `json:"index"`
		Delta        struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"`
}

// buildChatRequest 构建聊天请求体
func (c *SparkClient) buildChatRequest(req *ChatRequest) ([]byte, error) {
	type sparkMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type sparkRequest struct {
		Model       string          `json:"model"`
		Messages    []sparkMessage  `json:"messages"`
		Temperature float64         `json:"temperature,omitempty"`
		TopK        int             `json:"top_k,omitempty"`
		MaxTokens   int             `json:"max_tokens,omitempty"`
		Stream      bool            `json:"stream,omitempty"`
	}

	messages := make([]sparkMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = sparkMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	sparkReq := sparkRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	return json.Marshal(sparkReq)
}

// parseChatResponse 解析聊天响应
func (c *SparkClient) parseChatResponse(resp *http.Response, model string) (*ChatResponse, error) {
	var sparkResp sparkResponse
	if err := json.NewDecoder(resp.Body).Decode(&sparkResp); err != nil {
		return nil, &LLMError{Code: "decode_error", Message: "failed to decode response", Details: err.Error()}
	}

	if sparkResp.Code != 0 {
		return nil, &LLMError{Code: fmt.Sprintf("%d", sparkResp.Code), Message: sparkResp.Message}
	}

	choices := make([]Choice, len(sparkResp.Choices))
	for i, ch := range sparkResp.Choices {
		choices[i] = Choice{
			Index: ch.Index,
			Message: Message{
				Role:    ch.Delta.Role,
				Content: ch.Delta.Content,
			},
			FinishReason: ch.FinishReason,
		}
	}

	return &ChatResponse{
		ID:      sparkResp.ID,
		Object:  "chat.completion",
		Created: sparkResp.Created,
		Model:   model,
		Choices: choices,
		Usage:   sparkResp.Usage,
	}, nil
}

// generateAuthURL 生成带签名的授权 URL
func (c *SparkClient) generateAuthURL() (string, error) {
	// 解析 API Key: 格式为 "appId:apiKey:apiSecret"
	parts := strings.Split(c.config.APIKey, ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid API key format, expected 'appId:apiKey:apiSecret'")
	}
	appID := parts[0]
	apiKey := parts[1]
	apiSecret := parts[2]

	// 解析 BaseURL
	parsedURL, err := url.Parse(c.config.BaseURL)
	if err != nil {
		return "", err
	}

	// 生成鉴权参数
	host := parsedURL.Host
	path := parsedURL.Path
	now := time.Now().UTC().Format(http.TimeFormat)

	// 生成签名
	signatureOrigin := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1", host, now, path)
	signature := c.hmacSHA256(apiSecret, signatureOrigin)
	authorizationOrigin := fmt.Sprintf("api_key=\"%s\", algorithm=\"%s\", headers=\"%s\", signature=\"%s\"",
		apiKey, "hmac-sha256", "host date request-line", signature)
	authorization := base64.StdEncoding.EncodeToString([]byte(authorizationOrigin))

	// 构建最终 URL
	authURL := fmt.Sprintf("%s?authorization=%s&date=%s&host=%s",
		c.config.BaseURL,
		url.QueryEscape(authorization),
		url.QueryEscape(now),
		url.QueryEscape(host))

	// 添加 app_id 参数
	authURL += fmt.Sprintf("&app_id=%s", appID)

	return authURL, nil
}

// hmacSHA256 HMAC-SHA256 签名
func (c *SparkClient) hmacSHA256(key, data string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// handleErrorResponse 处理错误响应
func (c *SparkClient) handleErrorResponse(resp *http.Response) *LLMError {
	body, _ := io.ReadAll(resp.Body)

	var sparkResp sparkResponse
	if err := json.Unmarshal(body, &sparkResp); err == nil && sparkResp.Message != "" {
		return &LLMError{
			Code:    fmt.Sprintf("%d", sparkResp.Code),
			Message: sparkResp.Message,
			Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	return &LLMError{
		Code:    "http_error",
		Message: string(body),
		Details: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}
