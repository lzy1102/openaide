package orchestration

import (
	"context"
	"fmt"
	"time"

	"openaide/backend/src/models"

	"gopkg.in/yaml.v3"
)

// TeamPlannerTask 团队规划任务分配（避免与 models.TaskAssignment 冲突）
type TeamPlannerTask struct {
	ID            string                 `json:"id" yaml:"id"`
	Title         string                 `json:"title" yaml:"title"`
	Description   string                 `json:"description" yaml:"description"`
	AssignedTo    string                 `json:"assigned_to" yaml:"assigned_to"`
	Dependencies  []string               `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Priority      string                 `json:"priority" yaml:"priority"`
	Estimated     time.Duration          `json:"estimated" yaml:"estimated"`
	Context       map[string]interface{} `json:"context,omitempty" yaml:"context,omitempty"`
}

// TeamPlannerAnalysis 团队规划器专用任务分析（避免与 task_analyzer.TaskAnalysis 冲突）
type TeamPlannerAnalysis struct {
	Goal         string                   `json:"goal" yaml:"goal"`
	Description  string                   `json:"description" yaml:"description"`
	Type         string                   `json:"type" yaml:"type"`
	Complexity   string                   `json:"complexity" yaml:"complexity"`
	Requirements []string                 `json:"requirements,omitempty" yaml:"requirements,omitempty"`
	Constraints  []string                 `json:"constraints,omitempty" yaml:"constraints,omitempty"`
	Deliverables []string                 `json:"deliverables,omitempty" yaml:"deliverables,omitempty"`
	Metadata     map[string]interface{}   `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// TeamPlanner 团队规划器 - 根据任务分析结果生成团队配置和执行计划
type TeamPlanner struct {
	llmService LLMClient
}

// LLMClient LLM 客户端接口
type LLMClient interface {
	Chat(ctx context.Context, messages []map[string]string, options map[string]interface{}) (map[string]interface{}, error)
}

// NewTeamPlanner 创建团队规划器
func NewTeamPlanner(llmService LLMClient) *TeamPlanner {
	return &TeamPlanner{
		llmService: llmService,
	}
}

// TeamPlan 团队规划输出结构
type TeamPlan struct {
	TeamName       string           `json:"team_name" yaml:"team_name"`
	Description    string           `json:"description" yaml:"description"`
	Roles          []AgentRole      `json:"roles" yaml:"roles"`
	Tasks          []TeamPlannerTask `json:"tasks" yaml:"tasks"`
	ExecutionOrder []string         `json:"execution_order" yaml:"execution_order"`
	EstimatedTime  time.Duration    `json:"estimated_time" yaml:"estimated_time"`
	Dependencies   map[string]string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
}

// AgentRole Agent 角色定义
type AgentRole struct {
	Name         string                 `json:"name" yaml:"name"`
	Role         string                 `json:"role" yaml:"role"` // architect/developer/researcher/reviewer/tester
	DisplayName  string                 `json:"display_name" yaml:"display_name"`
	Description  string                 `json:"description" yaml:"description"`
	Skills       []string               `json:"skills" yaml:"skills"`
	LLM          string                 `json:"llm" yaml:"llm"` // LLMModel
	LLMModel     string                 `json:"llm_model" yaml:"llm_model"` // alias for LLM
	SystemPrompt string                 `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
}

// DeprecatedTaskAssignment 已弃用 - 使用 TeamPlannerTask
type DeprecatedTaskAssignment = TeamPlannerTask

// TeamTemplate 团队模板
type TeamTemplate struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Roles       []TemplateRole         `yaml:"roles"`
	Config      map[string]interface{} `yaml:"config,omitempty"`
}

// TemplateRole 模板角色
type TemplateRole struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description"`
	Skills      []string `yaml:"skills"`
	LLM         string   `yaml:"llm"`
	Optional    bool     `yaml:"optional,omitempty"`
}

// 预定义团队模板
var teamTemplates = map[string]TeamTemplate{
	"coding_team": {
		Name:        "开发团队",
		Description: "适用于软件开发、代码重构、bug 修复等任务",
		Roles: []TemplateRole{
			{
				Name:        "architect",
				DisplayName: "架构师",
				Description: "负责系统架构设计、技术方案制定",
				Skills:      []string{"architecture", "design", "system-design", "technical-planning"},
				LLM:         "claude-opus",
			},
			{
				Name:        "developer",
				DisplayName: "开发者",
				Description: "负责代码编写、功能实现",
				Skills:      []string{"coding", "debugging", "implementation"},
				LLM:         "claude-sonnet",
			},
			{
				Name:        "reviewer",
				DisplayName: "代码审查员",
				Description: "负责代码审查、质量保证",
				Skills:      []string{"code-review", "testing", "quality-assurance"},
				LLM:         "claude-sonnet",
				Optional:    true,
			},
		},
	},
	"research_team": {
		Name:        "研究团队",
		Description: "适用于信息收集、数据分析、文档编写等任务",
		Roles: []TemplateRole{
			{
				Name:        "researcher",
				DisplayName: "研究员",
				Description: "负责信息收集、数据分析",
				Skills:      []string{"research", "analysis", "data-gathering"},
				LLM:         "claude-sonnet",
			},
			{
				Name:        "summarizer",
				DisplayName: "总结员",
				Description: "负责信息整理、文档编写",
				Skills:      []string{"summarization", "writing", "documentation"},
				LLM:         "claude-sonnet",
			},
		},
	},
	"creative_team": {
		Name:        "创意团队",
		Description: "适用于内容创作、设计、创意策划等任务",
		Roles: []TemplateRole{
			{
				Name:        "creator",
				DisplayName: "创作者",
				Description: "负责创意内容生成",
				Skills:      []string{"creative-writing", "ideation", "content-creation"},
				LLM:         "claude-opus",
			},
			{
				Name:        "editor",
				DisplayName: "编辑",
				Description: "负责内容编辑、优化",
				Skills:      []string{"editing", "proofreading", "refinement"},
				LLM:         "claude-sonnet",
			},
		},
	},
	"analysis_team": {
		Name:        "分析团队",
		Description: "适用于数据分析、问题诊断、决策支持等任务",
		Roles: []TemplateRole{
			{
				Name:        "analyst",
				DisplayName: "分析师",
				Description: "负责数据分析、趋势预测",
				Skills:      []string{"data-analysis", "statistics", "pattern-recognition"},
				LLM:         "claude-opus",
			},
			{
				Name:        "validator",
				DisplayName: "验证员",
				Description: "负责结论验证、交叉检查",
				Skills:      []string{"validation", "verification", "critical-thinking"},
				LLM:         "claude-sonnet",
			},
		},
	},
	"fullstack_team": {
		Name:        "全栈开发团队",
		Description: "适用于完整的 Web 应用开发",
		Roles: []TemplateRole{
			{
				Name:        "architect",
				DisplayName: "架构师",
				Description: "负责整体架构设计",
				Skills:      []string{"architecture", "design", "system-design"},
				LLM:         "claude-opus",
			},
			{
				Name:        "frontend",
				DisplayName: "前端开发",
				Description: "负责前端开发",
				Skills:      []string{"frontend", "ui", "ux", "javascript", "css"},
				LLM:         "claude-sonnet",
			},
			{
				Name:        "backend",
				DisplayName: "后端开发",
				Description: "负责后端开发",
				Skills:      []string{"backend", "api", "database", "server"},
				LLM:         "claude-sonnet",
			},
			{
				Name:        "reviewer",
				DisplayName: "代码审查员",
				Description: "负责代码审查和测试",
				Skills:      []string{"code-review", "testing", "qa"},
				LLM:         "claude-sonnet",
				Optional:    true,
			},
		},
	},
	"mixed_team": {
		Name:        "综合团队",
		Description: "适用于需要多角色协作的复杂任务",
		Roles: []TemplateRole{
			{
				Name:        "planner",
				DisplayName: "规划师",
				Description: "负责任务规划和协调",
				Skills:      []string{"planning", "coordination", "management"},
				LLM:         "claude-opus",
			},
			{
				Name:        "researcher",
				DisplayName: "研究员",
				Description: "负责信息收集和分析",
				Skills:      []string{"research", "analysis", "data-gathering"},
				LLM:         "claude-sonnet",
			},
			{
				Name:        "developer",
				DisplayName: "开发者",
				Description: "负责代码实现",
				Skills:      []string{"coding", "debugging", "implementation"},
				LLM:         "claude-sonnet",
			},
			{
				Name:        "reviewer",
				DisplayName: "审查员",
				Description: "负责质量审查",
				Skills:      []string{"review", "testing", "validation"},
				LLM:         "claude-sonnet",
			},
		},
	},
}

// PlanTeam 根据任务分析生成团队规划
func (p *TeamPlanner) PlanTeam(ctx context.Context, analysis *TeamPlannerAnalysis) (*TeamPlan, error) {
	// 1. 选择合适的团队模板
	templateName := p.selectTemplate(analysis)
	template, exists := teamTemplates[templateName]
	if !exists {
		return nil, fmt.Errorf("template not found: %s", templateName)
	}

	// 2. 分析任务复杂度，决定是否需要 LLM 增强
	if p.neLLMEnhancement(analysis) {
		return p.generatePlanWithLLM(ctx, analysis, &template)
	}

	// 3. 使用模板生成基础规划
	return p.generatePlanFromTemplate(analysis, &template)
}

// selectTemplate 根据任务类型选择模板
func (p *TeamPlanner) selectTemplate(analysis *TeamPlannerAnalysis) string {
	switch analysis.Type {
	case "coding", "development", "refactoring", "bugfix":
		return "coding_team"
	case "research", "analysis", "investigation":
		return "research_team"
	case "creative", "writing", "content":
		return "creative_team"
	case "data_analysis", "diagnostics":
		return "analysis_team"
	case "fullstack", "web_development":
		return "fullstack_team"
	default:
		return "coding_team" // 默认
	}
}

// neLLMEnhancement 判断是否需要 LLM 增强规划
func (p *TeamPlanner) neLLMEnhancement(analysis *TeamPlannerAnalysis) bool {
	// 高复杂度任务使用 LLM 生成定制化规划
	return analysis.Complexity == "high" || len(analysis.Requirements) > 5
}

// generatePlanFromTemplate 从模板生成基础规划
func (p *TeamPlanner) generatePlanFromTemplate(analysis *TeamPlannerAnalysis, template *TeamTemplate) (*TeamPlan, error) {
	plan := &TeamPlan{
		TeamName:    template.Name,
		Description: template.Description,
		Roles:       make([]AgentRole, 0, len(template.Roles)),
		Tasks:       p.generateBasicTasks(analysis, template),
	}

	// 转换模板角色为 Agent 角色
	for _, templateRole := range template.Roles {
		role := AgentRole{
			Name:        templateRole.Name,
			DisplayName: templateRole.DisplayName,
			Description: templateRole.Description,
			Skills:      templateRole.Skills,
			LLM:         templateRole.LLM,
		}
		plan.Roles = append(plan.Roles, role)
	}

	// 生成执行顺序
	plan.ExecutionOrder = p.generateExecutionOrder(plan.Roles, plan.Tasks)

	// 估算时间
	plan.EstimatedTime = p.estimateTime(analysis, plan.Tasks)

	return plan, nil
}

// generatePlanWithLLM 使用 LLM 生成定制化规划
func (p *TeamPlanner) generatePlanWithLLM(ctx context.Context, analysis *TeamPlannerAnalysis, template *TeamTemplate) (*TeamPlan, error) {
	prompt := p.buildPlanningPrompt(analysis, template)

	messages := []map[string]string{
		{
			"role":    "system",
			"content": "You are a team planning expert. Generate a detailed team plan in JSON format.",
		},
		{
			"role":    "user",
			"content": prompt,
		},
	}

	response, err := p.llmService.Chat(ctx, messages, map[string]interface{}{
		"temperature": 0.7,
		"response_format": map[string]interface{}{
			"type": "json_object",
		},
	})
	if err != nil {
		// LLM 调用失败，回退到模板生成
		return p.generatePlanFromTemplate(analysis, template)
	}

	// 解析 LLM 响应
	return p.parseLLMResponse(response)
}

// buildPlanningPrompt 构建规划提示词
func (p *TeamPlanner) buildPlanningPrompt(analysis *TeamPlannerAnalysis, template *TeamTemplate) string {
	return fmt.Sprintf(`Generate a team plan for the following task:

Task Goal: %s
Description: %s
Type: %s
Complexity: %s

Requirements:
%s

Constraints:
%s

Deliverables:
%s

Available Team Template: %s
Available Roles:
%s

Generate a JSON response with the following structure:
{
  "team_name": "string",
  "description": "string",
  "roles": [
    {
      "name": "string",
      "display_name": "string",
      "description": "string",
      "skills": ["string"],
      "llm": "string"
    }
  ],
  "tasks": [
    {
      "id": "string",
      "title": "string",
      "description": "string",
      "assigned_to": "string",
      "priority": "high|medium|low",
      "estimated": "duration string (e.g., '30m', '1h')"
    }
  ],
  "execution_order": ["task_id1", "task_id2"],
  "estimated_time": "duration string"
}`,
		analysis.Goal,
		analysis.Description,
		analysis.Type,
		analysis.Complexity,
		formatStringList(analysis.Requirements),
		formatStringList(analysis.Constraints),
		formatStringList(analysis.Deliverables),
		template.Name,
		formatTemplateRoles(template.Roles))
}

// generateBasicTasks 生成基础任务列表
func (p *TeamPlanner) generateBasicTasks(analysis *TeamPlannerAnalysis, template *TeamTemplate) []TeamPlannerTask {
	tasks := []TeamPlannerTask{}

	// 根据团队模板生成任务
	for i, role := range template.Roles {
		task := TeamPlannerTask{
			ID:          fmt.Sprintf("task_%d_%s", i+1, role.Name),
			Title:       fmt.Sprintf("%s task", role.DisplayName),
			Description: fmt.Sprintf("Execute %s responsibilities for: %s", role.DisplayName, analysis.Goal),
			AssignedTo:  role.Name,
			Priority:    "medium",
			Estimated:   30 * time.Minute,
		}
		tasks = append(tasks, task)
	}

	return tasks
}

// generateExecutionOrder 生成执行顺序
func (p *TeamPlanner) generateExecutionOrder(roles []AgentRole, tasks []TeamPlannerTask) []string {
	order := make([]string, 0, len(tasks))

	// 简单策略：按角色优先级排序
	// 架构师 -> 开发者 -> 审查员
	priorityOrder := map[string]int{
		"architect":   1,
		"researcher":  1,
		"analyst":     1,
		"creator":     1,
		"developer":   2,
		"frontend":    2,
		"backend":     2,
		"reviewer":    3,
		"summarizer":  3,
		"validator":   3,
		"editor":      3,
	}

	// 按优先级排序任务
	sortedTasks := make([]TeamPlannerTask, len(tasks))
	copy(sortedTasks, tasks)

	for i := 0; i < len(sortedTasks)-1; i++ {
		for j := i + 1; j < len(sortedTasks); j++ {
			priorityI := priorityOrder[sortedTasks[i].AssignedTo]
			priorityJ := priorityOrder[sortedTasks[j].AssignedTo]
			if priorityI > priorityJ {
				sortedTasks[i], sortedTasks[j] = sortedTasks[j], sortedTasks[i]
			}
		}
	}

	for _, task := range sortedTasks {
		order = append(order, task.ID)
	}

	return order
}

// estimateTime 估算总时间
func (p *TeamPlanner) estimateTime(analysis *TeamPlannerAnalysis, tasks []TeamPlannerTask) time.Duration {
	total := time.Duration(0)
	for _, task := range tasks {
		total += task.Estimated
	}

	// 根据复杂度调整
	switch analysis.Complexity {
	case "high":
		total = time.Duration(float64(total) * 1.5)
	case "low":
		total = time.Duration(float64(total) * 0.7)
	}

	return total
}

// parseLLMResponse 解析 LLM 响应
func (p *TeamPlanner) parseLLMResponse(response map[string]interface{}) (*TeamPlan, error) {
	// 从响应中提取内容
	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("invalid response format")
	}

	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	content := message["content"].(string)

	// 解析 JSON
	var plan TeamPlan
	if err := yaml.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}

	return &plan, nil
}

// formatStringList 格式化字符串列表
func formatStringList(list []string) string {
	if len(list) == 0 {
		return "  - None"
	}
	result := ""
	for _, item := range list {
		result += fmt.Sprintf("  - %s\n", item)
	}
	return result
}

// formatTemplateRoles 格式化模板角色
func formatTemplateRoles(roles []TemplateRole) string {
	result := ""
	for _, role := range roles {
		result += fmt.Sprintf("  - %s: %s (skills: %v)\n", role.Name, role.Description, role.Skills)
	}
	return result
}

// GetTemplate 获取团队模板
func (p *TeamPlanner) GetTemplate(name string) (*TeamTemplate, error) {
	template, exists := teamTemplates[name]
	if !exists {
		return nil, fmt.Errorf("template not found: %s", name)
	}
	return &template, nil
}

// ListTemplates 列出所有可用模板
func (p *TeamPlanner) ListTemplates() []string {
	names := make([]string, 0, len(teamTemplates))
	for name := range teamTemplates {
		names = append(names, name)
	}
	return names
}

// ExportTeamConfig 导出团队配置为 YAML
func (p *TeamPlanner) ExportTeamConfig(plan *TeamPlan) ([]byte, error) {
	return yaml.Marshal(plan)
}

// ImportTeamConfig 从 YAML 导入团队配置
func (p *TeamPlanner) ImportTeamConfig(data []byte) (*TeamPlan, error) {
	var plan TeamPlan
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse team config: %w", err)
	}
	return &plan, nil
}

// AssignTaskToAgent 将任务分配给特定 Agent
func (p *TeamPlanner) AssignTaskToAgent(plan *TeamPlan, taskID string, agentName string) error {
	for i, task := range plan.Tasks {
		if task.ID == taskID {
			plan.Tasks[i].AssignedTo = agentName
			return nil
		}
	}
	return fmt.Errorf("task not found: %s", taskID)
}

// AddTaskToPlan 向计划中添加任务
func (p *TeamPlanner) AddTaskToPlan(plan *TeamPlan, task *TeamPlannerTask) {
	plan.Tasks = append(plan.Tasks, *task)
}

// UpdateTaskDependencies 更新任务依赖关系
func (p *TeamPlanner) UpdateTaskDependencies(plan *TeamPlan, taskID string, dependencies []string) error {
	for i, task := range plan.Tasks {
		if task.ID == taskID {
			plan.Tasks[i].Dependencies = dependencies
			if plan.Dependencies == nil {
				plan.Dependencies = make(map[string]string)
			}
			plan.Dependencies[taskID] = fmt.Sprintf("%v", dependencies)
			return nil
		}
	}
	return fmt.Errorf("task not found: %s", taskID)
}

// ValidatePlan 验证团队规划的有效性
func (p *TeamPlanner) ValidatePlan(plan *TeamPlan) error {
	if plan.TeamName == "" {
		return fmt.Errorf("team_name is required")
	}

	if len(plan.Roles) == 0 {
		return fmt.Errorf("at least one role is required")
	}

	if len(plan.Tasks) == 0 {
		return fmt.Errorf("at least one task is required")
	}

	// 验证任务分配的有效性
	roleNames := make(map[string]bool)
	for _, role := range plan.Roles {
		roleNames[role.Name] = true
	}

	for _, task := range plan.Tasks {
		if task.AssignedTo != "" && !roleNames[task.AssignedTo] {
			return fmt.Errorf("task %s assigned to unknown role: %s", task.ID, task.AssignedTo)
		}
	}

	// 验证执行顺序
	taskIDs := make(map[string]bool)
	for _, task := range plan.Tasks {
		taskIDs[task.ID] = true
	}

	for _, taskID := range plan.ExecutionOrder {
		if !taskIDs[taskID] {
			return fmt.Errorf("execution_order references unknown task: %s", taskID)
		}
	}

	return nil
}

// ConvertToTeamConfig 将 TeamPlan 转换为团队配置模型
// 修复: 根据实际的 models.Team 和 models.TeamMember 结构进行转换
func (p *TeamPlanner) ConvertToTeamConfig(plan *TeamPlan) *models.TeamConfig {
	// 创建 Team
	team := models.Team{
		ID:          fmt.Sprintf("team_%d", time.Now().Unix()),
		Name:        plan.TeamName,
		Description: plan.Description,
		Enabled:     true,
		Config:      make(map[string]interface{}),
	}

	// 将角色信息存入 Config
	roles := []map[string]interface{}{}
	for _, role := range plan.Roles {
		roleMap := map[string]interface{}{
			"name":         role.Name,
			"display_name": role.DisplayName,
			"description":  role.Description,
			"skills":       role.Skills,
			"llm":          role.LLM,
		}
		roles = append(roles, roleMap)
	}
	team.Config["roles"] = roles
	team.Config["execution_order"] = plan.ExecutionOrder
	team.Config["estimated_time"] = plan.EstimatedTime.String()

	// 创建团队成员 (基于 models.TeamMember 结构)
	members := []models.TeamMember{}
	for i, role := range plan.Roles {
		// 转换技能为 Capability 列表
		capabilities := []models.Capability{}
		for _, skill := range role.Skills {
			capabilities = append(capabilities, models.Capability{
				Name:        skill,
				Level:       0.8,
				ConfirmedAt: time.Now(),
			})
		}

		member := models.TeamMember{
			ID:             fmt.Sprintf("member_%d", i+1),
			TeamID:         team.ID,
			Name:           role.DisplayName,
			Role:           role.Name,
			Capabilities:   capabilities,
			Availability:   "available",
			CurrentLoad:    0,
			MaxLoad:        3,
			Specialization: role.Skills,
			Experience:     make(map[string]int),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		members = append(members, member)
	}

	// 创建任务 (基于 models.Task 结构)
	tasks := []models.Task{}
	for _, taskAssignment := range plan.Tasks {
		// 转换依赖关系
		dependencies := []models.TaskDependency{}
		for i, depID := range taskAssignment.Dependencies {
			dependencies = append(dependencies, models.TaskDependency{
				ID:        fmt.Sprintf("dep_%d_%s", i, taskAssignment.ID),
				TaskID:    taskAssignment.ID,
				DependsOn: depID,
				Type:      "after",
				CreatedAt: time.Now(),
			})
		}

		// 转换上下文
		taskContext := models.TaskContext{
			Metadata: taskAssignment.Context,
		}

		task := models.Task{
			ID:          taskAssignment.ID,
			TeamID:      team.ID,
			Title:       taskAssignment.Title,
			Description: taskAssignment.Description,
			Type:        plan.TeamName,
			Status:      "pending",
			Priority:    taskAssignment.Priority,
			AssignedTo:  taskAssignment.AssignedTo,
			Complexity:  5, // 默认中等复杂度
			Estimated:   int(taskAssignment.Estimated.Minutes()),
			CreatedBy:   "system",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Dependencies: dependencies,
			Context:     taskContext,
		}
		tasks = append(tasks, task)
	}

	return &models.TeamConfig{
		Team:    team,
		Members: members,
		Tasks:   tasks,
		RetryConfig: models.RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    1000, // 1 second in milliseconds
			MaxDelay:        60000, // 60 seconds
			BackoffFactor:   2.0,
			RetryableErrors: []string{"timeout", "network_error"},
		},
		Version:    "1.0",
		ExportedAt: time.Now(),
	}
}
