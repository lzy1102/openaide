// Package extensions 定义"按配置门控、可替换"的内核策略扩展接缝。
//
// 阶段 0 的目标是让内核算盘可裁剪:凡是"由配置段名选择实现"的策略
// (反思/规划/上下文压缩/自适应轮数,乃至后续的检索、记忆)都应登记到
// Registry 中,由装配层统一 Apply,而不是在 NewApplication 里写死 if/else。
// 这是一个稳定的加载期契约 —— 第三方能力正是挂在这样一个接缝上。
package extensions

import "openaide/backend/config"

// StrategyFactory 由具体实现提供:给定配置,自行判断是否启用并完成装配。
// 返回 error 表示装配失败;是否启用由工厂内部依据 cfg 决定。
type StrategyFactory func(cfg *config.Config) error

// Strategy 描述一个"由配置选择的策略"。Key 是其在配置中的语义键,
// 用于文档与诊断;真正决定行为的是 Factory。
type Strategy struct {
	Key     string
	Factory StrategyFactory
}

// Registry 收集按配置门控的策略,装配阶段按注册顺序执行。
type Registry struct {
	strategies []Strategy
}

// NewRegistry 创建策略注册表。
func NewRegistry(strategies ...Strategy) *Registry {
	return &Registry{strategies: strategies}
}

// Append 追加策略(支持按需增量装配)。
func (r *Registry) Append(strategies ...Strategy) *Registry {
	r.strategies = append(r.strategies, strategies...)
	return r
}

// Len 返回已登记策略数。
func (r *Registry) Len() int { return len(r.strategies) }

// Apply 依序执行全部策略工厂;任一失败即返回。
func (r *Registry) Apply(cfg *config.Config) error {
	for _, s := range r.strategies {
		if err := s.Factory(cfg); err != nil {
			return err
		}
	}
	return nil
}