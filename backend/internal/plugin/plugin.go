package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"openaide/backend/internal/kernel"
)

// Plugin 可插拔扩展
type Plugin struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Enabled     bool   `json:"enabled"`

	// 插件注入的提示词
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// MessageHook 消息拦截钩子
// 在渠道消息进入Kernel之前调用，可修改或拦截消息
type MessageHook func(ctx context.Context, msg *kernel.Message) (*kernel.Message, error)

// EventHook 事件钩子
// Kernel事件发布时调用（如 toolcall, thinking 等）
type EventHook func(ctx context.Context, event kernel.Event)

// Manager 插件管理器
type Manager struct {
	mu          sync.RWMutex
	dir         string
	plugins     map[string]*Plugin
	onLoad      []func(*Plugin)
	messageHooks []MessageHook
	eventHooks   []EventHook
}

// NewManager 创建插件管理器
func NewManager(dir string) *Manager {
	os.MkdirAll(dir, 0755)
	m := &Manager{
		dir:     dir,
		plugins: make(map[string]*Plugin),
	}
	m.loadFromDisk()
	return m
}

// OnLoad 注册加载回调
func (m *Manager) OnLoad(fn func(*Plugin)) {
	m.onLoad = append(m.onLoad, fn)
}

// Install 安装插件
func (m *Manager) Install(manifestJSON string) error {
	var p Plugin
	if err := json.Unmarshal([]byte(manifestJSON), &p); err != nil {
		return err
	}
	if p.ID == "" {
		return fmt.Errorf("plugin id required")
	}
	p.Enabled = true

	m.mu.Lock()
	m.plugins[p.ID] = &p
	m.mu.Unlock()

	return m.save(p.ID, &p)
}

// Uninstall 卸载插件
func (m *Manager) Uninstall(id string) error {
	m.mu.Lock()
	delete(m.plugins, id)
	m.mu.Unlock()
	return os.Remove(filepath.Join(m.dir, id+".json"))
}

// Enable 启用插件
func (m *Manager) Enable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.plugins[id]; ok {
		p.Enabled = true
		return m.save(id, p)
	}
	return fmt.Errorf("plugin not found: %s", id)
}

// Disable 禁用插件
func (m *Manager) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.plugins[id]; ok {
		p.Enabled = false
		return m.save(id, p)
	}
	return fmt.Errorf("plugin not found: %s", id)
}

// List 列出所有插件
func (m *Manager) List() []*Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Plugin
	for _, p := range m.plugins {
		result = append(result, p)
	}
	return result
}

// OnMessage 注册消息钩子
func (m *Manager) OnMessage(hook MessageHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messageHooks = append(m.messageHooks, hook)
}

// OnEvent 注册事件钩子
func (m *Manager) OnEvent(hook EventHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventHooks = append(m.eventHooks, hook)
}

// RunMessageHooks 依次执行所有消息钩子
// 任一钩子返回 error 将中止后续钩子并返回错误
func (m *Manager) RunMessageHooks(ctx context.Context, msg *kernel.Message) (*kernel.Message, error) {
	m.mu.RLock()
	hooks := make([]MessageHook, len(m.messageHooks))
	copy(hooks, m.messageHooks)
	m.mu.RUnlock()

	current := msg
	for _, hook := range hooks {
		var err error
		current, err = hook(ctx, current)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, fmt.Errorf("message hook returned nil, message intercepted")
		}
	}
	return current, nil
}

// RunEventHooks 依次执行所有事件钩子
func (m *Manager) RunEventHooks(ctx context.Context, event kernel.Event) {
	m.mu.RLock()
	hooks := make([]EventHook, len(m.eventHooks))
	copy(hooks, m.eventHooks)
	m.mu.RUnlock()

	for _, hook := range hooks {
		hook(ctx, event)
	}
}

// GetPrompt 获取所有启用插件的提示词注入
func (m *Manager) GetPrompt() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var parts []string
	for _, p := range m.plugins {
		if p.Enabled && p.SystemPrompt != "" {
			parts = append(parts, fmt.Sprintf("## 插件: %s\n%s", p.Name, p.SystemPrompt))
		}
	}
	return strings.Join(parts, "\n\n")
}

func (m *Manager) save(id string, p *Plugin) error {
	data, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(filepath.Join(m.dir, id+".json"), data, 0644)
}

func (m *Manager) loadFromDisk() {
	entries, _ := os.ReadDir(m.dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(m.dir, e.Name()))
		var p Plugin
		if json.Unmarshal(data, &p) == nil {
			m.plugins[p.ID] = &p
			for _, fn := range m.onLoad {
				fn(&p)
			}
		}
	}
}
