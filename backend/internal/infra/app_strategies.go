package infra

import (
	"log/slog"

	"openaide/backend/config"
	"openaide/backend/internal/compress"
	"openaide/backend/internal/infra/extensions"
	"openaide/backend/core"
	"openaide/backend/llm"
)

// kernelStrategyRegistry 汇聚内核可替换策略的装配工厂。
// 每个工厂依据 cfg.Kernel 自行决定是否启用并挂到 agentKernel,
// 取代原先在装配函数内写死的 SetReflection/SetPlanner 等 if/else。
func kernelStrategyRegistry(k *kernel.AgentKernel, gateway *llm.Gateway) *extensions.Registry {
	return extensions.NewRegistry(
		extensions.Strategy{
			Key: "kernel.reflection",
			Factory: func(cfg *config.Config) error {
				if cfg.Kernel.ReflectionEnabled != nil && !*cfg.Kernel.ReflectionEnabled {
					k.SetReflection(&kernel.NoReflection{})
					slog.Info("Reflection disabled by config")
					return nil
				}
				k.SetReflection(kernel.NewLLMReflection(gateway))
				return nil
			},
		},
		extensions.Strategy{
			Key: "kernel.adaptive_rounds",
			Factory: func(cfg *config.Config) error {
				minR := cfg.Kernel.MinRounds
				maxR := cfg.Kernel.MaxRoundsCap
				if minR <= 0 {
					minR = 5
				}
				if maxR <= 0 {
					maxR = 30
				}
				ar := kernel.NewAdaptiveRounds(minR, maxR)
				ar.SetLLM(gateway)
				k.SetAdaptiveRounds(ar)
				return nil
			},
		},
		extensions.Strategy{
			Key: "kernel.planner",
			Factory: func(cfg *config.Config) error {
				k.SetPlanner(kernel.NewPlanner(gateway))
				return nil
			},
		},
		extensions.Strategy{
			Key: "kernel.compressor",
			Factory: func(cfg *config.Config) error {
				k.SetContextCompressor(compress.NewLLMCompressor(gateway, compress.NewNovelCompressor()))
				return nil
			},
		},
	)
}