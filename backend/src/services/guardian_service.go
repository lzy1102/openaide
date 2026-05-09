package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"openaide/backend/src/services/llm"
)

const guardianPrompt = `You are a security guardian. Evaluate whether the following tool call is safe to execute.

Rate the operation on these dimensions:
1. data_exfiltration: Could this leak sensitive data? (IPs, credentials, personal info, API keys)
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
- "allow": Safe operations (reading files, searching code, listing directories)
- "confirm": Potentially risky operations that need user confirmation (writing files, executing commands, network requests)
- "deny": Dangerous operations that should be blocked (rm -rf, exposing credentials, disabling security)
- When in doubt, use "confirm" rather than "allow"
- Never "allow" operations that could cause irreversible changes without confirmation`

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
	modelCaller ModelCaller
	enabled     bool
	autoAllow   map[string]bool
	autoDeny    map[string]bool
}

func NewGuardianService(modelCaller ModelCaller) *GuardianService {
	return &GuardianService{
		modelCaller: modelCaller,
		enabled:     true,
		autoAllow: map[string]bool{
			"read_file": true, "search": true, "web_search": true,
			"list_directory": true, "get_file_info": true,
		},
		autoDeny: map[string]bool{
			"rm_rf": true, "format": true, "dd_if": true,
		},
	}
}

func (g *GuardianService) Review(ctx context.Context, toolName, arguments, contextStr string) (*GuardianReview, error) {
	if !g.enabled {
		return &GuardianReview{Verdict: VerdictAllow, RiskLevel: RiskNone}, nil
	}

	if g.autoAllow[toolName] {
		return &GuardianReview{
			Verdict:   VerdictAllow,
			RiskLevel: RiskNone,
			Reason:    "auto-allowed for read-only tool",
		}, nil
	}

	lowerArgs := strings.ToLower(arguments)
	for denied := range g.autoDeny {
		if strings.Contains(lowerArgs, denied) {
			return &GuardianReview{
				Verdict:   VerdictDeny,
				RiskLevel: RiskCritical,
				Reason:    fmt.Sprintf("auto-denied: contains dangerous pattern '%s'", denied),
			}, nil
		}
	}

	if strings.Contains(lowerArgs, "rm -rf") || strings.Contains(lowerArgs, "rm -r /") {
		return &GuardianReview{
			Verdict:   VerdictDeny,
			RiskLevel: RiskCritical,
			Risks: GuardianRisk{DestructiveAction: "high"},
			Reason:   "recursive force delete detected",
		}, nil
	}

	modelID := g.findReviewModel()
	if modelID == "" {
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
		return &GuardianReview{
			Verdict:   VerdictConfirm,
			RiskLevel: RiskMedium,
			Reason:    "guardian review failed, requiring confirmation as precaution",
		}, nil
	}

	if len(resp.Choices) == 0 {
		return &GuardianReview{Verdict: VerdictConfirm, RiskLevel: RiskMedium, Reason: "empty guardian response"}, nil
	}

	content := resp.Choices[0].Message.Content
	content = extractJSON(content)

	var review GuardianReview
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		slog.Error("Failed to parse guardian review", "component", "Guardian", "error", err)
		return &GuardianReview{
			Verdict:   VerdictConfirm,
			RiskLevel: RiskMedium,
			Reason:    "failed to parse guardian review",
		}, nil
	}

	slog.Info("Guardian review completed", "component", "Guardian",
		"tool", toolName,
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
