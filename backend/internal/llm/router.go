package llm

import (
	"regexp"
	"strings"
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

// DefaultRouter 内置路由规则
func DefaultRouter() *Router {
	return NewRouter([]RouteRule{
		// 代码相关任务 → 优先用支持思考的模型
		{Name: "coding", Pattern: "代码|写.*程序|编程|debug|调试|bug|错误|修复|fix|refactor|重构|函数|function|class|类|API|接口|实现|写.*代码|后端|前端|Go|Python|Rust|TypeScript|编译|build|test|测试", Provider: "", Model: "", Priority: 10},

		// 搜索/知识查询 → 用便宜快速的模型
		{Name: "search", Pattern: "搜索|查询|查找|最新|什么|怎么|如何|为什么|定义|解释|说明|介绍|简述", Provider: "", Model: "", Priority: 5},

		// 翻译任务
		{Name: "translate", Pattern: "翻译|translate|译成|英译|中译", Provider: "", Model: "", Priority: 5},

		// 简单对话/问候 → 用便宜模型
		{Name: "chat", Pattern: "^你好|^嗨|^hello|^hi|谢谢|再见|bye|好的|ok|嗯|哦|天气|笑话|故事", Provider: "", Model: "", Priority: 1},
	})
}

// ============ 路由配置解析 ============

// RouterConfig 用户路由器配置
type RouterConfig struct {
	Enabled bool        `json:"enabled" yaml:"enabled"`
	Rules   []RouteRule `json:"rules" yaml:"rules"`
}

// isComplexQuery 判断是否为复杂查询（用于自适应路由）
func isComplexQuery(query string) bool {
	indicators := []string{
		"分析", "设计", "架构", "优化", "重构", "实现",
		"修复", "排查", "部署", "配置", "集成",
		"最好", "推荐", "比较", "对比", "评估",
		"analyze", "design", "implement", "refactor",
		"optimize", "debug", "fix", "deploy",
	}
	count := 0
	for _, kw := range indicators {
		if strings.Contains(strings.ToLower(query), kw) {
			count++
		}
	}
	return count >= 2 || len([]rune(query)) > 200
}
