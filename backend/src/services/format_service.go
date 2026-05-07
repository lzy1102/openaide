package services

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"openaide/backend/src/services/llm"
)

type FormatService struct {
	BaseService
	llmClient llm.LLMClient
}

func NewFormatService(db *gorm.DB, logger *LoggerService, cache *CacheService, llmClient llm.LLMClient) *FormatService {
	return &FormatService{
		BaseService: BaseService{DB: db, Logger: logger, Cache: cache},
		llmClient:   llmClient,
	}
}

type FormatCodeRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Style    string `json:"style,omitempty"`
	Model    string `json:"model,omitempty"`
}

type FormatCodeResponse struct {
	FormattedCode string `json:"formatted_code"`
	Changes       []FormatChange `json:"changes"`
	Summary       string `json:"summary"`
}

type FormatChange struct {
	Line        int    `json:"line"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Before      string `json:"before,omitempty"`
	After       string `json:"after,omitempty"`
}

func (s *FormatService) FormatCode(ctx context.Context, req *FormatCodeRequest) (*FormatCodeResponse, error) {
	styleSection := ""
	if req.Style != "" {
		styleSection = fmt.Sprintf("代码风格规范: %s", req.Style)
	}

	prompt := fmt.Sprintf(`请按照 %s 语言的格式化规范，对以下代码进行格式化。

代码:
%s

%s

请提供:
1. 格式化后的代码
2. 格式化变更说明
3. 格式化摘要

以JSON格式返回:
{
  "formatted_code": "格式化后的代码",
  "changes": [
    {"line": 行号, "type": "变更类型", "description": "变更描述", "before": "变更前", "after": "变更后"}
  ],
  "summary": "格式化摘要"
}`, req.Language, req.Code, styleSection)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码格式化失败: %w", err)
	}

	var result FormatCodeResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = FormatCodeResponse{
			FormattedCode: response,
			Summary:       "格式化完成",
		}
	}

	return &result, nil
}

type LintCodeRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Rules    []string `json:"rules,omitempty"`
	Model    string `json:"model,omitempty"`
}

type LintCodeResponse struct {
	Issues     []FormatLintIssue `json:"issues"`
	Score      int         `json:"score"`
	Summary    string      `json:"summary"`
	Suggestions []string   `json:"suggestions"`
}

type FormatLintIssue struct {
	Line       int    `json:"line"`
	Column     int    `json:"column,omitempty"`
	Severity   string `json:"severity"`
	Rule       string `json:"rule"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

func (s *FormatService) LintCode(ctx context.Context, req *LintCodeRequest) (*LintCodeResponse, error) {
	rulesSection := ""
	if len(req.Rules) > 0 {
		rulesSection = "启用的Lint规则:\n"
		for _, r := range req.Rules {
			rulesSection += "- " + r + "\n"
		}
	}

	prompt := fmt.Sprintf(`请对以下 %s 代码进行Lint检查，报告风格问题和潜在错误。

代码:
%s

%s

请提供:
1. 发现的问题列表
2. 代码质量评分(0-100)
3. 检查摘要
4. 改进建议

以JSON格式返回:
{
  "issues": [
    {"line": 行号, "column": 列号, "severity": "error/warning/info", "rule": "规则名称", "message": "问题描述", "suggestion": "修复建议"}
  ],
  "score": 评分(0-100),
  "summary": "检查摘要",
  "suggestions": ["改进建议列表"]
}`, req.Language, req.Code, rulesSection)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码Lint失败: %w", err)
	}

	var result LintCodeResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = LintCodeResponse{
			Score:   50,
			Summary: response,
		}
	}

	return &result, nil
}

type FixStyleIssuesRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Issues   []string `json:"issues,omitempty"`
	Model    string `json:"model,omitempty"`
}

type FixStyleIssuesResponse struct {
	FixedCode  string       `json:"fixed_code"`
	Fixes      []StyleFix   `json:"fixes"`
	Summary    string       `json:"summary"`
	RemainingIssues []string `json:"remaining_issues,omitempty"`
}

type StyleFix struct {
	Line        int    `json:"line"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Before      string `json:"before,omitempty"`
	After       string `json:"after,omitempty"`
}

func (s *FormatService) FixStyleIssues(ctx context.Context, req *FixStyleIssuesRequest) (*FixStyleIssuesResponse, error) {
	issuesSection := ""
	if len(req.Issues) > 0 {
		issuesSection = "需要修复的已知问题:\n"
		for _, issue := range req.Issues {
			issuesSection += "- " + issue + "\n"
		}
	}

	prompt := fmt.Sprintf(`请自动修复以下 %s 代码中的风格问题。

代码:
%s

%s

请提供:
1. 修复后的代码
2. 修复说明列表
3. 修复摘要
4. 无法自动修复的剩余问题

以JSON格式返回:
{
  "fixed_code": "修复后的代码",
  "fixes": [
    {"line": 行号, "type": "修复类型", "description": "修复描述", "before": "修复前", "after": "修复后"}
  ],
  "summary": "修复摘要",
  "remaining_issues": ["无法自动修复的问题"]
}`, req.Language, req.Code, issuesSection)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("风格修复失败: %w", err)
	}

	var result FixStyleIssuesResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = FixStyleIssuesResponse{
			FixedCode: response,
			Summary:   "风格修复完成",
		}
	}

	return &result, nil
}

type DetectCodeSmellsRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model,omitempty"`
}

type DetectCodeSmellsResponse struct {
	Smells     []CodeSmell `json:"smells"`
	Score      int         `json:"score"`
	Summary    string      `json:"summary"`
	RefactorSuggestions []RefactorSuggestion `json:"refactor_suggestions"`
}

type CodeSmell struct {
	Name        string `json:"name"`
	Line        int    `json:"line"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type RefactorSuggestion struct {
	Target      string `json:"target"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

func (s *FormatService) DetectCodeSmells(ctx context.Context, req *DetectCodeSmellsRequest) (*DetectCodeSmellsResponse, error) {
	contextSection := ""
	if req.Context != "" {
		contextSection = fmt.Sprintf("上下文:\n%s", req.Context)
	}

	prompt := fmt.Sprintf(`请检测以下 %s 代码中的代码异味和反模式。

代码:
%s

%s

请检测以下类别的代码异味:
1. 过长方法
2. 重复代码
3. 过深嵌套
4. 过长参数列表
5. 上帝类/上帝函数
6. 魔法数字/魔法字符串
7. 死代码
8. 过度耦合
9. 不当命名
10. 违反单一职责

以JSON格式返回:
{
  "smells": [
    {"name": "异味名称", "line": 行号, "severity": "high/medium/low", "description": "描述", "category": "类别"}
  ],
  "score": 健康评分(0-100),
  "summary": "检测摘要",
  "refactor_suggestions": [
    {"target": "重构目标", "type": "重构类型", "description": "重构描述", "priority": "high/medium/low"}
  ]
}`, req.Language, req.Code, contextSection)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码异味检测失败: %w", err)
	}

	var result DetectCodeSmellsResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = DetectCodeSmellsResponse{
			Score:   50,
			Summary: response,
		}
	}

	return &result, nil
}

func (s *FormatService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个专业的代码风格专家，擅长代码格式化、Lint规则、代码风格修复和代码异味检测。请用中文回复。"},
			{Role: "user", Content: prompt},
		},
		Model:       model,
		Temperature: 0.2,
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
