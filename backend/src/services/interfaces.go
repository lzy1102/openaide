package services

import (
	"context"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// ============ 核心服务接口 ============

// ToolProvider 工具提供者接口 - 解耦 ToolCallingService 对 ToolService 的依赖
type ToolProvider interface {
	GetToolDefinitionsWithMCP() []map[string]interface{}
	GetToolDefinitionsWithMCPByNames(names []string) []map[string]interface{}
	ExecuteTool(ctx context.Context, toolCall *models.ToolCall, dialogueID, messageID, userID string) (*models.ToolResult, error)
}

// ModelCaller 模型调用接口 - 解耦对 ModelService 的依赖
type ModelCaller interface {
	ChatWithTools(modelID string, messages []llm.Message, tools []llm.ToolDefinition, options map[string]interface{}) (*llm.ChatResponse, error)
	Chat(modelID string, messages []llm.Message, options map[string]interface{}) (*llm.ChatResponse, error)
	GetModel(idOrName string) (*models.Model, error)
	ListModels() ([]models.Model, error)
	GetLLMClient() llm.LLMClient
}

// DialogueStore 对话存储接口 - 解耦对 DialogueService 的依赖
type DialogueStore interface {
	CreateDialogue(userID, title string) models.Dialogue
	AddMessage(dialogueID, sender, content string, reasoningContent ...string) (models.Message, error)
	GetMessages(dialogueID string) []models.Message
	GetDialogue(id string) (models.Dialogue, bool)
}

// Logger 日志接口 - 解耦对 LoggerService 的依赖
type Logger interface {
	Info(ctx context.Context, format string, args ...interface{})
	Error(ctx context.Context, format string, args ...interface{})
	Warn(ctx context.Context, format string, args ...interface{})
	Debug(ctx context.Context, format string, args ...interface{})
}

// EventPublisher 事件发布接口 - 解耦对 EventBus 的依赖
type EventPublisher interface {
	Publish(ctx context.Context, topic, eventType, source string, data map[string]interface{})
}

// UsageTracker 用量追踪接口 - 解耦对 UsageService 的依赖
type UsageTracker interface {
	RecordUsage(record *models.UsageRecord) error
}

// Cache 缓存接口 - 解耦对 CacheService 的依赖
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl int)
	Delete(key string)
}

// ============ 编排服务接口 ============

// TaskExecutor 任务执行接口 - 解耦 OrchestrationService 对 AgentExecutor 的依赖
type TaskExecutor interface {
	Execute(ctx context.Context, req *TaskExecRequest) (*TaskExecResult, error)
}

// SubtaskRunner 子任务运行接口 - 解耦 OrchestrationService 对 ToolCallingService 的依赖
type SubtaskRunner interface {
	SendMessageWithTools(ctx context.Context, dialogueID, userID, content, modelID string, options map[string]interface{}) (*models.Message, error)
}

// PlanPersister 计划持久化接口 - 解耦对数据库的直接依赖
type PlanPersister interface {
	SaveOrchestration(record *models.OrchestrationRecord) error
	SaveSubtask(record *models.SubtaskExecutionRecord) error
	UpdateOrchestration(sessionID string, status string) error
	GetOrchestrationHistory(userID string, limit int) ([]models.OrchestrationRecord, error)
	GetSubtaskRecords(sessionID string) ([]models.SubtaskExecutionRecord, error)
}
