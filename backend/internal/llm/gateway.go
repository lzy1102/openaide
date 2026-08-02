package llm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/kernel/actor"
)

// Gateway LLM 网关 - 统一接入所有提供商
type Gateway struct {
	providers       *actor.SafeMap[string, Provider]
	configs         *actor.SafeMap[string, *ProviderConfig]
	defaultProvider string
	cache           *PromptCache
	router          *Router
	rateLimiter     *RateLimiter // 全局 LLM 调用限流(可选,阻塞等待令牌)

	// 成本感知路由：小调用走 execution，核心推理走 reasoning
	ExecutionModel string
	ReasoningModel string
}

// Provider LLM 提供商接口（内部使用）
type Provider interface {
	Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error)
	ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error)
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	GetModelID() string
	SetModelID(model string)
	HealthCheck(ctx context.Context) error
}

// ProviderConfig 提供商配置
type ProviderConfig struct {
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	BaseURL        string            `json:"base_url"`
	APIKey         string            `json:"api_key"`
	DefaultModel   string            `json:"default_model"`
	EmbeddingModel string            `json:"embedding_model,omitempty"`
	Timeout        int               `json:"timeout"`
	Headers        map[string]string `json:"headers,omitempty"`
	Enabled        bool              `json:"enabled"`

	// DeepSeek 特有配置
	Thinking        *bool  `json:"thinking,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	JSONMode        bool   `json:"json_mode,omitempty"`
	StrictTools     bool   `json:"strict_tools,omitempty"`
}

// NewGateway 创建 LLM 网关
func NewGateway() *Gateway {
	return &Gateway{
		providers: actor.NewSafeMap[string, Provider](4),
		configs:   actor.NewSafeMap[string, *ProviderConfig](4),
	}
}

// RegisterProvider 注册提供商
func (g *Gateway) RegisterProvider(name string, provider Provider, config *ProviderConfig) {
	g.providers.Store(name, provider)
	g.configs.Store(name, config)

	if config.Enabled && g.defaultProvider == "" {
		g.defaultProvider = name
	}
}

// SetPromptCache 设置提示词缓存
func (g *Gateway) SetPromptCache(pc *PromptCache) { g.cache = pc }

// SetRateLimiter 设置全局 LLM 调用限流(阻塞等待令牌)。
// rate<=0 时禁用限流。rate=每秒令牌数, capacity=突发上限。
func (g *Gateway) SetRateLimiter(rate, capacity int) {
	if rate <= 0 {
		g.rateLimiter = nil
		return
	}
	g.rateLimiter = NewRateLimiter(rate, capacity)
}

// ReloadConfig 热更新 LLM 配置（不重建 provider，只更新模型和路由）
func (g *Gateway) ReloadConfig(newModels map[string]string, reasoningModel, executionModel string) {
	g.configs.Range(func(name string, config *ProviderConfig) bool {
		if model, ok := newModels[name]; ok {
			config.DefaultModel = model
		}
		return true
	})
	g.providers.Range(func(name string, prov Provider) bool {
		if model, ok := newModels[name]; ok && model != "" {
			prov.SetModelID(model)
		}
		return true
	})

	if reasoningModel != "" {
		g.ReasoningModel = reasoningModel
	}
	if executionModel != "" {
		g.ExecutionModel = executionModel
	}

	slog.Info("LLM gateway config reloaded", "reasoning", g.ReasoningModel, "execution", g.ExecutionModel)
}

// Shutdown 优雅关闭网关（停止缓存清理协程等）
func (g *Gateway) Shutdown() {
	if g.cache != nil {
		g.cache.Shutdown()
	}
}

// SetDefaultProvider 设置默认提供商
func (g *Gateway) SetDefaultProvider(name string) error {
	if _, ok := g.providers.Load(name); !ok {
		return fmt.Errorf("provider not found: %s", name)
	}
	g.defaultProvider = name
	return nil
}

// GetDefaultProvider 获取默认提供商
func (g *Gateway) GetDefaultProvider() string {
	return g.defaultProvider
}

// GetProviders 获取所有提供商名称
func (g *Gateway) GetProviders() []string {
	return g.providers.Keys()
}

// ProviderInfo 提供商概要信息
type ProviderInfo struct {
	Name    string
	Model   string
	Default bool
}

// GetProviderInfos 返回所有提供商及其当前模型
func (g *Gateway) GetProviderInfos() []ProviderInfo {
	infos := make([]ProviderInfo, 0)
	g.providers.Range(func(name string, p Provider) bool {
		info := ProviderInfo{
			Name:    name,
			Model:   p.GetModelID(),
			Default: name == g.defaultProvider,
		}
		infos = append(infos, info)
		return true
	})
	return infos
}

// GetEnabledProviders 获取启用的提供商
func (g *Gateway) GetEnabledProviders() []string {
	var names []string
	g.configs.Range(func(name string, config *ProviderConfig) bool {
		if config.Enabled {
			names = append(names, name)
		}
		return true
	})
	return names
}

// SetRouter 设置模型路由器
func (g *Gateway) SetRouter(r *Router) { g.router = r }

// routeProvider 根据用户输入选择最佳 provider
// findProviderForModel 查找默认模型匹配指定名称的 provider
func (g *Gateway) findProviderForModel(model string) string {
	var result string
	g.providers.Range(func(name string, p Provider) bool {
		if p.GetModelID() == model {
			result = name
			return false
		}
		return true
	})
	return result
}

func (g *Gateway) routeProvider(query string) string {
	if g.router != nil {
		provider, _, matched := g.router.Route(query)
		if matched && provider != "" {
			if _, ok := g.providers.Load(provider); ok {
				return provider
			}
		}
	}
	return g.GetDefaultProvider()
}

// Chat 发送聊天请求（智能路由 + 默认提供商）
// options["route"]: "execution" → 用 flash 模型, "reasoning" → 用 pro 模型
func (g *Gateway) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	// 成本感知路由：根据 options["route"] 选择模型
	if options == nil {
		options = make(map[string]interface{})
	}
	if route, _ := options["route"].(string); route != "" {
		switch route {
		case "execution":
			// 优先找默认模型匹配 execution model 的 provider（如 deepseek-flash）
			if g.ExecutionModel != "" {
				if pn := g.findProviderForModel(g.ExecutionModel); pn != "" {
					options["_force_provider"] = pn
				} else {
					options["model"] = g.ExecutionModel
				}
			}
		case "reasoning":
			if g.ReasoningModel != "" {
				if pn := g.findProviderForModel(g.ReasoningModel); pn != "" {
					options["_force_provider"] = pn
				} else {
					options["model"] = g.ReasoningModel
				}
			}
		}
	}

	// 从最后一条user消息提取query用于路由
	query := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			query = messages[i].Content
			break
		}
	}

	providerName := g.routeProvider(query)
	if forcePn, _ := options["_force_provider"].(string); forcePn != "" {
		providerName = forcePn
	}
	if providerName == "" {
		return nil, fmt.Errorf("no provider configured — edit ~/.openaide/config.yaml and add your API key, then restart")
	}
	return g.ChatWithProvider(ctx, providerName, messages, tools, options)
}

// ChatWithProvider 使用指定提供商发送聊天请求
func (g *Gateway) ChatWithProvider(ctx context.Context, providerName string, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	provider, ok := g.providers.Load(providerName)

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	// 支持 options["model"] 覆盖模型（成本感知路由）
	if opts, _ := options["model"].(string); opts != "" {
		prevModel := provider.GetModelID()
		provider.SetModelID(opts)
		defer provider.SetModelID(prevModel)
	}

	// 检查缓存
	if g.cache != nil {
		if cached := g.cache.Get(messages, tools, g.GetDefaultProvider()); cached != nil {
			return cached, nil
		}
	}

	// 全局限流:阻塞等待令牌(agent 内部调用不随机失败,排队等待)
	if g.rateLimiter != nil {
		if err := g.rateLimiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	start := time.Now()
	slog.Info("LLM call sent", "provider", providerName, "model", provider.GetModelID(), "msgs", len(messages), "tools", len(tools))

	var resp *kernel.LLMResponse
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = provider.Chat(ctx, messages, tools, options)
		if err == nil {
			break
		}
		if attempt < 2 {
			d := time.Duration(1<<attempt) * time.Second
			slog.Warn("LLM chat retry", "provider", providerName, "attempt", attempt+1, "wait", d, "error", err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
		}
	}
	if err != nil {
		slog.Error("LLM chat failed", "provider", providerName, "error", err, "duration", time.Since(start))
		return nil, humanizeError(err)
	}

	if g.cache != nil {
		g.cache.Set(messages, tools, g.GetDefaultProvider(), resp)
	}
	slog.Info("LLM response received", "provider", providerName, "model", resp.Model, "tokens", resp.Usage, "duration", time.Since(start))
	return resp, nil
}

// ChatStream 发送流式聊天请求（智能路由）
func (g *Gateway) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	query := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			query = messages[i].Content
			break
		}
	}
	providerName := g.routeProvider(query)
	if providerName == "" {
		return nil, fmt.Errorf("no provider configured — edit ~/.openaide/config.yaml and add your API key, then restart")
	}
	return g.ChatStreamWithProvider(ctx, providerName, messages, tools, options)
}

// ChatStreamWithProvider 使用指定提供商发送流式聊天请求
func (g *Gateway) ChatStreamWithProvider(ctx context.Context, providerName string, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	provider, ok := g.providers.Load(providerName)

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	// 全局限流:阻塞等待令牌(agent 内部调用不随机失败,排队等待)
	if g.rateLimiter != nil {
		if err := g.rateLimiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	slog.Info("LLM stream started", "provider", providerName, "model", provider.GetModelID(), "msgs", len(messages), "tools", len(tools))

	var ch <-chan kernel.StreamChunk
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		ch, err = provider.ChatStream(ctx, messages, tools, options)
		if err == nil {
			break
		}
		if attempt < 2 {
			d := time.Duration(1<<attempt) * time.Second
			slog.Warn("LLM stream retry", "provider", providerName, "attempt", attempt+1, "wait", d, "error", err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
		}
	}
	return ch, err
}

// SetDefaultModel 设置默认提供商的模型
// SetModelID 运行时切换默认提供商的模型
func (g *Gateway) SetModelID(model string) {
	g.SetDefaultModel(model)
}

func (g *Gateway) SetDefaultModel(model string) {
	if provider, ok := g.providers.Load(g.defaultProvider); ok {
		provider.SetModelID(model)
	}
	if config, ok := g.configs.Load(g.defaultProvider); ok {
		config.DefaultModel = model
	}
}

// GetModelID 获取当前模型 ID
func (g *Gateway) GetModelID() string {
	provider, ok := g.providers.Load(g.defaultProvider)
	if !ok {
		return ""
	}
	return provider.GetModelID()
}

// HealthCheck 健康检查所有提供商
func (g *Gateway) HealthCheck(ctx context.Context) map[string]error {
	providers := make(map[string]Provider)
	g.providers.Range(func(name string, p Provider) bool {
		providers[name] = p
		return true
	})

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

// Embed 使用默认提供商进行文本向量化
func (g *Gateway) Embed(ctx context.Context, text string) ([]float32, error) {
	providerName := g.GetDefaultProvider()
	if providerName == "" {
		return nil, fmt.Errorf("no provider configured for embedding")
	}

	provider, ok := g.providers.Load(providerName)

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	return provider.Embed(ctx, text)
}

// EmbedWithProvider 使用指定提供商进行文本向量化
func (g *Gateway) EmbedWithProvider(ctx context.Context, providerName, text string) ([]float32, error) {
	provider, ok := g.providers.Load(providerName)

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	return provider.Embed(ctx, text)
}

// EmbedBatch 批量向量化
func (g *Gateway) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	providerName := g.GetDefaultProvider()
	if providerName == "" {
		return nil, fmt.Errorf("no provider configured for embedding")
	}

	provider, ok := g.providers.Load(providerName)

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	return provider.EmbedBatch(ctx, texts)
}

// FallbackEmbed 带故障转移的向量化
func (g *Gateway) FallbackEmbed(ctx context.Context, text string) ([]float32, error) {
	providers := g.GetEnabledProviders()
	if len(providers) == 0 {
		return nil, fmt.Errorf("no enabled providers")
	}

	var lastErr error
	for _, name := range providers {
		vec, err := g.EmbedWithProvider(ctx, name, text)
		if err == nil {
			return vec, nil
		}
		lastErr = err
		slog.Warn("Embedding provider failed, trying next", "provider", name, "error", err)
	}

	return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
}

// ============ 实现 kernel.LLMProvider 接口 ============

// humanizeError wraps common API errors with user-friendly messages.
func humanizeError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case containsAny(msg, "429", "余额不足", "insufficient", "quota", "rate limit"):
		return fmt.Errorf("%w\n  💡 账户余额不足或超出速率限制。请检查 API Key 余额或稍后重试。", err)
	case containsAny(msg, "401", "unauthorized", "invalid api key", "access denied"):
		return fmt.Errorf("%w\n  💡 API Key 无效。请检查 ~/.openaide/config.yaml 中的 api_key 是否正确。", err)
	case containsAny(msg, "404", "not found"):
		return fmt.Errorf("%w\n  💡 模型或接口不存在。请检查 base_url 和 default_model 配置是否正确。", err)
	case containsAny(msg, "timeout", "deadline", "connection refused"):
		return fmt.Errorf("%w\n  💡 网络超时或无法连接。请检查网络和 base_url 是否正确。", err)
	case containsAny(msg, "context deadline"):
		return fmt.Errorf("%w\n  💡 请求超时。模型响应太慢，可以尝试增大 timeout 配置或换更快的模型。", err)
	}
	return err
}

func containsAny(s string, substrs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// Ensure Gateway implements kernel.LLMProvider
var _ kernel.LLMProvider = (*Gateway)(nil)
