package services

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type ToolProfile struct {
	Name         string
	Description  string
	Mode         string
	AllowedTools []string
	DeniedTools  []string
	Prompt       string
}

type SmartToolSelector struct {
	mu             sync.RWMutex
	profiles       map[string]*ToolProfile
	defaultProfile string
}

func NewSmartToolSelector() *SmartToolSelector {
	s := &SmartToolSelector{
		profiles:       make(map[string]*ToolProfile),
		defaultProfile: "build",
	}
	s.initBuiltinProfiles()
	return s
}

func (s *SmartToolSelector) initBuiltinProfiles() {
	s.profiles["build"] = &ToolProfile{
		Name:         "build",
		Description:  "Default primary agent for code editing and execution",
		Mode:         "primary",
		AllowedTools: []string{"*"},
		DeniedTools:  []string{},
		Prompt:       "You are a coding assistant. Execute tools based on configured permissions.",
	}
	s.profiles["explore"] = &ToolProfile{
		Name:         "explore",
		Description:  "Fast agent specialized for exploring codebases",
		Mode:         "subagent",
		AllowedTools: []string{"search", "web_search", "read_file", "list_directory", "execute_code"},
		DeniedTools:  []string{"write_file", "delete_file"},
		Prompt:       "Fast agent specialized for exploring codebases. Use this when you need to quickly find files, search code for keywords, or answer questions about the codebase. Specify thoroughness: quick/medium/very thorough.",
	}
	s.profiles["general"] = &ToolProfile{
		Name:         "general",
		Description:  "General-purpose agent for researching complex questions and executing multi-step tasks",
		Mode:         "subagent",
		AllowedTools: []string{"*"},
		DeniedTools:  []string{},
		Prompt:       "General-purpose agent for researching complex questions and executing multi-step tasks.",
	}
	s.profiles["plan"] = &ToolProfile{
		Name:         "plan",
		Description:  "Plan mode for analyzing tasks without making changes",
		Mode:         "compaction",
		AllowedTools: []string{"search", "web_search", "read_file", "list_directory"},
		DeniedTools:  []string{"write_file", "delete_file", "execute_code"},
		Prompt:       "Plan mode. Analyze the task and create a plan without making any changes.",
	}
}

func (s *SmartToolSelector) SelectTools(query string, allTools []map[string]interface{}) []map[string]interface{} {
	profileName := s.SelectProfile(query)
	return s.FilterToolsByProfile(profileName, allTools)
}

func (s *SmartToolSelector) SelectProfile(query string) string {
	lower := strings.ToLower(query)

	if strings.Contains(lower, "/mode ") {
		parts := strings.SplitN(lower, "/mode ", 2)
		if len(parts) == 2 {
			mode := strings.TrimSpace(parts[1])
			switch mode {
			case "build", "explore", "plan", "general":
				return mode
			}
		}
	}

	exploreKeywords := []string{"搜索", "查找", "探索", "search", "find", "explore"}
	for _, kw := range exploreKeywords {
		if strings.Contains(lower, kw) {
			return "explore"
		}
	}

	planKeywords := []string{"规划", "计划", "分析", "plan", "analyze", "design"}
	for _, kw := range planKeywords {
		if strings.Contains(lower, kw) {
			return "plan"
		}
	}

	buildKeywords := []string{"执行", "运行", "修改", "写", "execute", "run", "modify", "write"}
	for _, kw := range buildKeywords {
		if strings.Contains(lower, kw) {
			return "build"
		}
	}

	return "general"
}

func (s *SmartToolSelector) FilterToolsByProfile(profileName string, tools []map[string]interface{}) []map[string]interface{} {
	s.mu.RLock()
	profile, ok := s.profiles[profileName]
	s.mu.RUnlock()

	if !ok {
		return tools
	}

	allowedAll := false
	for _, a := range profile.AllowedTools {
		if a == "*" {
			allowedAll = true
			break
		}
	}

	var result []map[string]interface{}
	for _, tool := range tools {
		toolName := extractToolName(tool)
		if toolName == "" {
			result = append(result, tool)
			continue
		}

		if s.isDenied(profile, toolName) {
			continue
		}

		if allowedAll {
			result = append(result, tool)
			continue
		}

		if s.isAllowed(profile, toolName) {
			result = append(result, tool)
		}
	}

	return result
}

func (s *SmartToolSelector) isDenied(profile *ToolProfile, toolName string) bool {
	for _, d := range profile.DeniedTools {
		if d == "*" {
			return true
		}
		if strings.HasSuffix(d, "*") {
			prefix := strings.TrimSuffix(d, "*")
			if strings.HasPrefix(toolName, prefix) {
				return true
			}
		}
		if toolName == d {
			return true
		}
	}
	return false
}

func (s *SmartToolSelector) isAllowed(profile *ToolProfile, toolName string) bool {
	for _, a := range profile.AllowedTools {
		if a == "*" {
			return true
		}
		if strings.HasSuffix(a, "*") {
			prefix := strings.TrimSuffix(a, "*")
			if strings.HasPrefix(toolName, prefix) {
				return true
			}
		}
		if toolName == a {
			return true
		}
	}
	return false
}

func extractToolName(tool map[string]interface{}) string {
	fn, ok := tool["function"]
	if !ok {
		return ""
	}
	fnMap, ok := fn.(map[string]interface{})
	if !ok {
		return ""
	}
	name, ok := fnMap["name"]
	if !ok {
		return ""
	}
	nameStr, ok := name.(string)
	if !ok {
		return ""
	}
	return nameStr
}

func (s *SmartToolSelector) GetProfile(name string) *ToolProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profiles[name]
}

func (s *SmartToolSelector) ListProfiles() []ToolProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var names []string
	for k := range s.profiles {
		names = append(names, k)
	}
	sort.Strings(names)

	var result []ToolProfile
	for _, n := range names {
		result = append(result, *s.profiles[n])
	}
	return result
}

func (s *SmartToolSelector) RegisterProfile(profile *ToolProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[profile.Name] = profile
}

func (s *SmartToolSelector) AnalyzeContext(recentToolCalls []string) string {
	if len(recentToolCalls) == 0 {
		return "general"
	}

	readOnlyCount := 0
	writeCount := 0
	readOnlyTools := map[string]bool{
		"search": true, "web_search": true, "read_file": true, "list_directory": true,
	}
	writeTools := map[string]bool{
		"write_file": true, "delete_file": true, "execute_code": true,
	}

	recent := recentToolCalls
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}

	for _, call := range recent {
		if readOnlyTools[call] {
			readOnlyCount++
		}
		if writeTools[call] {
			writeCount++
		}
	}

	if writeCount > 0 {
		return "build"
	}

	if readOnlyCount >= 3 {
		return "explore"
	}

	return "general"
}

func (s *SmartToolSelector) DefaultProfile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaultProfile
}

func (s *SmartToolSelector) SetDefaultProfile(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.profiles[name]; !ok {
		return fmt.Errorf("profile not found: %s", name)
	}
	s.defaultProfile = name
	return nil
}
