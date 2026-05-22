package infra

import (
	"context"
	"log/slog"

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

func createKernel(cfg *config.Config, gateway *llm.Gateway, embedder llm.Embedder, toolRegistry kernel.ToolExecutor, memManager kernel.Memory, sessionStore kernel.SessionStore) (*kernel.AgentKernel, *knowledge.Base, *plugin.Manager) {
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
	agentKernel.SetReflection(kernel.NewLLMReflection(gateway, kernel.NewSimpleReflection()))
	sm := kernel.NewSkillManager(cfg.Storage.DataDir + "/skills")
	agentKernel.SetSkillManager(sm)
	if learner, err := kernel.NewSimpleLearner(cfg.Storage.DataDir + "/insights"); err == nil {
		agentKernel.SetLearner(learner)
		slog.Info("Learner enabled", "dir", cfg.Storage.DataDir+"/insights")
	} else {
		slog.Warn("Failed to create learner, learning disabled", "error", err)
	}
	agentKernel.SetPatternDetector(kernel.NewSimplePatternDetector())
	agentKernel.SetSkillEvolution(kernel.NewSkillEvolution(sm, cfg.Storage.DataDir+"/skills"))
	approver := kernel.NewAutoApprover()
	approver.UnsafeMode = true // 保留本地便利模式；设为 false 启用危险工具拦截
	agentKernel.SetApprover(approver)
	agentKernel.SetAdaptiveRounds(kernel.NewAdaptiveRounds(5, 30))

	if cp, err := kernel.NewFileCheckpointer(kernel.FileCheckpointerConfig{
		Dir: cfg.Storage.DataDir + "/checkpoints",
	}); err == nil {
		agentKernel.SetCheckpointer(cp)
		slog.Info("Checkpoint enabled", "dir", cfg.Storage.DataDir+"/checkpoints")
	} else {
		slog.Warn("Failed to create checkpointer, checkpoint disabled", "error", err)
	}

	if tracer, err := kernel.NewFileTracer(kernel.FileTracerConfig{
		FilePath: cfg.Storage.DataDir + "/traces.jsonl",
	}); err == nil {
		agentKernel.SetTracer(tracer)
		slog.Info("Tracing enabled", "file", cfg.Storage.DataDir+"/traces.jsonl")
	} else {
		slog.Warn("Failed to create tracer, tracing disabled", "error", err)
	}

	// 接入插件管理器
	pluginMgr := plugin.NewManager(cfg.Storage.DataDir + "/plugins")
	pluginPrompt := pluginMgr.GetPrompt()
	if pluginPrompt != "" {
		kernelConfig.SystemPrompt += "\n\n" + pluginPrompt
	}

	// 接入知识库 + 质量门控 + 语义搜索
	var kb *knowledge.Base
	var err error
	kb, err = knowledge.NewBase(cfg.Storage.DataDir + "/knowledge")
	if err == nil {
		kb.SetEmbedder(embedder)
		agentKernel.SetKnowledgeCollector(kb)
		gate := feedback.NewGate()
		agentKernel.SetQualityGate(gate)
	} else {
		slog.Warn("Failed to create knowledge base", "error", err)
	}

	// 接入身份检测 + 事件总线 + 高级压缩器
	if projIdentity, err := identity.NewDetector().Detect(context.Background(), "."); err == nil && projIdentity != nil {
		slog.Info("Project identity detected", "type", projIdentity.ProjectType)
	}
	eventBus := event.NewBus()
	eventBus.EnablePersistence(cfg.Storage.DataDir + "/events")
	agentKernel.SetContextCompressor(compress.NewLLMCompressor(gateway, compress.NewNovelCompressor()))

	agentKernel.Subscribe(kernel.EventHandlerFunc(func(evt kernel.Event) {
		pluginMgr.RunEventHooks(context.Background(), evt)
		eventBus.Publish(evt)
	}))

	return agentKernel, kb, pluginMgr
}
