package main

import (
	"fmt"
	"strings"
	"sync"
)

// ── Log Ring Buffer ────────────────────────────────────────

type logRing struct {
	mu  sync.Mutex
	buf []string
}

func (r *logRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, strings.TrimSpace(string(p)))
	if len(r.buf) > 50 {
		r.buf = r.buf[1:]
	}
	return len(p), nil
}

func (r *logRing) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := make([]string, len(r.buf))
	copy(c, r.buf)
	return c
}

var tuiLogBuf = &logRing{buf: make([]string, 0, 50)}

// ── String Utils ───────────────────────────────────────────

func trunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}

// oneLine 把多行文本折叠为单行（缩进对齐用，避免内嵌换行破坏前缀/缩进）
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// formatTokens 格式化 token 数：≥1000 显示为 k
func formatTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
