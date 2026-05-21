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
	handlers map[string][]kernel.EventHandler
	mu       sync.RWMutex

	// 持久化
	persistEnabled bool
	dataDir        string
	events         []kernel.Event
	eventsMu       sync.RWMutex
}

// NewBus 创建事件总线
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[string][]kernel.EventHandler),
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

// Publish 发布事件
func (b *Bus) Publish(event kernel.Event) {
	// 持久化
	if b.persistEnabled {
		b.eventsMu.Lock()
		b.events = append(b.events, event)
		b.eventsMu.Unlock()
		go b.persistEvent(event)
	}

	// 分发
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	b.mu.RUnlock()

	for _, handler := range handlers {
		go handler.HandleEvent(event)
	}

	// 通配符处理器
	b.mu.RLock()
	wildcard := b.handlers["*"]
	b.mu.RUnlock()

	for _, handler := range wildcard {
		go handler.HandleEvent(event)
	}
}

// Subscribe 订阅事件
func (b *Bus) Subscribe(eventType string, handler kernel.EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Unsubscribe 取消订阅
func (b *Bus) Unsubscribe(eventType string, handler kernel.EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.handlers[eventType]
	for i, h := range handlers {
		if h == handler {
			b.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

// GetEvents 获取事件历史
func (b *Bus) GetEvents(eventType string, limit int) []kernel.Event {
	b.eventsMu.RLock()
	defer b.eventsMu.RUnlock()

	var result []kernel.Event
	for i := len(b.events) - 1; i >= 0 && len(result) < limit; i-- {
		if eventType == "" || b.events[i].Type == eventType {
			result = append(result, b.events[i])
		}
	}

	// 反转顺序
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result
}

// Replay 回放事件
func (b *Bus) Replay(ctx context.Context, from time.Time, to time.Time, handler kernel.EventHandler) error {
	b.eventsMu.RLock()
	defer b.eventsMu.RUnlock()

	for _, event := range b.events {
		if event.Timestamp.After(from) && event.Timestamp.Before(to) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				handler.HandleEvent(event)
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
