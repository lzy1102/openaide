package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"openaide/backend/internal/api"
	"openaide/backend/internal/auth"
	"openaide/backend/internal/channel"
	"openaide/backend/internal/config"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/llm"
	"openaide/backend/internal/mcp"
	"openaide/backend/internal/memory"
	"openaide/backend/internal/orchestration"
	"openaide/backend/internal/plugin"
	"openaide/backend/internal/tools"
)

// Application 应用容器
type Application struct {
	Config          *config.Config
	Kernel          kernel.Kernel
	Orchestrator    *orchestration.Orchestrator
	APIServer       *api.Server
	LLMGateway      *llm.Gateway
	ChannelRegistry *channel.Registry
	TaskQueue       *channel.TaskQueue
	PluginManager   *plugin.Manager
	MCPManager      *mcp.Manager
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
	tools.SetBrowserEnabled(cfg.Browser.Enabled)
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

	// 4. 记忆管理器
	memManager, err := memory.NewFileMemory(cfg.Memory.DataDir)
	if err != nil {
		slog.Warn("Failed to create memory manager", "error", err)
		memManager = nil
	} else {
		memManager.SetEmbedder(embedder)
	}

	// 5. 会话存储（文件持久化，死机可恢复）
	var sessionStore kernel.SessionStore
	fileStore, err := kernel.NewFileSessionStore(cfg.Storage.DataDir + "/sessions")
	if err != nil {
		slog.Warn("Failed to create file session store, using memory", "error", err)
		sessionStore = kernel.NewSessionStoreAdapter()
	} else {
		sessionStore = fileStore
	}

	// 6. 内核 + 所有增强能力
	agentKernel, kb, pluginMgr := createKernel(cfg, gateway, embedder, toolRegistry, memManager, sessionStore)
	app.Kernel = agentKernel
	app.PluginManager = pluginMgr

	// 7. 编排器
	orch := orchestration.NewOrchestrator(agentKernel, gateway, toolRegistry, memManager, sessionStore)
	if cfg.Planning.PreviewTimeout > 0 {
		orch.PreviewTimeout = time.Duration(cfg.Planning.PreviewTimeout) * time.Second
	}
	if cfg.Planning.DeepTimeout > 0 {
		orch.DeepTimeout = time.Duration(cfg.Planning.DeepTimeout) * time.Second
	}
	if kb != nil {
		orch.SetKnowledgeCollector(kb)
	}
	orch.SetTeam(orchestration.NewTeam(orch))
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

// InitLogger 初始化日志
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

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: logLevel}

	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// SetAutoApprove enables/disables auto-approval for all tool calls (skips confirmation).
func (a *Application) SetAutoApprove(on bool) {
	if k, ok := a.Kernel.(*kernel.AgentKernel); ok {
		approver := kernel.NewAutoApprover()
		approver.UnsafeMode = on
		k.SetApprover(approver)
	}
}

// SetModel switches the default provider's model at runtime.
func (a *Application) SetModel(model string) {
	if a.LLMGateway != nil {
		a.LLMGateway.SetDefaultModel(model)
	}
}
