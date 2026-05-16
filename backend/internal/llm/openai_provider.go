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
				streamChunk.ToolCalls = make([]kernel.ToolCall, len(delta.ToolCalls))
				for i, tc := range delta.ToolCalls {
					streamChunk.ToolCalls[i] = kernel.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: kernel.FunctionCall{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
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

// HealthCheck 健康检查
func (p *OpenAIProvider) HealthCheck(ctx context.Context) error {
	_, err := p.Chat(ctx, []kernel.Message{
		{Role: "user", Content: "hi"},
	}, nil, map[string]interface{}{"max_tokens": 5})
	return err
}

// CompleteWithPrefix 对话前缀续写 (DeepSeek Beta)
// 最后一条消息必须是 assistant 角色，且设置 prefix=true
func (p *OpenAIProvider) CompleteWithPrefix(ctx context.Context, messages []kernel.Message, prefix string, options map[string]interface{}) (*kernel.LLMResponse, error) {
	// 添加前缀消息
	prefixMsg := kernel.Message{
		Role:    "assistant",
		Content: prefix,
	}
	// 使用特殊标记表示 prefix 模式
	messages = append(messages, prefixMsg)

	body := p.buildRequestBody(messages, nil, options)
	// 标记最后一条消息为 prefix
	if msgs, ok := body["messages"].([]map[string]interface{}); ok && len(msgs) > 0 {
		msgs[len(msgs)-1]["prefix"] = true
	}

	data, err := json.Marshal(body)
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
	llmResp := &kernel.LLMResponse{
		ID:      result.ID,
		Content: choice.Message.Content,
		Model:   result.Model,
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

// FIMComplete Fill In The Middle 补全 (DeepSeek Beta)
func (p *OpenAIProvider) FIMComplete(ctx context.Context, prompt, suffix string, options map[string]interface{}) (*kernel.FIMResponse, error) {
	body := map[string]interface{}{
		"model":  p.modelID,
		"prompt": prompt,
	}
	if suffix != "" {
		body["suffix"] = suffix
	}

	if options != nil {
		for k, v := range options {
			body[k] = v
		}
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	url := p.config.BaseURL + "/completions"
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

	var result fimResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	fimResp := &kernel.FIMResponse{
		Text:  result.Choices[0].Text,
		Model: result.Model,
	}

	if result.Usage != nil {
		fimResp.Usage = &kernel.TokenUsage{
			PromptTokens:          result.Usage.PromptTokens,
			CompletionTokens:      result.Usage.CompletionTokens,
			TotalTokens:           result.Usage.TotalTokens,
			PromptCacheHitTokens:  result.Usage.PromptCacheHitTokens,
			PromptCacheMissTokens: result.Usage.PromptCacheMissTokens,
		}
	}

	return fimResp, nil
}

func (p *OpenAIProvider) buildRequestBody(messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"model":    p.modelID,
		"messages": p.convertMessages(messages),
	}

	if len(tools) > 0 {
		body["tools"] = p.convertTools(tools)
	}

	// DeepSeek 特有参数 - 只在检测到 DeepSeek 时发送
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
		m := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.ReasoningContent != "" {
			m["reasoning_content"] = msg.ReasoningContent
		}
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

// fimResponse FIM 补全响应结构
type fimResponse struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []fimChoice   `json:"choices"`
	Usage   *openAIUsage  `json:"usage,omitempty"`
}

type fimChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
}
