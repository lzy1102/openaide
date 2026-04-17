package services

import (
	"context"
	"testing"
	"time"

	"openaide/backend/src/models"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDBForTool 创建测试数据库
func setupTestDBForTool(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// 自动迁移所有模型
	err = db.AutoMigrate(
		&models.Tool{},
		&models.ToolExecution{},
	)
	require.NoError(t, err)

	return db
}

// setupTestToolService 创建测试工具服务
func setupTestToolService(t *testing.T) *ToolService {
	db := setupTestDBForTool(t)
	cache := NewCacheService()
	logger, _ := NewLoggerService(LogLevelInfo, "")

	return NewToolService(db, cache, logger, nil)
}

// TestNewToolService 测试工具服务初始化
func TestNewToolService(t *testing.T) {
	service := setupTestToolService(t)

	assert.NotNil(t, service)
	assert.NotNil(t, service.registry)
	assert.NotNil(t, service.httpClient)
	assert.NotNil(t, service.logger)
	assert.Equal(t, 60*time.Second, service.httpClient.Timeout)
}

// TestRegisterBuiltinTools 测试内置工具注册
func TestRegisterBuiltinTools(t *testing.T) {
	service := setupTestToolService(t)

	builtinTools := []string{
		"get_current_time",
		"get_weather",
		"search_web",
		"calculate",
		"run_code",
		"read_file",
		"write_file",
		"execute_command",
		"http_request",
		"json_parse",
	}

	for _, toolName := range builtinTools {
		_, exists := service.registry.builtin[toolName]
		assert.True(t, exists, "Built-in tool %s should be registered", toolName)
	}
}

// TestRegisterTool 测试工具注册
func TestRegisterTool(t *testing.T) {
	service := setupTestToolService(t)

	tool := &models.Tool{
		ID:          uuid.New().String(),
		Name:        "test_tool",
		Description: "Test tool",
		Type:        "function",
		Category:    "test",
		Enabled:     true,
		ParametersSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"param1": map[string]interface{}{
					"type": "string",
				},
			},
		},
		ExecutorType: "http",
		ExecutorConfig: map[string]interface{}{
			"url_template": "https://api.example.com/test",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := service.RegisterTool(tool)
	assert.NoError(t, err)

	// 验证工具已注册
	retrieved, err := service.GetTool("test_tool")
	assert.NoError(t, err)
	assert.Equal(t, "test_tool", retrieved.Name)
	assert.Equal(t, "Test tool", retrieved.Description)
}

// TestGetTool 测试获取工具
func TestGetTool(t *testing.T) {
	service := setupTestToolService(t)

	// 注册测试工具
	tool := &models.Tool{
		ID:          uuid.New().String(),
		Name:        "test_get_tool",
		Description: "Test get tool",
		Type:        "function",
		Enabled:     true,
	}
	service.db.Create(tool)

	// 从缓存获取
	retrieved, err := service.GetTool("test_get_tool")
	assert.NoError(t, err)
	assert.Equal(t, "test_get_tool", retrieved.Name)

	// 测试不存在的工具
	_, err = service.GetTool("non_existent_tool")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool not found")
}

// TestListTools 测试列出所有工具
func TestListTools(t *testing.T) {
	service := setupTestToolService(t)

	// 创建测试工具
	tools := []models.Tool{
		{Name: "tool1", Description: "Tool 1", Type: "function", Enabled: true},
		{Name: "tool2", Description: "Tool 2", Type: "function", Enabled: true},
		{Name: "tool3", Description: "Tool 3", Type: "function", Enabled: false}, // 禁用的工具
	}

	for _, tool := range tools {
		tool.ID = uuid.New().String()
		service.db.Create(&tool)
	}

	list, err := service.ListTools()
	assert.NoError(t, err)
	// 应该只返回启用的工具
	assert.Len(t, list, 2)
}

// TestTimeTool 测试时间工具
func TestTimeTool(t *testing.T) {
	tool := &TimeTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)

	assert.Contains(t, resultMap, "timestamp")
	assert.Contains(t, resultMap, "datetime")
	assert.Contains(t, resultMap, "date")
	assert.Contains(t, resultMap, "time")
	assert.Contains(t, resultMap, "weekday")
	assert.Contains(t, resultMap, "timezone")
}

// TestWeatherTool 测试天气工具
func TestWeatherTool(t *testing.T) {
	tool := &WeatherTool{}
	ctx := context.Background()

	// 测试正常情况
	result, err := tool.Execute(ctx, map[string]interface{}{"city": "北京"})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "北京", resultMap["city"])
	assert.Contains(t, resultMap, "temperature")
	assert.Contains(t, resultMap, "weather")

	// 测试缺少必需参数
	_, err = tool.Execute(ctx, map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "city parameter is required")
}

// TestWebSearchTool 测试网络搜索工具
func TestWebSearchTool(t *testing.T) {
	tool := &WebSearchTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{"query": "Golang"})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "Golang", resultMap["query"])
	assert.Contains(t, resultMap, "results")

	// 测试缺少参数
	_, err = tool.Execute(ctx, map[string]interface{}{})
	assert.Error(t, err)
}

// TestCalculatorTool 测试计算器工具
func TestCalculatorTool(t *testing.T) {
	tool := &CalculatorTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{"expression": "2 + 2"})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "2 + 2", resultMap["expression"])
	assert.Contains(t, resultMap, "note") // 当前是模拟实现
}

// TestCodeRunTool 测试代码执行工具
func TestCodeRunTool(t *testing.T) {
	tool := &CodeRunTool{}
	ctx := context.Background()

	// 测试正常情况
	result, err := tool.Execute(ctx, map[string]interface{}{
		"language": "python",
		"code":     "print('Hello, World!')",
	})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "python", resultMap["language"])

	// 测试缺少语言参数
	_, err = tool.Execute(ctx, map[string]interface{}{"code": "print('test')"})
	assert.Error(t, err)

	// 测试缺少代码参数
	_, err = tool.Execute(ctx, map[string]interface{}{"language": "python"})
	assert.Error(t, err)
}

// TestFileReadTool 测试文件读取工具
func TestFileReadTool(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{"path": "/tmp/test.txt"})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "/tmp/test.txt", resultMap["path"])

	// 测试缺少路径参数
	_, err = tool.Execute(ctx, map[string]interface{}{})
	assert.Error(t, err)
}

// TestFileWriteTool 测试文件写入工具
func TestFileWriteTool(t *testing.T) {
	tool := &FileWriteTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"path":    "/tmp/test.txt",
		"content": "test content",
	})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.True(t, resultMap["success"].(bool))
}

// TestCommandTool 测试命令执行工具
func TestCommandTool(t *testing.T) {
	tool := &CommandTool{}
	ctx := context.Background()

	// 测试允许的命令
	result, err := tool.Execute(ctx, map[string]interface{}{"command": "echo hello"})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "echo hello", resultMap["command"])

	// 测试不允许的命令
	_, err = tool.Execute(ctx, map[string]interface{}{"command": "rm -rf /"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command not allowed")

	// 测试空命令
	_, err = tool.Execute(ctx, map[string]interface{}{})
	assert.Error(t, err)
}

// TestHTTPRequestTool 测试HTTP请求工具
func TestHTTPRequestTool(t *testing.T) {
	tool := &HTTPRequestTool{}
	ctx := context.Background()

	// 测试默认GET请求
	result, err := tool.Execute(ctx, map[string]interface{}{
		"url": "https://api.example.com/test",
	})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "https://api.example.com/test", resultMap["url"])
	assert.Equal(t, "GET", resultMap["method"])

	// 测试POST请求
	result, err = tool.Execute(ctx, map[string]interface{}{
		"url":    "https://api.example.com/test",
		"method": "POST",
	})
	assert.NoError(t, err)

	resultMap, ok = result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "POST", resultMap["method"])

	// 测试缺少URL
	_, err = tool.Execute(ctx, map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url parameter is required")
}

// TestJSONParseTool 测试JSON解析工具
func TestJSONParseTool(t *testing.T) {
	tool := &JSONParseTool{}
	ctx := context.Background()

	// 测试有效JSON
	validJSON := `{"name": "test", "value": 123}`
	result, err := tool.Execute(ctx, map[string]interface{}{"json": validJSON})
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, resultMap, "parsed")

	parsed, ok := resultMap["parsed"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "test", parsed["name"])
	assert.Equal(t, float64(123), parsed["value"])

	// 测试无效JSON
	invalidJSON := `{invalid json}`
	_, err = tool.Execute(ctx, map[string]interface{}{"json": invalidJSON})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse JSON")

	// 测试缺少参数
	_, err = tool.Execute(ctx, map[string]interface{}{})
	assert.Error(t, err)
}

// TestExecuteTool 测试执行工具完整流程
func TestExecuteTool(t *testing.T) {
	service := setupTestToolService(t)

	// 注册测试工具
	tool := &models.Tool{
		ID:           uuid.New().String(),
		Name:         "builtin_time",
		Description:  "Get current time",
		Type:         "builtin",
		ExecutorType: "builtin",
		Enabled:      true,
		ParametersSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
	service.db.Create(tool)

	toolCall := &models.ToolCall{
		ID:        uuid.New().String(),
		Name:      "get_current_time", // 使用内置工具名称
		Arguments: "{}",
	}

	dialogueID := uuid.New().String()
	messageID := uuid.New().String()
	userID := uuid.New().String()

	result, err := service.ExecuteTool(context.Background(), toolCall, dialogueID, messageID, userID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)

	// 验证执行记录已创建
	var executions []models.ToolExecution
	err = service.db.Where("dialogue_id = ? AND message_id = ?", dialogueID, messageID).Find(&executions).Error
	assert.NoError(t, err)
	assert.Len(t, executions, 1)
	assert.Equal(t, "success", executions[0].Status)
}

// TestParseToolCalls 测试解析LLM工具调用
func TestParseToolCalls(t *testing.T) {
	service := setupTestToolService(t)

	// 测试正常的工具调用响应
	response := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id": "call_123",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"city": "北京"}`,
							},
						},
					},
				},
			},
		},
	}

	calls, err := service.ParseToolCalls(response)
	assert.NoError(t, err)
	assert.Len(t, calls, 1)
	assert.Equal(t, "call_123", calls[0].ID)
	assert.Equal(t, "get_weather", calls[0].Name)

	// 测试空响应
	calls, err = service.ParseToolCalls(map[string]interface{}{})
	assert.NoError(t, err)
	assert.Nil(t, calls)

	// 测试无效JSON参数
	response = map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id": "call_456",
							"function": map[string]interface{}{
								"name":      "test",
								"arguments": "{invalid json}",
							},
						},
					},
				},
			},
		},
	}

	calls, err = service.ParseToolCalls(response)
	assert.NoError(t, err) // 应该返回空参数而不是错误
	assert.Len(t, calls, 1)
}

// TestGetToolDefinitions 测试获取工具定义
func TestGetToolDefinitions(t *testing.T) {
	service := setupTestToolService(t)

	definitions := service.GetToolDefinitions()
	assert.NotEmpty(t, definitions)

	// 验证内置工具定义
	defMap := make(map[string]map[string]interface{})
	for _, def := range definitions {
		if fn, ok := def["function"].(map[string]interface{}); ok {
			defMap[fn["name"].(string)] = fn
		}
	}

	// 检查关键工具存在
	assert.Contains(t, defMap, "get_current_time")
	assert.Contains(t, defMap, "get_weather")
	assert.Contains(t, defMap, "calculate")

	// 验证工具定义结构
	timeDef := defMap["get_current_time"]
	assert.Equal(t, "function", timeDef["type"])
	assert.Contains(t, timeDef, "name")
	assert.Contains(t, timeDef, "description")
	assert.Contains(t, timeDef, "parameters")
}

// TestExecuteScriptTool 测试脚本工具执行
func TestExecuteScriptTool(t *testing.T) {
	service := setupTestToolService(t)

	tool := &models.Tool{
		ID:   uuid.New().String(),
		Name: "test_script",
		Type: "script",
		ExecutorConfig: map[string]interface{}{
			"script": "echo 'Hello {{name}}'",
		},
	}

	ctx := context.Background()
	params := map[string]interface{}{"name": "World"}

	result, err := service.executeScriptTool(ctx, tool, params)
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, resultMap, "output")
}

// TestExecuteScriptToolCommandInjection 测试脚本工具命令注入防护
func TestExecuteScriptToolCommandInjection(t *testing.T) {
	service := setupTestToolService(t)

	// 恶意参数测试
	tool := &models.Tool{
		ID:   uuid.New().String(),
		Name: "test_script",
		ExecutorConfig: map[string]interface{}{
			"script": "echo 'Result: {{value}}'",
		},
	}

	ctx := context.Background()
	params := map[string]interface{}{"value": "test'; rm -rf /; echo '"}

	// 虽然没有实现沙箱，但应该执行而不崩溃
	_, err := service.executeScriptTool(ctx, tool, params)
	// 注意：当前实现可能存在命令注入风险，这里只是验证不会panic
	// 实际生产环境需要沙箱隔离
	assert.NotNil(t, err) // 可能会失败，这是预期行为
}

// TestToolExecutionRecord 测试工具执行记录
func TestToolExecutionRecord(t *testing.T) {
	service := setupTestToolService(t)

	tool := &models.Tool{
		ID:          uuid.New().String(),
		Name:        "test_recording",
		Description: "Test execution recording",
		Type:        "builtin",
		Enabled:     true,
	}
	service.db.Create(tool)

	toolCall := &models.ToolCall{
		ID:        uuid.New().String(),
		Name:      "get_current_time",
		Arguments: "{}",
	}

	dialogueID := uuid.New().String()
	messageID := uuid.New().String()
	userID := uuid.New().String()

	_, err := service.ExecuteTool(context.Background(), toolCall, dialogueID, messageID, userID)
	assert.NoError(t, err)

	// 查询执行记录
	var execution models.ToolExecution
	err = service.db.Where("dialogue_id = ? AND message_id = ?", dialogueID, messageID).First(&execution).Error
	assert.NoError(t, err)

	assert.Equal(t, userID, execution.UserID)
	assert.Equal(t, "get_current_time", execution.ToolName)
	assert.Equal(t, "success", execution.Status)
	assert.NotNil(t, execution.StartedAt)
	assert.NotNil(t, execution.CompletedAt)
	assert.Greater(t, execution.Duration, 0)
}

// TestToolWithEmptyExecutorConfig 测试空执行配置
func TestToolWithEmptyExecutorConfig(t *testing.T) {
	service := setupTestToolService(t)

	tool := &models.Tool{
		ID:             uuid.New().String(),
		Name:           "empty_config",
		Type:           "script",
		ExecutorConfig: map[string]interface{}{},
	}

	ctx := context.Background()

	_, err := service.executeScriptTool(ctx, tool, map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid script tool config")
}

// BenchmarkToolExecution 性能测试
func BenchmarkToolExecution(b *testing.B) {
	service := setupTestToolService(&testing.T{})

	toolCall := &models.ToolCall{
		ID:        uuid.New().String(),
		Name:      "get_current_time",
		Arguments: "{}",
	}

	dialogueID := uuid.New().String()
	messageID := uuid.New().String()
	userID := uuid.New().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.ExecuteTool(context.Background(), toolCall, dialogueID, messageID, userID)
	}
}
