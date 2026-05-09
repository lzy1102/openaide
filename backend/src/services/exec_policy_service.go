package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
)

type ExecPolicyDecision string

const (
	ExecPolicyAllow ExecPolicyDecision = "allow"
	ExecPolicyAsk   ExecPolicyDecision = "ask"
	ExecPolicyDeny  ExecPolicyDecision = "deny"
)

type ExecPolicyRule struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Pattern     string             `json:"pattern"`
	Decision    ExecPolicyDecision `json:"decision"`
	Reason      string             `json:"reason"`
	Permissions []string           `json:"permissions,omitempty"`
	Network     bool               `json:"network,omitempty"`
	Priority    int                `json:"priority"`
	Source      string             `json:"source"`
}

type ExecPolicyEvaluation struct {
	Decision      ExecPolicyDecision `json:"decision"`
	Reason        string             `json:"reason"`
	RuleMatched   string             `json:"rule_matched,omitempty"`
	RiskLevel     string             `json:"risk_level"`
	AutoApproved  bool               `json:"auto_approved"`
	Sandboxed     bool               `json:"sandboxed"`
}

var dangerousCommands = map[string]bool{
	"rm": true, "rmdir": true, "sudo": true, "su": true,
	"shutdown": true, "reboot": true, "halt": true, "poweroff": true,
	"mkfs": true, "dd": true, "fdisk": true, "format": true,
	"chmod": true, "chown": true, "chgrp": true,
	"kill": true, "killall": true, "pkill": true,
	"curl": true, "wget": true,
	"ssh": true, "scp": true, "rsync": true,
	"docker": true, "kubectl": true,
	"pip": true, "npm": true, "yarn": true, "gem": true,
	"apt": true, "yum": true, "brew": true,
	"git push": true, "git reset": true, "git clean": true,
	"python": true, "python3": true, "node": true, "ruby": true,
	"bash": true, "sh": true, "zsh": true,
	"eval": true, "exec": true,
}

var safeCommands = map[string]bool{
	"ls": true, "dir": true, "pwd": true, "echo": true, "cat": true,
	"head": true, "tail": true, "less": true, "more": true,
	"grep": true, "find": true, "which": true, "where": true,
	"wc": true, "sort": true, "uniq": true, "diff": true,
	"git status": true, "git diff": true, "git log": true, "git branch": true,
	"git show": true, "git stash": true, "git remote": true,
	"tree": true, "du": true, "df": true, "free": true,
	"ps": true, "top": true, "uptime": true,
	"whoami": true, "hostname": true, "uname": true,
	"env": true, "printenv": true, "type": true,
	"file": true, "stat": true,
	"go vet": true, "go test": true, "go list": true, "go doc": true,
	"cargo check": true, "cargo test": true, "cargo doc": true,
}

var dangerousPaths = []string{
	"/etc/passwd", "/etc/shadow", "/etc/hosts",
	"/.ssh/", "/.gnupg/", "/.env",
	"id_rsa", "id_ed25519", ".pem", ".key",
}

type ExecPolicyService struct {
	mu          sync.RWMutex
	rules       []ExecPolicyRule
	eventBus    *EventBus
	configDir   string
}

func NewExecPolicyService(eventBus *EventBus) *ExecPolicyService {
	s := &ExecPolicyService{
		eventBus:  eventBus,
		configDir: ".openaide",
	}
	s.initDefaultRules()
	s.loadRulesFromFiles()
	return s
}

func (s *ExecPolicyService) initDefaultRules() {
	s.rules = []ExecPolicyRule{
		{ID: "deny-shutdown", Name: "Block system shutdown", Pattern: "shutdown*", Decision: ExecPolicyDeny, Reason: "System shutdown commands are blocked", Priority: 100, Source: "builtin"},
		{ID: "deny-reboot", Name: "Block system reboot", Pattern: "reboot*", Decision: ExecPolicyDeny, Reason: "System reboot commands are blocked", Priority: 100, Source: "builtin"},
		{ID: "deny-mkfs", Name: "Block disk format", Pattern: "mkfs*", Decision: ExecPolicyDeny, Reason: "Disk formatting commands are blocked", Priority: 100, Source: "builtin"},
		{ID: "deny-dd", Name: "Block dd", Pattern: "dd *", Decision: ExecPolicyDeny, Reason: "dd command is blocked for safety", Priority: 100, Source: "builtin"},
		{ID: "ask-sudo", Name: "Confirm sudo", Pattern: "sudo *", Decision: ExecPolicyAsk, Reason: "Sudo requires confirmation", Priority: 90, Source: "builtin"},
		{ID: "ask-rm", Name: "Confirm file deletion", Pattern: "rm *", Decision: ExecPolicyAsk, Reason: "File deletion requires confirmation", Priority: 90, Source: "builtin"},
		{ID: "ask-rmdir", Name: "Confirm directory deletion", Pattern: "rmdir *", Decision: ExecPolicyAsk, Reason: "Directory deletion requires confirmation", Priority: 90, Source: "builtin"},
		{ID: "ask-curl", Name: "Confirm network requests", Pattern: "curl *", Decision: ExecPolicyAsk, Reason: "Network requests require confirmation", Network: true, Priority: 80, Source: "builtin"},
		{ID: "ask-wget", Name: "Confirm downloads", Pattern: "wget *", Decision: ExecPolicyAsk, Reason: "Downloads require confirmation", Network: true, Priority: 80, Source: "builtin"},
		{ID: "ask-docker", Name: "Confirm docker", Pattern: "docker *", Decision: ExecPolicyAsk, Reason: "Docker commands require confirmation", Priority: 80, Source: "builtin"},
		{ID: "ask-pip", Name: "Confirm package install", Pattern: "pip *", Decision: ExecPolicyAsk, Reason: "Package installation requires confirmation", Priority: 70, Source: "builtin"},
		{ID: "ask-npm", Name: "Confirm npm", Pattern: "npm *", Decision: ExecPolicyAsk, Reason: "npm commands require confirmation", Priority: 70, Source: "builtin"},
		{ID: "ask-git-push", Name: "Confirm git push", Pattern: "git push*", Decision: ExecPolicyAsk, Reason: "Git push requires confirmation", Priority: 85, Source: "builtin"},
		{ID: "ask-git-reset", Name: "Confirm git reset", Pattern: "git reset*", Decision: ExecPolicyAsk, Reason: "Git reset requires confirmation", Priority: 85, Source: "builtin"},
		{ID: "safe-ls", Name: "Allow listing", Pattern: "ls *", Decision: ExecPolicyAllow, Reason: "Directory listing is safe", Priority: 10, Source: "builtin"},
		{ID: "safe-cat", Name: "Allow reading", Pattern: "cat *", Decision: ExecPolicyAllow, Reason: "File reading is safe", Priority: 10, Source: "builtin"},
		{ID: "safe-git-status", Name: "Allow git status", Pattern: "git status*", Decision: ExecPolicyAllow, Reason: "Git status is safe", Priority: 10, Source: "builtin"},
		{ID: "safe-git-diff", Name: "Allow git diff", Pattern: "git diff*", Decision: ExecPolicyAllow, Reason: "Git diff is safe", Priority: 10, Source: "builtin"},
		{ID: "safe-git-log", Name: "Allow git log", Pattern: "git log*", Decision: ExecPolicyAllow, Reason: "Git log is safe", Priority: 10, Source: "builtin"},
		{ID: "safe-find", Name: "Allow find", Pattern: "find *", Decision: ExecPolicyAllow, Reason: "Find is safe", Priority: 10, Source: "builtin"},
		{ID: "safe-grep", Name: "Allow grep", Pattern: "grep *", Decision: ExecPolicyAllow, Reason: "Grep is safe", Priority: 10, Source: "builtin"},
		{ID: "safe-go-test", Name: "Allow go test", Pattern: "go test*", Decision: ExecPolicyAllow, Reason: "Go test is safe", Priority: 10, Source: "builtin"},
	}
}

func (s *ExecPolicyService) loadRulesFromFiles() {
	searchPaths := []string{
		filepath.Join(s.configDir, "execpolicy.rules"),
		".openaide/execpolicy.rules",
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		searchPaths = append(searchPaths, filepath.Join(homeDir, ".openaide", "execpolicy.rules"))
	}
	for _, p := range searchPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s.parseRulesFile(string(data), p)
		slog.Info("Loaded exec policy rules", "component", "ExecPolicy", "path", p)
	}
}

func (s *ExecPolicyService) parseRulesFile(content, source string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		decision := ExecPolicyDecision(strings.ToLower(parts[0]))
		if decision != ExecPolicyAllow && decision != ExecPolicyAsk && decision != ExecPolicyDeny {
			continue
		}
		pattern := parts[1]
		reason := strings.Join(parts[2:], " ")
		rule := ExecPolicyRule{
			ID:       fmt.Sprintf("file-%s-%s", source, pattern),
			Name:     fmt.Sprintf("Rule: %s %s", decision, pattern),
			Pattern:  pattern,
			Decision: decision,
			Reason:   reason,
			Priority: 50,
			Source:   source,
		}
		s.rules = append(s.rules, rule)
	}
}

func (s *ExecPolicyService) Evaluate(ctx context.Context, command string) ExecPolicyEvaluation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cmdLower := strings.ToLower(strings.TrimSpace(command))
	baseCmd := s.extractBaseCommand(cmdLower)

	if s.isDangerousPath(command) {
		s.publishEvent(ctx, "exec_policy_deny", command, "Dangerous path access")
		return ExecPolicyEvaluation{
			Decision:    ExecPolicyDeny,
			Reason:      "Command accesses a protected path",
			RiskLevel:   "critical",
			AutoApproved: false,
			Sandboxed:   true,
		}
	}

	for _, rule := range s.rules {
		if s.matchRule(rule.Pattern, cmdLower) || s.matchRule(rule.Pattern, baseCmd) {
			eval := ExecPolicyEvaluation{
				Decision:     rule.Decision,
				Reason:       rule.Reason,
				RuleMatched:  rule.ID,
				RiskLevel:    s.riskLevel(rule.Decision),
				AutoApproved: rule.Decision == ExecPolicyAllow,
				Sandboxed:    rule.Decision == ExecPolicyAsk || rule.Network,
			}
			s.publishEvent(ctx, "exec_policy_"+string(rule.Decision), command, rule.Reason)
			return eval
		}
	}

	if safeCommands[baseCmd] {
		return ExecPolicyEvaluation{
			Decision:     ExecPolicyAllow,
			Reason:       "Known safe command",
			RiskLevel:    "low",
			AutoApproved: true,
			Sandboxed:    false,
		}
	}

	if dangerousCommands[baseCmd] {
		s.publishEvent(ctx, "exec_policy_ask", command, "Dangerous command requires approval")
		return ExecPolicyEvaluation{
			Decision:     ExecPolicyAsk,
			Reason:       fmt.Sprintf("Command '%s' requires approval", baseCmd),
			RiskLevel:    "high",
			AutoApproved: false,
			Sandboxed:    true,
		}
	}

	return ExecPolicyEvaluation{
		Decision:     ExecPolicyAsk,
		Reason:       "Unknown command, requires approval by default",
		RiskLevel:    "medium",
		AutoApproved: false,
		Sandboxed:    false,
	}
}

func (s *ExecPolicyService) EvaluateFileAccess(ctx context.Context, path string, write bool) ExecPolicyEvaluation {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath, _ = filepath.Abs(path)
	}

	for _, dp := range dangerousPaths {
		if strings.Contains(absPath, dp) {
			return ExecPolicyEvaluation{
				Decision:     ExecPolicyDeny,
				Reason:       fmt.Sprintf("Access to protected path: %s", dp),
				RiskLevel:    "critical",
				AutoApproved: false,
				Sandboxed:    true,
			}
		}
	}

	if write {
		return ExecPolicyEvaluation{
			Decision:     ExecPolicyAsk,
			Reason:       "File write requires approval",
			RiskLevel:    "medium",
			AutoApproved: false,
			Sandboxed:    false,
		}
	}

	return ExecPolicyEvaluation{
		Decision:     ExecPolicyAllow,
		Reason:       "File read is allowed",
		RiskLevel:    "low",
		AutoApproved: true,
		Sandboxed:    false,
	}
}

func (s *ExecPolicyService) AddRule(rule ExecPolicyRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append(s.rules, rule)
	s.sortRules()
}

func (s *ExecPolicyService) RemoveRule(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.rules {
		if r.ID == id {
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			return true
		}
	}
	return false
}

func (s *ExecPolicyService) ListRules() []ExecPolicyRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ExecPolicyRule, len(s.rules))
	copy(result, s.rules)
	return result
}

func (s *ExecPolicyService) ReloadRules() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = nil
	s.initDefaultRules()
	s.loadRulesFromFiles()
	s.sortRules()
}

func (s *ExecPolicyService) extractBaseCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}
	base := parts[0]
	if len(parts) >= 2 {
		if parts[0] == "git" || parts[0] == "go" || parts[0] == "docker" || parts[0] == "cargo" {
			base = parts[0] + " " + parts[1]
		}
	}
	return base
}

func (s *ExecPolicyService) matchRule(pattern, target string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(target, pattern[:len(pattern)-1])
	}
	return pattern == target
}

func (s *ExecPolicyService) isDangerousPath(command string) bool {
	cmdLower := strings.ToLower(command)
	for _, dp := range dangerousPaths {
		if strings.Contains(cmdLower, strings.ToLower(dp)) {
			return true
		}
	}
	return false
}

func (s *ExecPolicyService) riskLevel(decision ExecPolicyDecision) string {
	switch decision {
	case ExecPolicyDeny:
		return "critical"
	case ExecPolicyAsk:
		return "high"
	case ExecPolicyAllow:
		return "low"
	default:
		return "medium"
	}
}

func (s *ExecPolicyService) sortRules() {
	sort.SliceStable(s.rules, func(i, j int) bool {
		return s.rules[i].Priority > s.rules[j].Priority
	})
}

func (s *ExecPolicyService) publishEvent(ctx context.Context, eventType, command, reason string) {
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicTool, eventType, "exec_policy", map[string]interface{}{
			"command": command,
			"reason":  reason,
			"ts":      time.Now().Format(time.RFC3339),
		})
	}
}
