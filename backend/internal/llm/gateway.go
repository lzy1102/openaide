package llm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
)

// ProviderConfig 提供商配置
type ProviderConfig struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	BaseURL     string            `json:"base_url"`
	APIKey      string            `json:"api_key"`
	DefaultModel string           `json:"default_model"`
	Timeout     int               `json:"timeout"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
}

// Gateway LLM 网关 - 统一接入所有提供商
type Gateway struct {
	providers   map[string]Provider
	configs     map[string]*ProviderConfig
	defaultProvider string
	mu          sync.RWMutex
}

// Provider LLM 提供商接口（内部使用）
type Provider interface {
	Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error)
	ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error)
	GetModelID() string
	HealthCheck(ctx context.Context) error
}

// NewGateway 创建 LLM 网关
func NewGateway() *Gateway {
	return &Gateway{
		providers: make(map[string]Provider),
		configs:   make(map[string]*ProviderConfig),
	}
}

// RegisterProvider 注册提供商
func (g *Gateway) RegisterProvider(name string, provider Provider, config *ProviderConfig) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.providers[name] = provider
	g.configs[name] = config

	if config.Enabled && g.defaultProvider == "" {
		g.defaultProvider = name
	}
}

// SetDefaultProvider 设置默认提供商
func (g *Gateway) SetDefaultProvider(name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.providers[name]; !ok {
		return fmt.Errorf("provider not found: %s", name)
	}

	g.defaultProvider = name
	return nil
}

// GetDefaultProvider 获取默认提供商
func (g *Gateway) GetDefaultProvider() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.defaultProvider
}

// GetProviders 获取所有提供商名称
func (g *Gateway) GetProviders() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	names := make([]string, 0, len(g.providers))
	for name := range g.providers {
		names = append(names, name)
	}
	return names
}

// GetEnabledProviders 获取启用的提供商
func (g *Gateway) GetEnabledProviders() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var names []string
	for name, config := range g.configs {
		if config.Enabled {
			names = append(names, name)
		}
	}
	return names
}

// Chat 发送聊天请求（使用默认提供商）
func (g *Gateway) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	providerName := g.GetDefaultProvider()
	if providerName == "" {
		return nil, fmt.Errorf("no default provider configured")
	}
	return g.ChatWithProvider(ctx, providerName, messages, tools, options)
}

// ChatWithProvider 使用指定提供商发送聊天请求
func (g *Gateway) ChatWithProvider(ctx context.Context, providerName string, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	g.mu.RLock()
	provider, ok := g.providers[providerName]
	g.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	start := time.Now()
	resp, err := provider.Chat(ctx, messages, tools, options)
	if err != nil {
		slog.Error("LLM chat failed", "provider", providerName, "error", err, "duration", time.Since(start))
		return nil, err
	}

	slog.Debug("LLM chat success", "provider", providerName, "model", resp.Model, "tokens", resp.Usage, "duration", time.Since(start))
	return resp, nil
}

// ChatStream 发送流式聊天请求
func (g *Gateway) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	providerName := g.GetDefaultProvider()
	if providerName == "" {
		return nil, fmt.Errorf("no default provider configured")
	}
	return g.ChatStreamWithProvider(ctx, providerName, messages, tools, options)
}

// ChatStreamWithProvider 使用指定提供商发送流式聊天请求
func (g *Gateway) ChatStreamWithProvider(ctx context.Context, providerName string, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	g.mu.RLock()
	provider, ok := g.providers[providerName]
	g.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	return provider.ChatStream(ctx, messages, tools, options)
}

// GetModelID 获取当前模型 ID
func (g *Gateway) GetModelID() string {
	g.mu.RLock()
	provider, ok := g.providers[g.defaultProvider]
	g.mu.RUnlock()

	if !ok {
		return ""
	}
	return provider.GetModelID()
}

// HealthCheck 健康检查所有提供商
func (g *Gateway) HealthCheck(ctx context.Context) map[string]error {
	g.mu.RLock()
	providers := make(map[string]Provider, len(g.providers))
	for name, p := range g.providers {
		providers[name] = p
	}
	g.mu.RUnlock()

	results := make(map[string]error)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, provider := range providers {
		wg.Add(1)
		go func(n string, p Provider) {
			defer wg.Done()
			err := p.HealthCheck(ctx)
			mu.Lock()
			results[n] = err
			mu.Unlock()
		}(name, provider)
	}

	wg.Wait()
	return results
}

// FallbackChat 带故障转移的聊天请求
func (g *Gateway) FallbackChat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	// 先尝试默认提供商
	providers := g.GetEnabledProviders()
	if len(providers) == 0 {
		return nil, fmt.Errorf("no enabled providers")
	}

	var lastErr error
	for _, name := range providers {
		resp, err := g.ChatWithProvider(ctx, name, messages, tools, options)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		slog.Warn("Provider failed, trying next", "provider", name, "error", err)
	}

	return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
}

// ============ 实现 kernel.LLMProvider 接口 ============

// Ensure Gateway implements kernel.LLMProvider
var _ kernel.LLMProvider = (*Gateway)(nil)
