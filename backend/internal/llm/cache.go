package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/kernel/actor"
)

const (
	defaultMaxEntries = 500
	defaultTTL        = 1 * time.Hour
)

// PromptCache is an in-memory LRU-like prompt cache.
// No file I/O — pure memory. SafeMap for concurrent access.
type PromptCache struct {
	entries   *actor.SafeMap[string, *cacheEntry]
	maxSize   int
	stopCh    chan struct{}
	hits      atomic.Int64
	misses    atomic.Int64
}

type cacheEntry struct {
	Response  *kernel.LLMResponse `json:"response"`
	CreatedAt time.Time           `json:"created_at"`
}

// NewPromptCache creates an in-memory cache. dataDir is ignored (kept for API compat).
func NewPromptCache(dataDir string) *PromptCache {
	c := &PromptCache{
		entries: actor.NewSafeMap[string, *cacheEntry](64),
		maxSize: defaultMaxEntries,
		stopCh:  make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Shutdown stops the cleanup goroutine.
func (c *PromptCache) Shutdown() { close(c.stopCh) }

// key generates a cache key from messages + tools + model hash.
func (c *PromptCache) key(messages []kernel.Message, tools []kernel.ToolDefinition, model string) string {
	h := sha256.New()
	data, _ := json.Marshal(messages)
	h.Write(data)
	data, _ = json.Marshal(tools)
	h.Write(data)
	h.Write([]byte(model))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Get retrieves a cached response. Returns nil on miss or expiry.
func (c *PromptCache) Get(messages []kernel.Message, tools []kernel.ToolDefinition, model string) *kernel.LLMResponse {
	e, ok := c.entries.Load(c.key(messages, tools, model))
	if !ok || time.Since(e.CreatedAt) > defaultTTL {
		c.misses.Add(1)
		return nil
	}
	c.hits.Add(1)
	return e.Response
}

// Set stores a response in the cache. Evicts oldest entries if over limit.
func (c *PromptCache) Set(messages []kernel.Message, tools []kernel.ToolDefinition, model string, resp *kernel.LLMResponse) {
	k := c.key(messages, tools, model)
	c.entries.Store(k, &cacheEntry{Response: resp, CreatedAt: time.Now()})
	// Evict if over limit — delete oldest 10%
	if c.entries.Len() > c.maxSize {
		toRemove := c.maxSize / 10
		var oldest []string
		cutoff := time.Now().Add(-defaultTTL)
		c.entries.Range(func(key string, e *cacheEntry) bool {
			if e.CreatedAt.Before(cutoff) {
				oldest = append(oldest, key)
			}
			return true
		})
		for i, key := range oldest {
			if i >= toRemove {
				break
			}
			c.entries.Delete(key)
		}
	}
}

// Stats returns cache statistics.
func (c *PromptCache) Stats() map[string]int {
	return map[string]int{
		"entries": c.entries.Len(),
		"hits":    int(c.hits.Load()),
		"misses":  int(c.misses.Load()),
	}
}

func (c *PromptCache) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup(2 * time.Hour)
		}
	}
}

func (c *PromptCache) cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	c.entries.Range(func(k string, e *cacheEntry) bool {
		if e.CreatedAt.Before(cutoff) {
			c.entries.Delete(k)
		}
		return true
	})
}
