package services

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"openaide/backend/src/services/llm"
)

type DeployService struct {
	BaseService
	llmClient llm.LLMClient
}

func NewDeployService(db *gorm.DB, logger *LoggerService, cache *CacheService, llmClient llm.LLMClient) *DeployService {
	return &DeployService{
		BaseService: BaseService{DB: db, Logger: logger, Cache: cache},
		llmClient:   llmClient,
	}
}

type DockerfileRequest struct {
	ProjectType    string   `json:"project_type" binding:"required"`
	Language       string   `json:"language" binding:"required"`
	Framework      string   `json:"framework,omitempty"`
	RuntimeVersion string   `json:"runtime_version,omitempty"`
	BuildTool      string   `json:"build_tool,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	ExposePort     int      `json:"expose_port,omitempty"`
	EnvVars        []string `json:"env_vars,omitempty"`
	Context        string   `json:"context,omitempty"`
	Model          string   `json:"model,omitempty"`
}

type DockerfileResponse struct {
	Dockerfile   string   `json:"dockerfile"`
	Explanation  string   `json:"explanation"`
	BuildCommand string   `json:"build_command"`
	RunCommand   string   `json:"run_command"`
	Tips         []string `json:"tips"`
}

func (s *DeployService) GenerateDockerfile(ctx context.Context, req *DockerfileRequest) (*DockerfileResponse, error) {
	depsStr := ""
	if len(req.Dependencies) > 0 {
		depsStr = "依赖项:\n"
		for _, d := range req.Dependencies {
			depsStr += "- " + d + "\n"
		}
	}

	envStr := ""
	if len(req.EnvVars) > 0 {
		envStr = "环境变量:\n"
		for _, e := range req.EnvVars {
			envStr += "- " + e + "\n"
		}
	}

	portStr := ""
	if req.ExposePort > 0 {
		portStr = fmt.Sprintf("暴露端口: %d", req.ExposePort)
	}

	prompt := fmt.Sprintf(`请为以下项目生成 Dockerfile。

项目类型: %s
编程语言: %s
%s
%s
%s
%s
%s

请生成生产级 Dockerfile，遵循最佳实践:
1. 使用多阶段构建减小镜像体积
2. 使用非 root 用户运行
3. 合理利用 Docker 缓存层
4. 设置健康检查
5. 添加 .dockerignore 建议

以JSON格式返回:
{
  "dockerfile": "完整的 Dockerfile 内容",
  "explanation": "构建说明和关键决策解释",
  "build_command": "构建命令",
  "run_command": "运行命令",
  "tips": ["部署建议列表"]
}`, req.ProjectType, req.Language, s.buildContextSection(req.Framework, "框架"), s.buildContextSection(req.RuntimeVersion, "运行时版本"), s.buildContextSection(req.BuildTool, "构建工具"), depsStr, envStr+"\n"+portStr)

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("Dockerfile生成失败: %w", err)
	}

	var result DockerfileResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = DockerfileResponse{
			Dockerfile:  response,
			Explanation: "Generated Dockerfile",
		}
	}

	return &result, nil
}

type CICDRequest struct {
	Platform      string   `json:"platform" binding:"required"`
	Language      string   `json:"language" binding:"required"`
	Framework     string   `json:"framework,omitempty"`
	BuildTool     string   `json:"build_tool,omitempty"`
	TestCommand   string   `json:"test_command,omitempty"`
	DeployTarget  string   `json:"deploy_target,omitempty"`
	Environments  []string `json:"environments,omitempty"`
	Features      []string `json:"features,omitempty"`
	Context       string   `json:"context,omitempty"`
	Model         string   `json:"model,omitempty"`
}

type CICDResponse struct {
	Config       string   `json:"config"`
	Filename     string   `json:"filename"`
	Explanation  string   `json:"explanation"`
	Stages       []string `json:"stages"`
	Requirements []string `json:"requirements"`
}

func (s *DeployService) GenerateCICD(ctx context.Context, req *CICDRequest) (*CICDResponse, error) {
	envsStr := ""
	if len(req.Environments) > 0 {
		envsStr = "部署环境:\n"
		for _, e := range req.Environments {
			envsStr += "- " + e + "\n"
		}
	}

	featuresStr := ""
	if len(req.Features) > 0 {
		featuresStr = "需要的功能:\n"
		for _, f := range req.Features {
			featuresStr += "- " + f + "\n"
		}
	}

	prompt := fmt.Sprintf(`请为以下项目生成 CI/CD 流水线配置。

CI/CD平台: %s
编程语言: %s
%s
%s
%s
%s
%s
%s
%s

请生成完整的 CI/CD 配置，包含:
1. 代码检出和依赖安装
2. 代码质量检查（lint）
3. 单元测试
4. 构建
5. 镜像构建和推送
6. 部署到目标环境

以JSON格式返回:
{
  "config": "完整的 CI/CD 配置文件内容",
  "filename": "配置文件名（含路径）",
  "explanation": "流水线说明和关键配置解释",
  "stages": ["流水线阶段列表"],
  "requirements": ["前置要求列表，如需要安装的插件或配置的密钥"]
}`, req.Platform, req.Language, s.buildContextSection(req.Framework, "框架"), s.buildContextSection(req.BuildTool, "构建工具"), s.buildContextSection(req.TestCommand, "测试命令"), s.buildContextSection(req.DeployTarget, "部署目标"), envsStr, featuresStr, s.buildOptionalContext(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("CI/CD配置生成失败: %w", err)
	}

	var result CICDResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = CICDResponse{
			Config:      response,
			Explanation: "Generated CI/CD config",
		}
	}

	return &result, nil
}

type DeployReadinessRequest struct {
	ProjectType     string   `json:"project_type" binding:"required"`
	Language        string   `json:"language" binding:"required"`
	Framework       string   `json:"framework,omitempty"`
	HasDockerfile   bool     `json:"has_dockerfile,omitempty"`
	HasCICD         bool     `json:"has_cicd,omitempty"`
	HasTests        bool     `json:"has_tests,omitempty"`
	HasEnvConfig    bool     `json:"has_env_config,omitempty"`
	HasHealthCheck  bool     `json:"has_health_check,omitempty"`
	HasLogging      bool     `json:"has_logging,omitempty"`
	HasMonitoring   bool     `json:"has_monitoring,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	Concerns        []string `json:"concerns,omitempty"`
	Context         string   `json:"context,omitempty"`
	Model           string   `json:"model,omitempty"`
}

type DeployReadinessResponse struct {
	Ready           bool                  `json:"ready"`
	Score           int                   `json:"score"`
	Checklist       []ReadinessCheckItem  `json:"checklist"`
	Recommendations []string              `json:"recommendations"`
	Risks           []DeployRisk          `json:"risks"`
	NextSteps       []string              `json:"next_steps"`
}

type ReadinessCheckItem struct {
	Category string `json:"category"`
	Item     string `json:"item"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

type DeployRisk struct {
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Mitigation  string `json:"mitigation"`
}

func (s *DeployService) AnalyzeDeployReadiness(ctx context.Context, req *DeployReadinessRequest) (*DeployReadinessResponse, error) {
	depsStr := ""
	if len(req.Dependencies) > 0 {
		depsStr = "依赖项:\n"
		for _, d := range req.Dependencies {
			depsStr += "- " + d + "\n"
		}
	}

	concernsStr := ""
	if len(req.Concerns) > 0 {
		concernsStr = "关注点:\n"
		for _, c := range req.Concerns {
			concernsStr += "- " + c + "\n"
		}
	}

	prompt := fmt.Sprintf(`请分析以下项目的部署就绪状态。

项目类型: %s
编程语言: %s
%s
已有Dockerfile: %v
已有CI/CD: %v
已有测试: %v
已有环境配置: %v
已有健康检查: %v
已有日志: %v
已有监控: %v
%s
%s
%s

请从以下维度评估部署就绪状态:
1. 容器化就绪
2. CI/CD就绪
3. 测试覆盖
4. 配置管理
5. 可观测性
6. 安全性
7. 可扩展性

以JSON格式返回:
{
  "ready": true/false,
  "score": 0-100,
  "checklist": [
    {"category": "维度", "item": "检查项", "status": "pass/fail/warning", "priority": "high/medium/low"}
  ],
  "recommendations": ["改进建议列表"],
  "risks": [
    {"severity": "high/medium/low", "category": "风险类别", "description": "风险描述", "mitigation": "缓解措施"}
  ],
  "next_steps": ["下一步行动列表"]
}`, req.ProjectType, req.Language, s.buildContextSection(req.Framework, "框架"), req.HasDockerfile, req.HasCICD, req.HasTests, req.HasEnvConfig, req.HasHealthCheck, req.HasLogging, req.HasMonitoring, depsStr, concernsStr, s.buildOptionalContext(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("部署就绪分析失败: %w", err)
	}

	var result DeployReadinessResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = DeployReadinessResponse{
			Ready: false,
			Score: 0,
			Recommendations: []string{response},
		}
	}

	return &result, nil
}

type K8sManifestRequest struct {
	AppName        string   `json:"app_name" binding:"required"`
	Namespace      string   `json:"namespace,omitempty"`
	Image          string   `json:"image" binding:"required"`
	Replicas       int      `json:"replicas,omitempty"`
	Port           int      `json:"port,omitempty"`
	TargetPort     int      `json:"target_port,omitempty"`
	Resources      string   `json:"resources,omitempty"`
	Environments   []string `json:"environments,omitempty"`
	ConfigMaps     []string `json:"config_maps,omitempty"`
	Secrets        []string `json:"secrets,omitempty"`
	IngressHost    string   `json:"ingress_host,omitempty"`
	HealthPath     string   `json:"health_path,omitempty"`
	AutoScaling    bool     `json:"auto_scaling,omitempty"`
	MinReplicas    int      `json:"min_replicas,omitempty"`
	MaxReplicas    int      `json:"max_replicas,omitempty"`
	Context        string   `json:"context,omitempty"`
	Model          string   `json:"model,omitempty"`
}

type K8sManifestResponse struct {
	Manifests    []K8sManifest `json:"manifests"`
	ApplyCommand string        `json:"apply_command"`
	Explanation  string        `json:"explanation"`
	Tips         []string      `json:"tips"`
}

type K8sManifest struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Filename string `json:"filename"`
}

func (s *DeployService) GenerateK8sManifest(ctx context.Context, req *K8sManifestRequest) (*K8sManifestResponse, error) {
	replicas := req.Replicas
	if replicas <= 0 {
		replicas = 3
	}

	port := req.Port
	if port <= 0 {
		port = 8080
	}

	targetPort := req.TargetPort
	if targetPort <= 0 {
		targetPort = port
	}

	envsStr := ""
	if len(req.Environments) > 0 {
		envsStr = "环境:\n"
		for _, e := range req.Environments {
			envsStr += "- " + e + "\n"
		}
	}

	cmStr := ""
	if len(req.ConfigMaps) > 0 {
		cmStr = "ConfigMap引用:\n"
		for _, c := range req.ConfigMaps {
			cmStr += "- " + c + "\n"
		}
	}

	secretStr := ""
	if len(req.Secrets) > 0 {
		secretStr = "Secret引用:\n"
		for _, sec := range req.Secrets {
			secretStr += "- " + sec + "\n"
		}
	}

	hpaStr := ""
	if req.AutoScaling {
		minR := req.MinReplicas
		if minR <= 0 {
			minR = replicas
		}
		maxR := req.MaxReplicas
		if maxR <= 0 {
			maxR = replicas * 3
		}
		hpaStr = fmt.Sprintf("自动扩缩容: 启用 (最小%d副本, 最大%d副本)", minR, maxR)
	}

	prompt := fmt.Sprintf(`请为以下应用生成 Kubernetes 部署清单。

应用名称: %s
%s
容器镜像: %s
副本数: %d
端口: %d
目标端口: %d
%s
%s
%s
%s
%s
%s
%s
%s

请生成完整的 Kubernetes 清单，包含:
1. Namespace（如需要）
2. Deployment（含资源限制、就绪探针、存活探针）
3. Service
4. Ingress（如指定了域名）
5. ConfigMap/Secret 引用
6. HPA（如启用自动扩缩容）

以JSON格式返回:
{
  "manifests": [
    {"kind": "资源类型", "name": "资源名称", "content": "YAML内容", "filename": "文件名"}
  ],
  "apply_command": "kubectl apply 命令",
  "explanation": "部署说明和关键配置解释",
  "tips": ["部署建议列表"]
}`, req.AppName, s.buildContextSection(req.Namespace, "命名空间"), req.Image, replicas, port, targetPort, s.buildContextSection(req.Resources, "资源限制"), envsStr, cmStr, secretStr, s.buildContextSection(req.IngressHost, "Ingress域名"), s.buildContextSection(req.HealthPath, "健康检查路径"), hpaStr, s.buildOptionalContext(req.Context))

	response, err := s.callLLM(ctx, prompt, req.Model)
	if err != nil {
		return nil, fmt.Errorf("K8s清单生成失败: %w", err)
	}

	var result K8sManifestResponse
	if err := parseJSONFromResponse(response, &result); err != nil {
		result = K8sManifestResponse{
			Manifests: []K8sManifest{
				{Kind: "Combined", Name: req.AppName, Content: response, Filename: "manifest.yaml"},
			},
			Explanation: "Generated K8s manifests",
		}
	}

	return &result, nil
}

func (s *DeployService) callLLM(ctx context.Context, prompt, model string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("LLM客户端未初始化")
	}

	request := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个专业的DevOps工程师，擅长Docker、Kubernetes、CI/CD和部署自动化。请用中文回复。"},
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

func (s *DeployService) buildContextSection(value, label string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s", label, value)
}

func (s *DeployService) buildOptionalContext(context string) string {
	if context == "" {
		return ""
	}
	return fmt.Sprintf("上下文:\n%s", context)
}
