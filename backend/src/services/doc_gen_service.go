package services

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"openaide/backend/src/services/llm"
)

type DocGenService struct {
	BaseService
	llmClient llm.LLMClient
}

func NewDocGenService(db *gorm.DB, logger *LoggerService, cache *CacheService, llmClient llm.LLMClient) *DocGenService {
	return &DocGenService{
		BaseService: BaseService{
			DB:     db,
			Logger: logger,
			Cache:  cache,
		},
		llmClient: llmClient,
	}
}

type GenerateAPIDocRequest struct {
	Code        string   `json:"code" binding:"required"`
	Language    string   `json:"language" binding:"required"`
	APIStyle    string   `json:"api_style,omitempty"`
	Context     string   `json:"context,omitempty"`
	Endpoints   []string `json:"endpoints,omitempty"`
	Model       string   `json:"model,omitempty"`
}

type GenerateAPIDocResponse struct {
	APIDocumentation string       `json:"api_documentation"`
	Endpoints        []APIEndpoint `json:"endpoints"`
	Examples         []APIExample  `json:"examples"`
}

type APIEndpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Parameters  []APIParam `json:"parameters,omitempty"`
	ResponseType string `json:"response_type,omitempty"`
}

type APIParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type APIExample struct {
	Title   string `json:"title"`
	Request string `json:"request"`
	Response string `json:"response"`
}

func (s *DocGenService) GenerateAPIDoc(ctx context.Context, req *GenerateAPIDocRequest) (*GenerateAPIDocResponse, error) {
	endpointsStr := ""
	if len(req.Endpoints) > 0 {
		endpointsStr = "关注的接口:\n"
		for _, e := range req.Endpoints {
			endpointsStr += "- " + e + "\n"
		}
	}

	apiStyle := req.APIStyle
	if apiStyle == "" {
		apiStyle = "OpenAPI/Swagger"
	}

	prompt := fmt.Sprintf(`请根据以下 %s 代码生成API文档，文档风格: %s。

代码:
%s

%s
%s

请提供:
1. 完整的API文档
2. 接口列表（包含方法、路径、描述、参数、响应类型）
3. 请求/响应示例

以JSON格式返回:
{
  "api_documentation": "完整的API文档（Markdown格式）",
  "endpoints": [
    {"method": "HTTP方法", "path": "接口路径", "description": "接口描述", "parameters": [{"name": "参数名", "type": "参数类型", "required": true/false, "description": "参数描述"}], "response_type": "响应类型"}
  ],
  "examples": [
    {"title": "示例标题", "request": "请求示例", "response": "响应示例"}
  ]
}`, req.Language, apiStyle, req.Code, s.buildContextSection(req.Context), endpointsStr)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("API文档生成失败: %w", err)
	}

	var result GenerateAPIDocResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = GenerateAPIDocResponse{
			APIDocumentation: response,
		}
	}

	return &result, nil
}

type GenerateReadmeRequest struct {
	ProjectName    string   `json:"project_name" binding:"required"`
	Description    string   `json:"description" binding:"required"`
	Language       string   `json:"language,omitempty"`
	Framework      string   `json:"framework,omitempty"`
	Features       []string `json:"features,omitempty"`
	InstallSteps   []string `json:"install_steps,omitempty"`
	Context        string   `json:"context,omitempty"`
	Model          string   `json:"model,omitempty"`
}

type GenerateReadmeResponse struct {
	Content      string   `json:"content"`
	Sections     []string `json:"sections"`
	Badges       []string `json:"badges,omitempty"`
	License      string   `json:"license,omitempty"`
}

func (s *DocGenService) GenerateReadme(ctx context.Context, req *GenerateReadmeRequest) (*GenerateReadmeResponse, error) {
	featuresStr := ""
	if len(req.Features) > 0 {
		featuresStr = "功能特性:\n"
		for _, f := range req.Features {
			featuresStr += "- " + f + "\n"
		}
	}

	installStr := ""
	if len(req.InstallSteps) > 0 {
		installStr = "安装步骤:\n"
		for _, step := range req.InstallSteps {
			installStr += "- " + step + "\n"
		}
	}

	prompt := fmt.Sprintf(`请为以下项目生成README.md文档。

项目名称: %s
项目描述: %s
%s
%s
%s
%s

请生成:
1. 完整的README.md内容（Markdown格式）
2. 包含的章节列表
3. 推荐的徽章（可选）
4. 推荐的开源协议

以JSON格式返回:
{
  "content": "完整的README.md内容",
  "sections": ["章节列表"],
  "badges": ["徽章列表"],
  "license": "推荐协议"
}`, req.ProjectName, req.Description, s.buildTechSection(req.Language, req.Framework), featuresStr, installStr, s.buildContextSection(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("README生成失败: %w", err)
	}

	var result GenerateReadmeResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = GenerateReadmeResponse{
			Content: response,
		}
	}

	return &result, nil
}

type GenerateChangelogRequest struct {
	Commits       []CommitEntry `json:"commits" binding:"required"`
	Version       string        `json:"version,omitempty"`
	PreviousTag   string        `json:"previous_tag,omitempty"`
	CurrentTag    string        `json:"current_tag,omitempty"`
	Style         string        `json:"style,omitempty"`
	Context       string        `json:"context,omitempty"`
	Model         string        `json:"model,omitempty"`
}

type CommitEntry struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Author  string `json:"author,omitempty"`
	Date    string `json:"date,omitempty"`
}

type GenerateChangelogResponse struct {
	Changelog    string        `json:"changelog"`
	Version      string        `json:"version"`
	Sections     []ChangeSection `json:"sections"`
	BreakingChanges []string   `json:"breaking_changes,omitempty"`
}

type ChangeSection struct {
	Type    string   `json:"type"`
	Entries []string `json:"entries"`
}

func (s *DocGenService) GenerateChangelog(ctx context.Context, req *GenerateChangelogRequest) (*GenerateChangelogResponse, error) {
	commitsStr := "提交记录:\n"
	for _, c := range req.Commits {
		commitsStr += fmt.Sprintf("- [%s] %s (%s, %s)\n", c.Hash, c.Message, c.Author, c.Date)
	}

	style := req.Style
	if style == "" {
		style = "Keep a Changelog"
	}

	versionInfo := ""
	if req.Version != "" {
		versionInfo = fmt.Sprintf("版本: %s", req.Version)
	}
	if req.PreviousTag != "" && req.CurrentTag != "" {
		versionInfo += fmt.Sprintf(" (从 %s 到 %s)", req.PreviousTag, req.CurrentTag)
	}

	prompt := fmt.Sprintf(`请根据以下Git提交记录生成变更日志，风格: %s。

%s
%s
%s

请生成:
1. 完整的变更日志（Markdown格式）
2. 版本号
3. 按类型分类的变更（新增、修复、变更、移除、安全等）
4. 破坏性变更列表（如有）

以JSON格式返回:
{
  "changelog": "完整的变更日志（Markdown格式）",
  "version": "版本号",
  "sections": [
    {"type": "变更类型（如added/fixed/changed/removed/security）", "entries": ["变更条目列表"]}
  ],
  "breaking_changes": ["破坏性变更列表"]
}`, style, commitsStr, versionInfo, s.buildContextSection(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("变更日志生成失败: %w", err)
	}

	var result GenerateChangelogResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = GenerateChangelogResponse{
			Changelog: response,
		}
	}

	return &result, nil
}

type GenerateCodeCommentsRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Style    string `json:"style,omitempty"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model,omitempty"`
}

type GenerateCodeCommentsResponse struct {
	CommentedCode string   `json:"commented_code"`
	DocStrings    []string `json:"doc_strings"`
	Summary       string   `json:"summary"`
}

func (s *DocGenService) GenerateCodeComments(ctx context.Context, req *GenerateCodeCommentsRequest) (*GenerateCodeCommentsResponse, error) {
	style := req.Style
	if style == "" {
		style = "标准文档注释"
	}

	prompt := fmt.Sprintf(`请为以下 %s 代码添加注释和文档，注释风格: %s。

代码:
%s

%s

请提供:
1. 添加了完整注释的代码
2. 提取的文档字符串列表
3. 代码功能概述

以JSON格式返回:
{
  "commented_code": "添加注释后的完整代码",
  "doc_strings": ["文档字符串列表"],
  "summary": "代码功能概述"
}`, req.Language, style, req.Code, s.buildContextSection(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码注释生成失败: %w", err)
	}

	var result GenerateCodeCommentsResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = GenerateCodeCommentsResponse{
			CommentedCode: response,
		}
	}

	return &result, nil
}

func (s *DocGenService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个专业的技术文档工程师，擅长编写API文档、README、变更日志和代码注释。请用中文回复。"},
			{Role: "user", Content: prompt},
		},
		Model:       model,
		Temperature: 0.3,
		MaxTokens:   4000,
	}

	response, err := s.llmClient.Chat(ctx, request)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("未收到响应")
	}

	return response.Choices[0].Message.Content, nil
}

func (s *DocGenService) buildContextSection(context string) string {
	if context == "" {
		return ""
	}
	return fmt.Sprintf("上下文:\n%s", context)
}

func (s *DocGenService) buildTechSection(language, framework string) string {
	parts := []string{}
	if language != "" {
		parts = append(parts, fmt.Sprintf("编程语言: %s", language))
	}
	if framework != "" {
		parts = append(parts, fmt.Sprintf("框架: %s", framework))
	}
	if len(parts) == 0 {
		return ""
	}
	result := ""
	for _, p := range parts {
		result += p + "\n"
	}
	return result
}
