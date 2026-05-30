package event

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
)

// Bus 事件总线
type Bus struct {
	handlers *kernel.SafeMap[string, []kernel.EventHandler]

	persistEnabled bool
	dataDir        string
	events         []kernel.Event
	eventsMu       sync.Mutex // write-heavy, keep as mutex
}

// NewBus 创建事件总线
func NewBus() *Bus {
	return &Bus{
		handlers: kernel.NewSafeMap[string, []kernel.EventHandler](8),
	}
}

// EnablePersistence 启用持久化
func (b *Bus) EnablePersistence(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	b.persistEnabled = true
	b.dataDir = dataDir
	return b.loadEvents()
}

const maxEvents = 10000

// Publish 发布事件
func (b *Bus) Publish(event kernel.Event) {
	if b.persistEnabled {
		b.eventsMu.Lock()
		b.events = append(b.events, event)
		if len(b.events) > maxEvents {
			b.events = b.events[len(b.events)-maxEvents:]
		}
		b.eventsMu.Unlock()
		go b.persistEvent(event)
	}
	// Dispatch
	if handlers, ok := b.handlers.Load(event.Type); ok {
		for _, h := range handlers { go h.HandleEvent(event) }
	}
	if wildcard, ok := b.handlers.Load("*"); ok {
		for _, h := range wildcard { go h.HandleEvent(event) }
	}
}

// Subscribe 订阅事件
func (b *Bus) Subscribe(eventType string, handler kernel.EventHandler) {
	existing, _ := b.handlers.Load(eventType)
	b.handlers.Store(eventType, append(existing, handler))
}

// Unsubscribe 取消订阅
func (b *Bus) Unsubscribe(eventType string, handler kernel.EventHandler) {
	handlers, ok := b.handlers.Load(eventType)
	if !ok { return }
	for i, h := range handlers {
		if h == handler {
			newHandlers := make([]kernel.EventHandler, len(handlers)-1)
			copy(newHandlers, handlers[:i])
			copy(newHandlers[i:], handlers[i+1:])
			b.handlers.Store(eventType, newHandlers)
			return
		}
	}
}

// GetEvents 获取事件历史
func (b *Bus) GetEvents(eventType string, limit int) []kernel.Event {
	b.eventsMu.Lock()
	defer b.eventsMu.Unlock()
	var result []kernel.Event
	for i := len(b.events) - 1; i >= 0 && len(result) < limit; i-- {
		if eventType == "" || b.events[i].Type == eventType {
			result = append(result, b.events[i])
		}
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// Replay 回放事件
func (b *Bus) Replay(ctx context.Context, from time.Time, to time.Time, handler kernel.EventHandler) error {
	b.eventsMu.Lock()
	defer b.eventsMu.Unlock()
	for _, event := range b.events {
		if event.Timestamp.After(from) && event.Timestamp.Before(to) {
			select {
			case <-ctx.Done(): return ctx.Err()
			default: handler.HandleEvent(event)
			}
		}
	}
	return nil
}

// ============ 内部方法 ============

func (b *Bus) persistEvent(event kernel.Event) {
	path := filepath.Join(b.dataDir, fmt.Sprintf("event_%d.json", time.Now().UnixNano()))
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)

	// Rotate: keep only the last 1000 event files
	entries, _ := os.ReadDir(b.dataDir)
	if len(entries) > 1100 {
		// Sort by name (which is time-sorted since names are timestamps)
		cutoff := len(entries) - 1000
		for _, e := range entries[:cutoff] {
			if strings.HasSuffix(e.Name(), ".json") {
				os.Remove(filepath.Join(b.dataDir, e.Name()))
			}
		}
	}
}

func (b *Bus) loadEvents() error {
	entries, err := os.ReadDir(b.dataDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(b.dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var event kernel.Event
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}

			b.events = append(b.events, event)
	}

	return nil
}

// FilteredHandler 过滤事件处理器
type FilteredHandler struct {
	filter   func(kernel.Event) bool
	handler  kernel.EventHandler
}

// NewFilteredHandler 创建过滤处理器
func NewFilteredHandler(filter func(kernel.Event) bool, handler kernel.EventHandler) *FilteredHandler {
	return &FilteredHandler{filter: filter, handler: handler}
}

// HandleEvent 处理事件（带过滤）
func (h *FilteredHandler) HandleEvent(event kernel.Event) {
	if h.filter(event) {
		h.handler.HandleEvent(event)
	}
}
