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

const guardianPrompt = `You are a security guardian for a development assistant. Evaluate whether the following tool call is safe to execute.

Rate the operation on these dimensions:
1. data_exfiltration: Could this leak sensitive data? (credentials, API keys, private user data)
2. credential_probing: Could this access or expose authentication credentials?
3. persistent_security_weakening: Could this permanently weaken system security? (opening ports, disabling firewalls, adding SSH keys)
4. destructive_action: Could this cause irreversible damage? (deleting files, dropping databases, formatting disks)

Tool: %s
Arguments: %s
Context: %s

Respond in JSON format:
{
  "verdict": "allow|confirm|deny",
  "risk_level": "none|low|medium|high|critical",
  "risks": {
    "data_exfiltration": "none|low|medium|high",
    "credential_probing": "none|low|medium|high",
    "persistent_security_weakening": "none|low|medium|high",
    "destructive_action": "none|low|medium|high"
  },
  "reason": "Brief explanation of the verdict",
  "suggestions": ["Safer alternatives if any"]
}

Rules:
- "allow": Safe operations, including:
  * Reading files, searching code, listing directories
  * Creating directories (mkdir) for project scaffolding
  * Writing source code files (.rs, .py, .js, .go, .toml, .yaml, .json, .html, .css, .md, etc.)
  * Installing packages via package managers (apt, yum, npm, cargo, pip)
  * Running build commands (cargo build, npm install, go build, make)
  * Git operations (clone, pull, push, commit, status, log)
  * Non-destructive system queries (ls, cat, grep, find, ps, df, free, top)
- "confirm": Operations with moderate risk that should be confirmed:
  * Executing commands that modify system state (systemctl, service, chmod, chown)
  * Writing to system directories (/etc, /usr, /bin, /sbin)
  * Network operations that expose services (starting servers on public interfaces)
  * Operations involving sudo or elevated privileges
- "deny": Dangerous operations that should be blocked:
  * rm -rf / or similar destructive patterns
  * Exposing credentials or API keys
  * Disabling security mechanisms
  * Formatting disks or dropping databases without explicit user intent
- For development tasks (creating projects, writing code, building software), prefer "allow"
- Only use "confirm" for operations that genuinely pose security risk
- Never "deny" operations that the user explicitly requested as part of their development workflow`

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
