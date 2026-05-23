package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
)

// PromptCache 提示词缓存 — 减少重复token消耗
type PromptCache struct {
	mu       sync.RWMutex
	dataDir  string
	entries  map[string]*cacheEntry
	stopCh   chan struct{}
}

type cacheEntry struct {
	Response  *kernel.LLMResponse `json:"response"`
	CreatedAt time.Time           `json:"created_at"`
	HitCount  int                 `json:"hit_count"`
}

// NewPromptCache 创建缓存
func NewPromptCache(dataDir string) *PromptCache {
	os.MkdirAll(dataDir, 0755)
	c := &PromptCache{
		dataDir: dataDir,
		entries: make(map[string]*cacheEntry),
		stopCh:  make(chan struct{}),
	}
	c.load()
	go c.cleanupLoop()
	return c
}

// Shutdown 停止后台清理 goroutine
func (c *PromptCache) Shutdown() {
	close(c.stopCh)
}

// key 生成缓存键 — 基于消息+工具+模型的哈希
func (c *PromptCache) key(messages []kernel.Message, tools []kernel.ToolDefinition, model string) string {
	h := sha256.New()
	data, err := json.Marshal(messages)
	if err != nil {
		slog.Warn("PromptCache: marshal messages failed, using fallback key", "error", err)
		h.Write([]byte(fmt.Sprintf("%v", messages)))
	} else {
		h.Write(data)
	}
	data, err = json.Marshal(tools)
	if err != nil {
		slog.Warn("PromptCache: marshal tools failed, using fallback key", "error", err)
		h.Write([]byte(fmt.Sprintf("%v", tools)))
	} else {
		h.Write(data)
	}
	h.Write([]byte(model))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Get 获取缓存
func (c *PromptCache) Get(messages []kernel.Message, tools []kernel.ToolDefinition, model string) *kernel.LLMResponse {
	k := c.key(messages, tools, model)
	c.mu.RLock()
	e, ok := c.entries[k]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	if time.Since(e.CreatedAt) > 1*time.Hour {
		return nil
	}
	c.mu.Lock()
	e.HitCount++
	c.mu.Unlock()
	return e.Response
}

// Set 存入缓存
func (c *PromptCache) Set(messages []kernel.Message, tools []kernel.ToolDefinition, model string, resp *kernel.LLMResponse) {
	k := c.key(messages, tools, model)
	c.mu.Lock()
	c.entries[k] = &cacheEntry{Response: resp, CreatedAt: time.Now()}
	c.mu.Unlock()
	c.save(k)
}

// Stats 缓存统计
func (c *PromptCache) Stats() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	hits := 0
	for _, e := range c.entries {
		hits += e.HitCount
	}
	return map[string]int{"entries": len(c.entries), "hits": hits}
}

func (c *PromptCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup(24 * time.Hour)
		}
	}
}

func (c *PromptCache) cleanup(maxAge time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for k, e := range c.entries {
		if e.CreatedAt.Before(cutoff) {
			delete(c.entries, k)
			if err := os.Remove(filepath.Join(c.dataDir, k+".json")); err != nil && !os.IsNotExist(err) {
				slog.Debug("PromptCache: remove expired entry failed", "key", k, "error", err)
			}
		}
	}
}

func (c *PromptCache) save(key string) {
	path := filepath.Join(c.dataDir, key+".json")
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		slog.Warn("PromptCache: marshal entry failed", "key", key, "error", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Warn("PromptCache: write entry failed", "path", path, "error", err)
	}
}

func (c *PromptCache) load() {
	entries, err := os.ReadDir(c.dataDir)
	if err != nil {
		slog.Warn("PromptCache: read dir failed", "dir", c.dataDir, "error", err)
		return
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.dataDir, e.Name()))
		if err != nil {
			slog.Debug("PromptCache: read entry failed", "file", e.Name(), "error", err)
			continue
		}
		var entry cacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			slog.Debug("PromptCache: unmarshal entry failed", "file", e.Name(), "error", err)
			continue
		}
		c.entries[e.Name()[:len(e.Name())-5]] = &entry
	}
}
