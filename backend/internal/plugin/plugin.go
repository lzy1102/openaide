package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

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
	plugins      *kernel.SafeMap[string, *Plugin]
	dir          string
	onLoad       atomic.Value // []func(*Plugin)
	messageHooks atomic.Value // []MessageHook
	eventHooks   atomic.Value // []EventHook
}

// NewManager 创建插件管理器
func NewManager(dir string) *Manager {
	os.MkdirAll(dir, 0755)
	m := &Manager{
		dir:     dir,
		plugins: kernel.NewSafeMap[string, *Plugin](8),
	}
	m.onLoad.Store([]func(*Plugin){})
	m.messageHooks.Store([]MessageHook{})
	m.eventHooks.Store([]EventHook{})
	m.loadFromDisk()
	m.loadClaudeFromDisk()
	return m
}

// loadClaudeFromDisk 发现 Claude 规范插件
func (m *Manager) loadClaudeFromDisk() {
	claudePlugins := DiscoverClaudePlugins(m.dir)
	for _, p := range claudePlugins {
		if _, exists := m.plugins.Load(p.ID); !exists {
			m.plugins.Store(p.ID, p)
		}
	}
}

// OnLoad 注册加载回调
func (m *Manager) OnLoad(fn func(*Plugin)) {
	old := m.onLoad.Load().([]func(*Plugin))
	newSlice := make([]func(*Plugin), len(old)+1)
	copy(newSlice, old)
	newSlice[len(old)] = fn
	m.onLoad.Store(newSlice)
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

	m.plugins.Store(p.ID, &p)
	for _, fn := range m.onLoad.Load().([]func(*Plugin)) {
		fn(&p)
	}
	return m.save(p.ID, &p)
}

// Uninstall 卸载插件
func (m *Manager) Uninstall(id string) error {
	m.plugins.Delete(id)
	return os.Remove(filepath.Join(m.dir, id+".json"))
}

// Enable 启用插件
func (m *Manager) Enable(id string) error {
	p, ok := m.plugins.Load(id)
	if !ok { return fmt.Errorf("plugin not found: %s", id) }
	p.Enabled = true
	return m.save(id, p)
}

// Disable 禁用插件
func (m *Manager) Disable(id string) error {
	p, ok := m.plugins.Load(id)
	if !ok { return fmt.Errorf("plugin not found: %s", id) }
	p.Enabled = false
	return m.save(id, p)
}

// List 列出所有插件
func (m *Manager) List() []*Plugin {
	var result []*Plugin
	m.plugins.Range(func(_ string, p *Plugin) bool {
		result = append(result, p)
		return true
	})
	return result
}

// Reload rescans the plugins directory and picks up newly added plugins.
// Existing plugins are not modified. Returns IDs of newly loaded plugins.
func (m *Manager) Reload() []string {
	prev := make(map[string]bool)
	m.plugins.Range(func(id string, _ *Plugin) bool { prev[id] = true; return true })

	m.loadFromDisk()
	m.loadClaudeFromDisk()

	var newIDs []string
	m.plugins.Range(func(id string, p *Plugin) bool {
		if !prev[id] {
			newIDs = append(newIDs, id)
			for _, fn := range m.onLoad.Load().([]func(*Plugin)) {
				fn(p)
			}
		}
		return true
	})
	if len(newIDs) > 0 {
		var names []string
		for _, id := range newIDs {
			names = append(names, id)
		}
	}
	return newIDs
}

// OnMessage 注册消息钩子
func (m *Manager) OnMessage(hook MessageHook) {
	old := m.messageHooks.Load().([]MessageHook)
	newSlice := make([]MessageHook, len(old)+1)
	copy(newSlice, old)
	newSlice[len(old)] = hook
	m.messageHooks.Store(newSlice)
}

// OnEvent 注册事件钩子
func (m *Manager) OnEvent(hook EventHook) {
	old := m.eventHooks.Load().([]EventHook)
	newSlice := make([]EventHook, len(old)+1)
	copy(newSlice, old)
	newSlice[len(old)] = hook
	m.eventHooks.Store(newSlice)
}

// RunMessageHooks 依次执行所有消息钩子
// 任一钩子返回 error 将中止后续钩子并返回错误
func (m *Manager) RunMessageHooks(ctx context.Context, msg *kernel.Message) (*kernel.Message, error) {
	hooks := m.messageHooks.Load().([]MessageHook)

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
	hooks := m.eventHooks.Load().([]EventHook)

	for _, hook := range hooks {
		hook(ctx, event)
	}
}

// GetPrompt 获取所有启用插件的提示词注入
func (m *Manager) GetPrompt() string {
	var parts []string
	m.plugins.Range(func(_ string, p *Plugin) bool {
		if p.Enabled && p.SystemPrompt != "" {
			parts = append(parts, fmt.Sprintf("## 插件: %s\n%s", p.Name, p.SystemPrompt))
		}
		return true
	})
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
			m.plugins.Store(p.ID, &p)
			for _, fn := range m.onLoad.Load().([]func(*Plugin)) {
				fn(&p)
			}
		}
	}
}
