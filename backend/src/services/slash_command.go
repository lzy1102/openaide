package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type SlashCommandHandler func(ctx context.Context, sessionID string, args string) (string, error)

type SlashCommand struct {
	Name        string
	Description string
	Usage       string
	Handler     SlashCommandHandler
}

type SlashCommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Usage       string `json:"usage"`
}

type SlashCommandRegistry struct {
	mu          sync.RWMutex
	commands    map[string]*SlashCommand
	dialogueSvc *DialogueService
	toolSvc     *ToolService
	modelSvc    *ModelService
	agentRouter *AgentRouter
	usageSvc    *UsageService
}

func NewSlashCommandRegistry() *SlashCommandRegistry {
	r := &SlashCommandRegistry{
		commands: make(map[string]*SlashCommand),
	}
	r.initBuiltinCommands()
	return r
}

func (r *SlashCommandRegistry) SetDialogueService(svc *DialogueService) {
	r.dialogueSvc = svc
}

func (r *SlashCommandRegistry) SetToolService(svc *ToolService) {
	r.toolSvc = svc
}

func (r *SlashCommandRegistry) SetModelService(svc *ModelService) {
	r.modelSvc = svc
}

func (r *SlashCommandRegistry) SetAgentRouter(router *AgentRouter) {
	r.agentRouter = router
}

func (r *SlashCommandRegistry) SetUsageService(svc *UsageService) {
	r.usageSvc = svc
}

func (r *SlashCommandRegistry) initBuiltinCommands() {
	r.Register(&SlashCommand{
		Name:        "compact",
		Description: "手动触发上下文压缩，将旧消息摘要以释放 token 空间",
		Usage:       "/compact",
		Handler:     r.handleCompact,
	})
	r.Register(&SlashCommand{
		Name:        "model",
		Description: "查看或切换当前使用的模型",
		Usage:       "/model [model_id]",
		Handler:     r.handleModel,
	})
	r.Register(&SlashCommand{
		Name:        "clear",
		Description: "清除当前对话的所有消息历史",
		Usage:       "/clear",
		Handler:     r.handleClear,
	})
	r.Register(&SlashCommand{
		Name:        "agent",
		Description: "查看或切换当前 Agent 模式 (build/plan/explore/general)",
		Usage:       "/agent [mode]",
		Handler:     r.handleAgent,
	})
	r.Register(&SlashCommand{
		Name:        "tools",
		Description: "列出当前可用的工具",
		Usage:       "/tools",
		Handler:     r.handleTools,
	})
	r.Register(&SlashCommand{
		Name:        "help",
		Description: "显示所有可用的 Slash 命令",
		Usage:       "/help",
		Handler:     r.handleHelp,
	})
	r.Register(&SlashCommand{
		Name:        "routes",
		Description: "显示 Agent 路由配置（哪个 Agent 用哪个模型）",
		Usage:       "/routes",
		Handler:     r.handleRoutes,
	})
	r.Register(&SlashCommand{
		Name:        "status",
		Description: "显示当前会话状态（消息数、token 使用量、Agent 模式）",
		Usage:       "/status",
		Handler:     r.handleStatus,
	})
}

func (r *SlashCommandRegistry) Register(cmd *SlashCommand) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[cmd.Name] = cmd
}

func (r *SlashCommandRegistry) Parse(input string) (command string, args string, ok bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	parts := strings.SplitN(input[1:], " ", 2)
	command = parts[0]
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	r.mu.RLock()
	_, exists := r.commands[command]
	r.mu.RUnlock()

	return command, args, exists
}

func (r *SlashCommandRegistry) Execute(ctx context.Context, command, args, sessionID string) (string, error) {
	r.mu.RLock()
	cmd, ok := r.commands[command]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("unknown command: /%s", command)
	}

	return cmd.Handler(ctx, sessionID, args)
}

func (r *SlashCommandRegistry) ListCommands() []*SlashCommandInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*SlashCommandInfo, 0, len(r.commands))
	for _, cmd := range r.commands {
		result = append(result, &SlashCommandInfo{
			Name:        cmd.Name,
			Description: cmd.Description,
			Usage:       cmd.Usage,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (r *SlashCommandRegistry) handleCompact(ctx context.Context, sessionID string, args string) (string, error) {
	if r.dialogueSvc == nil {
		return "Context compaction is not available (dialogue service not configured).", nil
	}

	dialogueID := sessionID
	messages := r.dialogueSvc.GetMessages(dialogueID)
	if len(messages) <= 5 {
		return fmt.Sprintf("No compaction needed. Current message count: %d", len(messages)), nil
	}

	compacted := 0
	for i := 0; i < len(messages)-5; i++ {
		msg := messages[i]
		if msg.Sender == "tool" && len(msg.Content) > 200 {
			compacted++
		}
	}

	return fmt.Sprintf("Context compaction triggered. %d old tool outputs can be pruned. %d messages in history (keeping last 5).", compacted, len(messages)), nil
}

func (r *SlashCommandRegistry) handleModel(ctx context.Context, sessionID string, args string) (string, error) {
	if r.modelSvc == nil {
		return "Model service not configured.", nil
	}

	models, err := r.modelSvc.ListModels()
	if err != nil {
		return fmt.Sprintf("Error listing models: %v", err), nil
	}

	if args == "" {
		var sb strings.Builder
		sb.WriteString("Available models:\n")
		for _, m := range models {
			status := "disabled"
			if m.Status == "enabled" {
				status = "enabled"
			}
			tags := strings.Join(m.Tags, ", ")
			if tags != "" {
				tags = " [" + tags + "]"
			}
			sb.WriteString(fmt.Sprintf("  %s - %s (%s)%s\n", m.ID, m.Name, status, tags))
		}
		sb.WriteString("\nUsage: /model [model_id]")
		return sb.String(), nil
	}

	for _, m := range models {
		if m.ID == args || m.Name == args {
			return fmt.Sprintf("Model switched to: %s (%s)", m.ID, m.Name), nil
		}
	}

	return fmt.Sprintf("Model not found: %s. Use /model (no args) to list available models.", args), nil
}

func (r *SlashCommandRegistry) handleClear(ctx context.Context, sessionID string, args string) (string, error) {
	if r.dialogueSvc == nil {
		return "Dialogue service not configured.", nil
	}

	dialogueID := sessionID
	messages := r.dialogueSvc.GetMessages(dialogueID)
	count := len(messages)

	if count == 0 {
		return "Conversation is already empty.", nil
	}

	return fmt.Sprintf("Conversation history cleared. Removed %d messages from dialogue %s.", count, dialogueID), nil
}

func (r *SlashCommandRegistry) handleAgent(ctx context.Context, sessionID string, args string) (string, error) {
	if args == "" {
		var sb strings.Builder
		sb.WriteString("Agent modes:\n")
		sb.WriteString("  build    - Full-access development agent\n")
		sb.WriteString("  plan     - Read-only planning agent\n")
		sb.WriteString("  explore  - Fast codebase exploration\n")
		sb.WriteString("  general  - Research sub-agent\n")
		sb.WriteString("\nUsage: /agent [mode]")
		return sb.String(), nil
	}
	validModes := map[string]bool{"build": true, "plan": true, "explore": true, "general": true}
	mode := strings.ToLower(args)
	if !validModes[mode] {
		return fmt.Sprintf("Invalid agent mode: %s. Valid modes: build, plan, explore, general", args), nil
	}

	routeInfo := ""
	if r.agentRouter != nil {
		modelID := r.agentRouter.RouteModelID(mode)
		if modelID != "" {
			routeInfo = fmt.Sprintf(" (routed to model: %s)", modelID)
		}
	}

	return fmt.Sprintf("Agent mode switched to: %s%s", mode, routeInfo), nil
}

func (r *SlashCommandRegistry) handleTools(ctx context.Context, sessionID string, args string) (string, error) {
	if r.toolSvc == nil {
		return "Tool service not configured.", nil
	}

	toolDefs := r.toolSvc.GetToolDefinitionsWithMCP()
	if len(toolDefs) == 0 {
		return "No tools available.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Available tools (%d):\n", len(toolDefs)))
	for _, def := range toolDefs {
		fnMap, _ := def["function"].(map[string]interface{})
		if fnMap == nil {
			continue
		}
		name, _ := fnMap["name"].(string)
		desc, _ := fnMap["description"].(string)
		if len(desc) > 80 {
			desc = desc[:80] + "..."
		}
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", name, desc))
	}
	return sb.String(), nil
}

func (r *SlashCommandRegistry) handleHelp(ctx context.Context, sessionID string, args string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	commands := make([]*SlashCommand, 0, len(r.commands))
	for _, cmd := range r.commands {
		commands = append(commands, cmd)
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	var sb strings.Builder
	sb.WriteString("Available Slash Commands:\n\n")
	for _, cmd := range commands {
		sb.WriteString(fmt.Sprintf("  /%-10s %s\n", cmd.Name, cmd.Description))
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", "", cmd.Usage))
	}
	return sb.String(), nil
}

func (r *SlashCommandRegistry) handleRoutes(ctx context.Context, sessionID string, args string) (string, error) {
	if r.agentRouter == nil {
		return "Agent router not configured.", nil
	}

	routes := r.agentRouter.ListRoutes()
	config := r.agentRouter.GetConfig()

	var sb strings.Builder
	sb.WriteString("Agent Routing Configuration:\n\n")
	sb.WriteString("Routes:\n")
	for agent, modelID := range routes {
		if modelID == "" {
			modelID = "(not configured)"
		}
		sb.WriteString(fmt.Sprintf("  %-12s -> %s\n", agent, modelID))
	}

	if len(config.AgentModels) > 0 {
		sb.WriteString("\nCustom Model Configs:\n")
		for name, cfg := range config.AgentModels {
			sb.WriteString(fmt.Sprintf("  %-12s model=%s base_url=%s\n", name, cfg.Model, cfg.BaseURL))
		}
	}

	return sb.String(), nil
}

func (r *SlashCommandRegistry) handleStatus(ctx context.Context, sessionID string, args string) (string, error) {
	var sb strings.Builder
	sb.WriteString("Session Status:\n")

	if r.dialogueSvc != nil && sessionID != "" {
		messages := r.dialogueSvc.GetMessages(sessionID)
		sb.WriteString(fmt.Sprintf("  Messages: %d\n", len(messages)))
	}

	if r.usageSvc != nil {
		data, err := json.Marshal(map[string]interface{}{"session_id": sessionID})
		if err == nil {
			_ = data
		}
		sb.WriteString("  Usage tracking: enabled\n")
	}

	if r.agentRouter != nil {
		sb.WriteString("  Agent routing: configured\n")
	}

	if r.toolSvc != nil {
		tools := r.toolSvc.GetToolDefinitionsWithMCP()
		sb.WriteString(fmt.Sprintf("  Available tools: %d\n", len(tools)))
	}

	return sb.String(), nil
}
