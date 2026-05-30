package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"openaide/backend/internal/kernel"
)

// PromptCache 提示词缓存 — 减少重复token消耗
type PromptCache struct {
	dataDir string
	entries *kernel.SafeMap[string, *cacheEntry]
	stopCh  chan struct{}
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
		entries: kernel.NewSafeMap[string, *cacheEntry](64),
		stopCh:  make(chan struct{}),
	}
	c.load()
	go c.cleanupLoop()
	return c
}

// Shutdown 停止后台清理 goroutine
func (c *PromptCache) Shutdown() { close(c.stopCh) }

// key 生成缓存键
func (c *PromptCache) key(messages []kernel.Message, tools []kernel.ToolDefinition, model string) string {
	h := sha256.New()
	data, _ := json.Marshal(messages)
	h.Write(data)
	data, _ = json.Marshal(tools)
	h.Write(data)
	h.Write([]byte(model))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Get 获取缓存
func (c *PromptCache) Get(messages []kernel.Message, tools []kernel.ToolDefinition, model string) *kernel.LLMResponse {
	e, ok := c.entries.Load(c.key(messages, tools, model))
	if !ok || time.Since(e.CreatedAt) > 1*time.Hour {
		return nil
	}
	e.HitCount++ // safe: only readers call Get, writers call Set
	return e.Response
}

// Set 存入缓存
func (c *PromptCache) Set(messages []kernel.Message, tools []kernel.ToolDefinition, model string, resp *kernel.LLMResponse) {
	k := c.key(messages, tools, model)
	c.entries.Store(k, &cacheEntry{Response: resp, CreatedAt: time.Now()})
	c.save(k)
}

// Stats 缓存统计
func (c *PromptCache) Stats() map[string]int {
	hits := 0
	c.entries.Range(func(_ string, e *cacheEntry) bool {
		hits += e.HitCount
		return true
	})
	return map[string]int{"entries": c.entries.Len(), "hits": hits}
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
	cutoff := time.Now().Add(-maxAge)
	c.entries.Range(func(k string, e *cacheEntry) bool {
		if e.CreatedAt.Before(cutoff) {
			c.entries.Delete(k)
			os.Remove(filepath.Join(c.dataDir, k+".json"))
		}
		return true
	})
}

func (c *PromptCache) save(key string) {
	e, ok := c.entries.Load(key)
	if !ok {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(c.dataDir, key+".json"), data, 0644)
}

func (c *PromptCache) load() {
	entries, err := os.ReadDir(c.dataDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.dataDir, e.Name()))
		if err != nil {
			continue
		}
		var entry cacheEntry
		if json.Unmarshal(data, &entry) == nil {
			c.entries.Store(e.Name()[:len(e.Name())-5], &entry)
		}
	}
}
