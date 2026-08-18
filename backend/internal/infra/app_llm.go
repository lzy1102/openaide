package infra

import (
	"log/slog"

	"openaide/backend/config"
	"openaide/backend/llm"
)

func createLLMGateway(cfg *config.Config) *llm.Gateway {
	gateway := llm.NewGateway()
	for _, providerCfg := range cfg.LLM.Providers {
		if !providerCfg.Enabled {
			continue
		}

		var provider llm.Provider
		llmConfig := &llm.ProviderConfig{
			Name:            providerCfg.Name,
			Type:            providerCfg.Type,
			BaseURL:         providerCfg.BaseURL,
			APIKey:          providerCfg.APIKey,
			DefaultModel:    providerCfg.DefaultModel,
			EmbeddingModel:  providerCfg.EmbeddingModel,
			Timeout:         providerCfg.Timeout,
			Headers:         providerCfg.Headers,
			Enabled:         providerCfg.Enabled,
			Thinking:        providerCfg.Thinking,
			ReasoningEffort: providerCfg.ReasoningEffort,
			JSONMode:        providerCfg.JSONMode,
			StrictTools:     providerCfg.StrictTools,
		}

		switch providerCfg.Type {
		case "openai", "openai-compatible":
			provider = llm.NewOpenAIProvider(llmConfig)
		case "anthropic":
			provider = llm.NewAnthropicProvider(llmConfig)
		default:
			slog.Warn("Unknown provider type, skipping", "name", providerCfg.Name, "type", providerCfg.Type)
			continue
		}

		gateway.RegisterProvider(providerCfg.Name, provider, llmConfig)
		slog.Info("Provider registered", "name", providerCfg.Name, "model", providerCfg.DefaultModel)
	}

	if cfg.LLM.DefaultProvider != "" {
		if err := gateway.SetDefaultProvider(cfg.LLM.DefaultProvider); err != nil {
			slog.Warn("Failed to set default provider", "error", err)
		}
	}

	gateway.SetPromptCache(llm.NewPromptCache(cfg.Storage.DataDir + "/cache"))

	// 全局 LLM 调用限流:10 tokens/s, 100 突发(多会话并发保护,防止 provider 配额打爆)
	gateway.SetRateLimiter(10, 100)

	if cfg.Router.Enabled {
		gateway.SetRouter(llm.NewRouter(cfg.Router.Rules))
	} else {
		gateway.SetRouter(llm.DefaultRouter())
	}

	// 成本感知路由：小调用用 execution，核心推理用 reasoning
	gateway.ExecutionModel = cfg.LLM.ModelRouting.Execution
	gateway.ReasoningModel = cfg.LLM.ModelRouting.Reasoning

	return gateway
}
