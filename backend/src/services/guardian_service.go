package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"openaide/backend/src/config"
	"openaide/backend/src/services/llm"
)

const guardianPrompt = `你是 OpenAIDE 的安全守卫，负责评估工具调用的安全性。

## 评估维度
1. **数据泄露**：是否会泄露敏感数据？（凭证、API Key、用户隐私）
2. **凭证探测**：是否会访问或暴露认证信息？
3. **安全弱化**：是否会永久降低系统安全性？（开放端口、关闭防火墙、添加 SSH 密钥）
4. **破坏性操作**：是否会造成不可逆损害？（删除文件、删除数据库、格式化磁盘）

## 输入信息
工具: %s
参数: %s
上下文: %s

## 输出格式（JSON）
{
  "verdict": "allow|confirm|deny",
  "risk_level": "none|low|medium|high|critical",
  "risks": {
    "data_exfiltration": "none|low|medium|high",
    "credential_probing": "none|low|medium|high",
    "persistent_security_weakening": "none|low|medium|high",
    "destructive_action": "none|low|medium|high"
  },
  "reason": "判定理由的简要说明",
  "suggestions": ["如果有更安全的替代方案，请列出"]
}

## 判定规则

### allow（允许）- 安全操作
- 读取文件、搜索代码、列出目录
- 创建目录（mkdir）用于项目搭建
- 写入源代码文件（.rs, .py, .js, .go, .toml, .yaml, .json, .html, .css, .md 等）
- 通过包管理器安装依赖（apt, yum, npm, cargo, pip）
- 执行构建命令（cargo build, npm install, go build, make）
- Git 操作（clone, pull, push, commit, status, log）
- 非破坏性系统查询（ls, cat, grep, find, ps, df, free, top）

### confirm（确认）- 中等风险
- 修改系统状态的命令（systemctl, service, chmod, chown）
- 写入系统目录（/etc, /usr, /bin, /sbin）
- 暴露服务的网络操作（在公共接口启动服务）
- 涉及 sudo 或提升权限的操作

### deny（拒绝）- 危险操作
- rm -rf / 或类似的破坏性模式
- 暴露凭证或 API Key
- 禁用安全机制
- 未经用户明确意图格式化磁盘或删除数据库

## 特殊规则
- 开发任务（创建项目、编写代码、构建软件）优先 allow
- 只有真正构成安全风险的操作才使用 confirm
- 用户明确请求的开发工作流操作，绝不 deny`

type GuardianVerdict string

const (
	VerdictAllow   GuardianVerdict = "allow"
	VerdictConfirm GuardianVerdict = "confirm"
	VerdictDeny    GuardianVerdict = "deny"
)

type RiskLevel string

const (
	RiskNone     RiskLevel = "none"
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type GuardianRisk struct {
	DataExfiltration              string `json:"data_exfiltration"`
	CredentialProbing             string `json:"credential_probing"`
	PersistentSecurityWeakening   string `json:"persistent_security_weakening"`
	DestructiveAction             string `json:"destructive_action"`
}

type GuardianReview struct {
	Verdict     GuardianVerdict `json:"verdict"`
	RiskLevel   RiskLevel       `json:"risk_level"`
	Risks       GuardianRisk    `json:"risks"`
	Reason      string          `json:"reason"`
	Suggestions []string        `json:"suggestions"`
}

type GuardianService struct {
	modelCaller      ModelCaller
	enabled          bool
	level            config.SecurityLevel
	autoAllow        map[string]bool
	autoDeny         map[string]bool
	sessionApprovals map[string]bool
}

func NewGuardianService(modelCaller ModelCaller) *GuardianService {
	cfg, _ := config.Load()
	level := config.SecurityStandard
	if cfg != nil && cfg.Security.Level != "" {
		level = cfg.Security.Level
	}

	return &GuardianService{
		modelCaller: modelCaller,
		enabled:     level != config.SecurityOff,
		level:       level,
		autoAllow: map[string]bool{
			"read_file": true, "search": true, "web_search": true,
			"list_directory": true, "get_file_info": true,
		},
		autoDeny: map[string]bool{
			"rm_rf": true, "format": true, "dd_if": true,
		},
		sessionApprovals: make(map[string]bool),
	}
}

func (g *GuardianService) GetLevel() config.SecurityLevel {
	return g.level
}

func (g *GuardianService) SetLevel(level config.SecurityLevel) {
	g.level = level
	g.enabled = level != config.SecurityOff
	slog.Info("Guardian security level changed", "component", "Guardian", "level", level)
}

func (g *GuardianService) ApproveSession(toolName string) {
	g.sessionApprovals[toolName] = true
	slog.Info("Session approval granted", "component", "Guardian", "tool", toolName)
}

func (g *GuardianService) IsSessionApproved(toolName string) bool {
	return g.sessionApprovals[toolName]
}

func (g *GuardianService) ClearSessionApprovals() {
	g.sessionApprovals = make(map[string]bool)
}

func (g *GuardianService) Review(ctx context.Context, toolName, arguments, contextStr string) (*GuardianReview, error) {
	if !g.enabled || g.level == config.SecurityOff {
		return &GuardianReview{Verdict: VerdictAllow, RiskLevel: RiskNone, Reason: "guardian disabled"}, nil
	}

	if g.autoAllow[toolName] {
		return &GuardianReview{
			Verdict:   VerdictAllow,
			RiskLevel: RiskNone,
			Reason:    "auto-allowed for read-only tool",
		}, nil
	}

	lowerArgs := strings.ToLower(arguments)

	// 始终阻止极端危险操作（除非完全关闭）
	for denied := range g.autoDeny {
		if strings.Contains(lowerArgs, denied) {
			return &GuardianReview{
				Verdict:   VerdictDeny,
				RiskLevel: RiskCritical,
				Reason:    fmt.Sprintf("auto-denied: contains dangerous pattern '%s'", denied),
			}, nil
		}
	}
	if strings.Contains(lowerArgs, "rm -rf") || strings.Contains(lowerArgs, "rm -r /") || strings.Contains(lowerArgs, "mkfs.") {
		return &GuardianReview{
			Verdict:   VerdictDeny,
			RiskLevel: RiskCritical,
			Risks:     GuardianRisk{DestructiveAction: "high"},
			Reason:    "recursive force delete or format detected",
		}, nil
	}

	// permissive 模式：仅阻止极端危险操作，其他全部放行
	if g.level == config.SecurityPermissive {
		return &GuardianReview{
			Verdict:   VerdictAllow,
			RiskLevel: RiskLow,
			Reason:    "permissive mode: auto-allowed",
		}, nil
	}

	// 宽松模式：开发操作自动放行
	devTools := map[string]bool{
		"write_file": true, "execute_command": true, "shell": true, "bash": true,
	}
	if g.level == config.SecurityStandard && devTools[toolName] {
		// 标准模式：检查是否为开发操作
		if !strings.Contains(lowerArgs, "systemctl") &&
			!strings.Contains(lowerArgs, "chmod") &&
			!strings.Contains(lowerArgs, "chown") &&
			!strings.Contains(lowerArgs, "iptables") &&
			!strings.Contains(lowerArgs, "passwd") &&
			!strings.Contains(lowerArgs, "sudo") {
			return &GuardianReview{
				Verdict:   VerdictAllow,
				RiskLevel: RiskLow,
				Reason:    "standard mode: development operation auto-allowed",
			}, nil
		}
	}

	modelID := g.findReviewModel()
	if modelID == "" {
		if g.level == config.SecurityStrict {
			return &GuardianReview{
				Verdict:   VerdictDeny,
				RiskLevel: RiskMedium,
				Reason:    "strict mode: no review model available, denying as precaution",
			}, nil
		}
		return &GuardianReview{
			Verdict:   VerdictConfirm,
			RiskLevel: RiskMedium,
			Reason:    "no model available for semantic review, requiring confirmation",
		}, nil
	}

	prompt := fmt.Sprintf(guardianPrompt, toolName, arguments, contextStr)

	resp, err := g.modelCaller.Chat(modelID, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, map[string]interface{}{"max_tokens": float64(1000), "temperature": float64(0.1)})
	if err != nil {
		slog.Error("Guardian review failed", "component", "Guardian", "error", err)
		if g.level == config.SecurityStrict {
			return &GuardianReview{
				Verdict:   VerdictDeny,
				RiskLevel: RiskMedium,
				Reason:    "strict mode: guardian review failed, denying as precaution",
			}, nil
		}
		return &GuardianReview{
			Verdict:   VerdictConfirm,
			RiskLevel: RiskMedium,
			Reason:    "guardian review failed, requiring confirmation as precaution",
		}, nil
	}

	if len(resp.Choices) == 0 {
		if g.level == config.SecurityStrict {
			return &GuardianReview{Verdict: VerdictDeny, RiskLevel: RiskMedium, Reason: "strict mode: empty guardian response"}, nil
		}
		return &GuardianReview{Verdict: VerdictConfirm, RiskLevel: RiskMedium, Reason: "empty guardian response"}, nil
	}

	content := resp.Choices[0].Message.Content
	content = extractJSON(content)

	var review GuardianReview
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		slog.Error("Failed to parse guardian review", "component", "Guardian", "error", err)
		if g.level == config.SecurityStrict {
			return &GuardianReview{
				Verdict:   VerdictDeny,
				RiskLevel: RiskMedium,
				Reason:    "strict mode: failed to parse guardian review",
			}, nil
		}
		return &GuardianReview{
			Verdict:   VerdictConfirm,
			RiskLevel: RiskMedium,
			Reason:    "failed to parse guardian review",
		}, nil
	}

	// strict 模式：任何非 allow 都转为 deny
	if g.level == config.SecurityStrict && review.Verdict != VerdictAllow {
		review.Verdict = VerdictDeny
		review.Reason = "strict mode: " + review.Reason
	}

	slog.Info("Guardian review completed", "component", "Guardian",
		"tool", toolName,
		"level", g.level,
		"verdict", string(review.Verdict),
		"risk_level", string(review.RiskLevel),
		"reason", review.Reason)

	return &review, nil
}

func (g *GuardianService) IsEnabled() bool {
	return g.enabled
}

func (g *GuardianService) SetEnabled(enabled bool) {
	g.enabled = enabled
}

func (g *GuardianService) findReviewModel() string {
	models, err := g.modelCaller.ListModels()
	if err != nil || len(models) == 0 {
		return ""
	}

	for _, m := range models {
		for _, tag := range m.Tags {
			if strings.TrimSpace(tag) == "fast" {
				return m.ID
			}
		}
	}

	for _, m := range models {
		if m.Status == "enabled" {
			return m.ID
		}
	}

	return models[0].ID
}
