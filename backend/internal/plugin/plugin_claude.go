package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClaudePluginManifest .claude-plugin/plugin.json 格式
type ClaudePluginManifest struct {
	Name        string       `json:"name" yaml:"name"`
	Version     string       `json:"version" yaml:"version"`
	Description string       `json:"description" yaml:"description"`
	Author      *ClaudeAuthor `json:"author,omitempty" yaml:"author,omitempty"`
}

// ClaudeAuthor 插件作者
type ClaudeAuthor struct {
	Name  string `json:"name" yaml:"name"`
	Email string `json:"email,omitempty" yaml:"email,omitempty"`
}

// ClaudeSkillFrontmatter skills/*/SKILL.md 的 YAML frontmatter
type ClaudeSkillFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	ArgumentHint string `yaml:"argument-hint,omitempty"`
	AllowedTools []string `yaml:"allowed-tools,omitempty"`
}

// DiscoverClaudePlugins 扫描目录，发现符合 Claude 规范的插件
// 目录结构: plugins/<name>/.claude-plugin/plugin.json
func DiscoverClaudePlugins(dir string) []*Plugin {
	var result []*Plugin

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, e.Name())
		manifestPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var manifest ClaudePluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		if manifest.Name == "" {
			continue
		}

		p := &Plugin{
			ID:          manifest.Name,
			Name:        manifest.Name,
			Description: manifest.Description,
			Version:     manifest.Version,
			Enabled:     true,
		}

		// 从 skills/*/SKILL.md 提取提示词注入
		p.SystemPrompt = buildPromptFromSkills(pluginDir)

		result = append(result, p)
	}

	return result
}

// DiscoverClaudeSkills 从 Claude 插件目录中发现 SKILL.md 技能
func DiscoverClaudeSkills(pluginsDir string) []ClaudeSkill {
	var result []ClaudeSkill

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginDir := filepath.Join(pluginsDir, e.Name())

		// 检查是否是 Claude 插件（有 .claude-plugin/plugin.json）
		if _, err := os.Stat(filepath.Join(pluginDir, ".claude-plugin", "plugin.json")); os.IsNotExist(err) {
			continue
		}

		// 扫描 skills/ 目录
		skillsDir := filepath.Join(pluginDir, "skills")
		skillEntries, err := os.ReadDir(skillsDir)
		if err != nil {
			continue
		}

		for _, se := range skillEntries {
			if !se.IsDir() {
				continue
			}
			skillDir := filepath.Join(skillsDir, se.Name())
			skillFile := filepath.Join(skillDir, "SKILL.md")
			fm, body, err := parseSkillMarkdown(skillFile)
			if err != nil {
				continue
			}

			result = append(result, ClaudeSkill{
				PluginName:   e.Name(),
				ID:           e.Name() + "/" + se.Name(),
				Name:         fm.Name,
				Description:  fm.Description,
				Prompt:       body,
				AllowedTools: mapClaudeTools(fm.AllowedTools),
				Keywords:     generateKeywords(fm.Name, fm.Description),
				ArgumentHint: fm.ArgumentHint,
			})
		}
	}

	return result
}

// ClaudeSkill 从 SKILL.md 解析出的技能
type ClaudeSkill struct {
	PluginName   string
	ID           string
	Name         string
	Description  string
	Prompt       string
	AllowedTools []string // Claude 工具名（已映射为 OpenAIDE 工具名）
	Keywords     []string // 自动生成的关键词
	ArgumentHint string
}

// parseSkillMarkdown 解析 SKILL.md: YAML frontmatter + Markdown body
func parseSkillMarkdown(path string) (*ClaudeSkillFrontmatter, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	content := string(data)
	// YAML frontmatter 以 --- 开头和结尾
	if !strings.HasPrefix(content, "---\n") {
		return nil, "", os.ErrInvalid
	}

	// 找到第二个 ---
	endIdx := strings.Index(content[4:], "\n---")
	if endIdx < 0 {
		endIdx = strings.Index(content[4:], "\n---\n")
	}
	if endIdx < 0 {
		return nil, "", os.ErrInvalid
	}

	yamlBlock := content[4 : 4+endIdx]
	body := strings.TrimSpace(content[4+endIdx+4:])

	var fm ClaudeSkillFrontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return nil, "", err
	}
	if fm.Name == "" {
		return nil, "", os.ErrInvalid
	}

	return &fm, body, nil
}

// MapClaudeTool 将 Claude Code 工具名映射到 OpenAIDE 工具名
func MapClaudeTool(claudeName string) string {
	if mapped, ok := claudeToolMap[claudeName]; ok {
		return mapped
	}
	return ""
}

// mapClaudeTools 批量映射工具名
func mapClaudeTools(claudeNames []string) []string {
	var result []string
	for _, n := range claudeNames {
		if mapped := MapClaudeTool(n); mapped != "" {
			result = append(result, mapped)
		}
	}
	return result
}

// claudeToolMap Claude Code 工具名 → OpenAIDE 工具名映射
var claudeToolMap = map[string]string{
	"Read":      "read_file",
	"Write":     "write_file",
	"Edit":      "diff_edit",
	"Glob":      "search_files",
	"Grep":      "search_files",
	"Bash":      "execute_command",
	"WebSearch": "web_search",
	"WebFetch":  "web_fetch",
	"Task":      "execute_command",
}

// generateKeywords 从 name 和 description 自动生成中英文关键词
func generateKeywords(name, description string) []string {
	var kw []string
	text := strings.ToLower(name + " " + description)

	// 英文关键词：name 的连字符分隔部分
	for _, part := range strings.Split(name, "-") {
		if len(part) > 1 {
			kw = append(kw, part)
		}
	}
	// name 整体
	kw = append(kw, name)

	// 英文常用词映射
	enKeywordMap := map[string][]string{
		"review":     {"review", "审查", "检查代码", "code review"},
		"commit":     {"commit", "提交", "git commit", "暂存"},
		"debug":      {"debug", "调试", "报错", "error", "bug", "不工作"},
		"refactor":   {"refactor", "重构", "重写", "整理代码"},
		"explain":    {"explain", "解释", "说明", "什么意思"},
		"test":       {"test", "测试", "单元测试", "集成测试"},
		"deploy":     {"deploy", "部署", "发布", "上线"},
		"security":   {"security", "安全", "漏洞", "审计"},
		"doc":        {"doc", "文档", "readme", "注释"},
		"format":     {"format", "格式化", "fmt", "lint"},
		"build":      {"build", "构建", "编译", "打包"},
		"ci":         {"ci", "pipeline", "流水线", "持续集成"},
		"migrate":    {"migrate", "迁移", "数据库迁移"},
		"translate":  {"translate", "翻译", "译文", "本地化"},
		"analyze":    {"analyze", "分析", "性能分析", "profiling"},
		"search":     {"search", "搜索", "查找", "检索"},
		"fix":        {"fix", "修复", "改正", "修正"},
		"generate":   {"generate", "生成", "创建", "新建"},
		"optimize":   {"optimize", "优化", "性能优化"},
	}

	added := map[string]bool{}
	for _, part := range strings.Split(name, "-") {
		if terms, ok := enKeywordMap[part]; ok {
			for _, t := range terms {
				if !added[t] {
					kw = append(kw, t)
					added[t] = true
				}
			}
		}
	}

	// 从 description 提取中文关键词（简单启发式）
	for _, term := range []string{"代码", "审查", "安全", "重构", "调试", "测试", "文档",
		"部署", "提交", "搜索", "解释", "分析", "翻译", "格式化", "迁移", "优化", "生成",
		"修复", "构建", "编译", "review", "debug", "test", "security", "deploy",
		"commit", "refactor", "format", "build", "search", "explain", "translate",
		"analyze", "migrate", "fix", "generate", "optimize"} {
		if strings.Contains(text, term) && !added[term] {
			kw = append(kw, term)
			added[term] = true
		}
	}

	return kw
}

// buildPromptFromSkills 从 plugins/<name>/skills/*/SKILL.md 构建 system prompt
func buildPromptFromSkills(pluginDir string) string {
	skillsDir := filepath.Join(pluginDir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return ""
	}

	var parts []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		fm, body, err := parseSkillMarkdown(skillFile)
		if err != nil {
			continue
		}
		if fm.Description != "" {
			parts = append(parts, "## "+fm.Name+"\n"+fm.Description+"\n\n"+body)
		} else {
			parts = append(parts, "## "+fm.Name+"\n\n"+body)
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// ============ .mcp.json 支持 ============

// MCPServerEntry .mcp.json 中的单个 MCP 服务器配置
type MCPServerEntry struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// DiscoverClaudeMCP 从 Claude 插件目录中发现 .mcp.json
func DiscoverClaudeMCP(dir string) map[string]MCPServerEntry {
	result := make(map[string]MCPServerEntry)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, e.Name())
		// 只处理 Claude 格式插件
		if _, err := os.Stat(filepath.Join(pluginDir, ".claude-plugin", "plugin.json")); os.IsNotExist(err) {
			continue
		}

		mcpPath := filepath.Join(pluginDir, ".mcp.json")
		data, err := os.ReadFile(mcpPath)
		if err != nil {
			continue
		}

		var servers map[string]MCPServerEntry
		if err := json.Unmarshal(data, &servers); err != nil {
			continue
		}

		for name, srv := range servers {
			key := e.Name() + "/" + name
			result[key] = srv
		}
	}

	return result
}

// ============ hooks/hooks.json 支持 ============

// HookEntry hooks.json 中的单个 hook
type HookEntry struct {
	Event   string   `json:"event"`
	Command string   `json:"command"`
	Tools   []string `json:"tools,omitempty"`
}

// HookConfig hooks.json 格式
type HookConfig struct {
	Hooks []HookEntry `json:"hooks"`
}

// Claude event → OpenAIDE event
var claudeToOpenAIDEEvent = map[string]string{
	"PreToolUse":   "tool_call_started",
	"PostToolUse":  "tool_call_ended",
	"Stop":         "session_ended",
	"SessionStart": "session_created",
	"UserPromptSubmit": "query_received",
}

// DiscoverClaudeHooks 从 Claude 插件目录中发现 hooks/hooks.json
func DiscoverClaudeHooks(dir string) []HookEntry {
	var result []HookEntry

	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(pluginDir, ".claude-plugin", "plugin.json")); os.IsNotExist(err) {
			continue
		}

		hookPath := filepath.Join(pluginDir, "hooks", "hooks.json")
		data, err := os.ReadFile(hookPath)
		if err != nil {
			continue
		}

		var cfg HookConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}

		result = append(result, cfg.Hooks...)
	}

	return result
}

// MapClaudeToolReverse 将 OpenAIDE 工具名反向映射回 Claude 工具名（用于 hook 匹配）
func MapClaudeToolReverse(openaideName string) string {
	for c, o := range claudeToolMap {
		if o == openaideName {
			return c
		}
	}
	return ""
}

// MapClaudeEvent 将 Claude 事件名映射到 OpenAIDE 事件名
func MapClaudeEvent(claudeEvent string) string {
	if mapped, ok := claudeToOpenAIDEEvent[claudeEvent]; ok {
		return mapped
	}
	return ""
}

// ============ OpenCode 配置兼容 ============

// OpenCodeConfig opencode.json 结构
type OpenCodeConfig struct {
	MCP          map[string]OpenCodeMCPServer `json:"mcp,omitempty"`
	Instructions []string                     `json:"instructions,omitempty"`
	Model        string                       `json:"model,omitempty"`
	Plugin       []string                     `json:"plugin,omitempty"`
}

// OpenCodeMCPServer OpenCode MCP 服务器配置
type OpenCodeMCPServer struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Enabled bool     `json:"enabled"`
}

// DiscoverOpenCodeConfig 从项目目录发现 opencode.json 配置
func DiscoverOpenCodeConfig(projectDir string) (*OpenCodeConfig, []MCPServerEntry, string) {
	path := filepath.Join(projectDir, "opencode.json")
	// 也检查 .opencode/opencode.json
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(projectDir, ".opencode", "opencode.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, ""
	}

	var cfg OpenCodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, ""
	}

	// 提取 MCP 服务器
	var mcpServers []MCPServerEntry
	for name, srv := range cfg.MCP {
		if srv.Command != "" && srv.Enabled {
			mcpServers = append(mcpServers, MCPServerEntry{
				Type:    srv.Type,
				Command: srv.Command,
				Args:    srv.Args,
			})
		} else {
			// 尝试 stdio 类型（OpenCode 常见格式）
			_ = name
		}
	}

	// 提取 instructions 作为额外提示词
	instructions := strings.Join(cfg.Instructions, "\n")

	return &cfg, mcpServers, instructions
}
