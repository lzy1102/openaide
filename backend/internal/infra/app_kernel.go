package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"openaide/backend/internal/codeindex"
	"openaide/backend/config"
	"openaide/backend/internal/event"
	"openaide/backend/internal/identity"
	"openaide/backend/core"
	"openaide/backend/core/trace"
	"openaide/backend/llm"
	"openaide/backend/internal/plugin"
	"openaide/backend/prompts"
	"openaide/backend/rag"
)

func createKernel(cfg *config.Config, gateway *llm.Gateway, retriever rag.Retriever, toolRegistry kernel.ToolExecutor, memManager kernel.Memory, sessionStore kernel.SessionStore) (*kernel.AgentKernel, *plugin.Manager, *codeindex.Indexer, error) {
	systemPrompt := cfg.Kernel.SystemPrompt
	kernelConfig := &kernel.Config{
		MaxRounds:    cfg.Kernel.MaxRounds,
		MaxTokens:    cfg.Kernel.MaxTokens,
		SystemPrompt: systemPrompt,
	}
	if kernelConfig.MaxRounds == 0 {
		kernelConfig.MaxRounds = 10
	}
	if kernelConfig.MaxTokens == 0 {
		kernelConfig.MaxTokens = 4000
	}

	agentKernel := kernel.NewAgentKernel(gateway, toolRegistry, memManager, sessionStore, kernelConfig)

	// 内核可替换策略(反思/自适应轮数/规划/上下文压缩)按配置经注册表装配。
	if err := kernelStrategyRegistry(agentKernel, gateway).Apply(cfg); err != nil {
		return nil, nil, nil, fmt.Errorf("apply kernel strategies: %w", err)
	}

	// Skill actor (CSP, zero-lock)
	skillActor := kernel.NewSkillActor(gateway)
	for _, cs := range plugin.DiscoverClaudeSkills(cfg.Storage.DataDir + "/plugins") {
		skillActor.AddClaudeSkill(cs.ID, cs.Name, cs.Description, cs.Prompt, cs.Keywords, cs.AllowedTools, cs.Scripts)
	}
	// Enable skill persistence — auto-extracted skills survive restarts
	autoSkillPath := cfg.Storage.DataDir + "/skills/auto_skills.json"
	skillActor.SetOnSave(func() {
		os.MkdirAll(filepath.Dir(autoSkillPath), 0755)
		data, err := json.MarshalIndent(skillActor.ExportSkills(), "", "  ")
		if err != nil {
			return
		}
		os.WriteFile(autoSkillPath, data, 0644)
	})
	// Load previously auto-extracted skills from disk
	if data, err := os.ReadFile(autoSkillPath); err == nil {
		var saved map[string]*kernel.Skill
		if json.Unmarshal(data, &saved) == nil {
			for _, s := range saved {
				if s != nil && strings.HasPrefix(s.ID, "auto-") {
					skillActor.AddSkill(s.ID, s.Name, s.Description, s.Prompt, s.Keywords)
				}
			}
			slog.Info("Auto-skills loaded", "count", len(saved))
		}
	}
	agentKernel.SetSkillActor(skillActor)

	// 人格/能力集插件:按 config 激活的 persona 提供 L0 层系统提示词。
	// 未配置 persona 时内核回退到内置默认 L0,行为不变。
	personaStore := prompts.NewStore(cfg.Storage.DataDir + "/prompts/personas")
	if cfg.Kernel.Persona != "" {
		if p, err := personaStore.SetActive(cfg.Kernel.Persona); err != nil {
			slog.Warn("Persona activation failed, using default", "persona", cfg.Kernel.Persona, "error", err)
		} else {
			agentKernel.SetPersona(personaStore)
			slog.Info("Persona activated", "persona", p.Name, "desc", p.Description)
		}
	}

	// 任务规划器:复杂查询(complexity >= 15)在 ReAct 前分解为子任务
	// (规划/自适应轮数/上下文压缩/反思已由 kernelStrategyRegistry 装配)

	if cp, err := trace.NewFileCheckpointer(trace.FileCheckpointerConfig{
		Dir: cfg.Storage.DataDir + "/checkpoints",
	}); err == nil {
		agentKernel.SetCheckpointer(cp)
		slog.Info("Checkpoint enabled", "dir", cfg.Storage.DataDir+"/checkpoints")
	} else {
		slog.Warn("Failed to create checkpointer, checkpoint disabled", "error", err)
	}

	if cfg.Log.PersistEnabled() {
		if tracer, err := trace.NewFileTracer(trace.FileTracerConfig{
			FilePath: cfg.Storage.DataDir + "/traces.jsonl",
		}); err == nil {
			agentKernel.SetTracer(tracer)
			slog.Info("Tracing enabled", "file", cfg.Storage.DataDir+"/traces.jsonl")
		} else {
			slog.Warn("Failed to create tracer, tracing disabled", "error", err)
		}
	} else {
		slog.Info("Persistence disabled in config")
	}

	// 接入插件管理器
	pluginMgr := plugin.NewManager(cfg.Storage.DataDir + "/plugins")
	pluginPrompt := pluginMgr.GetPrompt()
	if pluginPrompt != "" {
		kernelConfig.SystemPrompt += "\n\n" + pluginPrompt
	}

	// 接入身份检测 + 事件总线 + 高级压缩器
	if projIdentity, err := identity.NewDetector().Detect(context.Background(), "."); err == nil && projIdentity != nil {
		slog.Info("Project identity detected", "type", projIdentity.ProjectType)
	}
	eventBus := event.NewBus()
	if cfg.Log.PersistEnabled() {
		eventBus.EnablePersistence(cfg.Storage.DataDir + "/events")
	} else {
		slog.Info("Persistence disabled in config")
	}
	// 上下文压缩器已由 kernelStrategyRegistry 装配。

	// 代码索引:启动时异步全量索引 CWD,prompt 阶段注入相关代码 chunk。
	// 检索完全外挂到 rag.Retriever,未配置时返回空结果。
	// 通过 cfg.CodeIndex.Enabled = false 可关闭。
	var codeIdx *codeindex.Indexer
	if cfg.CodeIndex.EnabledOrDefault() {
		idxCfg := codeindex.Config{
			DBPath:       cfg.Storage.DataDir + "/codeindex.db",
			ChunkSize:    cfg.CodeIndex.ChunkSize,
			MaxChunks:    cfg.CodeIndex.MaxChunks,
			MinScore:     cfg.CodeIndex.MinScore,
			MinScoreMode: cfg.CodeIndex.ScoreMode,
			ScoreRatio:   cfg.CodeIndex.ScoreRatio,
		}
		if idx, err := codeindex.NewIndexer(idxCfg, retriever); err == nil {
			agentKernel.SetCodeIndexer(idx)
			codeIdx = idx
			slog.Info("CodeIndexer enabled", "db", idxCfg.DBPath)
		} else {
			slog.Warn("CodeIndexer init failed, code injection disabled", "error", err)
		}
	} else {
		slog.Info("CodeIndexer disabled by config")
	}

	// Metrics: task-level observability (JSONL persistence + in-memory ring buffer)
	metrics := kernel.NewMetricsStore(cfg.Storage.DataDir)
	agentKernel.SetMetrics(metrics)
	slog.Info("Task metrics enabled", "file", cfg.Storage.DataDir+"/metrics.jsonl")

	agentKernel.Subscribe(kernel.EventHandlerFunc(func(evt kernel.Event) {
		pluginMgr.RunEventHooks(context.Background(), evt)
		eventBus.Publish(evt)
	}))

	// Claude hooks.json: 事件 → shell 命令
	for _, hook := range plugin.DiscoverClaudeHooks(cfg.Storage.DataDir + "/plugins") {
		hook := hook
		oevt := plugin.MapClaudeEvent(hook.Event)
		if oevt == "" {
			slog.Debug("Unknown Claude hook event, skipping", "event", hook.Event)
			continue
		}
		agentKernel.Subscribe(kernel.EventHandlerFunc(func(evt kernel.Event) {
			if evt.Type != oevt {
				return
			}
			if len(hook.Tools) > 0 {
				toolName, _ := evt.Data["tool"].(string)
				if !contains(hook.Tools, toolName) && !contains(hook.Tools, plugin.MapClaudeToolReverse(toolName)) {
					return
				}
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "sh", "-c", hook.Command)
				cmd.Env = os.Environ()
				cmd.Env = append(cmd.Env, "OPENAIDE_EVENT="+evt.Type)
				if sid, _ := evt.Data["session_id"].(string); sid != "" {
					cmd.Env = append(cmd.Env, "OPENAIDE_SESSION_ID="+sid)
				}
				if tn, _ := evt.Data["tool"].(string); tn != "" {
					cmd.Env = append(cmd.Env, "OPENAIDE_TOOL_NAME="+tn)
				}
				if out, err := cmd.CombinedOutput(); err != nil {
					slog.Debug("Hook command failed", "event", hook.Event, "error", err, "output", string(out))
				}
			}()
		}))
		slog.Info("Claude hook registered", "event", hook.Event, "openaide_event", oevt)
	}

	return agentKernel, pluginMgr, codeIdx, nil
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
