package services

import (
	"context"
	"log/slog"
	"strings"
)

type IntentType string

const (
	IntentSimpleChat   IntentType = "simple_chat"
	IntentSkillExecution IntentType = "skill_execution"
	IntentToolCalling  IntentType = "tool_calling"
	IntentPlanning     IntentType = "planning"
	IntentKnowledge    IntentType = "knowledge"
)

type RoutingDecision struct {
	Intent      IntentType
	ModelID     string
	SkillMatch  *SkillMatchResult
	NeedsTools  bool
	NeedsPlan   bool
	NeedsRAG    bool
	Confidence  float64
	Reason      string
}

type RequestOrchestrator struct {
	router    *ModelRouter
	skillSvc  *SkillService
	planSvc   *PlanService
	toolSvc   *ToolCallingService
	eventBus  *EventBus
}

func NewRequestOrchestrator(
	router *ModelRouter,
	skillSvc *SkillService,
	planSvc *PlanService,
	toolSvc *ToolCallingService,
	eventBus *EventBus,
) *RequestOrchestrator {
	return &RequestOrchestrator{
		router:   router,
		skillSvc: skillSvc,
		planSvc:  planSvc,
		toolSvc:  toolSvc,
		eventBus: eventBus,
	}
}

func (o *RequestOrchestrator) AnalyzeIntent(ctx context.Context, content string) IntentType {
	lower := strings.ToLower(content)

	toolIndicators := []string{
		"执行", "运行", "跑一下", "curl", "wget", "ping ",
		"ls ", "cat ", "查看ip", "公网ip", "查ip",
		"执行命令", "运行命令", "shell", "终端",
		"docker", "git ", "npm ", "pip ", "go run",
		"读文件", "写文件", "创建文件", "删除文件",
		"搜索", "搜一下", "帮我查", "帮我执行", "帮我运行",
		"安装", "卸载", "更新", "升级",
		"启动", "停止", "重启", "部署",
		"编译", "打包", "发布",
		"天气", "气温", "天气预报",
		"连上", "连接", "服务器", "能不能", "可以", "试试",
		"查看", "检查", "测试", "诊断",
		"网络", "端口", "ip地址", "状态",
		"帮我", "给我", "我要", "我需要",
		"数据库", "mysql", "redis", "nginx",
		"日志", "监控", "进程", "磁盘", "内存",
		"配置", "修改", "设置",
	}
	for _, ind := range toolIndicators {
		if strings.Contains(lower, ind) {
			return IntentToolCalling
		}
	}

	planIndicators := []string{
		"帮我完成", "项目", "方案", "多步骤", "分步骤",
		"规划", "计划", "整体", "架构设计", "从零开始",
	}
	for _, ind := range planIndicators {
		if strings.Contains(lower, ind) {
			return IntentPlanning
		}
	}

	knowledgeIndicators := []string{
		"是什么", "为什么", "解释", "介绍", "什么是",
		"如何理解", "区别", "对比", "原理",
	}
	for _, ind := range knowledgeIndicators {
		if strings.Contains(lower, ind) {
			return IntentKnowledge
		}
	}

	return IntentSimpleChat
}

func (o *RequestOrchestrator) Route(ctx context.Context, content, modelID string, options map[string]interface{}) *RoutingDecision {
	decision := &RoutingDecision{
		Intent:     IntentSimpleChat,
		ModelID:    modelID,
		Confidence: 0.5,
	}

	intent := o.AnalyzeIntent(ctx, content)
	decision.Intent = intent
	slog.Info("Intent analyzed", "component", "Orchestrator", "intent", intent, "content_preview", truncateForLog(content, 50))

	if o.skillSvc != nil && o.skillSvc.NeedsSkillExecution(content) {
		match := o.skillSvc.MatchSkill(content)
		if match != nil {
			decision.SkillMatch = match
			decision.Intent = IntentSkillExecution
			decision.Confidence = match.Confidence
			decision.Reason = "skill matched: " + match.Skill.Name

			if match.Skill.ModelPreference != "" {
				skillModelID, err := o.skillSvc.ResolveModelID(ctx, match.Skill.ModelPreference)
				if err == nil {
					decision.ModelID = skillModelID
				}
			}

			if len(match.Skill.Tools) > 0 {
				decision.NeedsTools = true
			}

			slog.Info("Skill matched", "component", "Orchestrator", "skill", match.Skill.Name, "confidence", match.Confidence)
		}
	}

	if decision.Intent == IntentToolCalling {
		decision.NeedsTools = true
		decision.Reason = "tool execution indicators detected"
	}

	if decision.Intent == IntentPlanning {
		decision.NeedsPlan = true
		decision.NeedsTools = true
		decision.Reason = "planning indicators detected"
	}

	if decision.Intent == IntentKnowledge {
		decision.NeedsRAG = true
		decision.Reason = "knowledge query indicators detected"
	}

	if decision.Intent == IntentSimpleChat {
		chatLen := len([]rune(content))
		if chatLen > 200 {
			decision.NeedsTools = true
			decision.Reason = "long query, enabling tools for better assistance"
		} else {
			decision.NeedsTools = true
			decision.Reason = "always enable tools for agent capability"
		}
	}

	if decision.ModelID == "" && o.router != nil {
		routed, err := o.router.Route(ctx, content, nil)
		if err != nil {
			slog.Error("Auto route failed", "component", "Orchestrator", "error", err)
		} else {
			decision.ModelID = routed.ID
			slog.Info("Auto routed to model", "component", "Orchestrator", "model", routed.Name)
		}
	}

	if o.toolSvc != nil && decision.NeedsTools {
		decision.NeedsTools = true
	} else {
		decision.NeedsTools = false
	}

	slog.Info("Routing decision", "component", "Orchestrator",
		"intent", decision.Intent,
		"model", decision.ModelID,
		"needs_tools", decision.NeedsTools,
		"needs_plan", decision.NeedsPlan,
		"needs_rag", decision.NeedsRAG,
		"skill", skillName(decision.SkillMatch),
		"confidence", decision.Confidence,
		"reason", decision.Reason)

	return decision
}

func truncateForLog(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func skillName(match *SkillMatchResult) string {
	if match == nil {
		return ""
	}
	return match.Skill.Name
}
