package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
)

type PermissionAction string

const (
	PermissionAllow PermissionAction = "allow"
	PermissionAsk   PermissionAction = "ask"
	PermissionDeny  PermissionAction = "deny"
)

type PermissionName string

const (
	PermRead      PermissionName = "read"
	PermEdit      PermissionName = "edit"
	PermBash      PermissionName = "bash"
	PermWeb       PermissionName = "web"
	PermDocker    PermissionName = "docker"
	PermDatabase  PermissionName = "database"
	PermGit       PermissionName = "git"
	PermTask      PermissionName = "task"
	PermExternal  PermissionName = "external_directory"
)

type PermissionRule struct {
	Permission PermissionName `json:"permission"`
	Pattern    string         `json:"pattern"`
	Action     PermissionAction `json:"action"`
}

type AgentMode string

const (
	AgentModeBuild    AgentMode = "build"
	AgentModePlan     AgentMode = "plan"
	AgentModeGeneral  AgentMode = "general"
	AgentModeExplore  AgentMode = "explore"
)

type AgentPermissionProfile struct {
	Mode         AgentMode
	Description  string
	Rules        []PermissionRule
	AllowedTools []string
	DeniedTools  []string
}

type PermissionCheckResult struct {
	Action      PermissionAction
	Reason      string
	RuleMatched string
}

type PermissionService struct {
	mu             sync.RWMutex
	profiles       map[AgentMode]*AgentPermissionProfile
	globalRules    []PermissionRule
	sessionApproved map[string][]PermissionRule
	pendingAsks    map[string]chan PermissionAction
	toolSvc        *ToolService
	eventBus       *EventBus
}

func NewPermissionService(toolSvc *ToolService, eventBus *EventBus) *PermissionService {
	s := &PermissionService{
		toolSvc:        toolSvc,
		eventBus:       eventBus,
		profiles:       make(map[AgentMode]*AgentPermissionProfile),
		sessionApproved: make(map[string][]PermissionRule),
		pendingAsks:    make(map[string]chan PermissionAction),
	}

	s.initProfiles()
	s.initGlobalRules()

	return s
}

func (s *PermissionService) initProfiles() {
	s.profiles[AgentModeBuild] = &AgentPermissionProfile{
		Mode:        AgentModeBuild,
		Description: "Full-access agent for development work",
		Rules: []PermissionRule{
			{PermRead, "*", PermissionAllow},
			{PermEdit, "*", PermissionAllow},
			{PermBash, "*", PermissionAllow},
			{PermWeb, "*", PermissionAllow},
			{PermDocker, "*", PermissionAllow},
			{PermDatabase, "*", PermissionAllow},
			{PermGit, "*", PermissionAllow},
			{PermTask, "*", PermissionAllow},
			{PermExternal, "*", PermissionAsk},
		},
	}

	s.profiles[AgentModePlan] = &AgentPermissionProfile{
		Mode:        AgentModePlan,
		Description: "Read-only agent for analysis and planning",
		Rules: []PermissionRule{
			{PermRead, "*", PermissionAllow},
			{PermEdit, "*", PermissionDeny},
			{PermEdit, "*.md", PermissionAllow},
			{PermBash, "*", PermissionAsk},
			{PermBash, "git status*", PermissionAllow},
			{PermBash, "git diff*", PermissionAllow},
			{PermBash, "git log*", PermissionAllow},
			{PermBash, "ls*", PermissionAllow},
			{PermBash, "cat*", PermissionAllow},
			{PermBash, "find*", PermissionAllow},
			{PermBash, "grep*", PermissionAllow},
			{PermWeb, "*", PermissionAllow},
			{PermDocker, "*", PermissionDeny},
			{PermDatabase, "*", PermissionDeny},
			{PermGit, "git status*", PermissionAllow},
			{PermGit, "git diff*", PermissionAllow},
			{PermGit, "git log*", PermissionAllow},
			{PermTask, "*", PermissionDeny},
			{PermExternal, "*", PermissionAsk},
		},
		DeniedTools: []string{"write_file", "execute_command", "docker", "database", "run_code"},
	}

	s.profiles[AgentModeGeneral] = &AgentPermissionProfile{
		Mode:        AgentModeGeneral,
		Description: "General-purpose subagent for research and multi-step tasks",
		Rules: []PermissionRule{
			{PermRead, "*", PermissionAllow},
			{PermEdit, "*", PermissionDeny},
			{PermBash, "*", PermissionAsk},
			{PermWeb, "*", PermissionAllow},
			{PermDocker, "*", PermissionDeny},
			{PermDatabase, "*", PermissionDeny},
			{PermGit, "git status*", PermissionAllow},
			{PermGit, "git diff*", PermissionAllow},
			{PermTask, "*", PermissionDeny},
			{PermExternal, "*", PermissionAsk},
		},
		DeniedTools: []string{"write_file", "docker", "database", "run_code"},
	}

	s.profiles[AgentModeExplore] = &AgentPermissionProfile{
		Mode:        AgentModeExplore,
		Description: "Fast read-only agent for codebase exploration",
		Rules: []PermissionRule{
			{PermRead, "*", PermissionAllow},
			{PermEdit, "*", PermissionDeny},
			{PermBash, "*", PermissionAsk},
			{PermWeb, "*", PermissionAllow},
			{PermDocker, "*", PermissionDeny},
			{PermDatabase, "*", PermissionDeny},
			{PermGit, "git status*", PermissionAllow},
			{PermGit, "git diff*", PermissionAllow},
			{PermTask, "*", PermissionDeny},
			{PermExternal, "*", PermissionAsk},
		},
		DeniedTools: []string{"write_file", "execute_command", "docker", "database", "run_code", "code_format", "lint"},
	}
}

func (s *PermissionService) initGlobalRules() {
	s.globalRules = []PermissionRule{
		{PermRead, "*.env", PermissionDeny},
		{PermRead, "*.env.*", PermissionDeny},
		{PermRead, "*.key", PermissionDeny},
		{PermRead, "*.pem", PermissionDeny},
		{PermBash, "rm *", PermissionAsk},
		{PermBash, "sudo *", PermissionAsk},
		{PermBash, "shutdown*", PermissionDeny},
		{PermBash, "reboot*", PermissionDeny},
		{PermBash, "mkfs*", PermissionDeny},
		{PermBash, "dd *", PermissionDeny},
	}
}

func (s *PermissionService) Check(ctx context.Context, sessionID string, agentMode AgentMode, perm PermissionName, target string) PermissionCheckResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if sessionRules, ok := s.sessionApproved[sessionID]; ok {
		for i := len(sessionRules) - 1; i >= 0; i-- {
			rule := sessionRules[i]
			if rule.Permission == perm && matchPattern(rule.Pattern, target) {
				return PermissionCheckResult{
					Action:      rule.Action,
					Reason:      fmt.Sprintf("session approved rule: %s %s -> %s", rule.Permission, rule.Pattern, rule.Action),
					RuleMatched: rule.Pattern,
				}
			}
		}
	}

	profile, ok := s.profiles[agentMode]
	if ok {
		for i := len(profile.Rules) - 1; i >= 0; i-- {
			rule := profile.Rules[i]
			if rule.Permission == perm && matchPattern(rule.Pattern, target) {
				return PermissionCheckResult{
					Action:      rule.Action,
					Reason:      fmt.Sprintf("agent %s rule: %s %s -> %s", agentMode, rule.Permission, rule.Pattern, rule.Action),
					RuleMatched: rule.Pattern,
				}
			}
		}
	}

	for i := len(s.globalRules) - 1; i >= 0; i-- {
		rule := s.globalRules[i]
		if rule.Permission == perm && matchPattern(rule.Pattern, target) {
			return PermissionCheckResult{
				Action:      rule.Action,
				Reason:      fmt.Sprintf("global rule: %s %s -> %s", rule.Permission, rule.Pattern, rule.Action),
				RuleMatched: rule.Pattern,
			}
		}
	}

	return PermissionCheckResult{
		Action: PermissionAsk,
		Reason: "no matching rule, default ask for safety",
	}
}

func (s *PermissionService) CheckToolAccess(agentMode AgentMode, toolName string) PermissionCheckResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	profile, ok := s.profiles[agentMode]
	if !ok {
		return PermissionCheckResult{Action: PermissionAsk, Reason: "unknown agent mode, default ask for safety"}
	}

	for _, denied := range profile.DeniedTools {
		if denied == toolName {
			return PermissionCheckResult{
				Action: PermissionDeny,
				Reason: fmt.Sprintf("agent %s denies tool: %s", agentMode, toolName),
			}
		}
	}

	if len(profile.AllowedTools) > 0 {
		for _, allowed := range profile.AllowedTools {
			if allowed == toolName {
				return PermissionCheckResult{
					Action: PermissionAllow,
					Reason: fmt.Sprintf("agent %s allows tool: %s", agentMode, toolName),
				}
			}
		}
		return PermissionCheckResult{
			Action: PermissionAsk,
			Reason: fmt.Sprintf("tool %s not in agent %s allowed list", toolName, agentMode),
		}
	}

	return PermissionCheckResult{Action: PermissionAllow, Reason: "tool not in deny list"}
}

func (s *PermissionService) FilterToolsForAgent(agentMode AgentMode, toolDefs []map[string]interface{}) []map[string]interface{} {
	var filtered []map[string]interface{}
	for _, def := range toolDefs {
		fnMap, _ := def["function"].(map[string]interface{})
		if fnMap == nil {
			filtered = append(filtered, def)
			continue
		}
		name, _ := fnMap["name"].(string)
		result := s.CheckToolAccess(agentMode, name)
		if result.Action != PermissionDeny {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func (s *PermissionService) AskAndWait(ctx context.Context, sessionID string, perm PermissionName, target string) PermissionAction {
	askID := fmt.Sprintf("%s-%s-%d", sessionID, perm, time.Now().UnixNano())
	ch := make(chan PermissionAction, 1)

	s.mu.Lock()
	s.pendingAsks[askID] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pendingAsks, askID)
		s.mu.Unlock()
	}()

	if s.eventBus != nil {
		s.eventBus.Publish(ctx, models.EventTopicTool, "permission_ask", "permission", map[string]interface{}{
			"ask_id":     askID,
			"session_id": sessionID,
			"permission": string(perm),
			"target":     target,
		})
	}

	select {
	case action := <-ch:
		if action == PermissionAllow {
			s.mu.Lock()
			s.sessionApproved[sessionID] = append(s.sessionApproved[sessionID], PermissionRule{
				Permission: perm,
				Pattern:    inferSafePattern(perm, target),
				Action:     PermissionAllow,
			})
			s.mu.Unlock()
		}
		return action
	case <-ctx.Done():
		return PermissionDeny
	case <-time.After(5 * time.Minute):
		return PermissionDeny
	}
}

func (s *PermissionService) RespondAsk(askID string, action PermissionAction) error {
	s.mu.Lock()
	ch, ok := s.pendingAsks[askID]
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("pending ask not found: %s", askID)
	}

	ch <- action
	return nil
}

func (s *PermissionService) GetProfile(mode AgentMode) *AgentPermissionProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profiles[mode]
}

func (s *PermissionService) ListProfiles() []*AgentPermissionProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AgentPermissionProfile, 0, len(s.profiles))
	for _, p := range s.profiles {
		result = append(result, p)
	}
	return result
}

func (s *PermissionService) UpdateGlobalRules(rules []PermissionRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalRules = rules
}

func (s *PermissionService) ClearSessionApprovals(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionApproved, sessionID)
}

func matchPattern(pattern, target string) bool {
	if pattern == "*" {
		return true
	}
	if strings.Contains(pattern, "*") {
		matched, _ := filepath.Match(pattern, target)
		return matched
	}
	return pattern == target || strings.HasPrefix(target, pattern)
}

func inferSafePattern(perm PermissionName, target string) string {
	parts := strings.Fields(target)
	if len(parts) <= 1 {
		return target + "*"
	}

	switch perm {
	case PermBash:
		if len(parts) >= 2 {
			return parts[0] + " " + parts[1] + "*"
		}
		return parts[0] + "*"
	case PermEdit, PermRead:
		return target
	default:
		return target + "*"
	}
}

func PermissionNameForTool(toolName string) PermissionName {
	switch toolName {
	case "read_file", "file_search", "dependency":
		return PermRead
	case "write_file", "code_format", "lint":
		return PermEdit
	case "execute_command", "run_code":
		return PermBash
	case "http_request", "search_web", "api_test":
		return PermWeb
	case "docker":
		return PermDocker
	case "database":
		return PermDatabase
	case "git":
		return PermGit
	default:
		return PermRead
	}
}
