package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// CodeService 代码服务 - 提供代码分析、生成、执行、审查能力
type CodeService struct {
	db            *gorm.DB
	llmClient     llm.LLMClient
	thinkingSvc   *ThinkingService
}

// NewCodeService 创建代码服务实例
func NewCodeService(db *gorm.DB, llmClient llm.LLMClient, thinkingSvc *ThinkingService) *CodeService {
	return &CodeService{
		db:          db,
		llmClient:   llmClient,
		thinkingSvc: thinkingSvc,
	}
}

// ============ 代码分析 ============

// CodeAnalysisRequest 代码分析请求
type CodeAnalysisRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model,omitempty"`
}

// CodeAnalysisResponse 代码分析响应
type CodeAnalysisResponse struct {
	Summary      string            `json:"summary"`
	Complexity   string            `json:"complexity"`
	Issues       []CodeIssue       `json:"issues"`
	Suggestions  []CodeSuggestion  `json:"suggestions"`
	Dependencies []string          `json:"dependencies"`
	SecurityRisks []CodeSecurityRisk `json:"security_risks"`
}

// CodeIssue 代码问题
type CodeIssue struct {
	Line        int    `json:"line"`
	Severity    string `json:"severity"` // error, warning, info
	Message     string `json:"message"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// CodeSuggestion 代码建议
type CodeSuggestion struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

// CodeSecurityRisk 安全风险
type CodeSecurityRisk struct {
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

// AnalyzeCode 分析代码质量、复杂度、安全性和潜在问题
func (s *CodeService) AnalyzeCode(ctx context.Context, req *CodeAnalysisRequest) (*CodeAnalysisResponse, error) {
	prompt := fmt.Sprintf(`请对以下 %s 代码进行全面分析。

代码:
%s

%s

请提供以下分析结果（以JSON格式返回）:
{
  "summary": "代码功能概述",
  "complexity": "复杂度评估 (低/中/高)",
  "issues": [
    {"line": 行号, "severity": "error/warning/info", "message": "问题描述", "suggestion": "修复建议"}
  ],
  "suggestions": [
    {"type": "优化类型", "description": "建议描述", "example": "示例代码"}
  ],
  "dependencies": ["依赖项列表"],
  "security_risks": [
    {"severity": "high/medium/low", "category": "风险类别", "description": "风险描述", "line": 行号}
  ]
}`, req.Language, req.Code, s.buildContextSection(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("LLM分析失败: %w", err)
	}

	// 尝试解析JSON响应
	var result CodeAnalysisResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		// 如果JSON解析失败，返回基础分析
		result = CodeAnalysisResponse{
			Summary:    response,
			Complexity: "unknown",
		}
	}

	return &result, nil
}

// ============ 代码生成 ============

// CodeGenerationRequest 代码生成请求
type CodeGenerationRequest struct {
	Description string            `json:"description" binding:"required"`
	Language    string            `json:"language" binding:"required"`
	Context     string            `json:"context,omitempty"`
	Requirements []string         `json:"requirements,omitempty"`
	Model       string            `json:"model,omitempty"`
}

// CodeGenerationResponse 代码生成响应
type CodeGenerationResponse struct {
	Code        string   `json:"code"`
	Explanation string   `json:"explanation"`
	Usage       string   `json:"usage"`
	Tests       string   `json:"tests,omitempty"`
}

// GenerateCode 根据描述生成代码
func (s *CodeService) GenerateCode(ctx context.Context, req *CodeGenerationRequest) (*CodeGenerationResponse, error) {
	reqsStr := ""
	if len(req.Requirements) > 0 {
		reqsStr = "要求:\n"
		for _, r := range req.Requirements {
			reqsStr += "- " + r + "\n"
		}
	}

	prompt := fmt.Sprintf(`请根据以下描述生成 %s 代码。

描述: %s

%s
%s

请生成:
1. 完整、可运行的代码
2. 代码功能说明
3. 使用示例
4. 单元测试（可选）

以JSON格式返回:
{
  "code": "生成的代码",
  "explanation": "功能说明",
  "usage": "使用示例",
  "tests": "测试代码"
}`, req.Language, req.Description, s.buildContextSection(req.Context), reqsStr)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码生成失败: %w", err)
	}

	var result CodeGenerationResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = CodeGenerationResponse{
			Code:        response,
			Explanation: "Generated code",
		}
	}

	return &result, nil
}

// ============ 代码重构 ============

// CodeRefactorRequest 代码重构请求
type CodeRefactorRequest struct {
	Code        string `json:"code" binding:"required"`
	Language    string `json:"language" binding:"required"`
	Goal        string `json:"goal"` // improve_performance, enhance_readability, reduce_complexity
	Context     string `json:"context,omitempty"`
	Model       string `json:"model,omitempty"`
}

// CodeRefactorResponse 代码重构响应
type CodeRefactorResponse struct {
	RefactoredCode string       `json:"refactored_code"`
	Changes        []CodeChange `json:"changes"`
	Explanation    string       `json:"explanation"`
}

// CodeChange 代码变更说明
type CodeChange struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Before      string `json:"before,omitempty"`
	After       string `json:"after,omitempty"`
}

// RefactorCode 重构代码
func (s *CodeService) RefactorCode(ctx context.Context, req *CodeRefactorRequest) (*CodeRefactorResponse, error) {
	goalMap := map[string]string{
		"improve_performance":   "提升性能",
		"enhance_readability":   "增强可读性",
		"reduce_complexity":     "降低复杂度",
		"add_comments":          "添加注释",
		"modernize":             "现代化语法",
	}
	goal := goalMap[req.Goal]
	if goal == "" {
		goal = "优化代码"
	}

	prompt := fmt.Sprintf(`请对以下 %s 代码进行重构，目标: %s。

原始代码:
%s

%s

请提供:
1. 重构后的代码
2. 变更说明列表
3. 重构理由

以JSON格式返回:
{
  "refactored_code": "重构后的代码",
  "changes": [
    {"type": "变更类型", "description": "变更描述", "before": "变更前", "after": "变更后"}
  ],
  "explanation": "重构说明"
}`, req.Language, goal, req.Code, s.buildContextSection(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码重构失败: %w", err)
	}

	var result CodeRefactorResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = CodeRefactorResponse{
			RefactoredCode: response,
			Explanation:    "Refactored code",
		}
	}

	return &result, nil
}

// ============ 代码审查 ============

// CodeReviewRequest 代码审查请求
type CodeReviewRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model,omitempty"`
}

// CodeReviewResponse 代码审查响应
type CodeReviewResponse struct {
	OverallScore int            `json:"overall_score"`
	Categories   []ReviewCategory `json:"categories"`
	Comments     []ReviewComment  `json:"comments"`
	Approval     string         `json:"approval"` // approved, changes_requested, rejected
}

// ReviewCategory 审查分类
type ReviewCategory struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Issues []string `json:"issues,omitempty"`
}

// ReviewComment 审查评论
type ReviewComment struct {
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// ReviewCode 代码审查
func (s *CodeService) ReviewCode(ctx context.Context, req *CodeReviewRequest) (*CodeReviewResponse, error) {
	prompt := fmt.Sprintf(`请对以下 %s 代码进行代码审查。

代码:
%s

%s

请从以下维度进行审查:
1. 代码质量
2. 可读性
3. 性能
4. 安全性
5. 可维护性
6. 测试覆盖

以JSON格式返回:
{
  "overall_score": 总分(0-100),
  "categories": [
    {"name": "维度名称", "score": 分数(0-100), "issues": ["问题列表"]}
  ],
  "comments": [
    {"line": 行号, "severity": "critical/major/minor/info", "message": "评论内容", "suggestion": "建议"}
  ],
  "approval": "approved/changes_requested/rejected"
}`, req.Language, req.Code, s.buildContextSection(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码审查失败: %w", err)
	}

	var result CodeReviewResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = CodeReviewResponse{
			OverallScore: 50,
			Approval:     "changes_requested",
			Comments: []ReviewComment{
				{Line: 0, Severity: "info", Message: response},
			},
		}
	}

	return &result, nil
}

// ============ 代码执行（增强版） ============

// CodeExecutionRequest 代码执行请求
type CodeExecutionRequest struct {
	Language   string                 `json:"language" binding:"required"`
	Code       string                 `json:"code" binding:"required"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Timeout    int                    `json:"timeout,omitempty"` // 秒，默认30
}

// CodeExecutionResponse 代码执行响应
type CodeExecutionResponse struct {
	Output        string  `json:"output"`
	Error         string  `json:"error,omitempty"`
	ExecutionTime float64 `json:"execution_time"`
	Status        string  `json:"status"` // completed, failed, timeout
}

// ExecuteCode 执行代码（使用临时文件和子进程，带超时控制）
func (s *CodeService) ExecuteCode(ctx context.Context, req *CodeExecutionRequest) (*CodeExecutionResponse, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 30
	}

	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "code-exec-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var output string
	var execErr error
	var execTime float64

	start := time.Now()

	switch req.Language {
	case "python", "py":
		output, execErr = s.runPython(ctx, tmpDir, req.Code, timeout)
	case "javascript", "js", "node":
		output, execErr = s.runJavaScript(ctx, tmpDir, req.Code, timeout)
	case "go", "golang":
		output, execErr = s.runGo(ctx, tmpDir, req.Code, timeout)
	case "bash", "shell", "sh":
		output, execErr = s.runBash(ctx, tmpDir, req.Code, timeout)
	default:
		return nil, fmt.Errorf("不支持的编程语言: %s", req.Language)
	}

	execTime = time.Since(start).Seconds()

	resp := &CodeExecutionResponse{
		Output:        output,
		ExecutionTime: execTime,
	}

	if execErr != nil {
		resp.Error = execErr.Error()
		resp.Status = "failed"
		if ctx.Err() == context.DeadlineExceeded {
			resp.Status = "timeout"
		}
	} else {
		resp.Status = "completed"
	}

	return resp, nil
}

// runPython 执行 Python 代码
func (s *CodeService) runPython(ctx context.Context, tmpDir, code string, timeout int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	scriptPath := filepath.Join(tmpDir, "script.py")
	if err := os.WriteFile(scriptPath, []byte(code), 0644); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "python3", scriptPath)
	cmd.Dir = tmpDir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out.String(), fmt.Errorf("执行超时(%ds)", timeout)
		}
		return out.String(), fmt.Errorf("%v: %s", err, stderr.String())
	}

	return out.String(), nil
}

// runJavaScript 执行 JavaScript 代码
func (s *CodeService) runJavaScript(ctx context.Context, tmpDir, code string, timeout int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	scriptPath := filepath.Join(tmpDir, "script.js")
	if err := os.WriteFile(scriptPath, []byte(code), 0644); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "node", scriptPath)
	cmd.Dir = tmpDir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out.String(), fmt.Errorf("执行超时(%ds)", timeout)
		}
		return out.String(), fmt.Errorf("%v: %s", err, stderr.String())
	}

	return out.String(), nil
}

// runGo 执行 Go 代码
func (s *CodeService) runGo(ctx context.Context, tmpDir, code string, timeout int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	scriptPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(scriptPath, []byte(code), 0644); err != nil {
		return "", err
	}

	// 先编译
	binPath := filepath.Join(tmpDir, "main")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, scriptPath)
	buildCmd.Dir = tmpDir
	var buildErr bytes.Buffer
	buildCmd.Stderr = &buildErr

	if err := buildCmd.Run(); err != nil {
		return "", fmt.Errorf("编译失败: %v: %s", err, buildErr.String())
	}

	// 再运行
	cmd := exec.CommandContext(ctx, binPath)
	cmd.Dir = tmpDir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out.String(), fmt.Errorf("执行超时(%ds)", timeout)
		}
		return out.String(), fmt.Errorf("%v: %s", err, stderr.String())
	}

	return out.String(), nil
}

// runBash 执行 Bash 脚本
func (s *CodeService) runBash(ctx context.Context, tmpDir, code string, timeout int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	scriptPath := filepath.Join(tmpDir, "script.sh")
	if err := os.WriteFile(scriptPath, []byte(code), 0755); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Dir = tmpDir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out.String(), fmt.Errorf("执行超时(%ds)", timeout)
		}
		return out.String(), fmt.Errorf("%v: %s", err, stderr.String())
	}

	return out.String(), nil
}

// ============ 代码解释 ============

// ExplainCodeRequest 代码解释请求
type ExplainCodeRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Level    string `json:"level,omitempty"` // beginner, intermediate, expert
	Model    string `json:"model,omitempty"`
}

// ExplainCodeResponse 代码解释响应
type ExplainCodeResponse struct {
	Summary     string   `json:"summary"`
	LineByLine  []LineExplanation `json:"line_by_line,omitempty"`
	KeyConcepts []string `json:"key_concepts"`
}

// LineExplanation 行级解释
type LineExplanation struct {
	Line        int    `json:"line"`
	Code        string `json:"code"`
	Explanation string `json:"explanation"`
}

// ExplainCode 解释代码
func (s *CodeService) ExplainCode(ctx context.Context, req *ExplainCodeRequest) (*ExplainCodeResponse, error) {
	level := req.Level
	if level == "" {
		level = "intermediate"
	}

	prompt := fmt.Sprintf(`请用 %s 水平解释以下 %s 代码。

代码:
%s

请提供:
1. 整体功能概述
2. 逐行或逐段解释
3. 涉及的关键概念

以JSON格式返回:
{
  "summary": "整体概述",
  "line_by_line": [
    {"line": 行号, "code": "代码片段", "explanation": "解释"}
  ],
  "key_concepts": ["关键概念列表"]
}`, level, req.Language, req.Code)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码解释失败: %w", err)
	}

	var result ExplainCodeResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = ExplainCodeResponse{
			Summary: response,
		}
	}

	return &result, nil
}

// ============ 辅助方法 ============

// callLLM 调用 LLM
func (s *CodeService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个专业的代码专家，擅长代码分析、生成、重构和审查。请用中文回复。"},
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

// buildContextSection 构建上下文部分
func (s *CodeService) buildContextSection(context string) string {
	if context == "" {
		return ""
	}
	return fmt.Sprintf("上下文:\n%s", context)
}

// parseJSONFromResponse 从 LLM 响应中提取 JSON
func parseJSONFromResponse(response string, target interface{}) error {
	// 尝试找到 JSON 块
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || end <= start {
		return fmt.Errorf("未找到JSON")
	}

	jsonStr := response[start : end+1]
	return json.Unmarshal([]byte(jsonStr), target)
}

// ============ 原有方法兼容 ============

// CreateCodeExecution 创建代码执行记录
func (s *CodeService) CreateCodeExecution(execution *models.CodeExecution) error {
	execution.ID = uuid.New().String()
	execution.Status = "pending"
	execution.CreatedAt = time.Now()
	execution.UpdatedAt = time.Now()
	return s.db.Create(execution).Error
}

// GetCodeExecution 获取代码执行记录
func (s *CodeService) GetCodeExecution(id string) (*models.CodeExecution, error) {
	var execution models.CodeExecution
	err := s.db.First(&execution, id).Error
	return &execution, err
}

// ListCodeExecutions 列出代码执行记录
func (s *CodeService) ListCodeExecutions() ([]models.CodeExecution, error) {
	var executions []models.CodeExecution
	err := s.db.Find(&executions).Error
	return executions, err
}
