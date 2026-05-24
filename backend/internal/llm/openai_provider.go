package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"openaide/backend/internal/kernel"
)

// OpenAIProvider OpenAI 兼容提供商实现
type OpenAIProvider struct {
	config     *ProviderConfig
	httpClient *http.Client
	modelID    string
}

// NewOpenAIProvider 创建 OpenAI 提供商
func NewOpenAIProvider(config *ProviderConfig) *OpenAIProvider {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 120
	}

	return &OpenAIProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		modelID: config.DefaultModel,
	}
}

// Chat 发送聊天请求
func (p *OpenAIProvider) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	reqBody := p.buildRequestBody(messages, tools, options)

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	url := p.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	for k, v := range p.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http error %d: %s", resp.StatusCode, string(body))
	}

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	choice := result.Choices[0]

	var toolCalls []kernel.ToolCall
	for _, tc := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, kernel.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: kernel.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	llmResp := &kernel.LLMResponse{
		ID:               result.ID,
		Content:          choice.Message.Content,
		ReasoningContent: choice.Message.ReasoningContent,
		ToolCalls:        toolCalls,
		Model:            result.Model,
	}

	if result.Usage != nil {
		llmResp.Usage = &kernel.TokenUsage{
			PromptTokens:          result.Usage.PromptTokens,
			CompletionTokens:      result.Usage.CompletionTokens,
			TotalTokens:           result.Usage.TotalTokens,
			PromptCacheHitTokens:  result.Usage.PromptCacheHitTokens,
			PromptCacheMissTokens: result.Usage.PromptCacheMissTokens,
		}
	}

	return llmResp, nil
}

// ChatStream 发送流式聊天请求
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	reqBody := p.buildRequestBody(messages, tools, options)
	reqBody["stream"] = true

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := p.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range p.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	resultChan := make(chan kernel.StreamChunk, 10)

	go func() {
		defer close(resultChan)
		defer resp.Body.Close()

		// 跨 chunk 累加 tool calls（OpenAI 流式 tool call 参数会拆分到多个 delta chunk）
		accumulatedToolCalls := make([]kernel.ToolCall, 0)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				resultChan <- kernel.StreamChunk{Done: true}
				return
			}

			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta
			streamChunk := kernel.StreamChunk{
				Content:          delta.Content,
				ReasoningContent: delta.ReasoningContent,
				Done:             false,
			}

			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
					if tc.Index >= len(accumulatedToolCalls) {
						accumulatedToolCalls = append(accumulatedToolCalls, kernel.ToolCall{
							ID:   tc.ID,
							Type: tc.Type,
							Function: kernel.FunctionCall{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						})
					} else {
						// 合并到已有条目：只覆盖非空字段
						if tc.ID != "" {
							accumulatedToolCalls[tc.Index].ID = tc.ID
						}
						if tc.Type != "" {
							accumulatedToolCalls[tc.Index].Type = tc.Type
						}
						if tc.Function.Name != "" {
							accumulatedToolCalls[tc.Index].Function.Name = tc.Function.Name
						}
						accumulatedToolCalls[tc.Index].Function.Arguments += tc.Function.Arguments
					}
				}
				// 每个 chunk 都发出当前合并后的完整 tool calls
				streamChunk.ToolCalls = make([]kernel.ToolCall, len(accumulatedToolCalls))
				copy(streamChunk.ToolCalls, accumulatedToolCalls)
			}

			if chunk.Usage != nil {
				streamChunk.Usage = &kernel.TokenUsage{
					PromptTokens:          chunk.Usage.PromptTokens,
					CompletionTokens:      chunk.Usage.CompletionTokens,
					TotalTokens:           chunk.Usage.TotalTokens,
					PromptCacheHitTokens:  chunk.Usage.PromptCacheHitTokens,
					PromptCacheMissTokens: chunk.Usage.PromptCacheMissTokens,
				}
			}

			select {
			case resultChan <- streamChunk:
			case <-ctx.Done():
				return
			}
		}
	}()

	return resultChan, nil
}

// GetModelID 获取模型 ID
func (p *OpenAIProvider) GetModelID() string {
	return p.modelID
}

// SetModelID 设置模型 ID（运行时切换）
func (p *OpenAIProvider) SetModelID(model string) {
	p.modelID = model
}

// Embed 文本向量化
func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return p.embeddingRequest(ctx, []string{text})
}

// EmbedBatch 批量向量化
func (p *OpenAIProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("empty texts")
	}
	res, err := p.embeddingRequest(ctx, texts)
	if err != nil {
		return nil, err
	}
	return [][]float32{res}, nil
}

func (p *OpenAIProvider) embeddingRequest(ctx context.Context, inputs []string) ([]float32, error) {
	model := p.config.DefaultModel
	if p.config.EmbeddingModel != "" {
		model = p.config.EmbeddingModel
	}

	body := map[string]interface{}{
		"model": model,
		"input": inputs,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	url := p.config.BaseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	for k, v := range p.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding http error %d: %s", resp.StatusCode, string(body))
	}

	var result openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	return result.Data[0].Embedding, nil
}

// HealthCheck 健康检查
func (p *OpenAIProvider) HealthCheck(ctx context.Context) error {
	_, err := p.Chat(ctx, []kernel.Message{
		{Role: "user", Content: "hi"},
	}, nil, map[string]interface{}{"max_tokens": 5})
	return err
}

func (p *OpenAIProvider) buildRequestBody(messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"model":    p.modelID,
		"messages": p.convertMessages(messages),
	}

	if len(tools) > 0 {
		body["tools"] = p.convertTools(tools)
	}

	// DeepSeek 特有参数
	if p.isDeepSeek() {
		if p.config.Thinking != nil {
			body["thinking"] = map[string]string{
				"type": p.config.Thinking.Type,
			}
		}
		if p.config.ReasoningEffort != "" {
			body["reasoning_effort"] = p.config.ReasoningEffort
		}
	}

	// JSON mode - 标准 OpenAI 参数，所有兼容提供商都支持
	if p.config.JSONMode {
		body["response_format"] = map[string]string{
			"type": "json_object",
		}
	}

	if options != nil {
		if rf, ok := options["response_format"]; ok {
			body["response_format"] = rf
		}
	}
	if options != nil {
		for k, v := range options {
			body[k] = v
		}
	}

	return body
}

// isDeepSeek 检测是否为 DeepSeek 提供商
func (p *OpenAIProvider) isDeepSeek() bool {
	if p.config == nil {
		return false
	}
	return strings.Contains(strings.ToLower(p.config.BaseURL), "deepseek") ||
		strings.Contains(strings.ToLower(p.config.Name), "deepseek")
}

func (p *OpenAIProvider) convertMessages(messages []kernel.Message) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		content := p.convertContent(msg)
		m := map[string]interface{}{
			"role":    msg.Role,
			"content": content,
		}
		// 推理模型需要此字段, 含空串也传
		m["reasoning_content"] = msg.ReasoningContent
		if msg.Name != "" {
			m["name"] = msg.Name
		}
		if len(msg.ToolCalls) > 0 {
			m["tool_calls"] = p.convertToolCalls(msg.ToolCalls)
		}
		if msg.ToolCallID != "" {
			m["tool_call_id"] = msg.ToolCallID
		}
		result[i] = m
	}
	return result
}

// convertContent 转换消息内容，支持多模态（图片base64→vision格式）
func (p *OpenAIProvider) convertContent(msg kernel.Message) interface{} {
	// 检测是否包含 base64 图片
	if strings.Contains(msg.Content, "data:image/") && strings.Contains(msg.Content, ";base64,") {
		return p.splitMultimodal(msg.Content)
	}
	return msg.Content
}

// splitMultimodal 将文本+base64图片拆分为vision content array
func (p *OpenAIProvider) splitMultimodal(raw string) []map[string]interface{} {
	var parts []map[string]interface{}

	// 提取所有 data:image/...;base64,... 片段
	re := regexp.MustCompile(`data:image/\w+;base64,[A-Za-z0-9+/=]+`)
	images := re.FindAllString(raw, -1)

	// 文本部分（去掉所有图片数据URI）
	text := re.ReplaceAllString(raw, "")
	text = strings.TrimSpace(text)

	if text != "" {
		parts = append(parts, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	}

	for _, img := range images {
		parts = append(parts, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url":    img,
				"detail": "auto",
			},
		})
	}

	// 纯文本回退
	if len(parts) == 0 {
		return []map[string]interface{}{{"type": "text", "text": raw}}
	}
	return parts
}

func (p *OpenAIProvider) convertTools(tools []kernel.ToolDefinition) []map[string]interface{} {
	result := make([]map[string]interface{}, len(tools))
	for i, tool := range tools {
		fn := map[string]interface{}{
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"parameters":  tool.Function.Parameters,
		}
		if tool.Function.Strict {
			fn["strict"] = true
		}
		result[i] = map[string]interface{}{
			"type":     "function",
			"function": fn,
		}
	}
	return result
}

func (p *OpenAIProvider) convertToolCalls(calls []kernel.ToolCall) []map[string]interface{} {
	result := make([]map[string]interface{}, len(calls))
	for i, call := range calls {
		result[i] = map[string]interface{}{
			"id":   call.ID,
			"type": call.Type,
			"function": map[string]interface{}{
				"name":      call.Function.Name,
				"arguments": call.Function.Arguments,
			},
		}
	}
	return result
}

type openAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   *openAIUsage   `json:"usage,omitempty"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIUsage struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens,omitempty"`
}

type openAIStreamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
	Usage   *openAIUsage        `json:"usage,omitempty"`
}

type openAIStreamChoice struct {
	Index        int                `json:"index"`
	Delta        openAIMessageDelta `json:"delta"`
	FinishReason string             `json:"finish_reason"`
}

type openAIMessageDelta struct {
	Role             string           `json:"role,omitempty"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
}

// embedding 请求响应类型
type openAIEmbeddingResponse struct {
	Object string                `json:"object"`
	Data   []openAIEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Usage  *openAIUsage          `json:"usage,omitempty"`
}

type openAIEmbeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}
