package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
	"github.com/google/uuid"
)

// TaskAnalyzer 任务分析器 - 使用 LLM 分析任务类型和复杂度
type TaskAnalyzer struct {
	llmClient     llm.LLMClient
	model         string
	analysisCache map[string]*TaskAnalysis
	cacheTTL      time.Duration
}

// TaskAnalysis 任务分析结果
type TaskAnalysis struct {
	// TaskType 任务类型: coding, research, analysis, testing, documentation, mixed
	TaskType string `json:"task_type"`

	// Description 任务描述
	Description string `json:"description"`

	// Skills 所需技能列表
	Skills []string `json:"skills"`

	// Complexity 复杂度: low, medium, high
	Complexity string `json:"complexity"`

	// EstimatedTime 预估时间
	EstimatedTime time.Duration `json:"estimated_time"`

	// Subtasks 子任务规格
	Subtasks []SubtaskSpec `json:"subtasks"`

	// Dependencies 依赖项
	Dependencies []string `json:"dependencies"`

	// RecommendedTeam 推荐团队类型
	RecommendedTeam string `json:"recommended_team"`

	// Priority 推荐优先级
	Priority string `json:"priority"`

	// Metadata 额外元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// AnalyzedAt 分析时间
	AnalyzedAt time.Time `json:"analyzed_at"`
}

// SubtaskSpec 子任务规格
type SubtaskSpec struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Skills      []string `json:"skills"`
	Order       int      `json:"order"`
	Required    bool     `json:"required"`
	CanParallel bool     `json:"can_parallel"`
}

// AnalysisRequest 分析请求
type AnalysisRequest struct {
	UserMessage   string                 `json:"user_message"`
	Context       map[string]interface{} `json:"context,omitempty"`
	TeamConfig    *TeamConfig            `json:"team_config,omitempty"`
	PreviousTasks []string               `json:"previous_tasks,omitempty"`
}

// TeamConfig 团队配置
type TeamConfig struct {
	Members     []MemberCapability `json:"members"`
	TeamType    string             `json:"team_type"` // development, research, mixed
	MaxParallel int                `json:"max_parallel"`
}

// MemberCapability 成员能力
type MemberCapability struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Role          string   `json:"role"`
	Capabilities  []string `json:"capabilities"`
	Specialization []string `json:"specialization"`
	Availability  string   `json:"availability"` // available, busy, offline
}

// NewTaskAnalyzer 创建任务分析器
func NewTaskAnalyzer(llmClient llm.LLMClient, model string) *TaskAnalyzer {
	if model == "" {
		model = "gpt-4"
	}
	return &TaskAnalyzer{
		llmClient:     llmClient,
		model:         model,
		analysisCache: make(map[string]*TaskAnalysis),
		cacheTTL:      30 * time.Minute,
	}
}

// Analyze 分析任务
func (ta *TaskAnalyzer) Analyze(ctx context.Context, req *AnalysisRequest) (*TaskAnalysis, error) {
	// 生成缓存键
	cacheKey := ta.generateCacheKey(req)

	// 检查缓存
	if cached, ok := ta.getCachedAnalysis(cacheKey); ok {
		return cached, nil
	}

	// 构建分析提示
	prompt := ta.buildAnalysisPrompt(req)

	// 调用 LLM
	analysis, err := ta.callLLMForAnalysis(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}

	// 后处理
	analysis = ta.postProcess(analysis, req)

	// 缓存结果
	ta.cacheAnalysis(cacheKey, analysis)

	return analysis, nil
}

// AnalyzeFromTask 从现有任务模型分析
func (ta *TaskAnalyzer) AnalyzeFromTask(ctx context.Context, task *models.Task) (*TaskAnalysis, error) {
	req := &AnalysisRequest{
		UserMessage: fmt.Sprintf("%s: %s", task.Title, task.Description),
		Context: map[string]interface{}{
			"task_type":    task.Type,
			"priority":     task.Priority,
			"complexity":   task.Complexity,
			"tags":        task.Tags,
			"context":     task.Context,
		},
	}
	return ta.Analyze(ctx, req)
}

// buildAnalysisPrompt 构建分析提示词
func (ta *TaskAnalyzer) buildAnalysisPrompt(req *AnalysisRequest) string {
	var sb strings.Builder

	sb.WriteString("你是一个任务分析专家。请分析以下用户请求，输出结构化的任务分析结果。\n\n")

	// 添加团队上下文
	if req.TeamConfig != nil && len(req.TeamConfig.Members) > 0 {
		sb.WriteString("## 团队配置\n\n")
		for _, m := range req.TeamConfig.Members {
			sb.WriteString(fmt.Sprintf("- %s (%s): %v, 专长: %v\n",
				m.Name, m.Role, m.Capabilities, m.Specialization))
		}
		sb.WriteString("\n")
	}

	// 添加历史上下文
	if len(req.PreviousTasks) > 0 {
		sb.WriteString("## 前置任务\n\n")
		for i, pt := range req.PreviousTasks {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, pt))
		}
		sb.WriteString("\n")
	}

	// 添加用户消息
	sb.WriteString("## 用户请求\n\n")
	sb.WriteString(req.UserMessage)
	sb.WriteString("\n\n")

	// 添加额外上下文
	if len(req.Context) > 0 {
		sb.WriteString("## 额外上下文\n\n")
		if contextJSON, err := json.Marshal(req.Context); err == nil {
			sb.WriteString(string(contextJSON))
		}
		sb.WriteString("\n\n")
	}

	// 输出格式要求
	sb.WriteString("## 输出要求\n\n")
	sb.WriteString("请以 JSON 格式输出分析结果，包含以下字段:\n\n")
	sb.WriteString(`{
  "task_type": "coding|research|analysis|testing|documentation|mixed",
  "description": "简洁的任务描述",
  "skills": ["技能1", "技能2", ...],
  "complexity": "low|medium|high",
  "estimated_time_minutes": 数字,
  "subtasks": [
    {
      "id": "唯一ID",
      "title": "子任务标题",
      "description": "描述",
      "type": "coding|research|testing等",
      "skills": ["所需技能"],
      "order": 数字,
      "required": true/false,
      "can_parallel": true/false
    }
  ],
  "dependencies": ["依赖项描述"],
  "recommended_team": "development|research|mixed",
  "priority": "low|medium|high|urgent"
}`)

	return sb.String()
}

// callLLMForAnalysis 调用 LLM 进行分析
func (ta *TaskAnalyzer) callLLMForAnalysis(ctx context.Context, prompt string) (*TaskAnalysis, error) {
	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    llm.RoleSystem,
				Content: "你是一个任务分析专家，擅长理解用户需求并将其分解为可执行的任务。",
			},
			{
				Role:    llm.RoleUser,
				Content: prompt,
			},
		},
		Model:       ta.model,
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	resp, err := ta.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	// 解析 JSON 响应
	content := resp.Choices[0].Message.Content
	return ta.parseAnalysisResponse(content)
}

// parseAnalysisResponse 解析分析响应
func (ta *TaskAnalyzer) parseAnalysisResponse(content string) (*TaskAnalysis, error) {
	// 提取 JSON 部分
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")

	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}

	jsonStr := content[jsonStart : jsonEnd+1]

	// 解析
	var rawAnalysis struct {
		TaskType          string                `json:"task_type"`
		Description       string                `json:"description"`
		Skills            []string              `json:"skills"`
		Complexity        string                `json:"complexity"`
		EstimatedTimeMin  int                   `json:"estimated_time_minutes"`
		Subtasks          []SubtaskSpec         `json:"subtasks"`
		Dependencies      []string              `json:"dependencies"`
		RecommendedTeam   string                `json:"recommended_team"`
		Priority          string                `json:"priority"`
		Metadata          map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &rawAnalysis); err != nil {
		return nil, fmt.Errorf("failed to parse analysis JSON: %w", err)
	}

	// 转换为 TaskAnalysis
	analysis := &TaskAnalysis{
		TaskType:        ta.normalizeTaskType(rawAnalysis.TaskType),
		Description:     rawAnalysis.Description,
		Skills:          rawAnalysis.Skills,
		Complexity:      ta.normalizeComplexity(rawAnalysis.Complexity),
		EstimatedTime:   time.Duration(rawAnalysis.EstimatedTimeMin) * time.Minute,
		Subtasks:        rawAnalysis.Subtasks,
		Dependencies:    rawAnalysis.Dependencies,
		RecommendedTeam: ta.normalizeTeamType(rawAnalysis.RecommendedTeam),
		Priority:        ta.normalizePriority(rawAnalysis.Priority),
		Metadata:        rawAnalysis.Metadata,
		AnalyzedAt:      time.Now(),
	}

	// 确保子任务有 ID
	for i := range analysis.Subtasks {
		if analysis.Subtasks[i].ID == "" {
			analysis.Subtasks[i].ID = uuid.New().String()
		}
	}

	return analysis, nil
}

// postProcess 后处理分析结果
func (ta *TaskAnalyzer) postProcess(analysis *TaskAnalysis, req *AnalysisRequest) *TaskAnalysis {
	// 如果没有指定优先级，根据复杂度推断
	if analysis.Priority == "" {
		switch analysis.Complexity {
		case "high":
			analysis.Priority = "high"
		case "medium":
			analysis.Priority = "medium"
		default:
			analysis.Priority = "low"
		}
	}

	// 如果没有推荐团队，根据任务类型推断
	if analysis.RecommendedTeam == "" {
		switch analysis.TaskType {
		case "coding", "testing":
			analysis.RecommendedTeam = "development"
		case "research":
			analysis.RecommendedTeam = "research"
		default:
			analysis.RecommendedTeam = "mixed"
		}
	}

	// 根据团队配置调整子任务顺序（如果可用）
	if req.TeamConfig != nil && len(req.TeamConfig.Members) > 0 {
		analysis = ta.optimizeForTeam(analysis, req.TeamConfig)
	}

	return analysis
}

// optimizeForTeam 根据团队配置优化任务
func (ta *TaskAnalyzer) optimizeForTeam(analysis *TaskAnalysis, config *TeamConfig) *TaskAnalysis {
	// 这里可以添加更复杂的优化逻辑
	// 例如: 根据成员可用性调整子任务顺序
	return analysis
}

// normalizeTaskType 标准化任务类型
func (ta *TaskAnalyzer) normalizeTaskType(taskType string) string {
	switch strings.ToLower(taskType) {
	case "coding", "development", "programming", "code":
		return "coding"
	case "research", "investigation", "study":
		return "research"
	case "analysis", "analytics", "data":
		return "analysis"
	case "testing", "test", "qa":
		return "testing"
	case "documentation", "docs", "documenting":
		return "documentation"
	case "mixed", "hybrid", "multiple":
		return "mixed"
	default:
		return "mixed"
	}
}

// normalizeComplexity 标准化复杂度
func (ta *TaskAnalyzer) normalizeComplexity(complexity string) string {
	switch strings.ToLower(complexity) {
	case "low", "simple", "easy":
		return "low"
	case "medium", "moderate", "intermediate":
		return "medium"
	case "high", "complex", "hard", "difficult":
		return "high"
	default:
		return "medium"
	}
}

// normalizeTeamType 标准化团队类型
func (ta *TaskAnalyzer) normalizeTeamType(teamType string) string {
	switch strings.ToLower(teamType) {
	case "development", "dev", "coding":
		return "development"
	case "research", "researching":
		return "research"
	case "mixed", "hybrid", "cross-functional":
		return "mixed"
	default:
		return "mixed"
	}
}

// normalizePriority 标准化优先级
func (ta *TaskAnalyzer) normalizePriority(priority string) string {
	switch strings.ToLower(priority) {
	case "low", "minor":
		return "low"
	case "medium", "normal", "moderate":
		return "medium"
	case "high", "important", "major":
		return "high"
	case "urgent", "critical", "asap":
		return "urgent"
	default:
		return "medium"
	}
}

// generateCacheKey 生成缓存键
func (ta *TaskAnalyzer) generateCacheKey(req *AnalysisRequest) string {
	// 简单的哈希，实际应用可以使用更好的哈希函数
	return fmt.Sprintf("%s:%s", req.UserMessage, time.Now().Truncate(time.Hour))
}

// getCachedAnalysis 获取缓存的分析结果
func (ta *TaskAnalyzer) getCachedAnalysis(key string) (*TaskAnalysis, bool) {
	if analysis, ok := ta.analysisCache[key]; ok {
		if time.Since(analysis.AnalyzedAt) < ta.cacheTTL {
			return analysis, true
		}
		// 缓存过期，删除
		delete(ta.analysisCache, key)
	}
	return nil, false
}

// cacheAnalysis 缓存分析结果
func (ta *TaskAnalyzer) cacheAnalysis(key string, analysis *TaskAnalysis) {
	// 限制缓存大小
	if len(ta.analysisCache) > 100 {
		// 简单的 LRU: 删除最老的
		var oldestKey string
		var oldestTime time.Time
		for k, v := range ta.analysisCache {
			if oldestKey == "" || v.AnalyzedAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.AnalyzedAt
			}
		}
		if oldestKey != "" {
			delete(ta.analysisCache, oldestKey)
		}
	}
	ta.analysisCache[key] = analysis
}

// ClearCache 清空缓存
func (ta *TaskAnalyzer) ClearCache() {
	ta.analysisCache = make(map[string]*TaskAnalysis)
}

// GetTaskTypes 获取支持的任务类型
func (ta *TaskAnalyzer) GetTaskTypes() []string {
	return []string{"coding", "research", "analysis", "testing", "documentation", "mixed"}
}

// GetComplexityLevels 获取支持的复杂度级别
func (ta *TaskAnalyzer) GetComplexityLevels() []string {
	return []string{"low", "medium", "high"}
}

// ConvertToModel 将分析结果转换为任务模型
func (ta *TaskAnalyzer) ConvertToModel(analysis *TaskAnalysis, teamID, createdBy string) *models.Task {
	now := time.Now()

	task := &models.Task{
		ID:          uuid.New().String(),
		TeamID:      teamID,
		Title:       analysis.Description,
		Description: analysis.Description,
		Type:        analysis.TaskType,
		Priority:    analysis.Priority,
		Status:      "pending",
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
		Context: models.TaskContext{
			Requirements: analysis.Dependencies,
		},
	}

	// 设置复杂度
	switch analysis.Complexity {
	case "low":
		task.Complexity = 3
	case "medium":
		task.Complexity = 5
	case "high":
		task.Complexity = 8
	}

	// 设置预估时间
	task.Estimated = int(analysis.EstimatedTime.Minutes())

	// 转换子任务
	task.Subtasks = make([]models.Subtask, len(analysis.Subtasks))
	subtaskEstimate := int(analysis.EstimatedTime.Minutes()) / len(analysis.Subtasks)
	for i, st := range analysis.Subtasks {
		task.Subtasks[i] = models.Subtask{
			ID:          st.ID,
			Title:       st.Title,
			Description: st.Description,
			Type:        st.Type,
			Status:      "pending",
			Order:       st.Order,
			Estimated:   subtaskEstimate,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	return task
}

// EstimateConfidence 估算分析置信度
func (ta *TaskAnalyzer) EstimateConfidence(analysis *TaskAnalysis) float64 {
	confidence := 0.5

	// 有子任务提高置信度
	if len(analysis.Subtasks) > 0 {
		confidence += 0.2
	}

	// 有技能要求提高置信度
	if len(analysis.Skills) > 0 {
		confidence += 0.15
	}

	// 有明确的依赖项提高置信度
	if len(analysis.Dependencies) > 0 {
		confidence += 0.15
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}
