package services

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"openaide/backend/src/services/llm"
)

type TestGenService struct {
	BaseService
	llmClient llm.LLMClient
}

func NewTestGenService(db *gorm.DB, logger *LoggerService, cache *CacheService, llmClient llm.LLMClient) *TestGenService {
	return &TestGenService{
		BaseService: BaseService{
			DB:     db,
			Logger: logger,
			Cache:  cache,
		},
		llmClient: llmClient,
	}
}

type GenerateTestsRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	Framework string `json:"framework,omitempty"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model,omitempty"`
}

type GenerateTestsResponse struct {
	Tests       string   `json:"tests"`
	Explanation string   `json:"explanation"`
	Coverage    []string `json:"coverage"`
}

func (s *TestGenService) GenerateTests(ctx context.Context, req *GenerateTestsRequest) (*GenerateTestsResponse, error) {
	frameworkSection := ""
	if req.Framework != "" {
		frameworkSection = fmt.Sprintf("测试框架: %s", req.Framework)
	}

	prompt := fmt.Sprintf(`请为以下 %s 代码生成高质量的单元测试。

代码:
%s

%s
%s

请提供:
1. 完整的单元测试代码
2. 测试说明
3. 覆盖的测试场景列表

以JSON格式返回:
{
  "tests": "测试代码",
  "explanation": "测试说明",
  "coverage": ["覆盖的测试场景列表"]
}`, req.Language, req.Code, s.buildContextSection(req.Context), frameworkSection)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("单元测试生成失败: %w", err)
	}

	var result GenerateTestsResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = GenerateTestsResponse{
			Tests:       response,
			Explanation: "Generated unit tests",
		}
	}

	return &result, nil
}

type GenerateIntegrationTestsRequest struct {
	Code        string   `json:"code" binding:"required"`
	Language    string   `json:"language" binding:"required"`
	Framework   string   `json:"framework,omitempty"`
	Modules     []string `json:"modules,omitempty"`
	Context     string   `json:"context,omitempty"`
	Model       string   `json:"model,omitempty"`
}

type GenerateIntegrationTestsResponse struct {
	Tests       string   `json:"tests"`
	Explanation string   `json:"explanation"`
	Dependencies []string `json:"dependencies"`
}

func (s *TestGenService) GenerateIntegrationTests(ctx context.Context, req *GenerateIntegrationTestsRequest) (*GenerateIntegrationTestsResponse, error) {
	frameworkSection := ""
	if req.Framework != "" {
		frameworkSection = fmt.Sprintf("测试框架: %s", req.Framework)
	}

	modulesSection := ""
	if len(req.Modules) > 0 {
		modulesSection = "关联模块:\n"
		for _, m := range req.Modules {
			modulesSection += "- " + m + "\n"
		}
	}

	prompt := fmt.Sprintf(`请为以下 %s 代码生成集成测试。

代码:
%s

%s
%s
%s

请提供:
1. 完整的集成测试代码
2. 测试说明
3. 依赖项列表

以JSON格式返回:
{
  "tests": "集成测试代码",
  "explanation": "测试说明",
  "dependencies": ["依赖项列表"]
}`, req.Language, req.Code, s.buildContextSection(req.Context), frameworkSection, modulesSection)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("集成测试生成失败: %w", err)
	}

	var result GenerateIntegrationTestsResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = GenerateIntegrationTestsResponse{
			Tests:       response,
			Explanation: "Generated integration tests",
		}
	}

	return &result, nil
}

type AnalyzeCoverageRequest struct {
	Code        string   `json:"code" binding:"required"`
	Tests       string   `json:"tests" binding:"required"`
	Language    string   `json:"language" binding:"required"`
	Context     string   `json:"context,omitempty"`
	Model       string   `json:"model,omitempty"`
}

type AnalyzeCoverageResponse struct {
	Score       int               `json:"score"`
	Gaps        []CoverageGap     `json:"gaps"`
	Suggestions []string          `json:"suggestions"`
}

type CoverageGap struct {
	Function    string `json:"function"`
	Line        int    `json:"line"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

func (s *TestGenService) AnalyzeCoverage(ctx context.Context, req *AnalyzeCoverageRequest) (*AnalyzeCoverageResponse, error) {
	prompt := fmt.Sprintf(`请分析以下 %s 代码的测试覆盖率，找出测试覆盖不足的地方。

源代码:
%s

测试代码:
%s

%s

请提供:
1. 覆盖率评分(0-100)
2. 覆盖缺口列表
3. 改进建议

以JSON格式返回:
{
  "score": 覆盖率评分(0-100),
  "gaps": [
    {"function": "函数名", "line": 行号, "description": "缺口描述", "severity": "high/medium/low"}
  ],
  "suggestions": ["改进建议列表"]
}`, req.Language, req.Code, req.Tests, s.buildContextSection(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("覆盖率分析失败: %w", err)
	}

	var result AnalyzeCoverageResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = AnalyzeCoverageResponse{
			Score: 0,
			Suggestions: []string{response},
		}
	}

	return &result, nil
}

type GenerateMutationTestsRequest struct {
	Code     string `json:"code" binding:"required"`
	Tests    string `json:"tests" binding:"required"`
	Language string `json:"language" binding:"required"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model,omitempty"`
}

type GenerateMutationTestsResponse struct {
	Mutations   []MutationTest `json:"mutations"`
	Explanation string         `json:"explanation"`
	Score       int            `json:"score"`
}

type MutationTest struct {
	Original    string `json:"original"`
	Mutated     string `json:"mutated"`
	Description string `json:"description"`
	Killed      bool   `json:"killed"`
}

func (s *TestGenService) GenerateMutationTests(ctx context.Context, req *GenerateMutationTestsRequest) (*GenerateMutationTestsResponse, error) {
	prompt := fmt.Sprintf(`请为以下 %s 代码生成变异测试，评估测试套件的质量。

源代码:
%s

测试代码:
%s

%s

请提供:
1. 变异测试列表（对源代码进行小范围修改，检查测试是否能检测到）
2. 变异测试说明
3. 变异测试评分(0-100)

以JSON格式返回:
{
  "mutations": [
    {"original": "原始代码", "mutated": "变异代码", "description": "变异描述", "killed": true/false}
  ],
  "explanation": "变异测试说明",
  "score": 变异测试评分(0-100)
}`, req.Language, req.Code, req.Tests, s.buildContextSection(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("变异测试生成失败: %w", err)
	}

	var result GenerateMutationTestsResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = GenerateMutationTestsResponse{
			Explanation: response,
			Score:       0,
		}
	}

	return &result, nil
}

func (s *TestGenService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个专业的测试工程师，擅长编写高质量的单元测试、集成测试和变异测试。请用中文回复。"},
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

func (s *TestGenService) buildContextSection(context string) string {
	if context == "" {
		return ""
	}
	return fmt.Sprintf("上下文:\n%s", context)
}
