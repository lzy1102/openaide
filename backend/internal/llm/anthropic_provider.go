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

// AnthropicProvider Anthropic 原生 API 实现
// Anthropic 的格式与 OpenAI 不兼容，需要独立处理
type AnthropicProvider struct {
	config     *ProviderConfig
	httpClient *http.Client
	modelID    string
}

// NewAnthropicProvider 创建 Anthropic 提供商
func NewAnthropicProvider(config *ProviderConfig) *AnthropicProvider {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 120
	}

	return &AnthropicProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		modelID: config.DefaultModel,
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	reqBody := p.buildAnthropicBody(messages, tools, options)

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := p.config.BaseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic http %d: %s", resp.StatusCode, string(body))
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// 提取文本内容和工具调用
	var content string
	var toolCalls []kernel.ToolCall

	for _, block := range result.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, kernel.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: kernel.FunctionCall{
					Name:      block.Name,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	llmResp := &kernel.LLMResponse{
		ID:      result.ID,
		Content: content,
		Model:   result.Model,
	}

	if result.Usage.InputTokens > 0 {
		llmResp.Usage = &kernel.TokenUsage{
			PromptTokens:     result.Usage.InputTokens,
			CompletionTokens: result.Usage.OutputTokens,
			TotalTokens:      result.Usage.InputTokens + result.Usage.OutputTokens,
		}
	}

	// 转换 tool_calls 格式
	if len(toolCalls) > 0 {
		llmResp.ToolCalls = toolCalls
	}

	return llmResp, nil
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	reqBody := p.buildAnthropicBody(messages, tools, options)
	reqBody["stream"] = true

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := p.config.BaseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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

			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_delta":
				if event.Delta != nil && event.Delta.Text != "" {
					resultChan <- kernel.StreamChunk{Content: event.Delta.Text}
				}
			case "message_stop":
				resultChan <- kernel.StreamChunk{Done: true}
				return
			}
		}
	}()

	return resultChan, nil
}

func (p *AnthropicProvider) GetModelID() string {
	return p.modelID
}

func (p *AnthropicProvider) HealthCheck(ctx context.Context) error {
	_, err := p.Chat(ctx, []kernel.Message{
		{Role: "user", Content: "hi"},
	}, nil, map[string]interface{}{"max_tokens": 5})
	return err
}

func (p *AnthropicProvider) buildAnthropicBody(messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"model":      p.modelID,
		"max_tokens": 4096,
	}

	// 分离 system 消息
	var systemPrompt string
	var anthropicMessages []map[string]interface{}

	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt += msg.Content + "\n"
			continue
		}

		role := msg.Role
		content := []map[string]interface{}{}

		// Anthropic 格式: user 或 assistant
		switch role {
		case "tool":
			role = "user"
			content = append(content, map[string]interface{}{
				"type":          "tool_result",
				"tool_use_id":   msg.ToolCallID,
				"content":       msg.Content,
			})
		case "assistant":
			if msg.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
			// 转换 tool_calls
			for _, tc := range msg.ToolCalls {
				var input map[string]interface{}
				json.Unmarshal([]byte(tc.Function.Arguments), &input)
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
		default: // user
			// 多模态：检测base64图片 → Anthropic格式
			if strings.Contains(msg.Content, "data:image/") && strings.Contains(msg.Content, ";base64,") {
				content = append(content, splitMultimodalAnthropic(msg.Content)...)
			} else {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
		}

		anthropicMessages = append(anthropicMessages, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}

	if systemPrompt != "" {
		body["system"] = systemPrompt
	}
	body["messages"] = anthropicMessages

	// 转换工具定义
	if len(tools) > 0 {
		var anthropicTools []map[string]interface{}
		for _, t := range tools {
			anthropicTools = append(anthropicTools, map[string]interface{}{
				"name":         t.Function.Name,
				"description":  t.Function.Description,
				"input_schema": t.Function.Parameters,
			})
		}
		body["tools"] = anthropicTools
	}

	// 应用选项
	if options != nil {
		if temp, ok := options["temperature"]; ok {
			body["temperature"] = temp
		}
		if mt, ok := options["max_tokens"]; ok {
			body["max_tokens"] = mt
		}
	}

	return body
}

// ============ Anthropic API 结构体 ============

type anthropicResponse struct {
	ID      string                `json:"id"`
	Model   string                `json:"model"`
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage        `json:"usage"`
}

type anthropicContentBlock struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamEvent struct {
	Type  string              `json:"type"`
	Delta *anthropicStreamDelta `json:"delta,omitempty"`
}

type anthropicStreamDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}


// splitMultimodalAnthropic 将文本+base64图片拆分为Anthropic vision格式
func splitMultimodalAnthropic(raw string) []map[string]interface{} {
	var parts []map[string]interface{}
	re := regexpCompile(`data:image/(\w+);base64,([A-Za-z0-9+/=]+)`)
	matches := re.FindAllStringSubmatch(raw, -1)
	text := re.ReplaceAllString(raw, "")
	text = strings.TrimSpace(text)

	if text != "" {
		parts = append(parts, map[string]interface{}{"type": "text", "text": text})
	}
	for _, m := range matches {
		if len(m) >= 3 {
			parts = append(parts, map[string]interface{}{
				"type": "image",
				"source": map[string]interface{}{
					"type":       "base64",
					"media_type": "image/" + m[1],
					"data":       m[2],
				},
			})
		}
	}
	if len(parts) == 0 {
		return []map[string]interface{}{{"type": "text", "text": raw}}
	}
	return parts
}

func regexpCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

// Embed 向量化（Anthropic 不支持 embeddings API）
func (p *AnthropicProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("embeddings not supported by anthropic provider, use an openai provider")
}

// EmbedBatch 批量向量化（Anthropic 不支持 embeddings API）
func (p *AnthropicProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("embeddings not supported by anthropic provider, use an openai provider")
}
