package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"openaide/backend/src/models"
	oldllm "openaide/backend/src/services/llm"
)

// LLMAdapter 将旧的 LLM 客户端适配为新的 LLMProvider 接口
type LLMAdapter struct {
	modelSvc interface {
		ChatWithTools(modelID string, messages []oldllm.Message, tools []oldllm.ToolDefinition, options map[string]interface{}) (*oldllm.ChatResponse, error)
		Chat(modelID string, messages []oldllm.Message, options map[string]interface{}) (*oldllm.ChatResponse, error)
		GetModel(idOrName string) (*models.Model, error)
	}
	modelID string
}

// NewLLMAdapter 创建 LLM 适配器
func NewLLMAdapter(modelSvc interface {
	ChatWithTools(modelID string, messages []oldllm.Message, tools []oldllm.ToolDefinition, options map[string]interface{}) (*oldllm.ChatResponse, error)
	Chat(modelID string, messages []oldllm.Message, options map[string]interface{}) (*oldllm.ChatResponse, error)
	GetModel(idOrName string) (*models.Model, error)
}, modelID string) *LLMAdapter {
	return &LLMAdapter{
		modelSvc: modelSvc,
		modelID:  modelID,
	}
}

// Chat 发送聊天请求
func (a *LLMAdapter) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options map[string]interface{}) (*LLMResponse, error) {
	oldMessages := make([]oldllm.Message, len(messages))
	for i, msg := range messages {
		oldMessages[i] = oldllm.Message{
			Role:             msg.Role,
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			Name:             msg.Name,
			ToolCallID:       msg.ToolCallID,
		}
		if len(msg.ToolCalls) > 0 {
			oldMessages[i].ToolCalls = make([]oldllm.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				oldMessages[i].ToolCalls[j] = oldllm.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: oldllm.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	}

	var oldTools []oldllm.ToolDefinition
	if len(tools) > 0 {
		oldTools = make([]oldllm.ToolDefinition, len(tools))
		for i, tool := range tools {
			oldTools[i] = oldllm.ToolDefinition{
				Type: tool.Type,
				Function: oldllm.FunctionDef{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			}
		}
	}

	var resp *oldllm.ChatResponse
	var err error
	if len(oldTools) > 0 {
		resp, err = a.modelSvc.ChatWithTools(a.modelID, oldMessages, oldTools, options)
	} else {
		resp, err = a.modelSvc.Chat(a.modelID, oldMessages, options)
	}
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty LLM response")
	}

	choice := resp.Choices[0]
	result := &LLMResponse{
		ID:      resp.ID,
		Content: choice.Message.Content,
		Model:   resp.Model,
	}

	if resp.Usage != nil {
		result.Usage = &TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return result, nil
}

// ChatStream 发送流式聊天请求
func (a *LLMAdapter) ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, options map[string]interface{}) (<-chan StreamChunk, error) {
	resultChan := make(chan StreamChunk, 10)

	go func() {
		defer close(resultChan)
		resp, err := a.Chat(ctx, messages, tools, options)
		if err != nil {
			resultChan <- StreamChunk{Error: err, Done: true}
			return
		}
		resultChan <- StreamChunk{
			Content: resp.Content,
			Done:    true,
			Usage:   resp.Usage,
		}
	}()

	return resultChan, nil
}

// GetModelID 获取当前模型 ID
func (a *LLMAdapter) GetModelID() string {
	return a.modelID
}

// ToolAdapter 将旧的 ToolProvider 适配为新的 ToolExecutor 接口
type ToolAdapter struct {
	toolSvc interface {
		GetToolDefinitionsWithMCP() []map[string]interface{}
		GetToolDefinitionsWithMCPByNames(names []string) []map[string]interface{}
		ExecuteTool(ctx context.Context, toolCall *models.ToolCall, dialogueID, messageID, userID string) (*models.ToolResult, error)
	}
}

// NewToolAdapter 创建工具适配器
func NewToolAdapter(toolSvc interface {
	GetToolDefinitionsWithMCP() []map[string]interface{}
	GetToolDefinitionsWithMCPByNames(names []string) []map[string]interface{}
	ExecuteTool(ctx context.Context, toolCall *models.ToolCall, dialogueID, messageID, userID string) (*models.ToolResult, error)
}) *ToolAdapter {
	return &ToolAdapter{toolSvc: toolSvc}
}

// GetDefinitions 获取所有工具定义
func (a *ToolAdapter) GetDefinitions() []ToolDefinition {
	oldDefs := a.toolSvc.GetToolDefinitionsWithMCP()
	return a.convertDefs(oldDefs)
}

// GetDefinitionsByNames 按名称获取工具定义
func (a *ToolAdapter) GetDefinitionsByNames(names []string) []ToolDefinition {
	oldDefs := a.toolSvc.GetToolDefinitionsWithMCPByNames(names)
	return a.convertDefs(oldDefs)
}

// Execute 执行工具调用
func (a *ToolAdapter) Execute(ctx context.Context, call ToolCall, sessionID string) (*ToolResult, error) {
	toolCall := &models.ToolCall{
		ID:        call.ID,
		Name:      call.Function.Name,
		Arguments: call.Function.Arguments,
	}

	result, err := a.toolSvc.ExecuteTool(ctx, toolCall, sessionID, "", "")
	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}

	return &ToolResult{Content: result.Content}, nil
}

// Register 注册工具（旧系统不支持动态注册，返回错误）
func (a *ToolAdapter) Register(tool ToolDefinition, handler ToolHandler) error {
	return fmt.Errorf("dynamic tool registration not supported in adapter mode")
}

func (a *ToolAdapter) convertDefs(oldDefs []map[string]interface{}) []ToolDefinition {
	defs := make([]ToolDefinition, 0, len(oldDefs))
	for _, def := range oldDefs {
		fnMap, ok := def["function"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := fnMap["name"].(string)
		desc, _ := fnMap["description"].(string)
		params, _ := fnMap["parameters"].(map[string]interface{})

		if name == "" {
			continue
		}

		defs = append(defs, ToolDefinition{
			Type: "function",
			Function: FunctionDef{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}
	return defs
}

// MemoryAdapter 将旧的 MemoryService 适配为新的 Memory 接口
type MemoryAdapter struct {
	memorySvc interface {
		GetMessages(dialogueID string) []models.Message
		AddMessage(dialogueID, sender, content string, reasoningContent ...string) (models.Message, error)
	}
}

// NewMemoryAdapter 创建记忆适配器
func NewMemoryAdapter(memorySvc interface {
	GetMessages(dialogueID string) []models.Message
	AddMessage(dialogueID, sender, content string, reasoningContent ...string) (models.Message, error)
}) *MemoryAdapter {
	return &MemoryAdapter{memorySvc: memorySvc}
}

// Save 保存记忆
func (a *MemoryAdapter) Save(ctx context.Context, sessionID string, messages []Message) error {
	for _, msg := range messages {
		_, err := a.memorySvc.AddMessage(sessionID, msg.Role, msg.Content, msg.ReasoningContent)
		if err != nil {
			return err
		}
	}
	return nil
}

// Load 加载记忆
func (a *MemoryAdapter) Load(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	oldMessages := a.memorySvc.GetMessages(sessionID)
	if len(oldMessages) > limit {
		oldMessages = oldMessages[len(oldMessages)-limit:]
	}

	messages := make([]Message, len(oldMessages))
	for i, msg := range oldMessages {
		messages[i] = Message{
			Role:    msg.Sender,
			Content: msg.Content,
		}
	}
	return messages, nil
}

// Search 搜索相关记忆（旧系统不支持语义搜索，返回空）
func (a *MemoryAdapter) Search(ctx context.Context, query string, limit int) ([]Message, float64, error) {
	return nil, 0, nil
}

// Compress 压缩记忆（旧系统不支持，返回空）
func (a *MemoryAdapter) Compress(ctx context.Context, sessionID string) error {
	return nil
}

// SessionStoreAdapter 内存会话存储（用于新内核）
type SessionStoreAdapter struct {
	sessions map[string]*Session
}

// NewSessionStoreAdapter 创建内存会话存储
func NewSessionStoreAdapter() *SessionStoreAdapter {
	return &SessionStoreAdapter{
		sessions: make(map[string]*Session),
	}
}

// Create 创建会话
func (s *SessionStoreAdapter) Create(ctx context.Context, projectID, userID string) (*Session, error) {
	session := &Session{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		UserID:    userID,
		Messages:  make([]Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.sessions[session.ID] = session
	return session, nil
}

// Get 获取会话
func (s *SessionStoreAdapter) Get(ctx context.Context, sessionID string) (*Session, error) {
	if session, ok := s.sessions[sessionID]; ok {
		return session, nil
	}
	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// Update 更新会话
func (s *SessionStoreAdapter) Update(ctx context.Context, session *Session) error {
	s.sessions[session.ID] = session
	return nil
}

// List 列出会话
func (s *SessionStoreAdapter) List(ctx context.Context, projectID, userID string, limit int) ([]*Session, error) {
	var result []*Session
	for _, session := range s.sessions {
		if (projectID == "" || session.ProjectID == projectID) &&
			(userID == "" || session.UserID == userID) {
			result = append(result, session)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}
