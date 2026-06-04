package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"openaide/backend/internal/api"
	"openaide/backend/internal/auth"
	"openaide/backend/internal/channel"
	"openaide/backend/internal/config"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/llm"
	"openaide/backend/internal/lsp"
	"openaide/backend/internal/mcp"
	"openaide/backend/internal/memory"
	"openaide/backend/internal/orchestration"
	"openaide/backend/internal/plugin"
	"openaide/backend/internal/projectmind"
	"openaide/backend/internal/tools"
)

// Application 应用容器
type Application struct {
	Config             *config.Config
	Kernel             kernel.Kernel
	Orchestrator       *orchestration.Orchestrator
	APIServer          *api.Server
	LLMGateway         *llm.Gateway
	ChannelRegistry    *channel.Registry
	TaskQueue          *channel.TaskQueue
	PluginManager      *plugin.Manager
	MCPManager    *mcp.Manager
	sessionActor  *kernel.SessionActor // CSP actor, owns all session state
	pluginWatcher *PluginWatcher       // hot-reload
}

// NewApplication 创建应用容器
func NewApplication(cfg *config.Config) (*Application, error) {
	app := &Application{Config: cfg}

	// 1. LLM 网关
	gateway := createLLMGateway(cfg)
	app.LLMGateway = gateway

	// 2. Embedder（用于记忆和知识库语义搜索）
	embedderDim := 1536
	embedder := llm.NewEmbedderFunc(gateway.Embed, gateway.EmbedBatch, embedderDim)

	// 3. 工具注册表 + MCP
	tools.LoadIgnorePatterns(".")
	tools.SetBrowserEnabled(cfg.Browser.Enabled)
	if cfg.Search.SearXNGURL != "" {
		tools.SetSearXNGURL(cfg.Search.SearXNGURL)
	}
	toolRegistry := tools.NewRegistry()
	if err := tools.RegisterBuiltins(toolRegistry); err != nil {
		return nil, fmt.Errorf("register builtin tools failed: %w", err)
	}
	mcpManager := mcp.NewManager()
	if cfg.MCP.Enabled {
		for _, srvCfg := range cfg.MCP.Servers {
			slog.Info("Connecting MCP server", "id", srvCfg.ID, "command", srvCfg.Command)
			if err := mcpManager.ConnectServer(srvCfg.ID, srvCfg.Command, srvCfg.Args...); err != nil {
				slog.Warn("Failed to connect MCP server, skipping", "id", srvCfg.ID, "error", err)
				continue
			}
			for _, mcpTool := range mcpManager.GetServerTools(srvCfg.ID) {
				def := kernel.ToolDefinition{
					Type: "function",
					Function: kernel.FunctionDef{
						Name:        mcpTool.Name,
						Description: mcpTool.Description,
						Parameters:  mcpTool.InputSchema,
					},
				}
				serverID, toolName := srvCfg.ID, mcpTool.Name
				handler := kernel.ToolHandler(func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
					var args map[string]interface{}
					if arguments != "" {
						if err := json.Unmarshal([]byte(arguments), &args); err != nil {
							return &kernel.ToolResult{Error: fmt.Sprintf("invalid args: %v", err)}, nil
						}
					}
					result, err := mcpManager.CallTool(serverID, toolName, args)
					if err != nil {
						return &kernel.ToolResult{Error: err.Error()}, nil
					}
					var content string
					for _, item := range result.Content {
						if item.Type == "text" && item.Text != "" {
							if content != "" {
								content += "\n"
							}
							content += item.Text
						}
					}
					errStr := ""
					if result.IsError {
						errStr = content
					}
					return &kernel.ToolResult{Content: content, Error: errStr}, nil
				})
				if err := toolRegistry.Register(def, handler); err != nil {
					slog.Warn("MCP tool registration skipped, duplicate name", "tool", mcpTool.Name, "error", err)
				} else {
					slog.Info("MCP tool registered", "server", srvCfg.ID, "tool", mcpTool.Name)
				}
			}
		}
	}
	// Claude 插件中的 .mcp.json
	if cfg.MCP.Enabled {
		for name, srv := range plugin.DiscoverClaudeMCP(cfg.Storage.DataDir + "/plugins") {
			if srv.Type != "stdio" && srv.Type != "" {
				slog.Debug("MCP server type not supported, skipping", "name", name, "type", srv.Type)
				continue
			}
			cmd := srv.Command
			if cmd == "" {
				slog.Warn("MCP server missing command, skipping", "name", name)
				continue
			}
			slog.Info("Connecting MCP server (Claude plugin)", "name", name, "command", cmd)
			if err := mcpManager.ConnectServer(name, cmd, srv.Args...); err != nil {
				slog.Warn("Failed to connect Claude MCP server, skipping", "name", name, "error", err)
				continue
			}
			for _, mcpTool := range mcpManager.GetServerTools(name) {
				def := kernel.ToolDefinition{
					Type: "function",
					Function: kernel.FunctionDef{
						Name:        mcpTool.Name,
						Description: mcpTool.Description,
						Parameters:  mcpTool.InputSchema,
					},
				}
				serverID, toolName := name, mcpTool.Name
				handler := kernel.ToolHandler(func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
					var args map[string]interface{}
					if arguments != "" {
						if err := json.Unmarshal([]byte(arguments), &args); err != nil {
							return &kernel.ToolResult{Error: fmt.Sprintf("invalid args: %v", err)}, nil
						}
					}
					result, err := mcpManager.CallTool(serverID, toolName, args)
					if err != nil {
						return &kernel.ToolResult{Error: err.Error()}, nil
					}
					var content string
					for _, item := range result.Content {
						if item.Type == "text" && item.Text != "" {
							if content != "" {
								content += "\n"
							}
							content += item.Text
						}
					}
					errStr := ""
					if result.IsError {
						errStr = content
					}
					return &kernel.ToolResult{Content: content, Error: errStr}, nil
				})
				if err := toolRegistry.Register(def, handler); err != nil {
					slog.Warn("Claude MCP tool registration skipped, duplicate", "tool", mcpTool.Name, "error", err)
				} else {
					slog.Info("Claude MCP tool registered", "server", name, "tool", mcpTool.Name)
				}
			}
		}
	}

	app.MCPManager = mcpManager

	// 4. 记忆管理器（CSP actor）
	memManager, err := memory.NewMemoryActor(cfg.Storage.DataDir + "/memory.db")
	if err != nil {
		return nil, fmt.Errorf("memory actor: %w", err)
	}
	memManager.SetEmbedder(embedder)
	tools.SetMemoryManager(memManager)

	// 5. 会话存储（CSP actor）
	sessionStore, err := kernel.NewSessionActor(cfg.Storage.DataDir + "/sessions.db")
	if err != nil {
		return nil, fmt.Errorf("session actor: %w", err)
	}
	app.sessionActor = sessionStore

	// 6. 内核 + 所有增强能力
	agentKernel, pluginMgr, err := createKernel(cfg, gateway, embedder, toolRegistry, memManager, sessionStore)
	if err != nil {
		return nil, fmt.Errorf("create kernel: %w", err)
	}
	app.Kernel = agentKernel
	app.PluginManager = pluginMgr

	// Plugin hot-reload
	pluginWatcher := NewPluginWatcher(cfg.Storage.DataDir+"/plugins", pluginMgr.Reload)
	if err := pluginWatcher.Start(); err != nil {
		slog.Warn("Plugin hot-reload unavailable", "error", err)
	} else {
		app.pluginWatcher = pluginWatcher
	}

	// LSP: auto-start language servers for the current project
	startLSPServers()

	// 7. 编排器
	orch := orchestration.NewOrchestrator(agentKernel, gateway, toolRegistry, memManager, sessionStore)
	if cfg.Planning.PreviewTimeout > 0 {
		orch.PreviewTimeout = time.Duration(cfg.Planning.PreviewTimeout) * time.Second
	}
	if cfg.Planning.DeepTimeout > 0 {
		orch.DeepTimeout = time.Duration(cfg.Planning.DeepTimeout) * time.Second
	}
	orch.SetTeam(orchestration.NewTeam(orch))
	// 加载项目持久记忆
	pm := projectmind.Load(".")
	orch.SetProjectMind(pm)
	if pm.HasHighConfidenceConventions() {
		pm.SyncToSystemPrompt(cfg.Storage.DataDir+"/prompts", cfg.Lang)
	}
	// 配置模型路由
	orch.ModelRouting = orchestration.ModelRouting{
		Reasoning: cfg.LLM.ModelRouting.Reasoning,
		Execution: cfg.LLM.ModelRouting.Execution,
	}
	app.Orchestrator = orch

	// 8. API 服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	var authSvc *auth.Service
	if os.Getenv("OPENAIDE_AUTH") == "true" {
		authSvc = auth.NewService(os.Getenv("OPENAIDE_JWT_SECRET"))
	}
	app.APIServer = api.NewServer(orch, addr, authSvc)

	// 9. 渠道层（外部消息接入）
	if err := setupChannels(app, cfg, orch, pluginMgr); err != nil {
		return nil, err
	}

	// 10. OpenCode 配置兼容：发现 opencode.json 中的 MCP + instructions
	if opencodeCfg, opencodeMCP, opencodeInstr := plugin.DiscoverOpenCodeConfig("."); opencodeCfg != nil {
		slog.Info("OpenCode config discovered", "mcp_servers", len(opencodeMCP))
		for _, srv := range opencodeMCP {
			if srv.Command != "" {
				id := "opencode-" + srv.Command
				if err := mcpManager.ConnectServer(id, srv.Command, srv.Args...); err != nil {
					slog.Warn("OpenCode MCP connect failed", "id", id, "error", err)
					continue
				}
				for _, mcpTool := range mcpManager.GetServerTools(id) {
					def := kernel.ToolDefinition{Type: "function", Function: kernel.FunctionDef{
						Name: mcpTool.Name, Description: mcpTool.Description, Parameters: mcpTool.InputSchema,
					}}
					serverID, toolName := id, mcpTool.Name
					toolRegistry.Register(def, kernel.ToolHandler(func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
						var args map[string]interface{}
						if arguments != "" { json.Unmarshal([]byte(arguments), &args) }
						result, _ := mcpManager.CallTool(serverID, toolName, args)
						if result == nil { return &kernel.ToolResult{Error: "tool call failed"}, nil }
						var content string
						for _, item := range result.Content {
							if item.Type == "text" && item.Text != "" {
								if content != "" { content += "\n" }
								content += item.Text
							}
						}
						errStr := ""
						if result.IsError { errStr = content }
						return &kernel.ToolResult{Content: content, Error: errStr}, nil
					}))
				}
				slog.Info("OpenCode MCP connected", "id", id)
			}
		}
		if opencodeInstr != "" {
			currentPrompt := kernel.LoadSystemPrompt(cfg.Storage.DataDir + "/prompts")
			agentKernel.SetSystemPrompt(currentPrompt + "\n\n## OpenCode 项目指令\n" + opencodeInstr)
		}
	}

	return app, nil
}

// Start 启动应用
func (app *Application) Start() error {
	switch app.Config.Server.Mode {
	case "server":
		return app.APIServer.Start()
	case "direct", "":
		slog.Info("Direct mode - API server not started")
		return nil
	default:
		return fmt.Errorf("unknown server mode: %s", app.Config.Server.Mode)
	}
}

// Stop 停止应用（优雅关闭所有组件）
func (app *Application) Stop(ctx context.Context) error {
	app.Orchestrator.CleanupOldSessions(ctx)
	if app.pluginWatcher != nil {
		app.pluginWatcher.Stop()
	}
	if app.sessionActor != nil {
		if err := app.sessionActor.Stop(); err != nil {
			slog.Error("Failed to stop session actor", "error", err)
		}
	}
	if app.MCPManager != nil {
		app.MCPManager.Shutdown()
	}
	if app.LLMGateway != nil {
		app.LLMGateway.Shutdown()
	}
	tools.ShutdownBrowser()
	if app.TaskQueue != nil {
		if err := app.TaskQueue.Stop(ctx); err != nil {
			slog.Error("Failed to stop task queue", "error", err)
		}
	}
	if app.ChannelRegistry != nil {
		if err := app.ChannelRegistry.StopAll(ctx); err != nil {
			slog.Error("Failed to stop channels", "error", err)
		}
	}
	if app.APIServer != nil {
		if err := app.APIServer.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

// startLSPServers auto-starts language servers for the current project.
func startLSPServers() {
	cwd, _ := os.Getwd()
	entries, _ := os.ReadDir(cwd)

	// Project type → language mapping. Only start one server per project.
	detectors := map[string]string{
		"go.mod":          "go",
		"Cargo.toml":      "rust",
		"CMakeLists.txt":  "cpp",
		"build.zig":       "zig",
		"pyproject.toml":  "python",
		"setup.py":        "python",
		"requirements.txt": "python",
		"Gemfile":         "ruby",
		"composer.json":   "php",
		"pom.xml":         "java",
		"build.gradle":    "java",
		"build.gradle.kts": "kotlin",
		"build.sbt":       "scala",
		"package.json":    "typescript",
		".csproj":         "csharp",
		"Package.swift":   "swift",

		"mix.exs":         "elixir",
		"rebar.config":    "erlang",
		"stack.yaml":      "haskell",
		"pubspec.yaml":    "dart",
		".Rproj":          "r",
		"Project.toml":    "julia",
	}

	for _, e := range entries {
		name := e.Name()
		for indicator, lang := range detectors {
			if name == indicator && !e.IsDir() {
				if c, err := lsp.Start(cwd, lang); err == nil {
					tools.SetLSPClient(lang, c)
				} else {
					slog.Debug("LSP server not available", "lang", lang, "error", err)
				}
				return // Only one LSP per project
			}
		}
	}
}

// TUILogWriter TUI 日志环缓冲（由 cmd/cli 设置）
var TUILogWriter io.Writer

// InitLogger 初始化日志（stderr + 文件 + TUI buffer）
func InitLogger(level, format string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// 日志文件
	logDir := os.Getenv("HOME") + "/.openaide/logs"
	os.MkdirAll(logDir, 0755)
	logFile, err := os.OpenFile(logDir+"/openaide.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logFile = nil
	}

	opts := &slog.HandlerOptions{Level: logLevel}
	var handler slog.Handler

	var writers []io.Writer
	if TUILogWriter != nil {
		// 交互模式 (TUI/REPL): 文件 + ring buffer, 不污染终端
		writers = append(writers, TUILogWriter)
	} else {
		// 服务模式: stderr
		writers = append(writers, os.Stderr)
	}
	if logFile != nil {
		writers = append(writers, logFile)
	}
	multiWriter := io.MultiWriter(writers...)

	if format == "json" {
		handler = slog.NewJSONHandler(multiWriter, opts)
	} else {
		handler = slog.NewTextHandler(multiWriter, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// SetAutoApprove enables/disables auto-approval for all tool calls (skips confirmation).
func (a *Application) SetAutoApprove(on bool) {
	if k, ok := a.Kernel.(*kernel.AgentKernel); ok {
		approver := kernel.NewAutoApprover()
		approver.UnsafeMode = on
		approver.SetLLM(a.LLMGateway)
		k.SetApprover(approver)
	}
}

// SetModel switches the default provider's model at runtime.
func (a *Application) SetModel(model string) {
	if a.LLMGateway != nil {
		a.LLMGateway.SetDefaultModel(model)
	}
}
