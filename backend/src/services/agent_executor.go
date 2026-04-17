package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// AgentExecutor Agent 执行引擎
// 将 LLM 推理与工具调用结合，实现真正的任务执行闭环
type AgentExecutor struct {
	modelSvc       *ModelService
	toolSvc        *ToolService
	logger         *LoggerService
	maxRounds      int
	currentTaskID  string // 当前执行的任务 ID，用于确认关联
}

// TaskExecRequest 任务执行请求
type TaskExecRequest struct {
	TaskID          string                 `json:"task_id"`
	TaskTitle       string                 `json:"task_title"`
	TaskDescription string                 `json:"task_description"`
	AgentName       string                 `json:"agent_name"`
	AgentRole       string                 `json:"agent_role"`
	AgentPrompt     string                 `json:"agent_prompt"`
	ModelID         string                 `json:"model_id,omitempty"`
	Context         map[string]interface{} `json:"context,omitempty"`
	TeamGoal        string                 `json:"team_goal,omitempty"`
}

// TaskExecResult 任务执行结果
type TaskExecResult struct {
	Success    bool          `json:"success"`
	Output     string        `json:"output"`
	Summary    string        `json:"summary"`
	ToolCalls  int           `json:"tool_calls"`
	TokensUsed int           `json:"tokens_used"`
	Duration   time.Duration `json:"duration"`
}

// NewAgentExecutor 创建 Agent 执行引擎
func NewAgentExecutor(modelSvc *ModelService, toolSvc *ToolService, logger *LoggerService) *AgentExecutor {
	return &AgentExecutor{
		modelSvc:  modelSvc,
		toolSvc:   toolSvc,
		logger:    logger,
		maxRounds: 10,
	}
}

// Execute 执行任务：LLM 推理 + 工具调用循环
func (e *AgentExecutor) Execute(ctx context.Context, req *TaskExecRequest) (*TaskExecResult, error) {
	start := time.Now()
	e.currentTaskID = req.TaskID

	// 1. 确定使用的模型
	modelID := req.ModelID
	if modelID == "" {
		defaultModel, err := e.modelSvc.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no model available: %w", err)
		}
		modelID = defaultModel.ID
	}

	// 2. 获取工具定义（包含 MCP 工具）
	toolDefs := e.toolSvc.GetToolDefinitionsWithMCP()
	llmTools := e.convertToolDefs(toolDefs)

	// 3. 构建 system prompt
	systemPrompt := e.buildSystemPrompt(req)

	// 4. 构建 user 消息
	userContent := req.TaskDescription

	// 如果有前置上下文，附加到 user 消息
	if len(req.Context) > 0 {
		ctxJSON, _ := json.MarshalIndent(req.Context, "", "  ")
		userContent += fmt.Sprintf("\n\n前置任务输出：\n%s", string(ctxJSON))
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	e.logger.Info(ctx, "[AgentExecutor] %s starting task: %s (model=%s, tools=%d)",
		req.AgentName, req.TaskTitle, modelID, len(llmTools))

	options := map[string]interface{}{
		"temperature": 0.7,
	}

	totalToolCalls := 0
	totalTokens := 0

	// 5. 根据是否有工具选择执行模式
	if len(llmTools) > 0 {
		// 有工具：tool calling 循环
		for round := 0; round < e.maxRounds; round++ {
			resp, err := e.modelSvc.ChatWithTools(modelID, messages, llmTools, options)
			if err != nil {
				return nil, fmt.Errorf("LLM call failed (round %d): %w", round+1, err)
			}

			if len(resp.Choices) == 0 {
				return nil, fmt.Errorf("empty LLM response")
			}

			choice := resp.Choices[0]
			assistantMsg := choice.Message

			// 统计 token
			if resp.Usage != nil {
				totalTokens += resp.Usage.TotalTokens
			}

			// 追加 assistant 消息
			messages = append(messages, assistantMsg)

			// 无工具调用 → 返回文本结果
			if len(assistantMsg.ToolCalls) == 0 {
				output := assistantMsg.Content
				if output == "" {
					output = "(无回复内容)"
				}
				e.logger.Info(ctx, "[AgentExecutor] %s completed task: %s (rounds=%d, tool_calls=%d, tokens=%d)",
					req.AgentName, req.TaskTitle, round+1, totalToolCalls, totalTokens)

				return &TaskExecResult{
					Success:    true,
					Output:     output,
					Summary:    summarizeOutput(output),
					ToolCalls:  totalToolCalls,
					TokensUsed: totalTokens,
					Duration:   time.Since(start),
				}, nil
			}

			// 执行工具调用
			e.logger.Info(ctx, "[AgentExecutor] %s round %d: %d tool calls",
				req.AgentName, round+1, len(assistantMsg.ToolCalls))

			for _, tc := range assistantMsg.ToolCalls {
				toolResult := e.executeToolCall(ctx, tc)
				totalToolCalls++

				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    toolResult,
					ToolCallID: tc.ID,
				})
			}
		}

		// 超出最大轮次
		lastMsg := messages[len(messages)-1]
		e.logger.Warn(ctx, "[AgentExecutor] %s exceeded max rounds for task: %s",
			req.AgentName, req.TaskTitle)

		return &TaskExecResult{
			Success:    true,
			Output:     lastMsg.Content,
			Summary:    summarizeOutput(lastMsg.Content),
			ToolCalls:  totalToolCalls,
			TokensUsed: totalTokens,
			Duration:   time.Since(start),
		}, nil
	}

	// 无工具：直接 LLM 调用
	resp, err := e.modelSvc.Chat(modelID, messages, options)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty LLM response")
	}

	output := resp.Choices[0].Message.Content
	if resp.Usage != nil {
		totalTokens = resp.Usage.TotalTokens
	}

	e.logger.Info(ctx, "[AgentExecutor] %s completed task: %s (no tools, tokens=%d)",
		req.AgentName, req.TaskTitle, totalTokens)

	return &TaskExecResult{
		Success:    true,
		Output:     output,
		Summary:    summarizeOutput(output),
		ToolCalls:  0,
		TokensUsed: totalTokens,
		Duration:   time.Since(start),
	}, nil
}

// buildSystemPrompt 构建系统提示词
func (e *AgentExecutor) buildSystemPrompt(req *TaskExecRequest) string {
	prompt := fmt.Sprintf("你是 %s，角色是 %s。\n", req.AgentName, req.AgentRole)

	if req.AgentPrompt != "" {
		prompt += req.AgentPrompt + "\n"
	}

	if req.TeamGoal != "" {
		prompt += fmt.Sprintf("\n团队总体目标：%s\n", req.TeamGoal)
	}

	prompt += fmt.Sprintf("\n当前任务：%s\n", req.TaskTitle)
	prompt += "请完成上述任务。如果需要使用工具来完成任务，请调用相应的工具。完成后请给出清晰的结果总结。"

	return prompt
}

// convertToolDefs 转换工具定义为 LLM 格式
func (e *AgentExecutor) convertToolDefs(toolDefs []map[string]interface{}) []llm.ToolDefinition {
	llmTools := make([]llm.ToolDefinition, 0, len(toolDefs))
	for _, def := range toolDefs {
		fnMap, _ := def["function"].(map[string]interface{})
		if fnMap == nil {
			continue
		}
		name, _ := fnMap["name"].(string)
		desc, _ := fnMap["description"].(string)
		params, _ := fnMap["parameters"].(map[string]interface{})

		llmTools = append(llmTools, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}
	return llmTools
}

// executeToolCall 执行单个工具调用
func (e *AgentExecutor) executeToolCall(ctx context.Context, tc llm.ToolCall) string {
	toolCall := &models.ToolCall{
		ID:        tc.ID,
		Name:      tc.Function.Name,
		Arguments: tc.Function.Arguments,
	}

	e.logger.Info(ctx, "[AgentExecutor] executing tool: %s", tc.Function.Name)

	result, err := e.toolSvc.ExecuteTool(ctx, toolCall, "", "", "")
	if err != nil {
		// 检查是否为需要确认的危险命令
		var confErr *ConfirmationRequiredError
		if errors.As(err, &confErr) {
			return e.handleConfirmationRequest(ctx, confErr)
		}

		errMsg := fmt.Sprintf("Tool execution error: %v", err)
		e.logger.Error(ctx, "[AgentExecutor] tool %s failed: %v", tc.Function.Name, err)
		return errMsg
	}

	resultJSON, err := json.Marshal(result.Content)
	if err != nil {
		return fmt.Sprintf("%v", result.Content)
	}

	return string(resultJSON)
}

// handleConfirmationRequest 处理危险命令确认请求
func (e *AgentExecutor) handleConfirmationRequest(ctx context.Context, confErr *ConfirmationRequiredError) string {
	e.logger.Warn(ctx, "[AgentExecutor] Dangerous command requires confirmation: %s (risk: %s)",
		confErr.Command, confErr.Risk)

	// 请求用户确认（阻塞等待）
	approved, err := e.toolSvc.RequestCommandConfirmation(ctx, confErr, e.currentTaskID)
	if err != nil {
		return fmt.Sprintf("Command confirmation failed: %v (command: %s)", err, confErr.Command)
	}

	if !approved {
		e.logger.Info(ctx, "[AgentExecutor] Command rejected by user: %s", confErr.Command)
		return fmt.Sprintf("Command rejected by user: %s", confErr.Command)
	}

	e.logger.Info(ctx, "[AgentExecutor] Command approved by user: %s", confErr.Command)

	// 重新执行已批准的命令
	toolCall := &models.ToolCall{
		ID:        confErr.ID,
		Name:      "execute_command",
		Arguments: fmt.Sprintf(`{"command":"%s","approved":true}`, confErr.Command),
	}

	result, err := e.toolSvc.ExecuteTool(ctx, toolCall, "", "", "")
	if err != nil {
		return fmt.Sprintf("Command execution failed after approval: %v", err)
	}

	resultJSON, err := json.Marshal(result.Content)
	if err != nil {
		return fmt.Sprintf("%v", result.Content)
	}

	return string(resultJSON)
}

// summarizeOutput 截断过长的输出
func summarizeOutput(output string) string {
	if len(output) > 200 {
		return output[:200] + "..."
	}
	return output
}
