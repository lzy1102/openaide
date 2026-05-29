package projectmind

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"log/slog"
	"time"
)

// ProjectMind 项目持久记忆 — 跨会话积累的项目知识
type ProjectMind struct {
	mu           sync.RWMutex
	path         string
	CodeMap     map[string]CodeEntry  `json:"code_map"`
	RiskMap     map[string]RiskEntry  `json:"risk_map"`
	Learnings   LearningSet           `json:"learnings"`
	Architecture ArchInfo             `json:"architecture"`
	UpdatedAt    time.Time            `json:"updated_at"`

	// 持续学习
	ExecHistory  []ExecRecord    `json:"exec_history"`
	Strategies   []StrategyStats `json:"strategies"`
	Conventions  []Convention    `json:"conventions"`
	SessionCount int             `json:"session_count"`
}

type CodeEntry struct {
	Purpose    string    `json:"purpose"`
	Exports    []string  `json:"exports,omitempty"`
	Confidence float64   `json:"confidence"`  // 0-1, 过期自动降
	LastSeen   time.Time `json:"last_seen"`
	Source     string    `json:"source"`      // research/execution/review
}

type RiskEntry struct {
	Risk  string    `json:"risk"`
	Level string    `json:"level"` // low/medium/high/critical
	Fixed bool      `json:"fixed"`
	Found time.Time `json:"found"`
}

type LearningSet struct {
	Patterns     []string `json:"patterns"`
	Conventions  []string `json:"conventions"`
	Pitfalls     []string `json:"pitfalls"`
}

type ArchInfo struct {
	EntryPoints []string `json:"entry_points"`
	Framework   string   `json:"framework"`
	Database    string   `json:"database"`
	KeyModules  []string `json:"key_modules"`
	Language    string   `json:"language"`
}

// Load 加载项目记忆
func Load(projectDir string) *ProjectMind {
	path := filepath.Join(projectDir, "data", "memory", "project_mind.json")
	pm := &ProjectMind{
		path: path,
		CodeMap: make(map[string]CodeEntry),
		RiskMap: make(map[string]RiskEntry),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return pm // 首次使用，返回空
	}
	json.Unmarshal(data, pm)
	if pm.CodeMap == nil { pm.CodeMap = make(map[string]CodeEntry) }
	if pm.RiskMap == nil { pm.RiskMap = make(map[string]RiskEntry) }
	return pm
}

// Save 持久化
func (pm *ProjectMind) Save() error {
	slog.Debug("ProjectMind save", "path", pm.path)
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	pm.UpdatedAt = time.Now()
	os.MkdirAll(filepath.Dir(pm.path), 0755)
	data, err := json.MarshalIndent(pm, "", "  ")
	if err != nil { return err }
	return os.WriteFile(pm.path, data, 0644)
}

// AddCodeFact 记录代码事实
func (pm *ProjectMind) AddCodeFact(file, purpose string, exports []string, confidence float64, source string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	entry, exists := pm.CodeMap[file]
	if exists && entry.Confidence > confidence {
		confidence = entry.Confidence // 不降置信度
	}
	pm.CodeMap[file] = CodeEntry{
		Purpose: purpose, Exports: exports,
		Confidence: confidence, LastSeen: time.Now(), Source: source,
	}
}

// AddRisk 记录风险
func (pm *ProjectMind) AddRisk(file, risk, level string, fixed bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.RiskMap[file] = RiskEntry{Risk: risk, Level: level, Fixed: fixed, Found: time.Now()}
}

// AddLearning 添加学习经验
func (pm *ProjectMind) AddLearning(category, content string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	switch category {
	case "pattern":
		pm.Learnings.Patterns = appendUnique(pm.Learnings.Patterns, content)
	case "convention":
		pm.Learnings.Conventions = appendUnique(pm.Learnings.Conventions, content)
	pm.ConfirmConvention(content)
	case "pitfall":
		pm.Learnings.Pitfalls = appendUnique(pm.Learnings.Pitfalls, content)
	}
}

// SetArchitecture 更新架构信息（只更新非空字段）
func (pm *ProjectMind) SetArchitecture(entry string, framework, database string, modules []string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if entry != "" { pm.Architecture.EntryPoints = appendUnique(pm.Architecture.EntryPoints, entry) }
	if framework != "" { pm.Architecture.Framework = framework }
	if database != "" { pm.Architecture.Database = database }
	if len(modules) > 0 { pm.Architecture.KeyModules = modules }
}

// FactsForPrompt 生成注入 prompt 的已知事实摘要
func (pm *ProjectMind) FactsForPrompt() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var sb strings.Builder

	// 架构
	if pm.Architecture.Framework != "" {
		sb.WriteString(fmt.Sprintf("## 项目已知信息 (来自 ProjectMind)\n"))
		sb.WriteString(fmt.Sprintf("- 框架: %s | 数据库: %s | 语言: %s\n", pm.Architecture.Framework, pm.Architecture.Database, pm.Architecture.Language))
		if len(pm.Architecture.KeyModules) > 0 {
			sb.WriteString(fmt.Sprintf("- 核心模块: %s\n", strings.Join(pm.Architecture.KeyModules, ", ")))
		}
	}

	// 代码地图 (最近且有高置信度的)
	sb.WriteString("\n### 已知代码结构\n")
	count := 0
	for file, entry := range pm.CodeMap {
		if count >= 10 { break }
		age := time.Since(entry.LastSeen)
		if age > 7*24*time.Hour && entry.Confidence < 0.7 { continue } // 过期低置信度跳过
		sb.WriteString(fmt.Sprintf("- %s: %s (置信度: %.0f%%)\n", file, entry.Purpose, entry.Confidence*100))
		count++
	}

	// 风险
	if len(pm.RiskMap) > 0 {
		sb.WriteString("\n### 已知风险\n")
		for file, r := range pm.RiskMap {
			status := "⚠"
			if r.Fixed { status = "✅已修复" }
			sb.WriteString(fmt.Sprintf("- %s%s: %s [%s]\n", status, file, r.Risk, r.Level))
		}
	}

	// 学习
	if len(pm.Learnings.Patterns) > 0 {
		sb.WriteString("\n### 项目模式\n")
		for _, p := range pm.Learnings.Patterns {
			sb.WriteString(fmt.Sprintf("- %s\n", p))
		}
	}
	if len(pm.Learnings.Pitfalls) > 0 {
		sb.WriteString("\n### 已知陷阱\n")
		for _, p := range pm.Learnings.Pitfalls {
			sb.WriteString(fmt.Sprintf("- ⚠ %s\n", p))
		}
	}

	if sb.Len() == 0 { return "" }
	return sb.String()
}

// RisksForPlanning 返回影响计划的风险列表
func (pm *ProjectMind) RisksForPlanning() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var risks []string
	for file, r := range pm.RiskMap {
		if !r.Fixed {
			risks = append(risks, fmt.Sprintf("%s: %s [%s]", file, r.Risk, r.Level))
		}
	}
	return strings.Join(risks, "; ")
}

// GenerateLearnedRules returns high-confidence conventions formatted as system prompt rules.
func (pm *ProjectMind) GenerateLearnedRules() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var sb strings.Builder
	for _, conv := range pm.Conventions {
		if conv.Invalid { continue }
		if conv.Confidence >= 0.8 {
			sb.WriteString(fmt.Sprintf("- %s  (conf:%.0f%%)\n", conv.Rule, conv.Confidence*100))
		}
	}
	for _, p := range pm.Learnings.Pitfalls {
		sb.WriteString("- ⚠ " + p + "\n")
	}
	if sb.Len() == 0 {
		return ""
	}
	return "\n## 学到的规则（自动生成，跨会话积累）\n" + sb.String()
}

// HasHighConfidenceConventions returns true if there are rules ready to promote.
func (pm *ProjectMind) HasHighConfidenceConventions() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, conv := range pm.Conventions {
		if conv.Confidence >= 0.8 {
			return true
		}
	}
	return false
}

// SyncToSystemPrompt writes learned rules into the system prompt file.
func (pm *ProjectMind) SyncToSystemPrompt(promptsDir, lang string) error {
	pm.CleanupDeadConventions()
	rules := pm.GenerateLearnedRules()
	if rules == "" {
		return nil
	}

	suffix := ".en"
	if lang == "zh" {
		suffix = ".zh"
	}
	path := filepath.Join(promptsDir, "system"+suffix+".md")

	data, _ := os.ReadFile(path)
	current := strings.TrimRight(string(data), "\n")

	// Remove previous learned rules block
	marker := "\n## 学到的规则（自动生成"
	if idx := strings.Index(current, marker); idx >= 0 {
		current = current[:idx]
	}

	current += "\n" + rules + "\n"
	os.MkdirAll(promptsDir, 0755)
	os.WriteFile(path, []byte(current), 0644)
	return nil
}

// ExpireOldFacts 标记过期事实（降置信度）
func (pm *ProjectMind) ExpireOldFacts() {
	pm.DecayConventions()
	pm.mu.Lock()
	defer pm.mu.Unlock()
	now := time.Now()
	for key, entry := range pm.CodeMap {
		if now.Sub(entry.LastSeen) > 7*24*time.Hour {
			entry.Confidence *= 0.8
			pm.CodeMap[key] = entry
		}
		if now.Sub(entry.LastSeen) > 30*24*time.Hour {
			entry.Confidence *= 0.5
			pm.CodeMap[key] = entry
		}
	}
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item { return slice }
	}
	return append(slice, item)
}

// ============ 持续学习系统 ============

// ExecRecord 一次任务执行的完整记录
type ExecRecord struct {
	Task       string    `json:"task"`
	Approach   string    `json:"approach"`
	Success    bool      `json:"success"`
	FilesChanged []string `json:"files_changed,omitempty"`
	Errors     []string  `json:"errors,omitempty"`
	Fixes      []string  `json:"fixes,omitempty"`
	Time       float64   `json:"time_sec"`
	Model      string    `json:"model"`
	At         time.Time `json:"at"`
}

// StrategyStats 方案有效性统计
type StrategyStats struct {
	Name         string   `json:"name"`
	SuccessCount int      `json:"success_count"`
	TotalCount   int      `json:"total_count"`
	BestFor      []string `json:"best_for"`
	WorstFor     []string `json:"worst_for"`
}

// Convention 从错误中学习的项目约定
type Convention struct {
	Rule        string    `json:"rule"`
	Source      string    `json:"source"`
	Confidence  float64   `json:"confidence"`
	LearnedAt   time.Time `json:"learned_at"`
	ValidatedAt time.Time `json:"validated_at"` // last confirmed or contradicted
	Invalid     bool      `json:"invalid"`      // explicitly marked as wrong
}

// ConfirmConvention boosts confidence when the pattern is observed again.
func (pm *ProjectMind) ConfirmConvention(rule string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, c := range pm.Conventions {
		if c.Rule == rule && !c.Invalid {
			c.Confidence = min(0.95, c.Confidence+0.1)
			c.ValidatedAt = time.Now()
			pm.Conventions[i] = c
			return
		}
	}
	// New convention
	pm.Conventions = append(pm.Conventions, Convention{
		Rule: rule, Confidence: 0.5, Source: "observation",
		LearnedAt: time.Now(), ValidatedAt: time.Now(),
	})
}

// ContradictConvention decreases confidence when evidence contradicts.
// Returns true if the convention should be removed from the active prompt.
func (pm *ProjectMind) ContradictConvention(rule string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, c := range pm.Conventions {
		if c.Rule == rule && !c.Invalid {
			c.Confidence = max(0.1, c.Confidence-0.25)
			c.ValidatedAt = time.Now()
			if c.Confidence < 0.3 { c.Invalid = true }
			pm.Conventions[i] = c
			return c.Invalid
		}
	}
	return false
}

// InvalidateConvention marks a convention as explicitly wrong.
func (pm *ProjectMind) InvalidateConvention(rule string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, c := range pm.Conventions {
		if c.Rule == rule {
			c.Confidence = 0
			c.Invalid = true
			c.ValidatedAt = time.Now()
			pm.Conventions[i] = c
			return
		}
	}
}

// DecayConventions ages unvalidated conventions (call from ExpireOldFacts).
func (pm *ProjectMind) DecayConventions() {
	now := time.Now()
	for i, c := range pm.Conventions {
		if c.Invalid { continue }
		if now.Sub(c.ValidatedAt) > 14*24*time.Hour {
			c.Confidence *= 0.85
			pm.Conventions[i] = c
		}
		if now.Sub(c.ValidatedAt) > 30*24*time.Hour {
			c.Confidence *= 0.7
			pm.Conventions[i] = c
		}
	}
}

// CleanupDeadConventions removes invalidated and very-low-confidence conventions.
func (pm *ProjectMind) CleanupDeadConventions() {
	filtered := make([]Convention, 0, len(pm.Conventions))
	for _, c := range pm.Conventions {
		if !c.Invalid && c.Confidence >= 0.3 {
			filtered = append(filtered, c)
		}
	}
	pm.Conventions = filtered
}

// RecordExecution 记录一次任务执行
func (pm *ProjectMind) RecordExecution(task, approach string, success bool, files, errors, fixes []string, duration time.Duration, model string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.ExecHistory = append(pm.ExecHistory, ExecRecord{
		Task: task, Approach: approach, Success: success,
		FilesChanged: files, Errors: errors, Fixes: fixes,
		Time: duration.Seconds(), Model: model, At: time.Now(),
	})
	// 只保留最近 50 条
	if len(pm.ExecHistory) > 50 {
		pm.ExecHistory = pm.ExecHistory[len(pm.ExecHistory)-50:]
	}
}

// UpdateStrategy 更新方案有效性统计
func (pm *ProjectMind) UpdateStrategy(name string, success bool, taskType string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, s := range pm.Strategies {
		if s.Name == name {
			pm.Strategies[i].TotalCount++
			if success { pm.Strategies[i].SuccessCount++ }
			if success {
				pm.Strategies[i].BestFor = appendUnique(pm.Strategies[i].BestFor, taskType)
			} else {
				pm.Strategies[i].WorstFor = appendUnique(pm.Strategies[i].WorstFor, taskType)
			}
			return
		}
	}
	// 新方案
	s := StrategyStats{Name: name, TotalCount: 1}
	if success { s.SuccessCount = 1; s.BestFor = []string{taskType} } else { s.WorstFor = []string{taskType} }
	pm.Strategies = append(pm.Strategies, s)
}

// StrategyAdvice 返回方案选择建议（给 Propose 阶段使用）
func (pm *ProjectMind) StrategyAdvice() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if len(pm.Strategies) == 0 { return "" }
	var sb strings.Builder
	sb.WriteString("## 历史方案效果\n")
	for _, s := range pm.Strategies {
		rate := 0
		if s.TotalCount > 0 { rate = s.SuccessCount * 100 / s.TotalCount }
		sb.WriteString(fmt.Sprintf("- %s: 成功率 %d/%d (%d%%)\n", s.Name, s.SuccessCount, s.TotalCount, rate))
		if len(s.BestFor) > 0 { sb.WriteString(fmt.Sprintf("  适用: %s\n", strings.Join(dedup(s.BestFor), ", "))) }
		if len(s.WorstFor) > 0 { sb.WriteString(fmt.Sprintf("  不适用: %s\n", strings.Join(dedup(s.WorstFor), ", "))) }
	}
	return sb.String()
}

// LearnConvention 从错误中学习项目约定
func (pm *ProjectMind) LearnConvention(rule, source string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, c := range pm.Conventions {
		if c.Rule == rule {
			pm.Conventions[i].Confidence = min(1.0, c.Confidence+0.15)
			pm.Conventions[i].LearnedAt = time.Now()
			return
		}
	}
	pm.Conventions = append(pm.Conventions, Convention{Rule: rule, Source: source, Confidence: 0.6, LearnedAt: time.Now()})
}

// ConventionsForPrompt 生成约定的 prompt 注入
func (pm *ProjectMind) ConventionsForPrompt() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if len(pm.Conventions) == 0 { return "" }
	var sb strings.Builder
	sb.WriteString("\n### 项目约定（自动学习）\n")
	for _, c := range pm.Conventions {
		if c.Confidence > 0.5 {
			sb.WriteString(fmt.Sprintf("- %s (置信度: %.0f%%)\n", c.Rule, c.Confidence*100))
		}
	}
	if sb.Len() == 0 { return "" }
	return sb.String()
}

// AnalyzeBuildError 从构建/测试错误中提取约定
func (pm *ProjectMind) AnalyzeBuildError(output string) {
	// 命名约定检测
	if strings.Contains(output, "undefined:") && strings.Contains(output, "snake_case") {
		pm.LearnConvention("命名使用 camelCase, 不要用 snake_case", "go build 错误")
	}
	if strings.Contains(output, "exported") && strings.Contains(output, "should have comment") {
		pm.LearnConvention("导出符号需要注释", "go build 警告")
	}
	// 依赖检测
	if strings.Contains(output, "cannot find package") {
		pm.LearnConvention("缺少依赖时先检查 go.mod 和 import 路径", "go build 错误")
	}
	if strings.Contains(output, "testify") || strings.Contains(output, "stretchr/testify") {
		pm.LearnConvention("测试框架: testify (github.com/stretchr/testify)", "测试输出")
	}
	// 导入检测
	if strings.Contains(output, "import cycle") {
		pm.LearnConvention("注意包之间的循环依赖，考虑拆分或提取公共接口", "go build 错误")
	}
	// 错误处理
	if strings.Contains(output, "error strings should not be capitalized") {
		pm.LearnConvention("Go 错误消息不要大写开头", "go vet 警告")
	}
	if strings.Contains(output, "should not use underscores in Go names") {
		pm.LearnConvention("Go 命名不要用下划线，用驼峰命名", "go vet 警告")
	}
}

// RecentFailures 返回最近失败的模式
func (pm *ProjectMind) RecentFailures() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var failures []string
	for i := len(pm.ExecHistory) - 1; i >= 0 && len(failures) < 3; i-- {
		if !pm.ExecHistory[i].Success {
			failures = append(failures, fmt.Sprintf("- %s: %v", pm.ExecHistory[i].Task, pm.ExecHistory[i].Errors))
		}
	}
	if len(failures) == 0 { return "" }
	return "## 最近失败记录\n" + strings.Join(failures, "\n")
}

// SyncToKnowledgeBase 将 ProjectMind 事实同步到知识库，使语义搜索可用
func (pm *ProjectMind) SyncToKnowledgeBase(kb KnowledgeBaseWriter) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 代码地图 → 短文档
	for file, entry := range pm.CodeMap {
		if entry.Confidence < 0.6 { continue }
		content := fmt.Sprintf("文件: %s\n用途: %s", file, entry.Purpose)
		if len(entry.Exports) > 0 {
			content += fmt.Sprintf("\n导出: %s", strings.Join(entry.Exports, ", "))
		}
		kb.AddKnowledge(context.Background(), "pm-"+file, content, "projectmind", []string{"projectmind", "code-map"})
	}

	// 风险 → 短文档
	for file, r := range pm.RiskMap {
		status := "未修复"
		if r.Fixed { status = "已修复" }
		content := fmt.Sprintf("文件: %s\n风险: %s\n等级: %s\n状态: %s", file, r.Risk, r.Level, status)
		kb.AddKnowledge(context.Background(), "pm-risk-"+file, content, "projectmind", []string{"projectmind", "risk"})
	}

	// 约定 → 短文档
	for _, c := range pm.Conventions {
		if c.Confidence < 0.6 { continue }
		kb.AddKnowledge(context.Background(), "pm-conv-"+c.Rule[:min(20, len(c.Rule))],
			fmt.Sprintf("项目约定: %s\n来源: %s\n置信度: %.0f%%", c.Rule, c.Source, c.Confidence*100),
			"projectmind", []string{"projectmind", "convention"})
	}
}

// KnowledgeBaseWriter ProjectMind 写入知识库所需的最小接口
type KnowledgeBaseWriter interface {
	AddKnowledge(ctx context.Context, title, content, source string, tags []string) (string, error)
}

func dedup(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slice {
		if !seen[s] { seen[s] = true; result = append(result, s) }
	}
	return result
}

