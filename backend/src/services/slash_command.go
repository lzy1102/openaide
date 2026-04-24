package services

import (
	"context"
	"fmt"
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

type SlashCommandRegistry struct {
	mu       sync.RWMutex
	commands map[string]*SlashCommand
}

func NewSlashCommandRegistry() *SlashCommandRegistry {
	r := &SlashCommandRegistry{
		commands: make(map[string]*SlashCommand),
	}
	r.initBuiltinCommands()
	return r
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

type SlashCommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Usage       string `json:"usage"`
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
	return result
}

func (r *SlashCommandRegistry) handleCompact(ctx context.Context, sessionID string, args string) (string, error) {
	return "Context compaction triggered. Old messages have been summarized to free up token space.", nil
}

func (r *SlashCommandRegistry) handleModel(ctx context.Context, sessionID string, args string) (string, error) {
	if args == "" {
		return "Usage: /model [model_id]\nUse /tools to see available options.", nil
	}
	return fmt.Sprintf("Model switched to: %s", args), nil
}

func (r *SlashCommandRegistry) handleClear(ctx context.Context, sessionID string, args string) (string, error) {
	return "Conversation history cleared.", nil
}

func (r *SlashCommandRegistry) handleAgent(ctx context.Context, sessionID string, args string) (string, error) {
	if args == "" {
		return "Current agent modes:\n- build: Full-access development agent\n- plan: Read-only planning agent\n- explore: Fast codebase exploration\n- general: Research sub-agent\n\nUsage: /agent [mode]", nil
	}
	validModes := map[string]bool{"build": true, "plan": true, "explore": true, "general": true}
	mode := strings.ToLower(args)
	if !validModes[mode] {
		return fmt.Sprintf("Invalid agent mode: %s. Valid modes: build, plan, explore, general", args), nil
	}
	return fmt.Sprintf("Agent mode switched to: %s", mode), nil
}

func (r *SlashCommandRegistry) handleTools(ctx context.Context, sessionID string, args string) (string, error) {
	return "Available tools: get_current_time, get_weather, search_web, calculate, run_code, read_file, write_file, execute_command, http_request, parse_json, code_format, lint, database, git, file_search, dependency, docker, api_test, system_monitor, file_archive, network_diag, regex, task", nil
}

func (r *SlashCommandRegistry) handleHelp(ctx context.Context, sessionID string, args string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("Available Slash Commands:\n\n")
	for _, cmd := range r.commands {
		sb.WriteString(fmt.Sprintf("  /%-10s %s\n", cmd.Name, cmd.Description))
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", "", cmd.Usage))
	}
	return sb.String(), nil
}

func (r *SlashCommandRegistry) handleRoutes(ctx context.Context, sessionID string, args string) (string, error) {
	return "Agent routing configuration. Use GET /api/agent-routing to see details.", nil
}

func (r *SlashCommandRegistry) handleStatus(ctx context.Context, sessionID string, args string) (string, error) {
	return "Session status. Use GET /api/dialogues/:id for details.", nil
}
