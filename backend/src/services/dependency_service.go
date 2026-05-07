package services

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"openaide/backend/src/services/llm"
)

type DependencyService struct {
	BaseService
	llmClient llm.LLMClient
}

func NewDependencyService(db *gorm.DB, logger *LoggerService, cache *CacheService, llmClient llm.LLMClient) *DependencyService {
	return &DependencyService{
		BaseService: BaseService{DB: db, Logger: logger, Cache: cache},
		llmClient:   llmClient,
	}
}

type DependencyAnalysisRequest struct {
	ProjectPath     string   `json:"project_path" binding:"required"`
	Language        string   `json:"language" binding:"required"`
	DependencyFile  string   `json:"dependency_file,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	Model           string   `json:"model,omitempty"`
}

type DependencyAnalysisResponse struct {
	TotalCount      int                    `json:"total_count"`
	Outdated        []OutdatedDependency   `json:"outdated"`
	Vulnerable      []VulnerableDependency `json:"vulnerable"`
	Unused          []string               `json:"unused"`
	Recommendations []string               `json:"recommendations"`
	Summary         string                 `json:"summary"`
}

type OutdatedDependency struct {
	Name         string `json:"name"`
	Current      string `json:"current_version"`
	Latest       string `json:"latest_version"`
	Severity     string `json:"severity"`
	ChangelogURL string `json:"changelog_url,omitempty"`
}

type VulnerableDependency struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	CVEs        []CVEInfo `json:"cves"`
	Severity    string   `json:"severity"`
	FixedIn     string   `json:"fixed_in,omitempty"`
}

type CVEInfo struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Published   string `json:"published,omitempty"`
}

func (s *DependencyService) AnalyzeDependencies(ctx context.Context, req *DependencyAnalysisRequest) (*DependencyAnalysisResponse, error) {
	depsStr := ""
	if len(req.Dependencies) > 0 {
		depsStr = "依赖列表:\n"
		for _, dep := range req.Dependencies {
			depsStr += "- " + dep + "\n"
		}
	}

	depFileStr := ""
	if req.DependencyFile != "" {
		depFileStr = fmt.Sprintf("依赖文件内容:\n%s", req.DependencyFile)
	}

	prompt := fmt.Sprintf(`请对以下 %s 项目的依赖进行全面分析。

项目路径: %s

%s
%s

请提供以下分析结果（以JSON格式返回）:
{
  "total_count": 依赖总数,
  "outdated": [
    {"name": "依赖名", "current_version": "当前版本", "latest_version": "最新版本", "severity": "high/medium/low", "changelog_url": "变更日志URL"}
  ],
  "vulnerable": [
    {"name": "依赖名", "version": "版本", "cves": [{"id": "CVE编号", "severity": "严重程度", "description": "漏洞描述", "published": "发布日期"}], "severity": "high/medium/low", "fixed_in": "修复版本"}
  ],
  "unused": ["未使用的依赖列表"],
  "recommendations": ["优化建议列表"],
  "summary": "分析摘要"
}`, req.Language, req.ProjectPath, depsStr, depFileStr)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("依赖分析失败: %w", err)
	}

	var result DependencyAnalysisResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = DependencyAnalysisResponse{
			Summary: response,
		}
	}

	return &result, nil
}

type SecurityCheckRequest struct {
	ProjectPath  string   `json:"project_path" binding:"required"`
	Language     string   `json:"language" binding:"required"`
	Dependencies []string `json:"dependencies" binding:"required"`
	Severity     string   `json:"severity,omitempty"`
	Model        string   `json:"model,omitempty"`
}

type SecurityCheckResponse struct {
	VulnerabilityCount int                      `json:"vulnerability_count"`
	Critical          int                      `json:"critical"`
	High              int                      `json:"high"`
	Medium            int                      `json:"medium"`
	Low               int                      `json:"low"`
	Vulnerabilities   []VulnerabilityDetail    `json:"vulnerabilities"`
	RemediationSteps  []RemediationStep        `json:"remediation_steps"`
	SecurityScore     int                      `json:"security_score"`
}

type VulnerabilityDetail struct {
	Package      string   `json:"package"`
	Version      string   `json:"version"`
	CVE          string   `json:"cve"`
	Severity     string   `json:"severity"`
	Description  string   `json:"description"`
	Affected     []string `json:"affected_versions,omitempty"`
	FixedIn      string   `json:"fixed_in,omitempty"`
	References   []string `json:"references,omitempty"`
}

type RemediationStep struct {
	Package   string `json:"package"`
	Action    string `json:"action"`
	TargetVersion string `json:"target_version,omitempty"`
	Priority  string `json:"priority"`
	Details   string `json:"details"`
}

func (s *DependencyService) CheckSecurityVulnerabilities(ctx context.Context, req *SecurityCheckRequest) (*SecurityCheckResponse, error) {
	depsStr := ""
	for _, dep := range req.Dependencies {
		depsStr += "- " + dep + "\n"
	}

	severityFilter := ""
	if req.Severity != "" {
		severityFilter = fmt.Sprintf("仅报告 %s 及以上严重级别的漏洞。", req.Severity)
	}

	prompt := fmt.Sprintf(`请检查以下 %s 项目依赖中的已知安全漏洞。

项目路径: %s

依赖列表:
%s

%s

请提供以下结果（以JSON格式返回）:
{
  "vulnerability_count": 漏洞总数,
  "critical": 严重漏洞数,
  "high": 高危漏洞数,
  "medium": 中危漏洞数,
  "low": 低危漏洞数,
  "vulnerabilities": [
    {"package": "包名", "version": "版本", "cve": "CVE编号", "severity": "critical/high/medium/low", "description": "漏洞描述", "affected_versions": ["受影响版本"], "fixed_in": "修复版本", "references": ["参考链接"]}
  ],
  "remediation_steps": [
    {"package": "包名", "action": "升级/替换/移除", "target_version": "目标版本", "priority": "critical/high/medium/low", "details": "详细说明"}
  ],
  "security_score": 安全评分(0-100)
}`, req.Language, req.ProjectPath, depsStr, severityFilter)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("安全漏洞检查失败: %w", err)
	}

	var result SecurityCheckResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = SecurityCheckResponse{
			SecurityScore: 50,
		}
	}

	return &result, nil
}

type UpdateSuggestionRequest struct {
	ProjectPath     string   `json:"project_path" binding:"required"`
	Language        string   `json:"language" binding:"required"`
	Dependencies    []string `json:"dependencies" binding:"required"`
	Strategy        string   `json:"strategy,omitempty"`
	ExcludeBreaking bool     `json:"exclude_breaking,omitempty"`
	Model           string   `json:"model,omitempty"`
}

type UpdateSuggestionResponse struct {
	Updates          []UpdateSuggestion      `json:"updates"`
	BreakingChanges  []BreakingChange        `json:"breaking_changes"`
	CompatibilityMatrix []CompatibilityInfo  `json:"compatibility_matrix"`
	UpdateOrder      []string                `json:"update_order"`
	Risks            []string                `json:"risks"`
	Summary          string                  `json:"summary"`
}

type UpdateSuggestion struct {
	Package         string `json:"package"`
	CurrentVersion  string `json:"current_version"`
	SuggestedVersion string `json:"suggested_version"`
	Priority        string `json:"priority"`
	Reason          string `json:"reason"`
	IsBreaking      bool   `json:"is_breaking"`
	Effort          string `json:"effort"`
}

type BreakingChange struct {
	Package    string `json:"package"`
	Version    string `json:"version"`
	Change     string `json:"change"`
	Migration  string `json:"migration"`
}

type CompatibilityInfo struct {
	Package1 string `json:"package1"`
	Version1 string `json:"version1"`
	Package2 string `json:"package2"`
	Version2 string `json:"version2"`
	Compatible bool `json:"compatible"`
	Notes     string `json:"notes,omitempty"`
}

func (s *DependencyService) SuggestUpdates(ctx context.Context, req *UpdateSuggestionRequest) (*UpdateSuggestionResponse, error) {
	depsStr := ""
	for _, dep := range req.Dependencies {
		depsStr += "- " + dep + "\n"
	}

	strategyStr := "保守策略（仅安全更新）"
	if req.Strategy != "" {
		strategyMap := map[string]string{
			"conservative": "保守策略（仅安全更新）",
			"moderate":     "适度策略（安全+稳定更新）",
			"aggressive":   "激进策略（全部更新到最新版）",
		}
		if v, ok := strategyMap[req.Strategy]; ok {
			strategyStr = v
		}
	}

	excludeStr := ""
	if req.ExcludeBreaking {
		excludeStr = "请排除所有包含破坏性变更的更新建议。"
	}

	prompt := fmt.Sprintf(`请为以下 %s 项目提供依赖更新建议。

项目路径: %s

当前依赖:
%s

更新策略: %s
%s

请提供以下结果（以JSON格式返回）:
{
  "updates": [
    {"package": "包名", "current_version": "当前版本", "suggested_version": "建议版本", "priority": "critical/high/medium/low", "reason": "更新原因", "is_breaking": true/false, "effort": "升级工作量评估"}
  ],
  "breaking_changes": [
    {"package": "包名", "version": "版本", "change": "破坏性变更描述", "migration": "迁移指南"}
  ],
  "compatibility_matrix": [
    {"package1": "包1", "version1": "版本1", "package2": "包2", "version2": "版本2", "compatible": true/false, "notes": "兼容性说明"}
  ],
  "update_order": ["建议的更新顺序，包名列表"],
  "risks": ["风险提示列表"],
  "summary": "更新建议摘要"
}`, req.Language, req.ProjectPath, depsStr, strategyStr, excludeStr)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("更新建议生成失败: %w", err)
	}

	var result UpdateSuggestionResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = UpdateSuggestionResponse{
			Summary: response,
		}
	}

	return &result, nil
}

type DependencyGraphRequest struct {
	ProjectPath  string   `json:"project_path" binding:"required"`
	Language     string   `json:"language" binding:"required"`
	Dependencies []string `json:"dependencies,omitempty"`
	Depth        int      `json:"depth,omitempty"`
	Model        string   `json:"model,omitempty"`
}

type DependencyGraphResponse struct {
	Nodes    []GraphNode `json:"nodes"`
	Edges    []GraphEdge `json:"edges"`
	Clusters []Cluster   `json:"clusters"`
	Stats    GraphStats  `json:"stats"`
}

type GraphNode struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Type       string `json:"type"`
	IsDirect   bool   `json:"is_direct"`
	IsDevOnly  bool   `json:"is_dev_only"`
}

type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
	Label  string `json:"label,omitempty"`
}

type Cluster struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type GraphStats struct {
	TotalNodes      int `json:"total_nodes"`
	TotalEdges      int `json:"total_edges"`
	MaxDepth        int `json:"max_depth"`
	DirectDeps      int `json:"direct_deps"`
	TransitiveDeps  int `json:"transitive_deps"`
}

func (s *DependencyService) GenerateDependencyGraph(ctx context.Context, req *DependencyGraphRequest) (*DependencyGraphResponse, error) {
	depsStr := ""
	if len(req.Dependencies) > 0 {
		depsStr = "依赖列表:\n"
		for _, dep := range req.Dependencies {
			depsStr += "- " + dep + "\n"
		}
	}

	depthStr := ""
	if req.Depth > 0 {
		depthStr = fmt.Sprintf("分析深度: %d 层", req.Depth)
	}

	prompt := fmt.Sprintf(`请为以下 %s 项目生成依赖关系图。

项目路径: %s

%s
%s

请提供以下结果（以JSON格式返回）:
{
  "nodes": [
    {"id": "唯一标识", "name": "依赖名称", "version": "版本", "type": "direct/transitive/dev", "is_direct": true/false, "is_dev_only": true/false}
  ],
  "edges": [
    {"source": "源节点ID", "target": "目标节点ID", "type": "depends/peer/optional", "label": "关系说明"}
  ],
  "clusters": [
    {"id": "集群ID", "name": "集群名称", "members": ["节点ID列表"]}
  ],
  "stats": {
    "total_nodes": 节点总数,
    "total_edges": 边总数,
    "max_depth": 最大依赖深度,
    "direct_deps": 直接依赖数,
    "transitive_deps": 传递依赖数
  }
}`, req.Language, req.ProjectPath, depsStr, depthStr)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("依赖图生成失败: %w", err)
	}

	var result DependencyGraphResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = DependencyGraphResponse{
			Nodes: []GraphNode{},
			Edges: []GraphEdge{},
		}
	}

	return &result, nil
}

func (s *DependencyService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个专业的依赖管理专家，擅长分析项目依赖、检测安全漏洞和推荐更新策略。请用中文回复。"},
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
