package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Plugin 可插拔扩展
type Plugin struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Enabled     bool   `json:"enabled"`

	// 插件提供的额外工具
	Tools []ToolDef `json:"tools,omitempty"`

	// 插件注入的提示词
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// ToolDef 插件工具定义
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Manager 插件管理器
type Manager struct {
	mu      sync.RWMutex
	dir     string
	plugins map[string]*Plugin
	onLoad  []func(*Plugin)
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

// GetTools 获取所有启用插件的工具
func (m *Manager) GetTools() []ToolDef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var tools []ToolDef
	for _, p := range m.plugins {
		if p.Enabled {
			tools = append(tools, p.Tools...)
		}
	}
	return tools
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
