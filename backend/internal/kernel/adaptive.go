package kernel

import "strings"

// AdaptiveRounds 根据任务复杂度动态调整最大轮次
type AdaptiveRounds struct {
	MinRounds int
	MaxRounds int
}

// NewAdaptiveRounds 创建自适应轮次控制器
func NewAdaptiveRounds(min, max int) *AdaptiveRounds {
	if min <= 0 { min = 5 }
	if max <= 0 { max = 30 }
	return &AdaptiveRounds{MinRounds: min, MaxRounds: max}
}

// Calculate 根据查询复杂度计算合适的轮次
func (a *AdaptiveRounds) Calculate(query string, historyLength int) int {
	base := a.MinRounds

	// 长查询 → 更多轮次
	if len([]rune(query)) > 200 { base += 5 }
	if len([]rune(query)) > 500 { base += 5 }

	// 多步骤关键词 → 更多轮次
	multiStep := []string{"分析", "修复", "所有", "每个", "全部", "重构", "重新", "然后", "之后", "同时", "并且", "依次"}
	for _, kw := range multiStep {
		if strings.Contains(query, kw) { base += 2 }
	}

	// 历史长 → 当前对话复杂，给更多空间
	if historyLength > 10 { base += 3 }

	if base > a.MaxRounds { base = a.MaxRounds }
	return base
}
