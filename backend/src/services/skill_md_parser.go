package services

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// SKILLMDDefinition SKILL.md 定义结构
type SKILLMDDefinition struct {
	// YAML Frontmatter
	Name             string                 `yaml:"name"`
	Description      string                 `yaml:"description"`
	AllowedTools     []string               `yaml:"allowed-tools"`
	ModelPreference  string                 `yaml:"model-preference"`
	Triggers         []string               `yaml:"triggers"`
	Parameters       []SKILLMDParameter     `yaml:"parameters"`
	Metadata         SKILLMDMetadata        `yaml:"metadata"`
	Constraints      SKILLMDConstraints     `yaml:"constraints"`
	
	// Markdown Body
	InstructionBody  string
}

// SKILLMDParameter 参数定义
type SKILLMDParameter struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Description string      `yaml:"description"`
	Required    bool        `yaml:"required"`
	Default     interface{} `yaml:"default"`
	Values      []string    `yaml:"values"` // for enum type
}

// SKILLMDMetadata 元数据
type SKILLMDMetadata struct {
	Author      string   `yaml:"author"`
	Version     string   `yaml:"version"`
	Tags        []string `yaml:"tags"`
	Category    string   `yaml:"category"`
}

// SKILLMDConstraints 约束条件
type SKILLMDConstraints struct {
	MaxTokens   int      `yaml:"max-tokens"`
	Timeout     int      `yaml:"timeout"`
	RequireConfirm bool   `yaml:"require-confirm"`
}

// SKILLMDParser SKILL.md 解析器
type SKILLMDParser struct{}

// NewSKILLMDParser 创建解析器
func NewSKILLMDParser() *SKILLMDParser {
	return &SKILLMDParser{}
}

// Parse 解析 SKILL.md 内容
func (p *SKILLMDParser) Parse(content string) (*SKILLMDDefinition, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("empty skill content")
	}

	// 解析 YAML Frontmatter 和 Markdown Body
	frontmatter, body, err := p.splitFrontmatterAndBody(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse skill format: %w", err)
	}

	// 解析 YAML
	var def SKILLMDDefinition
	if err := yaml.Unmarshal([]byte(frontmatter), &def); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// 设置 Markdown Body
	def.InstructionBody = strings.TrimSpace(body)

	// 验证必需字段
	if err := p.validate(&def); err != nil {
		return nil, err
	}

	// 规范化字段
	p.normalize(&def)

	return &def, nil
}

// splitFrontmatterAndBody 分割 YAML Frontmatter 和 Markdown Body
func (p *SKILLMDParser) splitFrontmatterAndBody(content string) (string, string, error) {
	// 匹配 ---\n...\n--- 格式
	pattern := regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n(.*)$`)
	matches := pattern.FindStringSubmatch(content)
	
	if len(matches) != 3 {
		// 尝试没有 frontmatter 的格式（只有 name 和 description）
		return p.parseSimpleFormat(content)
	}

	return matches[1], matches[2], nil
}

// parseSimpleFormat 解析简化格式（没有 frontmatter 分隔符）
func (p *SKILLMDParser) parseSimpleFormat(content string) (string, string, error) {
	lines := strings.Split(content, "\n")
	var yamlLines []string
	var bodyLines []string
	var inBody bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// 如果遇到空行且已经收集了一些 YAML 行，则认为开始 body
		if trimmed == "" && len(yamlLines) > 0 && !inBody {
			inBody = true
			continue
		}
		
		// 如果行以 # 开头（Markdown 标题），则认为是 body
		if strings.HasPrefix(trimmed, "#") && len(yamlLines) > 0 {
			inBody = true
		}

		if inBody {
			bodyLines = append(bodyLines, line)
		} else {
			// 检查是否是 YAML 键值对
			if strings.Contains(line, ":") || trimmed == "" {
				yamlLines = append(yamlLines, line)
			} else if len(yamlLines) > 0 {
				// 可能是多行 YAML 的开始
				inBody = true
				bodyLines = append(bodyLines, line)
			}
		}
	}

	if len(yamlLines) == 0 {
		return "", "", fmt.Errorf("no YAML frontmatter found")
	}

	return strings.Join(yamlLines, "\n"), strings.Join(bodyLines, "\n"), nil
}

// validate 验证 SKILL.md 定义
func (p *SKILLMDParser) validate(def *SKILLMDDefinition) error {
	// 验证 name
	if def.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	if len(def.Name) > 64 {
		return fmt.Errorf("skill name must be at most 64 characters")
	}
	if !isValidSkillName(def.Name) {
		return fmt.Errorf("skill name must contain only lowercase letters, numbers, and hyphens")
	}

	// 验证 description
	if def.Description == "" {
		return fmt.Errorf("skill description is required")
	}
	if len(def.Description) > 1024 {
		return fmt.Errorf("skill description must be at most 1024 characters")
	}

	// 验证 model-preference
	if def.ModelPreference != "" {
		validPreferences := map[string]bool{
			"code":      true,
			"creative":  true,
			"reasoning": true,
			"fast":      true,
			"accurate":  true,
		}
		if !validPreferences[def.ModelPreference] {
			return fmt.Errorf("invalid model-preference: %s", def.ModelPreference)
		}
	}

	// 验证参数
	for i, param := range def.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameter %d: name is required", i)
		}
		validTypes := map[string]bool{
			"string":  true,
			"integer": true,
			"number":  true,
			"boolean": true,
			"array":   true,
			"object":  true,
			"enum":    true,
		}
		if param.Type != "" && !validTypes[param.Type] {
			return fmt.Errorf("parameter %s: invalid type %s", param.Name, param.Type)
		}
	}

	return nil
}

// normalize 规范化 SKILL.md 定义
func (p *SKILLMDParser) normalize(def *SKILLMDDefinition) {
	// 规范化 name
	def.Name = strings.ToLower(strings.TrimSpace(def.Name))
	
	// 规范化 description
	def.Description = strings.TrimSpace(def.Description)
	
	// 规范化 triggers
	for i, trigger := range def.Triggers {
		def.Triggers[i] = strings.TrimSpace(trigger)
	}
	
	// 规范化 allowed-tools
	for i, tool := range def.AllowedTools {
		def.AllowedTools[i] = strings.TrimSpace(tool)
	}
	
	// 设置默认 metadata
	if def.Metadata.Version == "" {
		def.Metadata.Version = "1.0.0"
	}
	
	// 设置默认参数类型
	for i := range def.Parameters {
		if def.Parameters[i].Type == "" {
			def.Parameters[i].Type = "string"
		}
	}
}

// isValidSkillName 检查技能名称是否有效
func isValidSkillName(name string) bool {
	// 只允许小写字母、数字和连字符
	match, _ := regexp.MatchString(`^[a-z0-9]+(-[a-z0-9]+)*$`, name)
	return match
}

// ParseFromFile 从文件解析 SKILL.md
func (p *SKILLMDParser) ParseFromFile(content string, references map[string]string) (*SKILLMDDefinition, error) {
	def, err := p.Parse(content)
	if err != nil {
		return nil, err
	}

	// 处理引用文件
	if len(references) > 0 {
		def.InstructionBody = p.processReferences(def.InstructionBody, references)
	}

	return def, nil
}

// processReferences 处理引用文件
func (p *SKILLMDParser) processReferences(body string, references map[string]string) string {
	// 替换 [filename](path) 为实际内容
	refPattern := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	
	result := refPattern.ReplaceAllStringFunc(body, func(match string) string {
		submatches := refPattern.FindStringSubmatch(match)
		if len(submatches) != 3 {
			return match
		}
		
		linkText := submatches[1]
		refPath := submatches[2]
		
		// 查找引用文件内容
		if content, ok := references[refPath]; ok {
			return fmt.Sprintf("\n<!-- Reference: %s -->\n%s\n", linkText, content)
		}
		
		return match
	})
	
	return result
}

// ToSkillMap 转换为 Skill Map（用于创建 Skill）
func (def *SKILLMDDefinition) ToSkillMap() map[string]interface{} {
	skill := map[string]interface{}{
		"name":        def.Name,
		"description": def.Description,
		"version":     def.Metadata.Version,
		"author":      def.Metadata.Author,
		"category":    def.Metadata.Category,
		"enabled":     true,
		"builtin":     false,
	}

	// 添加 triggers
	if len(def.Triggers) > 0 {
		skill["triggers"] = def.Triggers
	}

	// 添加 model preference
	if def.ModelPreference != "" {
		skill["model_preference"] = def.ModelPreference
	}

	// 添加 tools
	if len(def.AllowedTools) > 0 {
		skill["tools"] = def.AllowedTools
	}

	// 添加 system prompt override（使用 instruction body）
	if def.InstructionBody != "" {
		skill["system_prompt_override"] = def.InstructionBody
	}

	// 添加 metadata 到 config
	config := map[string]interface{}{
		"allowed_tools":     def.AllowedTools,
		"constraints":       def.Constraints,
		"metadata":          def.Metadata,
		"original_format":   "skill_md",
	}
	skill["config"] = config

	return skill
}

// ToSkillParameters 转换为 SkillParameter 列表
func (def *SKILLMDDefinition) ToSkillParameters() []map[string]interface{} {
	var params []map[string]interface{}
	
	for _, param := range def.Parameters {
		p := map[string]interface{}{
			"name":        param.Name,
			"description": param.Description,
			"type":        param.Type,
			"required":    param.Required,
		}
		
		if param.Default != nil {
			p["default"] = param.Default
		}
		
		if len(param.Values) > 0 {
			p["enum_values"] = param.Values
		}
		
		params = append(params, p)
	}
	
	return params
}
