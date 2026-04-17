package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// TaskService 任务服务
type TaskService struct {
	db        *gorm.DB
	llmClient llm.LLMClient
	mu        sync.RWMutex
	templates map[string]*models.TaskDecomposition
}

// NewTaskService 创建任务服务实例
func NewTaskService(db *gorm.DB, llmClient llm.LLMClient) *TaskService {
	service := &TaskService{
		db:        db,
		llmClient: llmClient,
		templates: make(map[string]*models.TaskDecomposition),
	}
	service.loadBuiltinTemplates()
	return service
}

// ============ 任务分解 ============

// DecomposeTaskRequest 任务分解请求
type DecomposeTaskRequest struct {
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Priority    string                 `json:"priority"`
	Context     models.TaskContext     `json:"context,omitempty"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
	TeamID      string                 `json:"team_id,omitempty"`
	CreatedBy   string                 `json:"created_by,omitempty"`
}

// DecomposeTaskResult 任务分解结果
type DecomposeTaskResult struct {
	Task       *models.Task           `json:"task"`
	Subtasks   []models.Subtask       `json:"subtasks"`
	Assignments []models.TaskAssignment `json:"assignments"`
	Reasoning  string                 `json:"reasoning,omitempty"`
}

// DecomposeTask 分解任务
func (s *TaskService) DecomposeTask(ctx context.Context, req DecomposeTaskRequest) (*DecomposeTaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 分析任务复杂度和类型
	analysis, err := s.analyzeTask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("task analysis failed: %w", err)
	}

	// 2. 选择合适的分解模板
	template := s.selectTemplate(req.Type, analysis)

	// 3. 生成子任务
	subtasks, err := s.generateSubtasks(ctx, req, template, analysis)
	if err != nil {
		return nil, fmt.Errorf("subtask generation failed: %w", err)
	}

	// 4. 计算依赖关系
	s.calculateDependencies(subtasks)

	// 5. 创建主任务
	task := &models.Task{
		ID:          uuid.New().String(),
		TeamID:      req.TeamID,
		Title:       req.Title,
		Description: req.Description,
		Type:        req.Type,
		Priority:    req.Priority,
		Status:      "pending",
		Complexity:  analysis.Complexity,
		Estimated:   analysis.EstimatedMinutes,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Context:     req.Context,
	}

	// 6. 智能分配子任务
	members, err := s.getAvailableMembers(req.TeamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}

	assignments := s.assignSubtasks(subtasks, members, analysis)

	// 7. 保存到数据库
	if err := s.saveTaskWithSubtasks(task, subtasks); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	// 8. 保存分配记录
	for i := range assignments {
		if err := s.db.Create(&assignments[i]).Error; err != nil {
			return nil, fmt.Errorf("failed to save assignment: %w", err)
		}
	}

	return &DecomposeTaskResult{
		Task:       task,
		Subtasks:   subtasks,
		Assignments: assignments,
		Reasoning:  analysis.Reasoning,
	}, nil
}

// TaskAnalysis 任务分析结果
type TaskAnalysis struct {
	Complexity        int      `json:"complexity"`
	EstimatedMinutes  int      `json:"estimated_minutes"`
	RequiredSkills    []string `json:"required_skills"`
	SuggestedSubtasks []string `json:"suggested_subtasks"`
	Dependencies      []string `json:"dependencies"`
	Risks             []string `json:"risks"`
	Reasoning         string   `json:"reasoning"`
}

// analyzeTask 分析任务
func (s *TaskService) analyzeTask(ctx context.Context, req DecomposeTaskRequest) (*TaskAnalysis, error) {
	// 构建分析提示
	prompt := s.buildAnalysisPrompt(req)

	// 调用 LLM 分析
	llmReq := &llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "你是一个任务分析专家。分析给定的任务，评估其复杂度、所需技能和风险。",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Model:       "glm-5",
		Temperature: 0.3,
	}

	resp, err := s.llmClient.Chat(ctx, llmReq)
	if err != nil {
		// 如果 LLM 调用失败，使用规则回退
		return s.fallbackAnalysis(req), nil
	}

	// 解析 LLM 响应
	return s.parseAnalysisResponse(resp.Choices[0].Message.Content)
}

// buildAnalysisPrompt 构建分析提示
func (s *TaskService) buildAnalysisPrompt(req DecomposeTaskRequest) string {
	return fmt.Sprintf(`请分析以下任务并提供结构化的分析结果：

任务标题: %s
任务描述: %s
任务类型: %s
优先级: %s

请以 JSON 格式返回分析结果，包含以下字段：
{
  "complexity": 1-10的复杂度评分,
  "estimated_minutes": 预估完成时间（分钟）,
  "required_skills": 所需技能列表,
  "suggested_subtasks": 建议的子任务列表,
  "dependencies": 依赖项列表,
  "risks": 潜在风险列表,
  "reasoning": 分析推理说明
}`, req.Title, req.Description, req.Type, req.Priority)
}

// fallbackAnalysis 回退分析（当LLM不可用时）
func (s *TaskService) fallbackAnalysis(req DecomposeTaskRequest) *TaskAnalysis {
	// 基于规则的分析
	complexity := 5
	estimated := 60

	// 根据描述长度调整
	descLen := len(req.Description)
	if descLen > 500 {
		complexity = 8
		estimated = 180
	} else if descLen > 200 {
		complexity = 6
		estimated = 120
	} else if descLen < 50 {
		complexity = 2
		estimated = 30
	}

	// 根据任务类型调整
	switch req.Type {
	case "coding":
		complexity += 1
		estimated = int(float64(estimated) * 1.5)
	case "research":
		complexity += 2
		estimated = int(float64(estimated) * 2.0)
	case "testing":
		complexity -= 1
		estimated = int(float64(estimated) * 0.8)
	}

	skills := []string{"general"}
	switch req.Type {
	case "coding":
		skills = []string{"programming", "code-review", "testing"}
	case "research":
		skills = []string{"analysis", "documentation", "critical-thinking"}
	case "plan":
		skills = []string{"planning", "architecture", "design"}
	}

	return &TaskAnalysis{
		Complexity:        min(complexity, 10),
		EstimatedMinutes:  estimated,
		RequiredSkills:    skills,
		SuggestedSubtasks: []string{"analysis", "implementation", "testing"},
		Dependencies:      []string{},
		Risks:             []string{"time_estimation_uncertainty"},
		Reasoning:         "基于规则的分析（LLM不可用）",
	}
}

// parseAnalysisResponse 解析分析响应
func (s *TaskService) parseAnalysisResponse(content string) (*TaskAnalysis, error) {
	// 尝试提取 JSON
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")

	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}

	jsonStr := content[jsonStart : jsonEnd+1]
	var analysis TaskAnalysis
	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse analysis JSON: %w", err)
	}

	return &analysis, nil
}

// selectTemplate 选择分解模板
func (s *TaskService) selectTemplate(taskType string, analysis *TaskAnalysis) *models.TaskDecomposition {
	// 首先尝试找到匹配类型的模板
	if template, ok := s.templates[taskType]; ok {
		return template
	}

	// 使用通用模板
	if template, ok := s.templates["general"]; ok {
		return template
	}

	// 返回空模板（将使用 LLM 生成）
	return &models.TaskDecomposition{
		Template: []models.SubtaskTemplate{},
		Variables: []models.TaskTemplateVariable{},
	}
}

// generateSubtasks 生成子任务
func (s *TaskService) generateSubtasks(ctx context.Context, req DecomposeTaskRequest, template *models.TaskDecomposition, analysis *TaskAnalysis) ([]models.Subtask, error) {
	var subtasks []models.Subtask

	// 如果模板有预设步骤，使用模板
	if len(template.Template) > 0 {
		for i, tmpl := range template.Template {
			subtask := models.Subtask{
				ID:           uuid.New().String(),
				Title:        s.applyVariables(tmpl.Title, req.Variables),
				Description:  s.applyVariables(tmpl.Description, req.Variables),
				Type:         tmpl.Type,
				Status:       "pending",
				Order:        i,
				Estimated:    30,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			subtasks = append(subtasks, subtask)
		}
	} else {
		// 使用 LLM 生成子任务
		generated, err := s.generateSubtasksWithLLM(ctx, req, analysis)
		if err != nil {
			// 回退到默认分解
			generated = s.getDefaultSubtasks(req, analysis)
		}
		subtasks = generated
	}

	return subtasks, nil
}

// generateSubtasksWithLLM 使用 LLM 生成子任务
func (s *TaskService) generateSubtasksWithLLM(ctx context.Context, req DecomposeTaskRequest, analysis *TaskAnalysis) ([]models.Subtask, error) {
	prompt := fmt.Sprintf(`请将以下任务分解为具体的子任务：

任务标题: %s
任务描述: %s
任务类型: %s
复杂度: %d

请以 JSON 格式返回子任务列表：
[
  {
    "title": "子任务标题",
    "description": "详细描述",
    "type": "coding|research|plan|testing|review",
    "order": 顺序号,
    "estimated": 预估分钟数
  }
]`, req.Title, req.Description, req.Type, analysis.Complexity)

	llmReq := &llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "你是一个任务分解专家。将大任务分解为可执行的小任务。",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Model:       "glm-5",
		Temperature: 0.5,
	}

	resp, err := s.llmClient.Chat(ctx, llmReq)
	if err != nil {
		return nil, err
	}

	var subtaskData []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Order       int    `json:"order"`
		Estimated   int    `json:"estimated"`
	}

	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &subtaskData); err != nil {
		return nil, err
	}

	subtasks := make([]models.Subtask, len(subtaskData))
	for i, st := range subtaskData {
		subtasks[i] = models.Subtask{
			ID:          uuid.New().String(),
			Title:       st.Title,
			Description: st.Description,
			Type:        st.Type,
			Status:      "pending",
			Order:       st.Order,
			Estimated:   st.Estimated,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
	}

	return subtasks, nil
}

// getDefaultSubtasks 获取默认子任务
func (s *TaskService) getDefaultSubtasks(req DecomposeTaskRequest, analysis *TaskAnalysis) []models.Subtask {
	baseSubtasks := []models.Subtask{
		{
			ID:          uuid.New().String(),
			Title:       "任务分析",
			Description: "分析任务需求和约束条件",
			Type:        "research",
			Status:      "pending",
			Order:       0,
			Estimated:   analysis.EstimatedMinutes / 10,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          uuid.New().String(),
			Title:       "方案设计",
			Description: "设计实现方案",
			Type:        "plan",
			Status:      "pending",
			Order:       1,
			Estimated:   analysis.EstimatedMinutes / 5,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          uuid.New().String(),
			Title:       "实施",
			Description: "执行任务实现",
			Type:        req.Type,
			Status:      "pending",
			Order:       2,
			Estimated:   analysis.EstimatedMinutes / 2,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          uuid.New().String(),
			Title:       "测试验证",
			Description: "测试和验证结果",
			Type:        "testing",
			Status:      "pending",
			Order:       3,
			Estimated:   analysis.EstimatedMinutes / 5,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}

	return baseSubtasks
}

// applyVariables 应用变量替换
func (s *TaskService) applyVariables(text string, variables map[string]interface{}) string {
	result := text
	for k, v := range variables {
		placeholder := fmt.Sprintf("{{.%s}}", k)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", v))
	}
	return result
}

// calculateDependencies 计算依赖关系
func (s *TaskService) calculateDependencies(subtasks []models.Subtask) {
	for i := range subtasks {
		// 每个子任务依赖于前面的子任务
		for j := 0; j < i; j++ {
			// 只添加必要的依赖（相邻任务）
			if j == i-1 {
				subtasks[i].Dependencies = append(subtasks[i].Dependencies, models.SubtaskDependency{
					ID:        uuid.New().String(),
					SubtaskID: subtasks[i].ID,
					DependsOn: subtasks[j].ID,
					Type:      "after",
					CreatedAt: time.Now(),
				})
			}
		}
	}
}

// ============ 智能分配 ============

// AssignmentScore 分配评分
type AssignmentScore struct {
	MemberID    string
	MemberName  string
	Score       float64
	Reasoning   []string
	Capability  float64
	Availability float64
	Experience  float64
}

// assignSubtasks 分配子任务
func (s *TaskService) assignSubtasks(subtasks []models.Subtask, members []models.TeamMember, analysis *TaskAnalysis) []models.TaskAssignment {
	var assignments []models.TaskAssignment

	for i := range subtasks {
		subtask := &subtasks[i]

		// 跳过已分配的任务
		if subtask.AssignedTo != "" {
			continue
		}

		// 计算每个成员的适配度
		scores := s.calculateAssignmentScores(subtask, members, analysis)

		// 选择最佳成员
		if len(scores) > 0 {
			best := scores[0]
			assignment := models.TaskAssignment{
				ID:         uuid.New().String(),
				TaskID:     "", // 将在保存时设置
				SubtaskID:  subtask.ID,
				AssignedTo: best.MemberID,
				AssignedBy: "system",
				Score:      best.Score,
				Status:     "pending",
				AssignedAt: time.Now(),
				Reason:     strings.Join(best.Reasoning, "; "),
			}

			// 更新子任务分配
			subtask.AssignedTo = best.MemberID
			subtask.Status = "assigned"

			// 更新成员负载
			for j := range members {
				if members[j].ID == best.MemberID {
					members[j].CurrentLoad++
					break
				}
			}

			assignments = append(assignments, assignment)
		}
	}

	return assignments
}

// calculateAssignmentScores 计算分配评分
func (s *TaskService) calculateAssignmentScores(subtask *models.Subtask, members []models.TeamMember, analysis *TaskAnalysis) []AssignmentScore {
	var scores []AssignmentScore

	for _, member := range members {
		// 检查成员是否可用
		if member.Availability == "offline" {
			continue
		}

		// 检查负载
		if member.CurrentLoad >= member.MaxLoad {
			continue
		}

		score := AssignmentScore{
			MemberID:   member.ID,
			MemberName: member.Name,
		}

		// 1. 能力匹配 (40%)
		capabilityScore := s.calculateCapabilityScore(subtask, &member)
		score.Capability = capabilityScore

		// 2. 可用性 (30%)
		availabilityScore := s.calculateAvailabilityScore(&member)
		score.Availability = availabilityScore

		// 3. 经验匹配 (30%)
		experienceScore := s.calculateExperienceScore(subtask, &member)
		score.Experience = experienceScore

		// 总分
		score.Score = capabilityScore*0.4 + availabilityScore*0.3 + experienceScore*0.3

		// 生成推理说明
		score.Reasoning = s.generateAssignmentReasoning(subtask, &member, score)

		scores = append(scores, score)
	}

	// 按分数降序排序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores
}

// calculateCapabilityScore 计算能力匹配分数
func (s *TaskService) calculateCapabilityScore(subtask *models.Subtask, member *models.TeamMember) float64 {
	taskType := subtask.Type

	// 查找匹配的能力
	for _, cap := range member.Capabilities {
		if strings.EqualFold(cap.Name, taskType) || strings.EqualFold(cap.Name, subtask.Title) {
			return cap.Level
		}
	}

	// 检查专长
	for _, spec := range member.Specialization {
		if strings.Contains(strings.ToLower(taskType), strings.ToLower(spec)) ||
		   strings.Contains(strings.ToLower(subtask.Title), strings.ToLower(spec)) {
			return 0.8
		}
	}

	// 检查角色匹配
	switch member.Role {
	case "lead":
		if taskType == "plan" || taskType == "review" {
			return 0.9
		}
	case "developer":
		if taskType == "coding" || taskType == "testing" {
			return 0.85
		}
	case "reviewer":
		if taskType == "review" || taskType == "testing" {
			return 0.9
		}
	case "researcher":
		if taskType == "research" {
			return 0.9
		}
	case "planner":
		if taskType == "plan" {
			return 0.9
		}
	}

	// 默认分数
	return 0.5
}

// calculateAvailabilityScore 计算可用性分数
func (s *TaskService) calculateAvailabilityScore(member *models.TeamMember) float64 {
	switch member.Availability {
	case "available":
		// 根据当前负载调整
		loadRatio := float64(member.CurrentLoad) / float64(member.MaxLoad)
		return 1.0 - loadRatio*0.5
	case "busy":
		return 0.3
	default:
		return 0.0
	}
}

// calculateExperienceScore 计算经验分数
func (s *TaskService) calculateExperienceScore(subtask *models.Subtask, member *models.TeamMember) float64 {
	if member.Experience == nil {
		return 0.5
	}

	// 查找相关经验
	taskType := strings.ToLower(subtask.Type)
	for skill, years := range member.Experience {
		if strings.Contains(strings.ToLower(skill), taskType) ||
		   strings.Contains(taskType, strings.ToLower(skill)) {
			// 经验越多分数越高，但有上限
			return math.Min(float64(years)/10.0+0.5, 1.0)
		}
	}

	// 默认中等分数
	return 0.5
}

// generateAssignmentReasoning 生成分配推理
func (s *TaskService) generateAssignmentReasoning(subtask *models.Subtask, member *models.TeamMember, score AssignmentScore) []string {
	var reasoning []string

	if score.Capability > 0.8 {
		reasoning = append(reasoning, fmt.Sprintf("高能力匹配 (%.2f)", score.Capability))
	} else if score.Capability < 0.5 {
		reasoning = append(reasoning, fmt.Sprintf("能力匹配较低 (%.2f)", score.Capability))
	}

	if score.Availability > 0.8 {
		reasoning = append(reasoning, "高可用性")
	} else if score.Availability < 0.5 {
		reasoning = append(reasoning, "可用性有限")
	}

	if score.Experience > 0.7 {
		reasoning = append(reasoning, "相关经验丰富")
	}

	if len(member.Specialization) > 0 {
		reasoning = append(reasoning, fmt.Sprintf("专长: %s", strings.Join(member.Specialization, ", ")))
	}

	if len(reasoning) == 0 {
		reasoning = append(reasoning, "基于综合评估")
	}

	return reasoning
}

// getAvailableMembers 获取可用成员
func (s *TaskService) getAvailableMembers(teamID string) ([]models.TeamMember, error) {
	var members []models.TeamMember
	err := s.db.Where("team_id = ? AND availability != ?", teamID, "offline").Find(&members).Error
	return members, err
}

// ============ 任务管理 ============

// CreateTask 创建任务
func (s *TaskService) CreateTask(task *models.Task) error {
	return s.db.Create(task).Error
}

// GetTask 获取任务
func (s *TaskService) GetTask(id string) (*models.Task, error) {
	var task models.Task
	err := s.db.Preload("Subtasks").Preload("Dependencies").First(&task, id).Error
	return &task, err
}

// UpdateTask 更新任务
func (s *TaskService) UpdateTask(task *models.Task) error {
	task.UpdatedAt = time.Now()
	return s.db.Save(task).Error
}

// UpdateTaskStatus 更新任务状态
func (s *TaskService) UpdateTaskStatus(taskID, status, updatedBy, reason string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var task models.Task
		if err := tx.First(&task, taskID).Error; err != nil {
			return err
		}

		oldStatus := task.Status
		task.Status = status
		task.UpdatedAt = time.Now()

		if status == "in_progress" && task.StartedAt == nil {
			now := time.Now()
			task.StartedAt = &now
		} else if status == "completed" {
			now := time.Now()
			task.CompletedAt = &now
		} else if status == "failed" {
			task.RetryCount++
			if task.RetryCount >= task.MaxRetries {
				// 达到最大重试次数
			}
		}

		if err := tx.Save(&task).Error; err != nil {
			return err
		}

		// 记录状态变更
		statusUpdate := models.TaskStatusUpdate{
			ID:        uuid.New().String(),
			TaskID:    taskID,
			OldStatus: oldStatus,
			NewStatus: status,
			UpdatedBy: updatedBy,
			Reason:    reason,
			CreatedAt: time.Now(),
		}
		return tx.Create(&statusUpdate).Error
	})
}

// ListTasks 列出任务
func (s *TaskService) ListTasks(filter TaskFilter) ([]models.Task, int64, error) {
	var tasks []models.Task
	var total int64

	query := s.db.Model(&models.Task{})

	if filter.TeamID != "" {
		query = query.Where("team_id = ?", filter.TeamID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	if filter.Priority != "" {
		query = query.Where("priority = ?", filter.Priority)
	}
	if filter.AssignedTo != "" {
		query = query.Where("assigned_to = ?", filter.AssignedTo)
	}

	query.Count(&total)

	if filter.Page > 0 && filter.PageSize > 0 {
		offset := (filter.Page - 1) * filter.PageSize
		query = query.Offset(offset).Limit(filter.PageSize)
	}

	query.Order("created_at DESC").Find(&tasks)

	return tasks, total, nil
}

// TaskFilter 任务过滤器
type TaskFilter struct {
	TeamID     string
	Status     string
	Type       string
	Priority   string
	AssignedTo string
	Page       int
	PageSize   int
}

// DeleteTask 删除任务
func (s *TaskService) DeleteTask(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 删除相关记录
		tx.Where("task_id = ?", id).Delete(&models.TaskDependency{})
		tx.Where("task_id = ?", id).Delete(&models.TaskAssignment{})
		tx.Where("task_id = ?", id).Delete(&models.Subtask{})
		tx.Where("task_id = ?", id).Delete(&models.TaskProgress{})

		return tx.Where("id = ?", id).Delete(&models.Task{}).Error
	})
}

// ============ 子任务管理 ============

// UpdateSubtaskStatus 更新子任务状态
func (s *TaskService) UpdateSubtaskStatus(subtaskID, status string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var subtask models.Subtask
		if err := tx.First(&subtask, subtaskID).Error; err != nil {
			return err
		}

		subtask.Status = status
		subtask.UpdatedAt = time.Now()

		if status == "in_progress" && subtask.StartedAt == nil {
			now := time.Now()
			subtask.StartedAt = &now
		} else if status == "completed" {
			now := time.Now()
			subtask.CompletedAt = &now
			if subtask.StartedAt != nil {
				subtask.Actual = int(now.Sub(*subtask.StartedAt).Minutes())
			}
		}

		return tx.Save(&subtask).Error
	})
}

// ============ 进度跟踪 ============

// UpdateProgress 更新任务进度
func (s *TaskService) UpdateProgress(taskID, subtaskID, stage string, percent int, message string) error {
	progress := models.TaskProgress{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		SubtaskID: subtaskID,
		Stage:     stage,
		Percent:   percent,
		Message:   message,
		CreatedAt: time.Now(),
	}
	return s.db.Create(&progress).Error
}

// GetTaskProgress 获取任务进度
func (s *TaskService) GetTaskProgress(taskID string) ([]models.TaskProgress, error) {
	var progresses []models.TaskProgress
	err := s.db.Where("task_id = ?", taskID).Order("created_at DESC").Find(&progresses).Error
	return progresses, err
}

// ============ 团队成员管理 ============

// CreateMember 创建团队成员
func (s *TaskService) CreateMember(member *models.TeamMember) error {
	return s.db.Create(member).Error
}

// GetMember 获取团队成员
func (s *TaskService) GetMember(id string) (*models.TeamMember, error) {
	var member models.TeamMember
	err := s.db.First(&member, id).Error
	return &member, err
}

// ListMembers 列出团队成员
func (s *TaskService) ListMembers(teamID string) ([]models.TeamMember, error) {
	var members []models.TeamMember
	err := s.db.Where("team_id = ?", teamID).Find(&members).Error
	return members, err
}

// UpdateMember 更新团队成员
func (s *TaskService) UpdateMember(member *models.TeamMember) error {
	member.UpdatedAt = time.Now()
	return s.db.Save(member).Error
}

// UpdateMemberAvailability 更新成员可用性
func (s *TaskService) UpdateMemberAvailability(memberID, availability string) error {
	return s.db.Model(&models.TeamMember{}).
		Where("id = ?", memberID).
		Update("availability", availability).
		Error
}

// ============ 模板管理 ============

// loadBuiltinTemplates 加载内置模板
func (s *TaskService) loadBuiltinTemplates() {
	// 编码任务模板
	codingTemplate := &models.TaskDecomposition{
		ID:          "coding",
		Name:        "编码任务",
		Description: "软件开发任务的分解模板",
		TaskType:    "coding",
		Template: []models.SubtaskTemplate{
			{
				ID:          "code-1",
				Title:       "需求分析",
				Description: "分析功能需求和技术规范",
				Type:        "research",
				Order:       0,
				Required:    true,
				Default:     true,
			},
			{
				ID:          "code-2",
				Title:       "设计",
				Description: "设计技术方案和接口",
				Type:        "plan",
				Order:       1,
				Required:    true,
				Default:     true,
			},
			{
				ID:          "code-3",
				Title:       "实现",
				Description: "编写代码实现功能",
				Type:        "coding",
				Order:       2,
				Required:    true,
				Default:     true,
			},
			{
				ID:          "code-4",
				Title:       "单元测试",
				Description: "编写单元测试用例",
				Type:        "testing",
				Order:       3,
				Required:    true,
				Default:     true,
			},
			{
				ID:          "code-5",
				Title:       "代码审查",
				Description: "进行代码审查",
				Type:        "review",
				Order:       4,
				Required:    true,
				Default:     true,
			},
		},
	}

	// 研究任务模板
	researchTemplate := &models.TaskDecomposition{
		ID:          "research",
		Name:        "研究任务",
		Description: "研究分析任务的分解模板",
		TaskType:    "research",
		Template: []models.SubtaskTemplate{
			{
				ID:          "research-1",
				Title:       "资料收集",
				Description: "收集相关资料和文献",
				Type:        "research",
				Order:       0,
				Required:    true,
				Default:     true,
			},
			{
				ID:          "research-2",
				Title:       "分析研究",
				Description: "深入分析资料",
				Type:        "research",
				Order:       1,
				Required:    true,
				Default:     true,
			},
			{
				ID:          "research-3",
				Title:       "撰写报告",
				Description: "整理研究结果并撰写报告",
				Type:        "plan",
				Order:       2,
				Required:    true,
				Default:     true,
			},
		},
	}

	// 通用模板
	generalTemplate := &models.TaskDecomposition{
		ID:          "general",
		Name:        "通用任务",
		Description: "通用任务分解模板",
		TaskType:    "general",
		Template: []models.SubtaskTemplate{
			{
				ID:          "general-1",
				Title:       "任务分析",
				Description: "分析任务目标",
				Type:        "research",
				Order:       0,
				Required:    true,
				Default:     true,
			},
			{
				ID:          "general-2",
				Title:       "执行任务",
				Description: "执行具体任务",
				Type:        "coding",
				Order:       1,
				Required:    true,
				Default:     true,
			},
			{
				ID:          "general-3",
				Title:       "验证",
				Description: "验证任务结果",
				Type:        "testing",
				Order:       2,
				Required:    true,
				Default:     true,
			},
		},
	}

	s.templates[codingTemplate.ID] = codingTemplate
	s.templates[researchTemplate.ID] = researchTemplate
	s.templates[generalTemplate.ID] = generalTemplate
}

// ============ 辅助方法 ============

// saveTaskWithSubtasks 保存任务和子任务
func (s *TaskService) saveTaskWithSubtasks(task *models.Task, subtasks []models.Subtask) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 保存主任务
		if err := tx.Create(task).Error; err != nil {
			return err
		}

		// 保存子任务
		for i := range subtasks {
			subtasks[i].ParentTaskID = task.ID
			if err := tx.Create(&subtasks[i]).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// ============ 任务查询 API ============

// GetTaskOverview 获取任务概览
func (s *TaskService) GetTaskOverview(teamID string) (map[string]interface{}, error) {
	var stats struct {
		Total      int64
		Pending    int64
		Assigned   int64
		InProgress int64
		Completed  int64
		Failed     int64
	}

	s.db.Model(&models.Task{}).Where("team_id = ?", teamID).Count(&stats.Total)
	s.db.Model(&models.Task{}).Where("team_id = ? AND status = ?", teamID, "pending").Count(&stats.Pending)
	s.db.Model(&models.Task{}).Where("team_id = ? AND status = ?", teamID, "assigned").Count(&stats.Assigned)
	s.db.Model(&models.Task{}).Where("team_id = ? AND status = ?", teamID, "in_progress").Count(&stats.InProgress)
	s.db.Model(&models.Task{}).Where("team_id = ? AND status = ?", teamID, "completed").Count(&stats.Completed)
	s.db.Model(&models.Task{}).Where("team_id = ? AND status = ?", teamID, "failed").Count(&stats.Failed)

	// 获取成员负载
	var members []models.TeamMember
	s.db.Where("team_id = ?", teamID).Find(&members)

	memberLoad := make([]map[string]interface{}, len(members))
	for i, m := range members {
		memberLoad[i] = map[string]interface{}{
			"id":          m.ID,
			"name":        m.Name,
			"current_load": m.CurrentLoad,
			"max_load":    m.MaxLoad,
			"availability": m.Availability,
		}
	}

	return map[string]interface{}{
		"stats":       stats,
		"member_load": memberLoad,
	}, nil
}

// GetMemberTasks 获取成员的任务
func (s *TaskService) GetMemberTasks(memberID string) ([]models.Task, []models.Subtask, error) {
	// 获取直接分配的任务
	var tasks []models.Task
	s.db.Where("assigned_to = ?", memberID).Find(&tasks)

	// 获取分配的子任务
	var subtasks []models.Subtask
	s.db.Where("assigned_to = ?", memberID).Find(&subtasks)

	return tasks, subtasks, nil
}

// RetryFailedTask 重试失败的任务
func (s *TaskService) RetryFailedTask(taskID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var task models.Task
		if err := tx.First(&task, taskID).Error; err != nil {
			return err
		}

		if task.Status != "failed" {
			return fmt.Errorf("task is not in failed status")
		}

		if task.RetryCount >= task.MaxRetries {
			return fmt.Errorf("max retry count reached")
		}

		task.Status = "pending"
		task.RetryCount++
		task.UpdatedAt = time.Now()

		return tx.Save(&task).Error
	})
}

// GenerateTaskSummary 生成任务总结
func (s *TaskService) GenerateTaskSummary(ctx context.Context, taskID string) (*models.TaskSummary, error) {
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	// 使用 LLM 生成总结
	prompt := fmt.Sprintf(`请为以下任务生成总结：

任务标题: %s
任务描述: %s
任务状态: %s
子任务数量: %d
已完成子任务: %d

请生成任务总结，包括：
1. 任务执行总结
2. 经验教训
3. 改进建议

以 JSON 格式返回：
{
  "summary": "总结内容",
  "lessons": "经验教训",
  "suggestions": "改进建议"
}`, task.Title, task.Description, task.Status, len(task.Subtasks), countCompletedSubtasks(task.Subtasks))

	llmReq := &llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "你是一个任务总结专家。分析任务执行情况并生成总结报告。",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Model:       "glm-5",
		Temperature: 0.5,
	}

	resp, err := s.llmClient.Chat(ctx, llmReq)
	if err != nil {
		return nil, err
	}

	var summaryData struct {
		Summary     string `json:"summary"`
		Lessons     string `json:"lessons"`
		Suggestions string `json:"suggestions"`
	}

	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &summaryData); err != nil {
		return nil, err
	}

	summary := &models.TaskSummary{
		ID:          uuid.New().String(),
		TaskID:      taskID,
		Summary:     summaryData.Summary,
		Lessons:     summaryData.Lessons,
		Suggestions: summaryData.Suggestions,
		GeneratedBy: "system",
		CreatedAt:   time.Now(),
	}

	s.db.Create(summary)

	return summary, nil
}

// countCompletedSubtasks 统计已完成的子任务
func countCompletedSubtasks(subtasks []models.Subtask) int {
	count := 0
	for _, st := range subtasks {
		if st.Status == "completed" {
			count++
		}
	}
	return count
}

// CancelTask 取消任务
func (s *TaskService) CancelTask(taskID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 取消主任务
		if err := tx.Model(&models.Task{}).Where("id = ?", taskID).Update("status", "cancelled").Error; err != nil {
			return err
		}

		// 取消所有子任务
		return tx.Model(&models.Subtask{}).Where("parent_task_id = ? AND status IN ?", taskID, []string{"pending", "assigned"}).Update("status", "cancelled").Error
	})
}

// GetTaskDependencies 获取任务依赖
func (s *TaskService) GetTaskDependencies(taskID string) ([]models.TaskDependency, error) {
	var deps []models.TaskDependency
	err := s.db.Where("task_id = ?", taskID).Find(&deps).Error
	return deps, err
}

// AddTaskDependency 添加任务依赖
func (s *TaskService) AddTaskDependency(taskID, dependsOn, depType string) error {
	dep := models.TaskDependency{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		DependsOn: dependsOn,
		Type:      depType,
		CreatedAt: time.Now(),
	}
	return s.db.Create(&dep).Error
}

// CanStartTask 检查任务是否可以开始
func (s *TaskService) CanStartTask(taskID string) (bool, []string, error) {
	deps, err := s.GetTaskDependencies(taskID)
	if err != nil {
		return false, nil, err
	}

	var blockedBy []string
	for _, dep := range deps {
		var depTask models.Task
		if err := s.db.First(&depTask, dep.DependsOn).Error; err != nil {
			continue
		}
		if depTask.Status != "completed" {
			blockedBy = append(blockedBy, depTask.Title)
		}
	}

	return len(blockedBy) == 0, blockedBy, nil
}

// ReassignTask 重新分配任务
func (s *TaskService) ReassignTask(taskID, newMemberID, reason string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 更新任务分配
		if err := tx.Model(&models.Task{}).Where("id = ?", taskID).Update("assigned_to", newMemberID).Error; err != nil {
			return err
		}

		// 创建新的分配记录
		assignment := models.TaskAssignment{
			ID:         uuid.New().String(),
			TaskID:     taskID,
			AssignedTo: newMemberID,
			AssignedBy: "system",
			Status:     "pending",
			AssignedAt: time.Now(),
			Reason:     reason,
		}
		return tx.Create(&assignment).Error
	})
}

// GetTemplate 获取分解模板
func (s *TaskService) GetTemplate(taskType string) (*models.TaskDecomposition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if template, ok := s.templates[taskType]; ok {
		return template, nil
	}

	return nil, fmt.Errorf("template not found for type: %s", taskType)
}

// ListTemplates 列出所有模板
func (s *TaskService) ListTemplates() []*models.TaskDecomposition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	templates := make([]*models.TaskDecomposition, 0, len(s.templates))
	for _, t := range s.templates {
		templates = append(templates, t)
	}

	return templates
}
