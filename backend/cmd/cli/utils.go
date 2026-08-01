package main

import (
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
