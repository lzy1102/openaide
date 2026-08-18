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
	"openaide/backend/internal/codeindex"
	"openaide/backend/config"
	"openaide/backend/core"
	"openaide/backend/llm"
	"openaide/backend/lsp"
	"openaide/backend/internal/mcp"
	"openaide/backend/internal/memory"
	"openaide/backend/orchestration"
	"openaide/backend/internal/plugin"
	"openaide/backend/projectmind"
	"openaide/backend/rag"
	"openaide/backend/tools"
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
	ToolRegistry    *tools.Registry
	MCPManager      *mcp.Manager
	sessionActor    *kernel.SessionActor // CSP actor, owns all session state
	pluginWatcher   *PluginWatcher       // hot-reload
	codeIndexer     *codeindex.Indexer   // 代码索引(prompt 阶段注入相关代码)
}

// NewApplication 创建应用容器
func NewApplication(cfg *config.Config) (*Application, error) {
	app := &Application{Config: cfg}

	// 1. LLM 网关
	gateway := createLLMGateway(cfg)
	app.LLMGateway = gateway

	// 2. 外部检索后端(RAG)。Type 选择后端,未配置或不可达时自动降级为 NoopRetriever。
	retriever := rag.NewFromConfig(rag.Config{
		Type:           cfg.RAG.Type,
		EmbeddingURL:   cfg.RAG.EmbeddingURL,
		EmbeddingKey:   cfg.RAG.EmbeddingKey,
		EmbeddingModel: cfg.RAG.EmbeddingModel,
		Collection:     cfg.RAG.Collection,
		DSN:            cfg.RAG.DSN,
		QdrantHost:     cfg.RAG.QdrantHost,
		QdrantPort:     cfg.RAG.QdrantPort,
		QdrantAPIKey:   cfg.RAG.QdrantAPIKey,
		QdrantTLS:      cfg.RAG.QdrantTLS,
		MilvusAddress:  cfg.RAG.MilvusAddress,
		MilvusUsername: cfg.RAG.MilvusUsername,
		MilvusPassword: cfg.RAG.MilvusPassword,
		RedisAddr:      cfg.RAG.RedisAddr,
		RedisPassword:  cfg.RAG.RedisPassword,
		RedisDB:        cfg.RAG.RedisDB,
		ChromaURL:      cfg.RAG.ChromaURL,
		ChromaToken:    cfg.RAG.ChromaToken,
	})

	// 3. 工具注册表 + MCP
	tools.LoadIgnorePatterns(".")
	toolRegistry := tools.NewRegistry()
	toolRegistry.EnableBrowser(cfg.Browser.Enabled)
	if cfg.Search.SearXNGURL != "" {
		toolRegistry.UseSearXNG(cfg.Search.SearXNGURL)
	}
	app.ToolRegistry = toolRegistry
	if err := tools.RegisterBuiltins(toolRegistry); err != nil {
		return nil, fmt.Errorf("register builtin tools failed: %w", err)
	}
	mcpManager := mcp.NewManager()
	if cfg.MCP.Enabled {
		for _, srvCfg := range cfg.MCP.Servers {
			var connErr error
			if srvCfg.Type == "sse" || srvCfg.URL != "" {
				slog.Info("Connecting MCP server (SSE)", "id", srvCfg.ID, "url", srvCfg.URL)
				connErr = mcpManager.ConnectServer(srvCfg.ID, "sse", []string{srvCfg.URL}, nil)
			} else {
				slog.Info("Connecting MCP server (stdio)", "id", srvCfg.ID, "command", srvCfg.Command)
				connErr = mcpManager.ConnectServer(srvCfg.ID, srvCfg.Command, srvCfg.Args, mcp.EnvMap(srvCfg.Env))
			}
			if connErr != nil {
				slog.Warn("Failed to connect MCP server, skipping", "id", srvCfg.ID, "error", connErr)
				continue
			}
			registerMCPTools(toolRegistry, mcpManager, srvCfg.ID, "")
		}
	}
	// Claude 插件中的 .mcp.json
	if cfg.MCP.Enabled {
		for name, srv := range plugin.DiscoverClaudeMCP(cfg.Storage.DataDir + "/plugins") {
			isSSE := srv.Type == "sse" || srv.URL != ""
			if srv.Type != "stdio" && srv.Type != "" && !isSSE {
				slog.Debug("MCP server type not supported, skipping", "name", name, "type", srv.Type)
				continue
			}
			if isSSE {
				url := srv.URL
				if url == "" {
					url = srv.Command
				}
				slog.Info("Connecting MCP server (Claude plugin SSE)", "name", name, "url", url)
				if err := mcpManager.ConnectServer(name, "sse", []string{url}, nil); err != nil {
					slog.Warn("Failed to connect Claude MCP server, skipping", "name", name, "error", err)
					continue
				}
			} else {
				cmd := srv.Command
				if cmd == "" {
					slog.Warn("MCP server missing command, skipping", "name", name)
					continue
				}
				slog.Info("Connecting MCP server (Claude plugin)", "name", name, "command", cmd)
				if err := mcpManager.ConnectServer(name, cmd, srv.Args, mcp.EnvMap(srv.Env)); err != nil {
					slog.Warn("Failed to connect Claude MCP server, skipping", "name", name, "error", err)
					continue
				}
			}
			registerMCPTools(toolRegistry, mcpManager, name, "Claude ")
		}
	}

	app.MCPManager = mcpManager

	// 4. 记忆管理器（CSP actor）
	memManager, err := memory.NewMemoryActor(cfg.Storage.DataDir + "/memory.db")
	if err != nil {
		return nil, fmt.Errorf("memory actor: %w", err)
	}
	memManager.SetRetriever(retriever)
	toolRegistry.RegisterMemory(memManager)

	// 5. 会话存储（CSP actor）
	sessionStore, err := kernel.NewSessionActor(cfg.Storage.DataDir + "/sessions.db")
	if err != nil {
		return nil, fmt.Errorf("session actor: %w", err)
	}
	app.sessionActor = sessionStore

	// 6. 内核 + 所有增强能力
	agentKernel, pluginMgr, codeIdx, err := createKernel(cfg, gateway, retriever, toolRegistry, memManager, sessionStore)
	if err != nil {
		return nil, fmt.Errorf("create kernel: %w", err)
	}
	app.Kernel = agentKernel
	app.PluginManager = pluginMgr
	app.codeIndexer = codeIdx

	// 代码索引:启动时异步全量索引 CWD 项目(不阻塞应用启动)。
	// 索引完成后,kernel_prompt.go 会在 coding/debugging 任务中
	// 自动检索 top-K 相关代码 chunk 注入 prompt。
	if codeIdx != nil {
		if cwd, err := os.Getwd(); err == nil {
			codeIdx.IndexProject(cwd)
			slog.Info("CodeIndexer: background full index started", "root", cwd)
		}
	}

	// Plugin hot-reload — reloads manager metadata + skills in one shot
	pluginDir := cfg.Storage.DataDir + "/plugins"
	skillActor := agentKernel.GetSkillActor()
	pluginReloader := func() []string {
		newIDs := pluginMgr.Reload()
		if skillActor != nil {
			for _, cs := range plugin.DiscoverClaudeSkills(pluginDir) {
				skillActor.AddClaudeSkill(cs.ID, cs.Name, cs.Description, cs.Prompt, cs.Keywords, cs.AllowedTools, cs.Scripts)
			}
		}
		return newIDs
	}
	pluginWatcher := NewPluginWatcher(pluginDir, pluginReloader)
	if err := pluginWatcher.Start(); err != nil {
		slog.Warn("Plugin hot-reload unavailable", "error", err)
	} else {
		app.pluginWatcher = pluginWatcher
	}

	// LSP: auto-start language servers for the current project
	startLSPServers(toolRegistry)

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
	app.APIServer.SetKernel(agentKernel)

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
				if err := mcpManager.ConnectServer(id, srv.Command, srv.Args, nil); err != nil {
					slog.Warn("OpenCode MCP connect failed", "id", id, "error", err)
					continue
				}
				registerMCPTools(toolRegistry, mcpManager, id, "OpenCode ")
				slog.Info("OpenCode MCP connected", "id", id)
			}
		}
		if opencodeInstr != "" {
			agentKernel.SetSystemPrompt(opencodeInstr)
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
	// 代码索引先停:确保 actor 中的 pending 写入落盘后再关 SQLite
	if app.codeIndexer != nil {
		app.codeIndexer.Stop()
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
	if app.ToolRegistry != nil {
		app.ToolRegistry.ShutdownLSP()
	}
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
func startLSPServers(reg *tools.Registry) {
	cwd, _ := os.Getwd()
	entries, _ := os.ReadDir(cwd)

	// Project type → language mapping. Only start one server per project.
	detectors := map[string]string{
		"go.mod":           "go",
		"Cargo.toml":       "rust",
		"CMakeLists.txt":   "cpp",
		"build.zig":        "zig",
		"pyproject.toml":   "python",
		"setup.py":         "python",
		"requirements.txt": "python",
		"Gemfile":          "ruby",
		"composer.json":    "php",
		"pom.xml":          "java",
		"build.gradle":     "java",
		"build.gradle.kts": "kotlin",
		"build.sbt":        "scala",
		"package.json":     "typescript",
		".csproj":          "csharp",
		"Package.swift":    "swift",

		"mix.exs":      "elixir",
		"rebar.config": "erlang",
		"stack.yaml":   "haskell",
		"pubspec.yaml": "dart",
		".Rproj":       "r",
		"Project.toml": "julia",
	}

	started := make(map[string]bool)
	for _, e := range entries {
		name := e.Name()
		for indicator, lang := range detectors {
			if name == indicator && !e.IsDir() && !started[lang] {
				if c, err := lsp.Start(cwd, lang); err == nil {
					reg.AttachLSPClient(lang, c)
					started[lang] = true
				} else {
					slog.Debug("LSP server not available", "lang", lang, "error", err)
				}
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

// SetModel switches the default provider's model at runtime.
func (a *Application) SetModel(model string) {
	if a.LLMGateway != nil {
		a.LLMGateway.SetDefaultModel(model)
	}
}

// registerMCPTools registers all tools from a connected MCP server into the tool registry.
func registerMCPTools(toolRegistry *tools.Registry, mcpManager *mcp.Manager, serverID string, logPrefix string) {
	for _, mcpTool := range mcpManager.GetServerTools(serverID) {
		def := kernel.ToolDefinition{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        mcpTool.Function.Name,
				Description: mcpTool.Function.Description,
				Parameters:  mcpTool.Function.Parameters,
			},
		}
		serverID, toolName := serverID, mcpTool.Function.Name
		handler := kernel.ToolHandler(func(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
			var args map[string]interface{}
			if arguments != "" {
				if err := json.Unmarshal([]byte(arguments), &args); err != nil {
					return &kernel.ToolResult{Error: fmt.Sprintf("invalid args: %v", err)}, nil
				}
			}
			return mcpManager.CallTool(serverID, toolName, args)
		})
		if err := toolRegistry.Register(def, handler); err != nil {
			slog.Warn(logPrefix+"MCP tool registration skipped, duplicate", "tool", mcpTool.Function.Name, "error", err)
		} else {
			slog.Info(logPrefix+"MCP tool registered", "server", serverID, "tool", mcpTool.Function.Name)
		}
	}
}
