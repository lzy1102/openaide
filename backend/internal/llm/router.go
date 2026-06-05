package llm

import (
	"regexp"
)

// RouteRule 路由规则
type RouteRule struct {
	Name     string `json:"name" yaml:"name"`
	Pattern  string `json:"pattern" yaml:"pattern"`   // 正则匹配用户输入
	Provider string `json:"provider" yaml:"provider"` // 目标provider
	Model    string `json:"model" yaml:"model"`       // 目标model（可选，覆盖provider默认）
	Priority int    `json:"priority" yaml:"priority"` // 优先级(数字越大越高)
}

// Router 模型路由器 — 根据任务类型分配模型
type Router struct {
	rules   []RouteRule
	matcher []*regexp.Regexp
}

// NewRouter 创建路由器
func NewRouter(rules []RouteRule) *Router {
	r := &Router{rules: rules}
	for _, rule := range rules {
		re, err := regexp.Compile("(?i)" + rule.Pattern)
		if err == nil {
			r.matcher = append(r.matcher, re)
		} else {
			r.matcher = append(r.matcher, nil)
		}
	}
	return r
}

// Route 根据用户输入选择最佳 provider 和 model
// 返回 provider名称, model名称（空表示用默认）, 是否匹配到
func (r *Router) Route(query string) (provider, model string, matched bool) {
	if r == nil || len(r.rules) == 0 {
		return "", "", false
	}

	best := -1
	bestPriority := -1
	for i, re := range r.matcher {
		if re == nil {
			continue
		}
		if re.MatchString(query) {
			if r.rules[i].Priority > bestPriority {
				best = i
				bestPriority = r.rules[i].Priority
			}
		}
	}

	if best >= 0 {
		rule := r.rules[best]
		return rule.Provider, rule.Model, true
	}
	return "", "", false
}

// ============ 内置路由规则 ============

// DefaultRouter returns an empty router. Model routing decisions are made by
// the LLM via route:"execution"/"reasoning" options and the model_routing config.
// Users can add custom regex rules via config if needed for specific providers.
func DefaultRouter() *Router {
	return NewRouter(nil)
}

// ============ 路由配置解析 ============

// RouterConfig 用户路由器配置
type RouterConfig struct {
	Enabled bool        `json:"enabled" yaml:"enabled"`
	Rules   []RouteRule `json:"rules" yaml:"rules"`
}

// isComplexQuery 轻量预估查询复杂度（用于预路由）
// 复杂度的真正判断由 orchestrator 的 LLM 规划完成
func isComplexQuery(query string) bool {
	return len([]rune(query)) > 200
}
