package kernel

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskMetrics 单次任务的完整指标
type TaskMetrics struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Duration  float64   `json:"duration_ms"` // 毫秒

	// 任务分类
	TaskType   string `json:"task_type"`  // coding/review/think/debugging/general
	Complexity string `json:"complexity"` // low/medium/high

	// LLM 调用
	Rounds           int    `json:"rounds"`            // ReAct 轮次
	PromptTokens     int    `json:"prompt_tokens"`     // 总 prompt tokens
	CompletionTokens int    `json:"completion_tokens"` // 总 completion tokens
	TotalTokens      int    `json:"total_tokens"`
	Model            string `json:"model"` // 使用的模型

	// 工具
	ToolCalls   int `json:"tool_calls"`   // 工具调用次数
	ToolErrors  int `json:"tool_errors"`  // 工具错误次数
	UniqueTools int `json:"unique_tools"` // 使用的不同工具数

	// 结果
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`

	// 反思
	ReflectionScore int `json:"reflection_score"` // 1-10 质量分
}

// MetricsStore 指标存储 — append-only JSONL + 内存环形缓冲
type MetricsStore struct {
	mu      sync.RWMutex
	file    *os.File
	encoder *json.Encoder
	ring    []TaskMetrics // 最近 N 条
	ringCap int
	total   int64 // 历史总任务数
}

// NewMetricsStore 创建指标存储
func NewMetricsStore(dataDir string) *MetricsStore {
	if dataDir == "" {
		dataDir = "."
	}
	metricsFile := filepath.Join(dataDir, "metrics.jsonl")

	ms := &MetricsStore{
		ringCap: 500, // 保留最近 500 条
		ring:    make([]TaskMetrics, 0, 500),
	}

	// 打开/创建文件
	f, err := os.OpenFile(metricsFile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		slog.Warn("MetricsStore: 无法打开指标文件", "error", err)
	} else {
		ms.file = f
		ms.encoder = json.NewEncoder(f)
		ms.loadFromDisk()
	}

	return ms
}

// loadFromDisk 启动时加载最近 ringCap 条到内存
func (ms *MetricsStore) loadFromDisk() {
	if ms.file == nil {
		return
	}
	scanner := bufio.NewScanner(ms.file)
	var all []TaskMetrics
	for scanner.Scan() {
		var m TaskMetrics
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			continue
		}
		all = append(all, m)
		ms.total++
	}
	// 只保留最近 ringCap 条
	if len(all) > ms.ringCap {
		ms.ring = all[len(all)-ms.ringCap:]
	} else {
		ms.ring = all
	}
}

// Record 记录一次任务指标
func (ms *MetricsStore) Record(m TaskMetrics) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// 追加到内存环形缓冲
	if len(ms.ring) >= ms.ringCap {
		ms.ring = ms.ring[1:] // 滑出最旧
	}
	ms.ring = append(ms.ring, m)
	ms.total++

	// 追加到磁盘
	if ms.encoder != nil {
		ms.encoder.Encode(m)
	}
}

// Recent 返回最近 N 条指标
func (ms *MetricsStore) Recent(n int) []TaskMetrics {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	total := len(ms.ring)
	if n <= 0 || n > total {
		n = total
	}
	result := make([]TaskMetrics, n)
	copy(result, ms.ring[total-n:])
	return result
}

// Summary 返回汇总统计
func (ms *MetricsStore) Summary() map[string]interface{} {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	summary := map[string]interface{}{
		"total_tasks":  ms.total,
		"recent_count": len(ms.ring),
	}

	if len(ms.ring) == 0 {
		return summary
	}

	var totalDuration, totalTokens float64
	var totalRounds, totalToolCalls, totalToolErrors int
	var successes, failures int
	taskTypeCounts := map[string]int{}
	modelCounts := map[string]int{}

	for _, m := range ms.ring {
		totalDuration += m.Duration
		totalTokens += float64(m.TotalTokens)
		totalRounds += m.Rounds
		totalToolCalls += m.ToolCalls
		totalToolErrors += m.ToolErrors
		if m.Success {
			successes++
		} else {
			failures++
		}
		taskTypeCounts[m.TaskType]++
		modelCounts[m.Model]++
	}

	n := float64(len(ms.ring))
	summary["avg_duration_ms"] = totalDuration / n
	summary["avg_tokens"] = totalTokens / n
	summary["avg_rounds"] = float64(totalRounds) / n
	summary["avg_tool_calls"] = float64(totalToolCalls) / n
	summary["success_rate"] = float64(successes) / n
	summary["failure_rate"] = float64(failures) / n
	summary["total_tokens"] = int(totalTokens)
	summary["task_types"] = taskTypeCounts
	summary["models"] = modelCounts

	// 平均反思分数（非零）
	var reflectionSum, reflectionCount int
	for _, m := range ms.ring {
		if m.ReflectionScore > 0 {
			reflectionSum += m.ReflectionScore
			reflectionCount++
		}
	}
	if reflectionCount > 0 {
		summary["avg_reflection_score"] = float64(reflectionSum) / float64(reflectionCount)
	}

	return summary
}

// StatsForAPI 返回 /api/v1/tasks 需要的格式
func (ms *MetricsStore) StatsForAPI() map[string]interface{} {
	return ms.Summary()
}

// Close 关闭文件
func (ms *MetricsStore) Close() {
	if ms.file != nil {
		ms.file.Close()
	}
}

// RecordTaskMetrics is a convenience method on AgentKernel that delegates to MetricsStore.
func (k *AgentKernel) RecordTaskMetrics(m TaskMetrics) {
	if k.metrics != nil {
		k.metrics.Record(m)
	}
}

// TaskMetricsSummary returns summary stats for the /api/v1/tasks endpoint.
func (k *AgentKernel) TaskMetricsSummary() map[string]interface{} {
	if k.metrics != nil {
		return k.metrics.Summary()
	}
	return map[string]interface{}{"total_tasks": 0, "recent_count": 0}
}

// RecentTasks returns the N most recent task metrics.
func (k *AgentKernel) RecentTasks(n int) []TaskMetrics {
	if k.metrics != nil {
		return k.metrics.Recent(n)
	}
	return nil
}
