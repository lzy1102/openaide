package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

type TaskTool struct {
	toolSvc        *ToolService
	modelSvc       *ModelService
	permSvc        *PermissionService
	dialogueSvc    *DialogueService
	eventBus       *EventBus
	activeTasks    map[string]*TaskContext
	mu             sync.RWMutex
}

type TaskContext struct {
	ID           string
	ParentMsgID  string
	SubAgentType AgentMode
	Description  string
	Status       string
	StartedAt    time.Time
	CompletedAt  *time.Time
	Result       string
	ToolCalls    []TaskToolCall
}

type TaskToolCall struct {
	ToolName string
	Params   map[string]interface{}
	Result   string
	Duration time.Duration
}

func NewTaskTool(toolSvc *ToolService, modelSvc *ModelService, permSvc *PermissionService, dialogueSvc *DialogueService, eventBus *EventBus) *TaskTool {
	return &TaskTool{
		toolSvc:     toolSvc,
		modelSvc:    modelSvc,
		permSvc:     permSvc,
		dialogueSvc: dialogueSvc,
		eventBus:    eventBus,
		activeTasks: make(map[string]*TaskContext),
	}
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Definition() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "task",
			"description": "Launch a sub-agent to handle a specialized task autonomously. The sub-agent runs in isolation and returns a summary. Use this for: exploring codebases (@explore), researching complex questions (@general), or any task that would benefit from focused attention. Sub-agents cannot spawn further sub-agents.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"subagent_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of sub-agent to use",
						"enum":        []string{"general", "explore"},
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Clear description of what the sub-agent should accomplish",
					},
					"thoroughness": map[string]interface{}{
						"type":        "string",
						"description": "How thorough the sub-agent should be (for explore agent)",
						"enum":        []string{"quick", "medium", "thorough"},
					},
				},
				"required": []string{"subagent_type", "description"},
			},
		},
	}
}

func (t *TaskTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	subAgentType, _ := params["subagent_type"].(string)
	description, _ := params["description"].(string)

	if subAgentType == "" || description == "" {
		return nil, fmt.Errorf("subagent_type and description are required")
	}

	agentMode := AgentModeGeneral
	if subAgentType == "explore" {
		agentMode = AgentModeExplore
	}

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	taskCtx := &TaskContext{
		ID:           taskID,
		SubAgentType: agentMode,
		Description:  description,
		Status:       "running",
		StartedAt:    time.Now(),
	}
	t.mu.Lock()
	t.activeTasks[taskID] = taskCtx
	t.mu.Unlock()

	defer func() {
		now := time.Now()
		taskCtx.CompletedAt = &now
		taskCtx.Status = "completed"
	}()

	result, err := t.runSubAgent(ctx, taskCtx, agentMode, description, params)
	if err != nil {
		taskCtx.Status = "failed"
		return map[string]interface{}{
			"task_id": taskID,
			"status":  "failed",
			"error":   err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"task_id":    taskID,
		"status":     "completed",
		"subagent":   subAgentType,
		"result":     result,
		"tool_calls": len(taskCtx.ToolCalls),
		"duration":   time.Since(taskCtx.StartedAt).String(),
	}, nil
}

func (t *TaskTool) runSubAgent(ctx context.Context, taskCtx *TaskContext, agentMode AgentMode, description string, params map[string]interface{}) (string, error) {
	allToolDefs := t.toolSvc.GetToolDefinitionsWithMCP()
	filteredDefs := t.permSvc.FilterToolsForAgent(agentMode, allToolDefs)

	var taskToolDefs []llm.ToolDefinition
	for _, def := range filteredDefs {
		fnMap, _ := def["function"].(map[string]interface{})
		if fnMap == nil {
			continue
		}
		name, _ := fnMap["name"].(string)
		desc, _ := fnMap["description"].(string)
		paramsSchema, _ := fnMap["parameters"].(map[string]interface{})
		taskToolDefs = append(taskToolDefs, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        name,
				Description: desc,
				Parameters:  paramsSchema,
			},
		})
	}

	systemPrompt := t.buildSubAgentPrompt(agentMode, params)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: description},
	}

	modelID, err := t.selectModelForAgent(agentMode)
	if err != nil {
		return "", fmt.Errorf("failed to select model: %w", err)
	}

	maxRounds := 10
	if agentMode == AgentModeExplore {
		maxRounds = 15
	}

	for round := 0; round < maxRounds; round++ {
		resp, err := t.modelSvc.ChatWithTools(modelID, messages, taskToolDefs, map[string]interface{}{
			"max_tokens": 4000,
		})
		if err != nil {
			return "", fmt.Errorf("LLM call failed at round %d: %w", round, err)
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no response from LLM")
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   assistantMsg.Content,
			ToolCalls: assistantMsg.ToolCalls,
		})

		if len(assistantMsg.ToolCalls) == 0 {
			return assistantMsg.Content, nil
		}

		if len(assistantMsg.ToolCalls) > 1 {
			type toolResult struct {
				toolCallID string
				content    string
			}
			results := make([]toolResult, len(assistantMsg.ToolCalls))
			var wg sync.WaitGroup
			for i, tc := range assistantMsg.ToolCalls {
				wg.Add(1)
				go func(idx int, toolCall llm.ToolCall) {
					defer wg.Done()
					content := t.executeSubAgentTool(ctx, taskCtx, agentMode, toolCall)
					results[idx] = toolResult{toolCallID: toolCall.ID, content: content}
				}(i, tc)
			}
			wg.Wait()
			for _, r := range results {
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    r.content,
					ToolCallID: r.toolCallID,
				})
			}
		} else {
			tc := assistantMsg.ToolCalls[0]
			content := t.executeSubAgentTool(ctx, taskCtx, agentMode, tc)
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    content,
				ToolCallID: tc.ID,
			})
		}

		messages = compressToolOutputs(messages)
	}

	return "sub-agent reached maximum rounds", nil
}

func (t *TaskTool) executeSubAgentTool(ctx context.Context, taskCtx *TaskContext, agentMode AgentMode, toolCall llm.ToolCall) string {
	toolName := toolCall.Function.Name

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		args = make(map[string]interface{})
	}

	permName := PermissionNameForTool(toolName)
	target := extractTarget(toolName, args)

	check := t.permSvc.Check(ctx, taskCtx.ID, agentMode, permName, target)
	switch check.Action {
	case PermissionDeny:
		return fmt.Sprintf("Permission denied: %s (reason: %s)", toolName, check.Reason)
	case PermissionAsk:
		action := t.permSvc.AskAndWait(ctx, taskCtx.ID, permName, target)
		if action != PermissionAllow {
			return fmt.Sprintf("Permission denied by user: %s", toolName)
		}
	}

	startTime := time.Now()
	toolCallRecord := TaskToolCall{
		ToolName: toolName,
		Params:   args,
	}

	result, err := t.toolSvc.ExecuteTool(ctx, &models.ToolCall{
		ID:        toolCall.ID,
		Name:      toolName,
		Arguments: toolCall.Function.Arguments,
	}, "", "", "")

	toolCallRecord.Duration = time.Since(startTime)

	var content string
	if err != nil {
		content = fmt.Sprintf("Error: %s", err.Error())
		toolCallRecord.Result = content
	} else if result != nil {
		resultJSON, _ := json.Marshal(result.Content)
		content = string(resultJSON)
		if len(content) > 10000 {
			content = content[:10000] + "\n...[truncated, use read_file with offset/limit for full content]"
		}
		toolCallRecord.Result = content[:min(500, len(content))]
	}

	t.mu.Lock()
	taskCtx.ToolCalls = append(taskCtx.ToolCalls, toolCallRecord)
	t.mu.Unlock()

	return content
}

func (t *TaskTool) buildSubAgentPrompt(agentMode AgentMode, params map[string]interface{}) string {
	thoroughness, _ := params["thoroughness"].(string)
	if thoroughness == "" {
		thoroughness = "medium"
	}

	switch agentMode {
	case AgentModeExplore:
		return fmt.Sprintf(`You are a file search specialist. You excel at thoroughly navigating and exploring codebases.

Thoroughness level: %s

Your goal is to find relevant files, code, and patterns. Use grep, glob, list, and read tools to explore.
Be efficient - start broad, then narrow down.
Report your findings clearly with file paths and relevant code snippets.

IMPORTANT: You are a sub-agent. You cannot edit files or spawn further sub-agents.
When done, provide a clear summary of your findings.`, thoroughness)

	case AgentModeGeneral:
		return `You are a general-purpose research agent. You handle complex multi-step tasks autonomously.

Your capabilities:
- Read and search files
- Execute safe shell commands
- Search the web
- Analyze code and data

IMPORTANT: You are a sub-agent. You cannot edit files or spawn further sub-agents.
When done, provide a clear summary of your findings and conclusions.`

	default:
		return "You are a helpful sub-agent. Complete the task and return a summary."
	}
}

func (t *TaskTool) selectModelForAgent(agentMode AgentMode) (string, error) {
	models, err := t.modelSvc.ListModels()
	if err != nil || len(models) == 0 {
		return "", fmt.Errorf("no models available")
	}

	for _, m := range models {
		for _, tag := range m.Tags {
			tag = strings.TrimSpace(tag)
			if agentMode == AgentModeExplore && tag == "fast" {
				return m.ID, nil
			}
			if agentMode == AgentModeGeneral && tag == "code" {
				return m.ID, nil
			}
		}
	}

	return models[0].ID, nil
}

func (t *TaskTool) GetActiveTasks() []*TaskContext {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*TaskContext, 0, len(t.activeTasks))
	for _, tc := range t.activeTasks {
		result = append(result, tc)
	}
	return result
}

func extractTarget(toolName string, args map[string]interface{}) string {
	switch toolName {
	case "execute_command", "run_code":
		if cmd, ok := args["command"].(string); ok {
			return cmd
		}
	case "read_file", "write_file":
		if path, ok := args["path"].(string); ok {
			return path
		}
	case "http_request", "api_test":
		if url, ok := args["url"].(string); ok {
			return url
		}
	case "docker":
		if action, ok := args["action"].(string); ok {
			return action
		}
	case "git":
		if action, ok := args["action"].(string); ok {
			return action
		}
	}
	return "*"
}

func compressToolOutputs(messages []llm.Message) []llm.Message {
	const maxMessages = 30
	const keepRecent = 8
	if len(messages) <= maxMessages {
		return messages
	}

	compressed := make([]llm.Message, 0, len(messages))
	oldCount := len(messages) - keepRecent
	for i, msg := range messages {
		if i < oldCount && msg.Role == llm.RoleTool && len(msg.Content) > 200 {
			compressed = append(compressed, llm.Message{
				Role:       msg.Role,
				Content:    "[Old tool output cleared for context compression]",
				ToolCallID: msg.ToolCallID,
			})
		} else {
			compressed = append(compressed, msg)
		}
	}
	return compressed
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	log.Println("[TaskTool] Sub-agent delegation system initialized")
}
