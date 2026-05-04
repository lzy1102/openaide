package services

import (
	"log/slog"
	"sync"
	"time"
)

type LLMBudgetManager struct {
	mu           sync.Mutex
	dailyLimit   int
	dailyUsed    int
	dailyReset   time.Time
	hourlyLimit  int
	hourlyUsed   int
	hourlyReset  time.Time
	enabled      bool
}

func NewLLMBudgetManager(dailyLimit, hourlyLimit int) *LLMBudgetManager {
	now := time.Now()
	return &LLMBudgetManager{
		dailyLimit:  dailyLimit,
		hourlyLimit: hourlyLimit,
		dailyReset:  time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour),
		hourlyReset: now.Truncate(time.Hour).Add(time.Hour),
		enabled:     dailyLimit > 0 || hourlyLimit > 0,
	}
}

func (m *LLMBudgetManager) CanCall() bool {
	if !m.enabled {
		return true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if now.After(m.dailyReset) {
		m.dailyUsed = 0
		m.dailyReset = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
	}
	if now.After(m.hourlyReset) {
		m.hourlyUsed = 0
		m.hourlyReset = now.Truncate(time.Hour).Add(time.Hour)
	}

	if m.dailyLimit > 0 && m.dailyUsed >= m.dailyLimit {
		slog.Warn("LLM daily budget exceeded", "component", "LLMBudget", "daily_used", m.dailyUsed, "daily_limit", m.dailyLimit)
		return false
	}
	if m.hourlyLimit > 0 && m.hourlyUsed >= m.hourlyLimit {
		slog.Warn("LLM hourly budget exceeded", "component", "LLMBudget", "hourly_used", m.hourlyUsed, "hourly_limit", m.hourlyLimit)
		return false
	}

	return true
}

func (m *LLMBudgetManager) RecordCall(tokensUsed int) {
	if !m.enabled {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dailyUsed++
	m.hourlyUsed++
}
