package projectmind

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// ExpireOldFacts 标记过期事实（降置信度）
func (pm *ProjectMind) ExpireOldFacts() {
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
