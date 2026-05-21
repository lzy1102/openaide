package feedback

import (
	"math"

	"openaide/backend/internal/kernel"
)

// Verdict 用户反馈结论
type Verdict int

const (
	VerdictNone Verdict = iota // 无反馈
	VerdictGood                // 用户认可
	VerdictBad                 // 用户否定/纠正
)

// Result 执行结果快照（用于质量评估）
type Result struct {
	Query         string                   `json:"query"`
	Response      string                   `json:"response"`
	ToolSuccesses int                      `json:"tool_successes"`
	ToolFailures  int                      `json:"tool_failures"`
	Reflection    *kernel.ReflectionResult `json:"reflection,omitempty"`
	UserVerdict   Verdict                  `json:"user_verdict"`
}

// QualityScore 计算综合质量评分 (0.0-1.0)
// 40% 工具成功率 + 30% 用户反馈 + 30% 反思自评
func (r *Result) QualityScore() float64 {
	toolScore := 0.5 // 默认中性
	total := r.ToolSuccesses + r.ToolFailures
	if total > 0 {
		toolScore = float64(r.ToolSuccesses) / float64(total)
	}

	userScore := 0.5 // 默认中性（无反馈）
	switch r.UserVerdict {
	case VerdictGood:
		userScore = 1.0
	case VerdictBad:
		userScore = 0.0
	}

	reflectScore := 0.5
	if r.Reflection != nil {
		reflectScore = float64(r.Reflection.Quality) / 10.0
	}

	score := 0.4*toolScore + 0.3*userScore + 0.3*reflectScore
	return math.Round(score*100) / 100
}

// ShouldKeep 判断是否应该存入知识库 (阈值 >0.6)
func (r *Result) ShouldKeep() bool {
	return r.QualityScore() > 0.6
}

// ShouldDiscard 判断是否应该丢弃 (阈值 <0.3)
func (r *Result) ShouldDiscard() bool {
	return r.QualityScore() < 0.3
}

// NeedsConfirmation 判断是否需要用户确认 (0.3-0.6之间)
func (r *Result) NeedsConfirmation() bool {
	s := r.QualityScore()
	return s >= 0.3 && s <= 0.6
}

// Gate 质量门控
type Gate struct {
	MinScore float64 // 存入知识库的最低分数 (默认 0.6)
}

// NewGate 创建质量门控
func NewGate() *Gate {
	return &Gate{MinScore: 0.6}
}

// check 检查 Result 是否通过门控
func (g *Gate) check(r *Result) bool {
	return r.QualityScore() >= g.MinScore
}

// ExtractKnowledge 从成功执行中提取知识点
func (g *Gate) ExtractKnowledge(r *Result) (title, content string, ok bool) {
	if !g.check(r) {
		return "", "", false
	}

	// 用 Reflection 的建议作为标题
	title = r.Query
	if len(title) > 80 {
		title = title[:80] + "..."
	}

	content = r.Response
	if r.Reflection != nil && len(r.Reflection.Learned) > 0 {
		content += "\n\n[经验] " + r.Reflection.Learned
	}

	return title, content, true
}

// Pass 实现 kernel.QualityGate 接口
func (g *Gate) Pass(query, response string, toolSuccesses, toolFailures int, reflection *kernel.ReflectionResult) bool {
	r := &Result{
		Query:         query,
		Response:      response,
		ToolSuccesses: toolSuccesses,
		ToolFailures:  toolFailures,
		Reflection:    reflection,
	}
	return r.QualityScore() >= g.MinScore
}
