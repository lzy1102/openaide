package kernel

import (
	"context"
	"time"
)

// Kernel 内核接口 - Agent 智能核心
type Kernel interface {
	// Process 处理用户查询（同步）
	Process(ctx context.Context, query *Query) (*Response, error)

	// ProcessStream 处理用户查询（流式）
	ProcessStream(ctx context.Context, query *Query) (<-chan StreamChunk, error)

	// GetState 获取当前状态
	GetState() KernelState

	// Subscribe 订阅事件
	Subscribe(handler EventHandler)

	// Unsubscribe 取消订阅
	Unsubscribe(handler EventHandler)

	// GetSlashCommands 获取所有可用的斜杠命令（/name → skillID）
	GetSlashCommands() map[string]string
}

// EventHandler 事件处理器
type EventHandler interface {
	HandleEvent(event Event)
}

// EventHandlerFunc 事件处理函数
type EventHandlerFunc func(event Event)

func (f EventHandlerFunc) HandleEvent(event Event) {
	f(event)
}

// LLMProvider LLM 提供者接口（内核层抽象）
type LLMProvider interface {
	// Chat 发送聊天请求
	Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options map[string]interface{}) (*LLMResponse, error)

	// ChatStream 发送流式聊天请求
	ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, options map[string]interface{}) (<-chan StreamChunk, error)

	// GetModelID 获取当前模型 ID
	GetModelID() string
}

// ModelSwitcher is an optional interface for providers that support runtime model switching.
// Use type assertion: if ms, ok := provider.(ModelSwitcher); ok { ms.SetModelID(m) }
type ModelSwitcher interface {
	SetModelID(model string)
}

// ToolExecutor 工具执行器接口
type ToolExecutor interface {
	// GetDefinitions 获取所有工具定义
	GetDefinitions() []ToolDefinition

	// GetDefinitionsByNames 按名称获取工具定义
	GetDefinitionsByNames(names []string) []ToolDefinition

	// Execute 执行工具调用
	Execute(ctx context.Context, call ToolCall, sessionID string) (*ToolResult, error)

	// Register 注册工具
	Register(tool ToolDefinition, handler ToolHandler) error
}

// ToolHandler 工具处理函数
type ToolHandler func(ctx context.Context, arguments string) (*ToolResult, error)

// Memory 记忆接口（多级记忆抽象）
type Memory interface {
	// Save 保存记忆
	Save(ctx context.Context, sessionID string, messages []Message) error

	// Load 加载记忆
	Load(ctx context.Context, sessionID string, limit int) ([]Message, error)

	// Search 搜索相关记忆
	Search(ctx context.Context, query string, limit int) ([]Message, float64, error)
}

// SessionStore 会话存储接口
type SessionStore interface {
	// Create 创建会话
	Create(ctx context.Context, projectID, userID string) (*Session, error)

	// Get 获取会话
	Get(ctx context.Context, sessionID string) (*Session, error)

	// Update 更新会话
	Update(ctx context.Context, session *Session) error

	// List 列出会话
	List(ctx context.Context, projectID, userID string, limit, offset int) ([]*Session, error)

	// Delete 删除会话
	Delete(ctx context.Context, sessionID string) error

	// CleanupOldSessions 删除超过 ttl 的过期会话，返回删除数量
	CleanupOldSessions(ctx context.Context, ttl time.Duration) (int, error)
}

// ContextCompressor 上下文压缩接口
type ContextCompressor interface {
	// Compress 压缩消息列表
	Compress(ctx context.Context, messages []Message, maxTokens int) ([]Message, int, error)

	// EstimateTokens 估算 Token 数
	EstimateTokens(messages []Message) int
}

// PermissionChecker 权限检查接口
type PermissionChecker interface {
	// Check 检查操作是否允许
	Check(ctx context.Context, action, resource string, context map[string]interface{}) (bool, string)

	// GetLevel 获取当前权限级别
	GetLevel() PermissionLevel
}

// PermissionLevel 权限级别
type PermissionLevel int

const (
	LevelStrict PermissionLevel = iota // 严格模式（默认）
	LevelStandard                      // 标准模式
	LevelRelaxed                       // 宽松模式
	LevelBypass                        // 绕过模式（需显式确认）
)

func (l PermissionLevel) String() string {
	switch l {
	case LevelStrict:
		return "strict"
	case LevelStandard:
		return "standard"
	case LevelRelaxed:
		return "relaxed"
	case LevelBypass:
		return "bypass"
	default:
		return "unknown"
	}
}

// Reflection 反思接口（增强能力）
type Reflection interface {
	// Reflect 对执行过程进行反思
	Reflect(ctx context.Context, sessionID string, execution ExecutionRecord) (*ReflectionResult, error)
}

// ExecutionRecord 执行记录
type ExecutionRecord struct {
	Query      string    `json:"query"`
	Response   string    `json:"response"`
	ToolCalls  []ToolCall `json:"tool_calls"`
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
	Duration   int64     `json:"duration"`
}

// ReflectionResult 反思结果
type ReflectionResult struct {
	Quality    int      `json:"quality"`     // 1-10
	Issues     []string `json:"issues"`      // 发现的问题
	Suggestions []string `json:"suggestions"` // 改进建议
	Learned    string   `json:"learned"`     // 学到的经验
}

// PatternDetector 模式检测接口（增强能力）
type PatternDetector interface {
	// Detect 检测模式
	Detect(ctx context.Context, sessionID string, messages []Message) ([]Pattern, error)
}

// Pattern 检测到的模式
type Pattern struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
	Frequency   int     `json:"frequency"`
}

// KnowledgeCollector 知识收集接口 — 质量门控后的知识积累
type KnowledgeCollector interface {
	// AddKnowledge 添加知识条目（标题、内容、来源、标签）
	AddKnowledge(ctx context.Context, title, content, source string, tags []string) (string, error)

	// SearchKnowledge 搜索知识库
	SearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeItem, error)

	// InjectContext 将相关知识注入为提示词片段，返回 (contextText, docIDs, error)
	InjectContext(ctx context.Context, query string, maxTokens int) (string, []string, error)

	// RecordKnowledgeUsage 记录知识被使用后的质量反馈
	RecordKnowledgeUsage(ctx context.Context, docIDs []string, qualityScore float64)
}

// KnowledgeItem 知识条目
type KnowledgeItem struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}
