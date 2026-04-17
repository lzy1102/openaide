package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MCPServerConfig MCP Server 配置
type MCPServerConfig struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name"`
	Transport string    `json:"transport"` // stdio, sse
	Command   string    `json:"command"`
	Args      string    `json:"args"` // JSON array string
	URL       string    `json:"url"`
	Env       string    `json:"env"` // JSON map string
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MCPToolInfo 聚合的 MCP 工具信息
type MCPToolInfo struct {
	ServerID    string                 `json:"server_id"`
	ServerName  string                 `json:"server_name"`
	ToolName    string                 `json:"tool_name"`
	FullName    string                 `json:"full_name"` // mcp:{serverID}:{toolName}
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// MCPManager MCP Server 管理器
type MCPManager struct {
	clients map[string]*MCPClient // ID → client
	mu      sync.RWMutex
	logger  Logger
	db      *gorm.DB
}

// NewMCPManager 创建 MCP 管理器
func NewMCPManager(db *gorm.DB, logger Logger) *MCPManager {
	m := &MCPManager{
		clients: make(map[string]*MCPClient),
		logger:   logger,
		db:       db,
	}

	// 自动迁移
	db.AutoMigrate(&MCPServerConfig{})

	// 启动时恢复已启用的 Server
	m.restoreServers()

	return m
}

// AddServer 添加并连接 MCP Server
func (m *MCPManager) AddServer(ctx context.Context, config MCPServerConfig) (*MCPClient, error) {
	if config.ID == "" {
		config.ID = uuid.New().String()
	}
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	// 解析参数和环境变量
	args := parseJSONArray(config.Args)
	env := parseJSONMap(config.Env)

	// 创建客户端
	client := NewMCPClient(config.ID, config.Name, config.Transport, config.Command, config.URL, args, env, m.logger)

	// 连接
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", config.Name, err)
	}

	// 初始化（能力协商）
	if err := client.Initialize(ctx); err != nil {
		client.Disconnect()
		return nil, fmt.Errorf("failed to initialize %s: %w", config.Name, err)
	}

	// 发现工具
	if _, err := client.ListTools(ctx); err != nil {
		m.logger.Warn(ctx, "[MCPManager] Failed to list tools for %s: %v", config.Name, err)
		// 不算致命错误，Server 可能没有工具
	}

	// 存入数据库
	if err := m.db.Save(&config).Error; err != nil {
		client.Disconnect()
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// 注册客户端
	m.mu.Lock()
	m.clients[config.ID] = client
	m.mu.Unlock()

	m.logger.Info(ctx, "[MCPManager] Server '%s' added with %d tools", config.Name, len(client.Tools))

	return client, nil
}

// RemoveServer 移除 MCP Server
func (m *MCPManager) RemoveServer(id string) error {
	m.mu.Lock()
	client, ok := m.clients[id]
	m.mu.Unlock()

	if ok {
		client.Disconnect()
	}

	if err := m.db.Delete(&MCPServerConfig{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}

	m.mu.Lock()
	delete(m.clients, id)
	m.mu.Unlock()

	return nil
}

// GetServer 获取 MCP 客户端
func (m *MCPManager) GetServer(id string) (*MCPClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[id]
	if !ok {
		return nil, fmt.Errorf("MCP server not found: %s", id)
	}
	return client, nil
}

// ListServers 列出所有 MCP Server 配置
func (m *MCPManager) ListServers() []MCPServerConfig {
	var configs []MCPServerConfig
	m.db.Order("created_at DESC").Find(&configs)
	return configs
}

// RefreshTools 重新发现指定 Server 的工具
func (m *MCPManager) RefreshTools(ctx context.Context, serverID string) ([]MCPTool, error) {
	client, err := m.GetServer(serverID)
	if err != nil {
		return nil, err
	}

	return client.ListTools(ctx)
}

// Reconnect 重新连接指定 Server
func (m *MCPManager) Reconnect(ctx context.Context, serverID string) error {
	m.mu.Lock()
	client, ok := m.clients[serverID]
	m.mu.Unlock()

	if ok {
		client.Disconnect()
	}

	var config MCPServerConfig
	if err := m.db.First(&config, "id = ?", serverID).Error; err != nil {
		return fmt.Errorf("server config not found: %w", err)
	}

	newClient, err := m.AddServer(ctx, config)
	if err != nil {
		return err
	}

	m.logger.Info(ctx, "[MCPManager] Server '%s' reconnected with %d tools", newClient.Name, len(newClient.Tools))
	return nil
}

// GetAllTools 聚合所有 MCP Server 的工具
func (m *MCPManager) GetAllTools() []MCPToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []MCPToolInfo

	for id, client := range m.clients {
		client.mu.RLock()
		for _, t := range client.Tools {
			tools = append(tools, MCPToolInfo{
				ServerID:    id,
				ServerName:  client.Name,
				ToolName:    t.Name,
				FullName:    fmt.Sprintf("mcp:%s:%s", id, t.Name),
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
		client.mu.RUnlock()
	}

	return tools
}

// FindTool 根据 FullName 查找工具并返回对应的 MCPClient 和工具名
func (m *MCPManager) FindTool(fullName string) (*MCPClient, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, client := range m.clients {
		prefix := fmt.Sprintf("mcp:%s:", id)
		if strings.HasPrefix(fullName, prefix) {
			toolName := strings.TrimPrefix(fullName, prefix)
			return client, toolName, nil
		}
	}

	return nil, "", fmt.Errorf("MCP tool not found: %s", fullName)
}

// restoreServers 启动时恢复已启用的 Server
func (m *MCPManager) restoreServers() {
	var configs []MCPServerConfig
	if err := m.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		return
	}

	ctx := context.Background()

	for _, config := range configs {
		args := parseJSONArray(config.Args)
		env := parseJSONMap(config.Env)

		client := NewMCPClient(config.ID, config.Name, config.Transport, config.Command, config.URL, args, env, m.logger)

		if err := client.Connect(ctx); err != nil {
			m.logger.Warn(ctx, "[MCPManager] Failed to restore server '%s': %v", config.Name, err)
			continue
		}

		if err := client.Initialize(ctx); err != nil {
			m.logger.Warn(ctx, "[MCPManager] Failed to initialize server '%s': %v", config.Name, err)
			client.Disconnect()
			continue
		}

		if _, err := client.ListTools(ctx); err != nil {
			m.logger.Warn(ctx, "[MCPManager] Failed to list tools for '%s': %v", config.Name, err)
		}

		m.mu.Lock()
		m.clients[config.ID] = client
		m.mu.Unlock()

		m.logger.Info(ctx, "[MCPManager] Restored server '%s' with %d tools", config.Name, len(client.Tools))
	}
}

// parseJSONArray 解析 JSON 数组字符串
func parseJSONArray(s string) []string {
	if s == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil
	}
	return arr
}

// parseJSONMap 解析 JSON map 字符串
func parseJSONMap(s string) map[string]string {
	if s == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}
