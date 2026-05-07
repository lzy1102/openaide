package services

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"openaide/backend/src/services/llm"
)

type CodeSearchService struct {
	BaseService
	llmClient llm.LLMClient
}

func NewCodeSearchService(db *gorm.DB, logger *LoggerService, cache *CacheService, llmClient llm.LLMClient) *CodeSearchService {
	return &CodeSearchService{
		BaseService: BaseService{
			DB:     db,
			Logger: logger,
			Cache:  cache,
		},
		llmClient: llmClient,
	}
}

type SearchCodeRequest struct {
	Query      string `json:"query" binding:"required"`
	Language   string `json:"language,omitempty"`
	Repository string `json:"repository,omitempty"`
	Path       string `json:"path,omitempty"`
	Model      string `json:"model,omitempty"`
}

type SearchCodeResult struct {
	FilePath    string  `json:"file_path"`
	LineNumber  int     `json:"line_number"`
	Code        string  `json:"code"`
	Relevance   float64 `json:"relevance"`
	Explanation string  `json:"explanation"`
}

type SearchCodeResponse struct {
	Results []SearchCodeResult `json:"results"`
	Total   int                `json:"total"`
	Query   string             `json:"query"`
}

func (s *CodeSearchService) SearchCode(ctx context.Context, req *SearchCodeRequest) (*SearchCodeResponse, error) {
	prompt := fmt.Sprintf(`请在代码库中搜索与以下查询语义相关的代码。

查询: %s
%s
%s

请分析代码库，找出与查询语义最相关的代码片段，以JSON格式返回:
{
  "results": [
    {
      "file_path": "文件路径",
      "line_number": 行号,
      "code": "匹配的代码片段",
      "relevance": 相关度(0.0-1.0),
      "explanation": "为何匹配的解释"
    }
  ],
  "total": 匹配结果总数,
  "query": "原始查询"
}`, req.Query, s.buildLanguageSection(req.Language), s.buildPathSection(req.Repository, req.Path))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码搜索失败: %w", err)
	}

	var result SearchCodeResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = SearchCodeResponse{
			Results: []SearchCodeResult{},
			Total:   0,
			Query:   req.Query,
		}
	}

	return &result, nil
}

type FindSimilarCodeRequest struct {
	Code       string `json:"code" binding:"required"`
	Language   string `json:"language" binding:"required"`
	Threshold  float64 `json:"threshold,omitempty"`
	Repository string `json:"repository,omitempty"`
	Model      string `json:"model,omitempty"`
}

type SimilarCodeResult struct {
	FilePath    string  `json:"file_path"`
	LineNumber  int     `json:"line_number"`
	Code        string  `json:"code"`
	Similarity  float64 `json:"similarity"`
	PatternType string  `json:"pattern_type"`
}

type FindSimilarCodeResponse struct {
	Results []SimilarCodeResult `json:"results"`
	Pattern string              `json:"pattern"`
}

func (s *CodeSearchService) FindSimilarCode(ctx context.Context, req *FindSimilarCodeRequest) (*FindSimilarCodeResponse, error) {
	threshold := req.Threshold
	if threshold <= 0 {
		threshold = 0.7
	}

	prompt := fmt.Sprintf(`请在代码库中查找与以下代码模式相似的代码片段。

代码:
%s

语言: %s
相似度阈值: %.1f
%s

请识别代码的核心模式，并找出相似的实现，以JSON格式返回:
{
  "results": [
    {
      "file_path": "文件路径",
      "line_number": 行号,
      "code": "相似代码片段",
      "similarity": 相似度(0.0-1.0),
      "pattern_type": "模式类型(如:算法模式/设计模式/代码结构)"
    }
  ],
  "pattern": "识别出的核心模式描述"
}`, req.Code, req.Language, threshold, s.buildRepoSection(req.Repository))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("相似代码查找失败: %w", err)
	}

	var result FindSimilarCodeResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = FindSimilarCodeResponse{
			Results: []SimilarCodeResult{},
			Pattern: "",
		}
	}

	return &result, nil
}

type AnalyzeCodeStructureRequest struct {
	Repository string `json:"repository,omitempty"`
	Path       string `json:"path,omitempty"`
	Language   string `json:"language,omitempty"`
	Depth      string `json:"depth,omitempty"`
	Model      string `json:"model,omitempty"`
}

type ModuleInfo struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Type         string   `json:"type"`
	Dependencies []string `json:"dependencies"`
	Description  string   `json:"description"`
}

type ArchitecturePattern struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Confidence  float64 `json:"confidence"`
}

type AnalyzeCodeStructureResponse struct {
	Modules    []ModuleInfo        `json:"modules"`
	Patterns   []ArchitecturePattern `json:"patterns"`
	EntryPoint string              `json:"entry_point"`
	Summary    string              `json:"summary"`
	Metrics    StructureMetrics    `json:"metrics"`
}

type StructureMetrics struct {
	TotalFiles     int     `json:"total_files"`
	TotalModules   int     `json:"total_modules"`
	CouplingScore  float64 `json:"coupling_score"`
	CohesionScore  float64 `json:"cohesion_score"`
	ComplexityScore float64 `json:"complexity_score"`
}

func (s *CodeSearchService) AnalyzeCodeStructure(ctx context.Context, req *AnalyzeCodeStructureRequest) (*AnalyzeCodeStructureResponse, error) {
	depth := req.Depth
	if depth == "" {
		depth = "detailed"
	}

	prompt := fmt.Sprintf(`请分析代码库的结构和架构。

%s
%s
%s
分析深度: %s

请提供以下分析结果，以JSON格式返回:
{
  "modules": [
    {
      "name": "模块名称",
      "path": "模块路径",
      "type": "模块类型(如:service/controller/model/util)",
      "dependencies": ["依赖的其他模块"],
      "description": "模块功能描述"
    }
  ],
  "patterns": [
    {
      "name": "架构模式名称",
      "description": "模式描述",
      "confidence": 置信度(0.0-1.0)
    }
  ],
  "entry_point": "入口文件路径",
  "summary": "架构概述",
  "metrics": {
    "total_files": 文件总数,
    "total_modules": 模块总数,
    "coupling_score": 耦合度评分(0.0-1.0,越低越好),
    "cohesion_score": 内聚度评分(0.0-1.0,越高越好),
    "complexity_score": 复杂度评分(0.0-1.0)
  }
}`, s.buildLanguageSection(req.Language), s.buildPathSection(req.Repository, req.Path), s.buildRepoSection(req.Repository), depth)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("代码结构分析失败: %w", err)
	}

	var result AnalyzeCodeStructureResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = AnalyzeCodeStructureResponse{
			Modules:  []ModuleInfo{},
			Patterns: []ArchitecturePattern{},
			Summary:  response,
		}
	}

	return &result, nil
}

type FindDeadCodeRequest struct {
	Repository string   `json:"repository,omitempty"`
	Path       string   `json:"path,omitempty"`
	Language   string   `json:"language,omitempty"`
	Exclude    []string `json:"exclude,omitempty"`
	Model      string   `json:"model,omitempty"`
}

type DeadCodeItem struct {
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Code        string `json:"code"`
	Type        string `json:"type"`
	Confidence  float64 `json:"confidence"`
	Reason      string `json:"reason"`
	Suggestion  string `json:"suggestion"`
}

type FindDeadCodeResponse struct {
	DeadCode      []DeadCodeItem `json:"dead_code"`
	TotalFound    int            `json:"total_found"`
	EstimatedSavings string      `json:"estimated_savings"`
	Summary       string         `json:"summary"`
}

func (s *CodeSearchService) FindDeadCode(ctx context.Context, req *FindDeadCodeRequest) (*FindDeadCodeResponse, error) {
	excludeStr := ""
	if len(req.Exclude) > 0 {
		excludeStr = "排除路径:\n"
		for _, e := range req.Exclude {
			excludeStr += "- " + e + "\n"
		}
	}

	prompt := fmt.Sprintf(`请在代码库中查找未使用的/死代码。

%s
%s
%s
%s

请识别以下类型的死代码:
1. 未被调用的函数/方法
2. 未使用的变量/常量
3. 不可达的代码
4. 未使用的导入
5. 废弃的类/接口

以JSON格式返回:
{
  "dead_code": [
    {
      "file_path": "文件路径",
      "line_number": 行号,
      "code": "死代码片段",
      "type": "类型(unused_function/unused_variable/unreachable_code/unused_import/deprecated_class)",
      "confidence": 置信度(0.0-1.0),
      "reason": "判定为死代码的原因",
      "suggestion": "处理建议"
    }
  ],
  "total_found": 发现总数,
  "estimated_savings": "预计可节省的代码量",
  "summary": "死代码分析摘要"
}`, s.buildLanguageSection(req.Language), s.buildPathSection(req.Repository, req.Path), s.buildRepoSection(req.Repository), excludeStr)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("死代码查找失败: %w", err)
	}

	var result FindDeadCodeResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = FindDeadCodeResponse{
			DeadCode: []DeadCodeItem{},
			Summary:  response,
		}
	}

	return &result, nil
}

func (s *CodeSearchService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个专业的代码搜索和分析专家，擅长代码模式识别、架构分析和代码质量评估。请用中文回复。"},
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

func (s *CodeSearchService) buildLanguageSection(language string) string {
	if language == "" {
		return ""
	}
	return fmt.Sprintf("编程语言: %s", language)
}

func (s *CodeSearchService) buildPathSection(repository, path string) string {
	parts := []string{}
	if repository != "" {
		parts = append(parts, fmt.Sprintf("仓库: %s", repository))
	}
	if path != "" {
		parts = append(parts, fmt.Sprintf("路径: %s", path))
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

func (s *CodeSearchService) buildRepoSection(repository string) string {
	if repository == "" {
		return ""
	}
	return fmt.Sprintf("仓库: %s", repository)
}
