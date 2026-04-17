package services

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// SkillImportService 技能导入服务
type SkillImportService struct {
	db          *gorm.DB
	skillSvc    *SkillService
	parser      *SKILLMDParser
	httpClient  *http.Client
}

// NewSkillImportService 创建导入服务
func NewSkillImportService(db *gorm.DB, skillSvc *SkillService) *SkillImportService {
	return &SkillImportService{
		db:         db,
		skillSvc:   skillSvc,
		parser:     NewSKILLMDParser(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ImportFromContent 从内容导入技能
func (s *SkillImportService) ImportFromContent(content string, references map[string]string) (*models.Skill, []models.SkillParameter, error) {
	// 解析 SKILL.md
	def, err := s.parser.ParseFromFile(content, references)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse SKILL.md: %w", err)
	}

	return s.createSkillFromDefinition(def)
}

// ImportFromFile 从本地文件导入
func (s *SkillImportService) ImportFromFile(filePath string) (*models.Skill, []models.SkillParameter, error) {
	// 读取主文件
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 查找引用文件
	dir := filepath.Dir(filePath)
	references := s.loadReferences(dir)

	skill, params, err := s.ImportFromContent(string(content), references)
	if err != nil {
		return nil, nil, err
	}

	// 记录来源路径
	skill.SourcePath = filePath
	s.db.Save(skill)

	return skill, params, nil
}

// ImportFromDirectory 从目录导入（包含 SKILL.md 和引用文件）
func (s *SkillImportService) ImportFromDirectory(dirPath string) (*models.Skill, []models.SkillParameter, error) {
	skillFile := filepath.Join(dirPath, "SKILL.md")
	
	// 检查 SKILL.md 是否存在
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("SKILL.md not found in directory: %s", dirPath)
	}

	return s.ImportFromFile(skillFile)
}

// ImportFromURL 从 URL 导入
func (s *SkillImportService) ImportFromURL(skillURL string) (*models.Skill, []models.SkillParameter, error) {
	// 处理 GitHub 等特定 URL
	processedURL := processGitHubURL(skillURL)

	// 下载内容
	resp, err := s.httpClient.Get(processedURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("failed to download: status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	skill, params, err := s.ImportFromContent(string(content), nil)
	if err != nil {
		return nil, nil, err
	}

	// 记录来源
	skill.SourcePath = skillURL
	s.db.Save(skill)

	return skill, params, nil
}

// ImportFromZip 从 ZIP 文件导入
func (s *SkillImportService) ImportFromZip(zipPath string) ([]*models.Skill, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	var skills []*models.Skill
	references := make(map[string]string)

	// 第一遍：收集所有文件
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			continue
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		// 检查是否是 SKILL.md
		if strings.EqualFold(filepath.Base(file.Name), "SKILL.md") {
			// 暂时存储，稍后处理
			references[file.Name] = string(content)
		} else {
			// 存储引用文件
			references[file.Name] = string(content)
		}
	}

	// 第二遍：处理所有 SKILL.md
	for path, content := range references {
		if !strings.EqualFold(filepath.Base(path), "SKILL.md") {
			continue
		}

		// 获取该 SKILL.md 所在目录的引用文件
		dir := filepath.Dir(path)
		localRefs := make(map[string]string)
		for refPath, refContent := range references {
			if strings.HasPrefix(refPath, dir) && refPath != path {
				relPath, _ := filepath.Rel(dir, refPath)
				localRefs[relPath] = refContent
			}
		}

		skill, _, err := s.ImportFromContent(content, localRefs)
		if err != nil {
			continue
		}

		skills = append(skills, skill)
	}

	return skills, nil
}

// BatchImportFromDirectory 批量导入目录中的所有技能
func (s *SkillImportService) BatchImportFromDirectory(rootDir string) (*BatchImportResult, error) {
	result := &BatchImportResult{
		Success: []string{},
		Failed:  map[string]string{},
	}

	// 遍历目录查找所有 SKILL.md
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if !strings.EqualFold(info.Name(), "SKILL.md") {
			return nil
		}

		// 导入该技能
		skill, _, err := s.ImportFromFile(path)
		if err != nil {
			result.Failed[path] = err.Error()
		} else {
			result.Success = append(result.Success, skill.Name)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// createSkillFromDefinition 从定义创建技能
func (s *SkillImportService) createSkillFromDefinition(def *SKILLMDDefinition) (*models.Skill, []models.SkillParameter, error) {
	// 检查是否已存在同名技能
	var existing models.Skill
	err := s.db.Where("name = ?", def.Name).First(&existing).Error
	if err == nil {
		return nil, nil, fmt.Errorf("skill with name '%s' already exists", def.Name)
	}

	// 创建技能
	skillMap := def.ToSkillMap()
	skillMap["source_format"] = "skill_md"

	skill, err := s.skillSvc.CreateSkillFromMap(skillMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create skill: %w", err)
	}

	// 创建参数
	paramDefs := def.ToSkillParameters()
	var params []models.SkillParameter

	for _, paramDef := range paramDefs {
		param := &models.SkillParameter{
			SkillID:     skill.ID,
			Name:        getString(paramDef, "name"),
			Description: getString(paramDef, "description"),
			Type:        getString(paramDef, "type"),
			Required:    getBool(paramDef, "required"),
		}

		if defVal, ok := paramDef["default"]; ok {
			param.Default = &models.JSONAny{Data: defVal}
		}

		if err := s.skillSvc.CreateSkillParameter(param); err != nil {
			continue
		}

		params = append(params, *param)
	}

	return skill, params, nil
}

// loadReferences 加载引用文件
func (s *SkillImportService) loadReferences(dir string) map[string]string {
	references := make(map[string]string)

	// 常见引用目录
	refDirs := []string{
		"references",
		"refs",
		"examples",
		"docs",
	}

	for _, refDir := range refDirs {
		fullPath := filepath.Join(dir, refDir)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			relPath, _ := filepath.Rel(dir, path)
			references[relPath] = string(content)
			references[filepath.Base(path)] = string(content)

			return nil
		})
	}

	return references
}

// processGitHubURL 处理 GitHub URL
func processGitHubURL(skillURL string) string {
	// 转换 GitHub blob URL 到 raw URL
	if strings.Contains(skillURL, "github.com") && strings.Contains(skillURL, "/blob/") {
		return regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/blob/`).
			ReplaceAllString(skillURL, "raw.githubusercontent.com/$1/$2/")
	}
	return skillURL
}

// BatchImportResult 批量导入结果
type BatchImportResult struct {
	Success []string          `json:"success"`
	Failed  map[string]string `json:"failed"`
}

// ValidateSKILLMD 验证 SKILL.md 内容
func (s *SkillImportService) ValidateSKILLMD(content string) error {
	_, err := s.parser.Parse(content)
	return err
}

// ExportToSKILLMD 将技能导出为 SKILL.md 格式
func (s *SkillImportService) ExportToSKILLMD(skillID string) (string, error) {
	skill, err := s.skillSvc.GetSkill(skillID)
	if err != nil {
		return "", fmt.Errorf("skill not found: %w", err)
	}

	params, err := s.skillSvc.GetSkillParameters(skillID)
	if err != nil {
		return "", err
	}

	// 构建 YAML frontmatter
	var yamlLines []string
	yamlLines = append(yamlLines, "---")
	yamlLines = append(yamlLines, fmt.Sprintf("name: %s", skill.Name))
	yamlLines = append(yamlLines, fmt.Sprintf("description: |"))
	yamlLines = append(yamlLines, indentLines(skill.Description, 2))

	if skill.ModelPreference != "" {
		yamlLines = append(yamlLines, fmt.Sprintf("model-preference: %s", skill.ModelPreference))
	}

	if len(skill.Triggers) > 0 {
		yamlLines = append(yamlLines, "triggers:")
		for _, t := range skill.Triggers {
			yamlLines = append(yamlLines, fmt.Sprintf("  - \"%s\"", t))
		}
	}

	if len(skill.AllowedTools) > 0 {
		yamlLines = append(yamlLines, "allowed-tools:")
		for _, t := range skill.AllowedTools {
			yamlLines = append(yamlLines, fmt.Sprintf("  - %s", t))
		}
	}

	if len(params) > 0 {
		yamlLines = append(yamlLines, "parameters:")
		for _, p := range params {
			yamlLines = append(yamlLines, fmt.Sprintf("  - name: %s", p.Name))
			yamlLines = append(yamlLines, fmt.Sprintf("    type: %s", p.Type))
			if p.Description != "" {
				yamlLines = append(yamlLines, fmt.Sprintf("    description: %s", p.Description))
			}
			if p.Required {
				yamlLines = append(yamlLines, "    required: true")
			}
			if p.Default != nil {
				yamlLines = append(yamlLines, fmt.Sprintf("    default: %v", p.Default.Data))
			}
		}
	}

	// Metadata
	yamlLines = append(yamlLines, "metadata:")
	if skill.Author != "" {
		yamlLines = append(yamlLines, fmt.Sprintf("  author: \"%s\"", skill.Author))
	}
	if skill.Version != "" {
		yamlLines = append(yamlLines, fmt.Sprintf("  version: \"%s\"", skill.Version))
	}
	if skill.Category != "" {
		yamlLines = append(yamlLines, fmt.Sprintf("  category: \"%s\"", skill.Category))
	}
	if len(skill.Tags) > 0 {
		yamlLines = append(yamlLines, "  tags:")
		for _, t := range skill.Tags {
			yamlLines = append(yamlLines, fmt.Sprintf("    - %s", t))
		}
	}

	yamlLines = append(yamlLines, "---")
	yamlLines = append(yamlLines, "")

	// Body
	if skill.InstructionBody != "" {
		yamlLines = append(yamlLines, skill.InstructionBody)
	} else if skill.SystemPromptOverride != "" {
		yamlLines = append(yamlLines, skill.SystemPromptOverride)
	}

	return strings.Join(yamlLines, "\n"), nil
}

// 辅助函数
func indentLines(text string, spaces int) string {
	indent := strings.Repeat(" ", spaces)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}
