package infra

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"openaide/backend/internal/compress"
	"openaide/backend/internal/config"
	"openaide/backend/internal/event"
	"openaide/backend/internal/feedback"
	"openaide/backend/internal/identity"
	"openaide/backend/internal/kernel"
	"openaide/backend/internal/knowledge"
	"openaide/backend/internal/llm"
	"openaide/backend/internal/plugin"
)

func createKernel(cfg *config.Config, gateway *llm.Gateway, embedder llm.Embedder, toolRegistry kernel.ToolExecutor, memManager kernel.Memory, sessionStore kernel.SessionStore) (*kernel.AgentKernel, *plugin.Manager, error) {
	systemPrompt := cfg.Kernel.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = kernel.LoadSystemPrompt(cfg.Storage.DataDir + "/prompts")
	}
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

	// 接入增强能力 — LLM Reflection（降级到 SimpleReflection）
	agentKernel.SetReflection(kernel.NewLLMReflection(gateway))

	// Skill actor (CSP, zero-lock)
	skillActor := kernel.NewSkillActor(gateway)
	for _, cs := range plugin.DiscoverClaudeSkills(cfg.Storage.DataDir + "/plugins") {
		skillActor.AddClaudeSkill(cs.ID, cs.Name, cs.Description, cs.Prompt, cs.Keywords, cs.AllowedTools)
	}
	agentKernel.SetSkillActor(skillActor)

	if learner, err := kernel.NewSimpleLearner(cfg.Storage.DataDir + "/insights"); err == nil {
		learner.SetLLM(gateway)
		agentKernel.SetLearner(learner)
		slog.Info("Learner enabled", "dir", cfg.Storage.DataDir+"/insights")
	} else {
		slog.Warn("Failed to create learner, learning disabled", "error", err)
	}
	agentKernel.SetPatternDetector(kernel.NewSimplePatternDetector())
	approver := kernel.NewAutoApprover()
	approver.SetLLM(gateway)
	if cfg.Kernel.UnsafeMode != nil {
		approver.UnsafeMode = *cfg.Kernel.UnsafeMode
	} else {
		approver.UnsafeMode = true // 默认本地信任模式
	}
	agentKernel.SetApprover(approver)
	minR := cfg.Kernel.MinRounds
	maxR := cfg.Kernel.MaxRoundsCap
	if minR <= 0 { minR = 5 }
	if maxR <= 0 { maxR = 30 }
	ar := kernel.NewAdaptiveRounds(minR, maxR)
	ar.SetLLM(gateway)
	agentKernel.SetAdaptiveRounds(ar)

	if cp, err := kernel.NewFileCheckpointer(kernel.FileCheckpointerConfig{
		Dir: cfg.Storage.DataDir + "/checkpoints",
	}); err == nil {
		agentKernel.SetCheckpointer(cp)
		slog.Info("Checkpoint enabled", "dir", cfg.Storage.DataDir+"/checkpoints")
	} else {
		slog.Warn("Failed to create checkpointer, checkpoint disabled", "error", err)
	}

	if cfg.Log.PersistEnabled() {
		if tracer, err := kernel.NewFileTracer(kernel.FileTracerConfig{
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

	// 接入知识库（CSP actor）+ 质量门控 + 语义搜索
	kAct, kerr := knowledge.NewActor(cfg.Storage.DataDir + "/knowledge.db")
	if kerr != nil {
		return nil, nil, fmt.Errorf("knowledge actor: %w", kerr)
	}
	kAct.SetEmbedder(embedder)
	kAct.SetLLM(gateway)
	agentKernel.SetKnowledgeCollector(kAct)
	agentKernel.SetQualityGate(feedback.NewGate())

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
	agentKernel.SetContextCompressor(compress.NewLLMCompressor(gateway, compress.NewNovelCompressor()))

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
				cmd.Env = append(os.Environ(), "OPENAIDE_EVENT="+evt.Type)
				if out, err := cmd.CombinedOutput(); err != nil {
					slog.Debug("Hook command failed", "event", hook.Event, "error", err, "output", string(out))
				}
			}()
		}))
		slog.Info("Claude hook registered", "event", hook.Event, "openaide_event", oevt)
	}

	return agentKernel, pluginMgr, nil
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
