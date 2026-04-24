package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/mcp"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ToolService 工具服务
type ToolService struct {
	db                   *gorm.DB
	cache                *CacheService
	logger               *LoggerService
	registry             *ToolRegistry
	httpClient           *http.Client
	pendingConfirmations map[string]*PendingCommandConfirmation
	confirmationMu       sync.RWMutex
	mcpManager           *mcp.MCPManager
}

// PendingCommandConfirmation 待用户确认的危险命令
type PendingCommandConfirmation struct {
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	Risk      string    `json:"risk"`
	TaskID    string    `json:"task_id"`
	CreatedAt time.Time `json:"created_at"`
	Response  chan bool `json:"-"`
}

// ConfirmationRequiredError 需要用户确认
type ConfirmationRequiredError struct {
	ID      string
	Command string
	Risk    string
}

func (e *ConfirmationRequiredError) Error() string {
	return fmt.Sprintf("confirmation required: %s (risk: %s)", e.Command, e.Risk)
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
	mu      sync.RWMutex
	tools   map[string]*models.Tool
	builtin map[string]ToolExecutor
	schemas map[string]map[string]interface{}
}

// ToolExecutor 工具执行器接口
type ToolExecutor interface {
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// SelfRegisteringTool 自注册工具接口（参考 Hermes Agent 的自注册模式）
// 每个工具同时提供 schema 和 handler，不再分散定义
type SelfRegisteringTool interface {
	ToolExecutor
	Definition() map[string]interface{}
	Name() string
}

// NewToolService 创建工具服务
func NewToolService(db *gorm.DB, cache *CacheService, logger *LoggerService, mcpManager *mcp.MCPManager) *ToolService {
	s := &ToolService{
		mcpManager: mcpManager,
		db:         db,
		cache:      cache,
		logger:     logger,
		registry: &ToolRegistry{
			tools:   make(map[string]*models.Tool),
			builtin: make(map[string]ToolExecutor),
			schemas: make(map[string]map[string]interface{}),
		},
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		pendingConfirmations: make(map[string]*PendingCommandConfirmation),
	}

	// 注册内置工具
	s.registerBuiltinTools()

	// 从数据库加载工具
	s.loadToolsFromDB()

	return s
}

// RegisterSelfRegisteringTool 注册外部自注册工具（用于需要依赖注入的工具如 TaskTool）
func (s *ToolService) RegisterSelfRegisteringTool(tool SelfRegisteringTool) {
	s.registry.mu.Lock()
	defer s.registry.mu.Unlock()

	name := tool.Name()
	s.registry.builtin[name] = tool
	s.registry.schemas[name] = tool.Definition()
}

// registerBuiltinTools 注册内置工具（自注册模式：每个工具定义自己的 schema 和 handler）
func (s *ToolService) registerBuiltinTools() {
	selfRegistering := []SelfRegisteringTool{
		&TimeTool{},
		&WeatherTool{},
		&WebSearchTool{},
		&CalculatorTool{},
		&CodeRunTool{},
		&FileReadTool{},
		&FileWriteTool{},
		&CommandTool{},
		&HTTPRequestTool{},
		&JSONParseTool{},
		&CodeFormatTool{},
		&LintTool{},
		&DatabaseTool{},
		&GitTool{},
		&FileSearchTool{},
		&DependencyTool{},
		&DockerTool{},
		&APITestTool{},
		&SystemMonitorTool{},
		&FileArchiveTool{},
		&NetworkDiagTool{},
		&RegexTool{},
	}

	for _, tool := range selfRegistering {
		name := tool.Name()
		s.registry.builtin[name] = tool
		s.registry.schemas[name] = tool.Definition()
	}
}

// loadToolsFromDB 从数据库加载工具
func (s *ToolService) loadToolsFromDB() {
	var tools []models.Tool
	if err := s.db.Where("enabled = ?", true).Find(&tools).Error; err != nil {
		s.logger.Error(context.Background(), "Failed to load tools from database: %v", err)
		return
	}

	s.registry.mu.Lock()
	defer s.registry.mu.Unlock()

	for _, tool := range tools {
		s.registry.tools[tool.Name] = &tool
	}

	s.logger.Info(context.Background(), "Loaded tools from database: count=%d", len(tools))
}

// RegisterTool 注册工具
func (s *ToolService) RegisterTool(tool *models.Tool) error {
	if err := s.db.Create(tool).Error; err != nil {
		return fmt.Errorf("failed to register tool: %w", err)
	}

	s.registry.mu.Lock()
	s.registry.tools[tool.Name] = tool
	s.registry.mu.Unlock()

	s.logger.Info(context.Background(), "Tool registered: %s", tool.Name)
	return nil
}

// GetTool 获取工具
func (s *ToolService) GetTool(name string) (*models.Tool, error) {
	// 先从缓存 map 读取
	s.registry.mu.RLock()
	tool, ok := s.registry.tools[name]
	s.registry.mu.RUnlock()

	if ok {
		return tool, nil
	}

	// 检查是否为内置工具
	s.registry.mu.RLock()
	_, isBuiltin := s.registry.builtin[name]
	s.registry.mu.RUnlock()

	if isBuiltin {
		return &models.Tool{
			ID:        "builtin_" + name,
			Name:      name,
			Type:      "builtin",
			Enabled:   true,
			CreatedAt: time.Now(),
		}, nil
	}

	// 检查是否为 MCP 工具
	if s.mcpManager != nil && strings.HasPrefix(name, "mcp:") {
		return s.getMCPToolModel(name)
	}

	// 从数据库查询
	var dbTool models.Tool
	if err := s.db.Where("name = ? AND enabled = ?", name, true).First(&dbTool).Error; err != nil {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	// 更新缓存
	s.registry.mu.Lock()
	s.registry.tools[name] = &dbTool
	s.registry.mu.Unlock()

	return &dbTool, nil
}

// ListTools 列出所有工具
func (s *ToolService) ListTools() ([]models.Tool, error) {
	var tools []models.Tool
	if err := s.db.Where("enabled = ?", true).Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

// ExecuteTool 执行工具
func (s *ToolService) ExecuteTool(ctx context.Context, toolCall *models.ToolCall, dialogueID, messageID, userID string) (*models.ToolResult, error) {
	tool, err := s.GetTool(toolCall.Name)
	if err != nil {
		return nil, err
	}

	// 解析参数
	var params models.JSONMap
	if toolCall.Arguments != "" {
		if err := json.Unmarshal([]byte(toolCall.Arguments), &params); err != nil {
			params = models.JSONMap{}
		}
	}

	// 创建执行记录
	execution := &models.ToolExecution{
		ID:         uuid.New().String(),
		ToolID:     tool.ID,
		ToolName:   tool.Name,
		DialogueID: dialogueID,
		MessageID:  messageID,
		UserID:     userID,
		Parameters: params,
		Status:     "pending",
	}
	if err := s.db.Create(execution).Error; err != nil {
		s.logger.Error(ctx, "Failed to create tool execution record: %v", err)
		// 继续执行，不因记录失败而中断
	}

	// 更新状态为运行中
	now := time.Now()
	execution.Status = "running"
	execution.StartedAt = &now
	if err := s.db.Save(execution).Error; err != nil {
		s.logger.Error(ctx, "Failed to update tool execution status: %v", err)
	}

	// 执行工具
	startTime := time.Now()
	result, execErr := s.execute(ctx, tool, params)
	duration := time.Since(startTime).Milliseconds()

	// 更新执行记录
	completedAt := time.Now()
	execution.CompletedAt = &completedAt
	execution.Duration = int(duration)

	if execErr != nil {
		execution.Status = "failed"
		execution.Error = execErr.Error()
		if err := s.db.Save(execution).Error; err != nil {
			s.logger.Error(ctx, "Failed to save failed tool execution: %v", err)
		}

		return &models.ToolResult{
			ToolCallID: toolCall.ID,
			Content:    execErr.Error(),
			IsError:    true,
		}, nil
	}

	execution.Status = "success"
	if resultMap, ok := result.(map[string]interface{}); ok {
		execution.Result = resultMap
	}
	if err := s.db.Save(execution).Error; err != nil {
		s.logger.Error(ctx, "Failed to save successful tool execution: %v", err)
	}

	return &models.ToolResult{
		ToolCallID: toolCall.ID,
		Content:    result,
		IsError:    false,
	}, nil
}

// execute 执行工具
func (s *ToolService) execute(ctx context.Context, tool *models.Tool, params map[string]interface{}) (interface{}, error) {
	switch tool.Type {
	case "builtin":
		executor, ok := s.registry.builtin[tool.Name]
		if !ok {
			return nil, fmt.Errorf("builtin tool not found: %s", tool.Name)
		}
		return executor.Execute(ctx, params)

	case "http":
		return s.executeHTTPTool(ctx, tool, params)

	case "script":
		return s.executeScriptTool(ctx, tool, params)

	case "mcp":
		return s.executeMCPTool(ctx, tool, params)

	default:
		return nil, fmt.Errorf("unsupported tool type: %s", tool.Type)
	}
}

// executeHTTPTool 执行 HTTP 工具
func (s *ToolService) executeHTTPTool(ctx context.Context, tool *models.Tool, params map[string]interface{}) (interface{}, error) {
	urlTemplate, ok := tool.ExecutorConfig["url_template"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid http tool config: missing url_template")
	}

	method, _ := tool.ExecutorConfig["method"].(string)
	if method == "" {
		method = "GET"
	}

	// 参数替换到 URL 模板
	url := urlTemplate
	for key, value := range params {
		placeholder := "{{" + key + "}}"
		url = strings.ReplaceAll(url, placeholder, fmt.Sprintf("%v", value))
	}

	// 构建请求
	var bodyReader io.Reader
	if method == "POST" || method == "PUT" {
		bodyJSON, _ := json.Marshal(params)
		bodyReader = bytes.NewReader(bodyJSON)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 注入请求头
	if headers, ok := tool.ExecutorConfig["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}
	req.Header.Set("Content-Type", "application/json")

	// 超时控制
	timeout, _ := tool.ExecutorConfig["timeout"].(float64)
	if timeout > 0 {
		client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
		return s.doHTTPRequest(client, req)
	}
	return s.doHTTPRequest(s.httpClient, req)
}

// doHTTPRequest 执行 HTTP 请求并返回结果
func (s *ToolService) doHTTPRequest(client *http.Client, req *http.Request) (interface{}, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 尝试解析为 JSON
	var jsonBody interface{}
	if jsonErr := json.Unmarshal(body, &jsonBody); jsonErr == nil {
		return map[string]interface{}{
			"status_code": resp.StatusCode,
			"headers":     resp.Header,
			"body":        jsonBody,
		}, nil
	}

	return map[string]interface{}{
		"status_code": resp.StatusCode,
		"headers":     resp.Header,
		"body":        string(body),
	}, nil
}

// executeScriptTool 执行脚本工具
func (s *ToolService) executeScriptTool(ctx context.Context, tool *models.Tool, params map[string]interface{}) (interface{}, error) {
	script, ok := tool.ExecutorConfig["script"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid script tool config")
	}

	// 参数替换
	for key, value := range params {
		placeholder := "{{" + key + "}}"
		script = strings.ReplaceAll(script, placeholder, fmt.Sprintf("%v", value))
	}

	// 安全检查：危险命令走确认流程，安全命令直接放行
	if err := s.checkScriptSafety(ctx, script, tool.ID); err != nil {
		return nil, err
	}

	// 执行脚本
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("script execution failed: %w, output: %s", err, string(output))
	}

	return map[string]interface{}{
		"output": string(output),
	}, nil
}

// checkScriptSafety 检查脚本安全性，危险命令走确认流程
func (s *ToolService) checkScriptSafety(ctx context.Context, script, toolID string) error {
	words := strings.Fields(script)
	if len(words) == 0 {
		return fmt.Errorf("empty script")
	}

	baseCmd := filepath.Base(words[0])
	if !isDangerousCommand(baseCmd) {
		return nil
	}

	// 检查是否已批准
	if ct, ok := s.registry.builtin["execute_command"].(*CommandTool); ok {
		ct.mu.RLock()
		allowed := ct.allowedDangerous != nil && ct.allowedDangerous[script]
		ct.mu.RUnlock()
		if allowed {
			return nil
		}
	}

	// 需要用户确认
	confErr := &ConfirmationRequiredError{
		ID:      uuid.New().String(),
		Command: script,
		Risk:    getCommandRisk(baseCmd),
	}

	approved, err := s.RequestCommandConfirmation(ctx, confErr, toolID)
	if err != nil {
		return fmt.Errorf("script confirmation failed: %w", err)
	}
	if !approved {
		return fmt.Errorf("script rejected by user: contains dangerous command %s", baseCmd)
	}

	return nil
}

// executeMCPTool 执行 MCP 工具
func (s *ToolService) executeMCPTool(ctx context.Context, tool *models.Tool, params map[string]interface{}) (interface{}, error) {
	if s.mcpManager == nil {
		return nil, fmt.Errorf("MCP manager not initialized")
	}

	client, toolName, err := s.mcpManager.FindTool(tool.Name)
	if err != nil {
		return nil, err
	}

	result, err := client.CallTool(ctx, toolName, params)
	if err != nil {
		return nil, err
	}

	// 转换结果为统一格式
	var output string
	for _, content := range result.Content {
		if content.Text != "" {
			output += content.Text
		}
	}
	if result.IsError {
		return map[string]interface{}{
			"output": output,
			"error":  true,
		}, nil
	}

	return map[string]interface{}{
		"output":  output,
		"success": true,
	}, nil
}

// getMCPToolModel 将 MCP 工具信息转换为 models.Tool 格式
func (s *ToolService) getMCPToolModel(fullName string) (*models.Tool, error) {
	tools := s.mcpManager.GetAllTools()
	for _, t := range tools {
		if t.FullName == fullName {
			return &models.Tool{
				ID:               "mcp_" + t.ServerID,
				Name:             t.FullName,
				Type:             "mcp",
				Description:      t.Description,
				ParametersSchema: models.JSONMap(t.InputSchema),
				Enabled:          true,
				CreatedAt:        time.Now(),
			}, nil
		}
	}
	return nil, fmt.Errorf("MCP tool not found: %s", fullName)
}

// GetToolDefinitions 获取工具定义列表 (供 LLM 使用)
func (s *ToolService) GetToolDefinitions() []map[string]interface{} {
	s.registry.mu.RLock()
	defer s.registry.mu.RUnlock()

	definitions := make([]map[string]interface{}, 0, len(s.registry.tools)+len(s.registry.builtin))

	// 添加数据库工具
	for _, tool := range s.registry.tools {
		definitions = append(definitions, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.ParametersSchema,
			},
		})
	}

	// 添加内置工具定义（使用自注册的 schema）
	for name := range s.registry.builtin {
		if schema, ok := s.registry.schemas[name]; ok {
			definitions = append(definitions, schema)
		}
	}

	return definitions
}

// GetToolDefinitionsWithMCP 获取包含 MCP 工具的所有定义
func (s *ToolService) GetToolDefinitionsWithMCP() []map[string]interface{} {
	definitions := s.GetToolDefinitions()

	// 追加 MCP 工具
	if s.mcpManager != nil {
		for _, tool := range s.mcpManager.GetAllTools() {
			definitions = append(definitions, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        tool.FullName,
					"description": tool.Description,
					"parameters":  tool.InputSchema,
				},
			})
		}
	}

	return definitions
}

// GetToolDefinitionsByNames 按名称列表获取工具定义（用于技能绑定）
// 如果 names 为空，返回所有工具定义
func (s *ToolService) GetToolDefinitionsByNames(names []string) []map[string]interface{} {
	if len(names) == 0 {
		return s.GetToolDefinitions()
	}

	allDefs := s.GetToolDefinitions()
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	var filtered []map[string]interface{}
	for _, def := range allDefs {
		fnMap, _ := def["function"].(map[string]interface{})
		if fnMap == nil {
			continue
		}
		name, _ := fnMap["name"].(string)
		if nameSet[name] {
			filtered = append(filtered, def)
		}
	}

	return filtered
}

// GetToolDefinitionsWithMCPByNames 按名称列表获取包含 MCP 工具的定义
func (s *ToolService) GetToolDefinitionsWithMCPByNames(names []string) []map[string]interface{} {
	if len(names) == 0 {
		return s.GetToolDefinitionsWithMCP()
	}

	definitions := s.GetToolDefinitionsByNames(names)

	// 追加匹配的 MCP 工具
	if s.mcpManager != nil {
		nameSet := make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[n] = true
		}
		for _, tool := range s.mcpManager.GetAllTools() {
			if nameSet[tool.FullName] || nameSet[tool.ToolName] {
				definitions = append(definitions, map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name":        tool.FullName,
						"description": tool.Description,
						"parameters":  tool.InputSchema,
					},
				})
			}
		}
	}

	return definitions
}

// ParseToolCalls 从 LLM 响应解析工具调用
func (s *ToolService) ParseToolCalls(response map[string]interface{}) ([]models.ToolCall, error) {
	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, nil
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	toolCalls, ok := message["tool_calls"].([]interface{})
	if !ok {
		return nil, nil
	}

	var result []models.ToolCall
	for _, tc := range toolCalls {
		tcMap, ok := tc.(map[string]interface{})
		if !ok {
			continue
		}

		function, ok := tcMap["function"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := function["name"].(string)
		argsStr, _ := function["arguments"].(string)

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			args = make(map[string]interface{})
		}

		id, _ := tcMap["id"].(string)
		if id == "" {
			id = uuid.New().String()
		}

		// 将 args 序列化为 JSON 字符串
		argsJSON, _ := json.Marshal(args)

		result = append(result, models.ToolCall{
			ID:        id,
			Name:      name,
			Arguments: string(argsJSON),
		})
	}

	return result, nil
}

// ==================== 危险命令确认管理 ====================

// RequestCommandConfirmation 请求用户确认危险命令（阻塞等待响应）
func (s *ToolService) RequestCommandConfirmation(ctx context.Context, confReq *ConfirmationRequiredError, taskID string) (bool, error) {
	id := confReq.ID
	pending := &PendingCommandConfirmation{
		ID:        id,
		Command:   confReq.Command,
		Risk:      confReq.Risk,
		TaskID:    taskID,
		CreatedAt: time.Now(),
		Response:  make(chan bool, 1),
	}

	s.confirmationMu.Lock()
	s.pendingConfirmations[id] = pending
	s.confirmationMu.Unlock()

	s.logger.Info(ctx, "[ToolService] Dangerous command pending confirmation: %s (id=%s)", confReq.Command, id)

	select {
	case approved := <-pending.Response:
		s.confirmationMu.Lock()
		delete(s.pendingConfirmations, id)
		s.confirmationMu.Unlock()
		return approved, nil
	case <-ctx.Done():
		s.confirmationMu.Lock()
		delete(s.pendingConfirmations, id)
		s.confirmationMu.Unlock()
		return false, fmt.Errorf("confirmation cancelled: context done")
	case <-time.After(5 * time.Minute):
		s.confirmationMu.Lock()
		delete(s.pendingConfirmations, id)
		s.confirmationMu.Unlock()
		return false, fmt.Errorf("confirmation timeout: no response within 5 minutes")
	}
}

// ApproveCommand 确认批准危险命令
func (s *ToolService) ApproveCommand(id string) error {
	s.confirmationMu.RLock()
	pending, ok := s.pendingConfirmations[id]
	s.confirmationMu.RUnlock()

	if !ok {
		return fmt.Errorf("pending confirmation not found: %s", id)
	}

	// 将命令加入已批准列表
	if ct, ok := s.registry.builtin["execute_command"].(*CommandTool); ok {
		ct.AllowCommand(pending.Command)
	}

	pending.Response <- true
	return nil
}

// RejectCommand 拒绝危险命令
func (s *ToolService) RejectCommand(id string) error {
	s.confirmationMu.RLock()
	pending, ok := s.pendingConfirmations[id]
	s.confirmationMu.RUnlock()

	if !ok {
		return fmt.Errorf("pending confirmation not found: %s", id)
	}

	pending.Response <- false
	return nil
}

// ListPendingConfirmations 列出待确认的危险命令
func (s *ToolService) ListPendingConfirmations() []PendingCommandConfirmation {
	s.confirmationMu.RLock()
	defer s.confirmationMu.RUnlock()

	result := make([]PendingCommandConfirmation, 0, len(s.pendingConfirmations))
	for _, p := range s.pendingConfirmations {
		result = append(result, PendingCommandConfirmation{
			ID:        p.ID,
			Command:   p.Command,
			Risk:      p.Risk,
			TaskID:    p.TaskID,
			CreatedAt: p.CreatedAt,
		})
	}
	return result
}

// ========================================
// 内置工具实现
// ========================================

// TimeTool 时间工具
type TimeTool struct{}

func (t *TimeTool) Name() string { return "get_current_time" }

func (t *TimeTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "get_current_time",
			"description": "获取当前日期和时间",
			"parameters": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
}

func (t *TimeTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	now := time.Now()
	return map[string]interface{}{
		"timestamp": now.Unix(),
		"datetime":  now.Format("2006-01-02 15:04:05"),
		"date":      now.Format("2006-01-02"),
		"time":      now.Format("15:04:05"),
		"weekday":   now.Weekday().String(),
		"timezone":  now.Location().String(),
	}, nil
}

// WeatherTool 天气工具
type WeatherTool struct{}

func (t *WeatherTool) Name() string { return "get_weather" }

func (t *WeatherTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "get_weather",
			"description": "获取指定城市的天气信息",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"city": map[string]interface{}{"type": "string", "description": "城市名称"},
				},
				"required": []string{"city"},
			},
		},
	}
}

func (t *WeatherTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	city, _ := params["city"].(string)
	if city == "" {
		return nil, fmt.Errorf("city parameter is required")
	}

	// 使用 wttr.in 免费天气 API（无需 API Key）
	apiURL := fmt.Sprintf("https://wttr.in/%s?format=j1", url.QueryEscape(city))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return t.fallbackWeather(city), nil
	}
	req.Header.Set("User-Agent", "curl/7.68.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return t.fallbackWeather(city), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return t.fallbackWeather(city), nil
	}

	var data struct {
		CurrentCondition []struct {
			TempC       string `json:"temp_C"`
			TempF       string `json:"temp_F"`
			Humidity    string `json:"humidity"`
			WeatherDesc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
			WindspeedKmph   string `json:"windspeedKmph"`
			Winddir16Point  string `json:"winddir16Point"`
			FeelsLikeC      string `json:"FeelsLikeC"`
			ObservationTime string `json:"observation_time"`
		} `json:"current_condition"`
		NearestArea []struct {
			Region []struct {
				Value string `json:"value"`
			} `json:"region"`
			Country []struct {
				Value string `json:"value"`
			} `json:"country"`
		} `json:"nearest_area"`
	}
	if err := json.Unmarshal(body, &data); err != nil || len(data.CurrentCondition) == 0 {
		return t.fallbackWeather(city), nil
	}

	cc := data.CurrentCondition[0]
	weather := ""
	if len(cc.WeatherDesc) > 0 {
		weather = cc.WeatherDesc[0].Value
	}
	region := ""
	if len(data.NearestArea) > 0 && len(data.NearestArea[0].Region) > 0 {
		region = data.NearestArea[0].Region[0].Value
	}

	return map[string]interface{}{
		"city":        city,
		"region":      region,
		"temperature": cc.TempC + "°C",
		"feels_like":  cc.FeelsLikeC + "°C",
		"weather":     weather,
		"humidity":    cc.Humidity + "%",
		"wind":        cc.Winddir16Point + " " + cc.WindspeedKmph + "km/h",
		"observed_at": cc.ObservationTime,
	}, nil
}

// fallbackWeather API 不可用时的降级返回
func (t *WeatherTool) fallbackWeather(city string) map[string]interface{} {
	return map[string]interface{}{
		"city":        city,
		"temperature": "N/A",
		"weather":     "天气服务暂不可用",
		"note":        "无法连接天气 API，请检查网络或稍后重试",
	}
}

// WebSearchTool 网络搜索工具
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string { return "search_web" }

func (t *WebSearchTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "search_web",
			"description": "在网络上搜索信息",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "搜索关键词"},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query parameter is required")
	}

	maxResults := 5
	if n, ok := params["max_results"].(float64); ok && int(n) > 0 && int(n) <= 20 {
		maxResults = int(n)
	}

	// 使用 DuckDuckGo Instant Answer API（无需 API Key）
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read search response: %w", err)
	}

	var data struct {
		AbstractText   string `json:"AbstractText"`
		AbstractSource string `json:"AbstractSource"`
		AbstractURL    string `json:"AbstractURL"`
		Heading        string `json:"Heading"`
		RelatedTopics  []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	results := []map[string]interface{}{}

	// 添加摘要结果
	if data.AbstractText != "" {
		results = append(results, map[string]interface{}{
			"title":   data.Heading,
			"url":     data.AbstractURL,
			"snippet": data.AbstractText,
			"source":  data.AbstractSource,
		})
	}

	// 添加相关主题
	for _, topic := range data.RelatedTopics {
		if topic.Text == "" {
			continue
		}
		results = append(results, map[string]interface{}{
			"title":   data.Heading,
			"url":     topic.FirstURL,
			"snippet": topic.Text,
		})
		if len(results) >= maxResults {
			break
		}
	}

	if len(results) == 0 {
		return map[string]interface{}{
			"query":   query,
			"results": []interface{}{},
			"note":    "未找到相关结果，请尝试更具体的关键词",
		}, nil
	}

	return map[string]interface{}{
		"query":   query,
		"results": results,
		"count":   len(results),
	}, nil
}

// CalculatorTool 计算器工具
type CalculatorTool struct{}

func (t *CalculatorTool) Name() string { return "calculate" }

func (t *CalculatorTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "calculate",
			"description": "执行数学计算",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"expression": map[string]interface{}{"type": "string", "description": "数学表达式"},
				},
				"required": []string{"expression"},
			},
		},
	}
}

func (t *CalculatorTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	expression, _ := params["expression"].(string)
	if expression == "" {
		return nil, fmt.Errorf("expression parameter is required")
	}

	// 使用 expr 库或简单解析
	// 这里使用一个简化的实现
	result := "计算结果"
	return map[string]interface{}{
		"expression": expression,
		"result":     result,
		"note":       "请使用 govaluate 或其他表达式解析库实现",
	}, nil
}

// CodeRunTool 代码执行工具
type CodeRunTool struct{}

func (t *CodeRunTool) Name() string { return "run_code" }

func (t *CodeRunTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "run_code",
			"description": "执行代码并返回结果",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"language": map[string]interface{}{"type": "string", "description": "编程语言 (python, javascript, go)"},
					"code":     map[string]interface{}{"type": "string", "description": "要执行的代码"},
				},
				"required": []string{"language", "code"},
			},
		},
	}
}

func (t *CodeRunTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	language, _ := params["language"].(string)
	code, _ := params["code"].(string)

	if language == "" || code == "" {
		return nil, fmt.Errorf("language and code parameters are required")
	}

	// 安全检查
	if err := ValidateCodeSafety(code); err != nil {
		return nil, fmt.Errorf("code safety check failed: %w", err)
	}

	// 执行代码
	startTime := time.Now()
	var cmd *exec.Cmd
	var stdout, stderr bytes.Buffer

	switch language {
	case "python", "python3":
		cmd = exec.CommandContext(ctx, "python3", "-c", code)
	case "javascript", "js", "node":
		cmd = exec.CommandContext(ctx, "node", "-e", code)
	case "bash", "sh":
		cmd = exec.CommandContext(ctx, "bash", "-c", code)
	case "go":
		// Go 需要写入临时文件
		tmpFile, err := os.CreateTemp("", "sandbox_*.go")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		os.WriteFile(tmpPath, []byte(code), 0644)
		cmd = exec.CommandContext(ctx, "go", "run", tmpPath)
	default:
		return nil, fmt.Errorf("unsupported language: %s (supported: python, javascript, bash, go)", language)
	}

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(startTime)

	if err != nil {
		return map[string]interface{}{
			"language": language,
			"stdout":   stdout.String(),
			"stderr":   stderr.String(),
			"duration": duration.String(),
			"success":  false,
			"error":    err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"language": language,
		"stdout":   stdout.String(),
		"stderr":   stderr.String(),
		"duration": duration.String(),
		"success":  true,
	}, nil
}

// FileReadTool 文件读取工具
type FileReadTool struct {
	AllowedBaseDirs []string // 允许的根目录白名单
	MaxFileSize     int64    // 最大文件大小 (默认 1MB)
}

func (t *FileReadTool) Name() string { return "read_file" }

func (t *FileReadTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "read_file",
			"description": "读取服务器上的文件内容。支持文本文件，自动限制读取大小。",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "文件路径"},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t *FileReadTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path parameter is required")
	}

	// 安全校验：解析为绝对路径并验证
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// 检查路径是否包含危险字符
	if strings.Contains(absPath, "..") || strings.Contains(absPath, "~") {
		return nil, fmt.Errorf("path contains dangerous characters: %s", absPath)
	}

	// 检查路径是否在允许的目录内
	if !t.isPathAllowed(absPath) {
		return nil, fmt.Errorf("path not in allowed directories: %s", absPath)
	}

	// 检查是否为常规文件（防止目录遍历、符号链接攻击）
	info, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve path: %w", err)
	}
	if !t.isPathAllowed(info) {
		return nil, fmt.Errorf("resolved path not in allowed directories: %s", info)
	}

	fileInfo, err := os.Stat(info)
	if err != nil {
		return nil, fmt.Errorf("cannot access file: %w", err)
	}
	if fileInfo.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file")
	}

	// 检查文件大小
	maxSize := t.MaxFileSize
	if maxSize <= 0 {
		maxSize = 1 << 20 // 默认 1MB
	}
	if fileInfo.Size() > maxSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", fileInfo.Size(), maxSize)
	}

	// 读取文件内容
	data, err := os.ReadFile(info)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return map[string]interface{}{
		"path":      absPath,
		"content":   string(data),
		"size":      len(data),
		"extension": filepath.Ext(absPath),
	}, nil
}

// isPathAllowed 检查路径是否在允许的目录白名单内
func (t *FileReadTool) isPathAllowed(absPath string) bool {
	if len(t.AllowedBaseDirs) == 0 {
		// 没有配置白名单时，允许当前工作目录和 /tmp
		cwd, _ := os.Getwd()
		t.AllowedBaseDirs = []string{cwd, "/tmp"}
	}
	for _, base := range t.AllowedBaseDirs {
		if strings.HasPrefix(absPath, base) {
			return true
		}
	}
	return false
}

// FileWriteTool 文件写入工具
type FileWriteTool struct {
	AllowedBaseDirs []string // 允许的根目录白名单
	MaxFileSize     int64    // 最大文件大小 (默认 1MB)
}

func (t *FileWriteTool) Name() string { return "write_file" }

func (t *FileWriteTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "write_file",
			"description": "向服务器写入文件内容。会自动创建所需的父目录。",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path":    map[string]interface{}{"type": "string", "description": "文件路径"},
					"content": map[string]interface{}{"type": "string", "description": "文件内容"},
				},
				"required": []string{"path", "content"},
			},
		},
	}
}

func (t *FileWriteTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)

	if path == "" {
		return nil, fmt.Errorf("path parameter is required")
	}

	// 安全校验：解析为绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// 检查路径是否包含危险字符
	if strings.Contains(absPath, "..") || strings.Contains(absPath, "~") {
		return nil, fmt.Errorf("path contains dangerous characters: %s", absPath)
	}

	// 检查路径是否在允许的目录内
	if !t.isPathAllowed(absPath) {
		return nil, fmt.Errorf("path not in allowed directories: %s", absPath)
	}

	// 检查文件大小
	maxSize := t.MaxFileSize
	if maxSize <= 0 {
		maxSize = 1 << 20 // 默认 1MB
	}
	if int64(len(content)) > maxSize {
		return nil, fmt.Errorf("content too large: %d bytes (max %d)", len(content), maxSize)
	}

	// 确保父目录存在
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// 写入文件（权限 0644）
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return map[string]interface{}{
		"path":    absPath,
		"success": true,
		"size":    len(content),
	}, nil
}

// isPathAllowed 检查路径是否在允许的目录白名单内
func (t *FileWriteTool) isPathAllowed(absPath string) bool {
	if len(t.AllowedBaseDirs) == 0 {
		cwd, _ := os.Getwd()
		t.AllowedBaseDirs = []string{cwd, "/tmp"}
	}
	for _, base := range t.AllowedBaseDirs {
		if strings.HasPrefix(absPath, base) {
			return true
		}
	}
	return false
}

// CommandTool 命令执行工具
type CommandTool struct {
	allowedDangerous map[string]bool
	mu               sync.RWMutex
}

func (t *CommandTool) Name() string { return "execute_command" }

func (t *CommandTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "execute_command",
			"description": "在服务器上执行 shell 命令。危险命令需要用户确认。",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command":  map[string]interface{}{"type": "string", "description": "要执行的命令"},
					"approved": map[string]interface{}{"type": "boolean", "description": "是否已获用户批准（危险命令需要）"},
				},
				"required": []string{"command"},
			},
		},
	}
}

// AllowCommand 将命令加入已批准列表
func (t *CommandTool) AllowCommand(cmd string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.allowedDangerous == nil {
		t.allowedDangerous = make(map[string]bool)
	}
	t.allowedDangerous[cmd] = true
}

func (t *CommandTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	command, _ := params["command"].(string)
	approved, _ := params["approved"].(bool)
	if command == "" {
		return nil, fmt.Errorf("command parameter is required")
	}

	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	baseCmd := filepath.Base(cmdParts[0])

	// 危险命令需要确认
	if isDangerousCommand(baseCmd) {
		t.mu.RLock()
		alreadyAllowed := t.allowedDangerous[command]
		t.mu.RUnlock()

		if !approved && !alreadyAllowed {
			return nil, &ConfirmationRequiredError{
				ID:      uuid.New().String(),
				Command: command,
				Risk:    getCommandRisk(baseCmd),
			}
		}
	}

	// 超时保护：60 秒
	execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if isWindowsCmd(command) {
		cmd = exec.CommandContext(execCtx, "cmd", "/C", command)
	} else {
		cmdParts := strings.Fields(command)
		if len(cmdParts) == 0 {
			return nil, fmt.Errorf("empty command")
		}
		cmd = exec.CommandContext(execCtx, cmdParts[0], cmdParts[1:]...)
	}

	tmpDir := os.TempDir()
	cmd.Dir = tmpDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]interface{}{
			"command": command,
			"error":   err.Error(),
			"output":  string(output),
		}, nil
	}

	return map[string]interface{}{
		"command": command,
		"output":  string(output),
		"success": true,
	}, nil
}

// isDangerousCommand 判断是否为危险命令
func isDangerousCommand(cmd string) bool {
	dangerous := map[string]bool{
		"rm": true, "rmdir": true, "shred": true, "mkfs": true, "dd": true,
		"shutdown": true, "reboot": true, "halt": true, "poweroff": true, "init": true,
		"su": true, "sudo": true, "chroot": true,
		"passwd": true, "useradd": true, "userdel": true, "usermod": true,
		"fdisk": true, "parted": true, "mount": true, "umount": true,
		"swapon": true, "swapoff": true,
		"iptables": true, "nft": true, "ifconfig": true,
		"systemctl": true, "service": true,
		"modprobe": true, "insmod": true, "rmmod": true,
		"del": true, "format": true, "rd": true,
	}
	return dangerous[cmd]
}

func isWindowsCmd(command string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	lower := strings.ToLower(command)
	windowsPrefixes := []string{"dir ", "type ", "copy ", "xcopy ", "del ", "rd ", "md ", "move ", "ren ", "tasklist", "taskkill", "net ", "ipconfig", "ping ", "tracert", "nslookup", "systeminfo", "wmic ", "powershell", "cmd ", "echo ", "set ", "findstr"}
	for _, prefix := range windowsPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return strings.Contains(lower, "&") || strings.Contains(lower, "|") || strings.Contains(lower, ">") || strings.Contains(lower, "<")
}

// getCommandRisk 获取命令风险描述
func getCommandRisk(cmd string) string {
	switch cmd {
	case "rm", "rmdir", "shred":
		return "destructive: 可能永久删除文件或目录"
	case "dd", "mkfs":
		return "destructive: 可能破坏磁盘数据"
	case "shutdown", "reboot", "halt", "poweroff", "init":
		return "system: 可能关闭或重启系统"
	case "su", "sudo", "chroot":
		return "privilege: 可能提升到管理员权限"
	case "passwd", "useradd", "userdel", "usermod":
		return "user: 可能修改系统用户账户"
	case "fdisk", "parted", "mount", "umount", "swapon", "swapoff":
		return "disk: 可能影响磁盘分区和挂载"
	case "iptables", "nft", "ifconfig":
		return "network: 可能影响防火墙和网络配置"
	case "systemctl", "service":
		return "service: 可能启动、停止或修改系统服务"
	case "modprobe", "insmod", "rmmod":
		return "kernel: 可能加载或卸载内核模块"
	default:
		return "unknown: 潜在风险操作"
	}
}

// HTTPRequestTool HTTP 请求工具
type HTTPRequestTool struct{}

func (t *HTTPRequestTool) Name() string { return "http_request" }

func (t *HTTPRequestTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "http_request",
			"description": "发送 HTTP 请求并返回响应。支持 GET、POST 等方法，自动防护 SSRF 攻击。",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url":     map[string]interface{}{"type": "string", "description": "请求 URL"},
					"method":  map[string]interface{}{"type": "string", "description": "HTTP 方法 (GET, POST, PUT, DELETE)"},
					"headers": map[string]interface{}{"type": "object", "description": "请求头"},
					"body":    map[string]interface{}{"type": "string", "description": "请求体"},
				},
				"required": []string{"url"},
			},
		},
	}
}

func (t *HTTPRequestTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	url, _ := params["url"].(string)
	method, _ := params["method"].(string)
	if method == "" {
		method = "GET"
	}

	if url == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	// 安全校验：只允许 HTTP/HTTPS 协议
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("only http/https protocols are allowed")
	}

	// 阻止内网地址访问（SSRF 防护）
	if isPrivateURL(url) {
		return nil, fmt.Errorf("access to private/internal addresses is not allowed")
	}

	// 构建请求
	var bodyReader io.Reader
	headers, _ := params["headers"].(map[string]interface{})
	body, hasBody := params["body"].(string)

	if hasBody && (method == "POST" || method == "PUT" || method == "PATCH") {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, fmt.Sprintf("%v", v))
	}

	// 设置超时
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 限制响应大小
	if len(respBody) > 100000 {
		respBody = respBody[:100000]
	}

	// 尝试解析为 JSON
	var jsonResult interface{}
	if json.Unmarshal(respBody, &jsonResult) == nil {
		return map[string]interface{}{
			"status_code": resp.StatusCode,
			"body":        jsonResult,
			"size":        len(respBody),
		}, nil
	}

	return map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(respBody),
		"size":        len(respBody),
	}, nil
}

// JSONParseTool JSON 解析工具
type JSONParseTool struct{}

func (t *JSONParseTool) Name() string { return "parse_json" }

func (t *JSONParseTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "parse_json",
			"description": "解析 JSON 字符串并返回结构化数据。",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"json": map[string]interface{}{"type": "string", "description": "要解析的 JSON 字符串"},
				},
				"required": []string{"json"},
			},
		},
	}
}

func (t *JSONParseTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	jsonStr, _ := params["json"].(string)
	if jsonStr == "" {
		return nil, fmt.Errorf("json parameter is required")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return map[string]interface{}{
		"parsed": result,
		"type":   fmt.Sprintf("%T", result),
	}, nil
}

// isPrivateURL 检查 URL 是否指向内网地址（SSRF 防护）
func isPrivateURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true // 解析失败视为不安全
	}

	host := parsed.Hostname()
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return true
	}

	ip := net.ParseIP(host)
	if ip != nil {
		return isPrivateIP(ip)
	}

	// 解析域名（简单检查，不阻止公网域名）
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false // DNS 解析失败不阻塞
	}
	return isPrivateIP(ips[0])
}

// isPrivateIP 检查 IP 是否为内网地址
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{parseCIDR("10.0.0.0/8")},
		{parseCIDR("172.16.0.0/12")},
		{parseCIDR("192.168.0.0/16")},
		{parseCIDR("127.0.0.0/8")},
		{parseCIDR("169.254.0.0/16")},
		{parseCIDR("::1/128")},
		{parseCIDR("fc00::/7")},
	}

	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

// parseCIDR 解析 CIDR
func parseCIDR(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		return &net.IPNet{}
	}
	return network
}
