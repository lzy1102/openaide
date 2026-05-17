package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	}
	c.load()
	// 定期清理过期缓存
	go func() {
		for range time.Tick(1 * time.Hour) {
			c.cleanup(24 * time.Hour)
		}
	}()
	return c
}

// key 生成缓存键 — 基于消息+工具+模型的哈希
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
	k := c.key(messages, tools, model)
	c.mu.RLock()
	e, ok := c.entries[k]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	// 缓存1小时有效
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

// Cleanup 清理过期条目
func (c *PromptCache) cleanup(maxAge time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for k, e := range c.entries {
		if e.CreatedAt.Before(cutoff) {
			delete(c.entries, k)
			os.Remove(filepath.Join(c.dataDir, k+".json"))
		}
	}
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

func (c *PromptCache) save(key string) {
	path := filepath.Join(c.dataDir, key+".json")
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return
	}
	data, _ := json.Marshal(e)
	os.WriteFile(path, data, 0644)
}

func (c *PromptCache) load() {
	entries, _ := os.ReadDir(c.dataDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(c.dataDir, e.Name()))
		var entry cacheEntry
		if json.Unmarshal(data, &entry) == nil {
			c.entries[e.Name()[:len(e.Name())-5]] = &entry
		}
	}
}
