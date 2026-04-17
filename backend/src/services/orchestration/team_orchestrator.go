package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"openaide/backend/src/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SimpleTaskCoordinator 简单任务协调器接口 - 避免循环导入
type SimpleTaskCoordinator interface {
	AssignTask(teamID, taskID, agentID string) error
	CompleteTask(teamID, taskID string, result map[string]interface{}) error
	FailTask(teamID, taskID string, errMsg string) error
}

// AgentExecutorInterface Agent 执行引擎接口 - 避免循环导入
type AgentExecutorInterface interface {
	Execute(ctx context.Context, req *AgentExecRequest) (*AgentExecResult, error)
}

// AgentExecRequest 任务执行请求
type AgentExecRequest struct {
	TaskID          string
	TaskTitle       string
	TaskDescription string
	AgentName       string
	AgentRole       string
	AgentPrompt     string
	ModelID         string
	Context         map[string]interface{}
	TeamGoal        string
}

// AgentExecResult 任务执行结果
type AgentExecResult struct {
	Success    bool
	Output     string
	Summary    string
	ToolCalls  int
	TokensUsed int
}

// TeamOrchestrator 团队编排器 - 负责创建团队、分配任务、监控执行和聚合结果
type TeamOrchestrator struct {
	db          *gorm.DB
	coordinator SimpleTaskCoordinator
	executor    AgentExecutorInterface
	mu          sync.RWMutex

	// 运行中的团队执行
	runningTeams map[string]*TeamExecution
}

// TeamExecution 团队执行上下文
type TeamExecution struct {
	TeamID        string
	Context       context.Context
	CancelFunc    context.CancelFunc
	StartedAt     time.Time
	TeamPlan      *OrchestratorTeamPlan
	ActiveAgents  map[string]*AgentInstance
	CompletedTasks map[string]bool
	mu            sync.RWMutex
}

// AgentInstance Agent 实例
type AgentInstance struct {
	ID           string
	Name         string
	Role         string
	Capabilities []string
	Status       string // idle, busy, offline
	CurrentTask  *models.Task
	LastActivity time.Time
}

// OrchestratorTeamPlan 团队执行计划（编排器专用）
type OrchestratorTeamPlan struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Goal        string                 `json:"goal"`
	Strategy    string                 `json:"strategy"`
	Members     []TeamMemberPlan       `json:"members"`
	Tasks       []TaskPlanItem         `json:"tasks"`
	Config      map[string]interface{} `json:"config"`
	CreatedAt   time.Time              `json:"created_at"`
}

// TeamMemberPlan 团队成员计划
type TeamMemberPlan struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
	LLMModel     string   `json:"llm_model"`
	SystemPrompt string   `json:"system_prompt"`
}

// TaskPlanItem 任务计划项
type TaskPlanItem struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Type         string                 `json:"type"`
	Priority     string                 `json:"priority"`
	AssignedTo   string                 `json:"assigned_to"` // member ID
	Dependencies []string               `json:"dependencies"`
	Estimated    time.Duration          `json:"estimated"` // minutes
	Context      map[string]interface{} `json:"context"`
}

// ExecutionResult 执行结果
type ExecutionResult struct {
	TeamID        string                 `json:"team_id"`
	TeamName      string                 `json:"team_name"`
	Status        string                 `json:"status"`
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	Duration      int64                  `json:"duration"` // seconds
	TaskSummary   TaskResultSummary      `json:"task_summary"`
	MemberResults []MemberResult        `json:"member_results"`
	FinalOutput   string                 `json:"final_output"`
	Artifacts     map[string]interface{} `json:"artifacts"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// TaskResultSummary 任务结果摘要
type TaskResultSummary struct {
	Total      int                    `json:"total"`
	Completed  int                    `json:"completed"`
	Failed     int                    `json:"failed"`
	Skipped    int                    `json:"skipped"`
	ByStatus   map[string]int         `json:"by_status"`
	ByType     map[string]int         `json:"by_type"`
	ByMember   map[string]int         `json:"by_member"`
	TaskDetails []TaskResultDetail    `json:"task_details"`
}

// TaskResultDetail 任务结果详情
type TaskResultDetail struct {
	TaskID      string        `json:"task_id"`
	Title       string        `json:"title"`
	AssignedTo  string        `json:"assigned_to"`
	Status      string        `json:"status"`
	Duration    int64         `json:"duration"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	Output      string        `json:"output,omitempty"`
	Error       string        `json:"error,omitempty"`
	RetryCount  int           `json:"retry_count"`
}

// MemberResult 成员执行结果
type MemberResult struct {
	MemberID       string        `json:"member_id"`
	MemberName     string        `json:"member_name"`
	Role           string        `json:"role"`
	TasksAssigned  int           `json:"tasks_assigned"`
	TasksCompleted int           `json:"tasks_completed"`
	TasksFailed    int           `json:"tasks_failed"`
	TotalDuration  int64         `json:"total_duration"` // seconds
	AverageTaskTime float64      `json:"average_task_time"` // seconds
	Outputs        []string      `json:"outputs"`
}

// NewTeamOrchestrator 创建团队编排器
func NewTeamOrchestrator(db *gorm.DB, coordinator SimpleTaskCoordinator, executor AgentExecutorInterface) *TeamOrchestrator {
	return &TeamOrchestrator{
		db:           db,
		coordinator:  coordinator,
		executor:     executor,
		runningTeams: make(map[string]*TeamExecution),
	}
}

// CreateTeamFromPlan 根据计划创建团队
func (o *TeamOrchestrator) CreateTeamFromPlan(plan *OrchestratorTeamPlan) (*models.Team, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// 验证计划
	if err := o.validatePlan(plan); err != nil {
		return nil, fmt.Errorf("invalid plan: %w", err)
	}

	// 开始事务
	tx := o.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 创建团队记录
	team := &models.Team{
		ID:          uuid.New().String(),
		Name:        plan.Name,
		Description: plan.Description,
		Enabled:     true,
		Config:      plan.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := tx.Create(team).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	// 创建团队成员
	for _, memberPlan := range plan.Members {
		capabilities := make([]models.Capability, len(memberPlan.Capabilities))
		for i, cap := range memberPlan.Capabilities {
			capabilities[i] = models.Capability{
				Name:        cap,
				Level:       0.8,
				ConfirmedAt: time.Now(),
			}
		}

		member := &models.TeamMember{
			ID:           uuid.New().String(),
			TeamID:       team.ID,
			Name:         memberPlan.Name,
			Role:         memberPlan.Role,
			Capabilities: capabilities,
			Availability: "available",
			CurrentLoad:  0,
			MaxLoad:      3,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := tx.Create(member).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create member %s: %w", memberPlan.Name, err)
		}
	}

	// 创建任务
	taskMap := make(map[string]string) // plan ID -> actual ID
	for _, taskPlan := range plan.Tasks {
		task := &models.Task{
			ID:          uuid.New().String(),
			TeamID:      team.ID,
			Title:       taskPlan.Title,
			Description: taskPlan.Description,
			Type:        taskPlan.Type,
			Priority:    taskPlan.Priority,
			Status:      "pending",
			Estimated:   int(taskPlan.Estimated.Minutes()),
			AssignedTo:  taskPlan.AssignedTo,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// 设置上下文
		if taskPlan.Context != nil {
			task.Context = models.TaskContext{
				Metadata: taskPlan.Context,
			}
		}

		if err := tx.Create(task).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create task %s: %w", taskPlan.Title, err)
		}

		taskMap[taskPlan.ID] = task.ID
	}

	// 创建任务依赖关系
	for _, taskPlan := range plan.Tasks {
		if len(taskPlan.Dependencies) > 0 {
			taskID := taskMap[taskPlan.ID]
			for _, depID := range taskPlan.Dependencies {
				actualDepID, ok := taskMap[depID]
				if !ok {
					tx.Rollback()
					return nil, fmt.Errorf("dependency task not found: %s", depID)
				}
				dep := &models.TaskDependency{
					ID:        uuid.New().String(),
					TaskID:    taskID,
					DependsOn: actualDepID,
					Type:      "after",
					CreatedAt: time.Now(),
				}
				if err := tx.Create(dep).Error; err != nil {
					tx.Rollback()
					return nil, fmt.Errorf("failed to create dependency: %w", err)
				}
			}
		}
	}

	// 创建团队活动记录
	activity := &models.TeamActivity{
		ID:          uuid.New().String(),
		TeamID:      team.ID,
		Action:      "team_created",
		Description: fmt.Sprintf("Team '%s' created with %d members and %d tasks", plan.Name, len(plan.Members), len(plan.Tasks)),
		Metadata: map[string]interface{}{
			"plan_id":     plan.ID,
			"member_count": len(plan.Members),
			"task_count":   len(plan.Tasks),
		},
		CreatedAt: time.Now(),
	}
	if err := tx.Create(activity).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create activity: %w", err)
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return team, nil
}

// StartExecution 启动团队执行
func (o *TeamOrchestrator) StartExecution(teamID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// 检查团队是否已在运行
	if _, exists := o.runningTeams[teamID]; exists {
		return fmt.Errorf("team %s is already running", teamID)
	}

	// 获取团队信息
	var team models.Team
	if err := o.db.First(&team, "id = ?", teamID).Error; err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// 获取团队成员
	var members []models.TeamMember
	if err := o.db.Where("team_id = ?", teamID).Find(&members).Error; err != nil {
		return fmt.Errorf("failed to load members: %w", err)
	}

	// 获取任务
	var tasks []models.Task
	if err := o.db.Where("team_id = ?", teamID).Find(&tasks).Error; err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// 创建执行上下文
	ctx, cancel := context.WithCancel(context.Background())

	execution := &TeamExecution{
		TeamID:         teamID,
		Context:        ctx,
		CancelFunc:     cancel,
		StartedAt:      time.Now(),
		ActiveAgents:   make(map[string]*AgentInstance),
		CompletedTasks: make(map[string]bool),
	}

	// 初始化 Agent 实例
	for _, member := range members {
		capabilities := make([]string, len(member.Capabilities))
		for i, c := range member.Capabilities {
			capabilities[i] = c.Name
		}

		agent := &AgentInstance{
			ID:           member.ID,
			Name:         member.Name,
			Role:         member.Role,
			Capabilities: capabilities,
			Status:       "idle",
			LastActivity: time.Now(),
		}
		execution.ActiveAgents[member.ID] = agent
	}

	// 构建 TeamPlan
	teamPlan := o.buildTeamPlanFromDB(team, members, tasks)
	execution.TeamPlan = teamPlan

	// 保存执行上下文
	o.runningTeams[teamID] = execution

	// 启动异步执行
	go o.executeTeam(execution)

	// 记录活动
	o.createTeamActivity(teamID, "execution_started", fmt.Sprintf("Team execution started with %d tasks", len(tasks)), nil)

	return nil
}

// executeTeam 执行团队任务
func (o *TeamOrchestrator) executeTeam(execution *TeamExecution) {
	// 按依赖顺序执行任务
	taskQueue := o.buildTaskQueue(execution)

	for _, task := range taskQueue {
		// 检查上下文是否已取消
		select {
		case <-execution.Context.Done():
			return
		default:
		}

		// 检查依赖是否完成
		if !o.areDependenciesComplete(execution, task) {
			continue
		}

		// 分配任务给 Agent
		agent, ok := execution.ActiveAgents[task.AssignedTo]
		if !ok {
			o.handleTaskFailure(execution, task, fmt.Errorf("agent not found"))
			continue
		}

		// 执行任务
		if err := o.executeTask(execution, task, agent); err != nil {
			o.handleTaskFailure(execution, task, err)
		} else {
			execution.mu.Lock()
			execution.CompletedTasks[task.ID] = true
			execution.mu.Unlock()
		}
	}

	// 所有任务完成，生成最终结果
	o.generateFinalResult(execution)
}

// executeTask 执行单个任务
func (o *TeamOrchestrator) executeTask(execution *TeamExecution, task *models.Task, agent *AgentInstance) error {
	// 更新任务状态
	now := time.Now()
	task.Status = "in_progress"
	task.StartedAt = &now
	o.db.Save(task)

	agent.Status = "busy"
	agent.CurrentTask = task
	agent.LastActivity = now

	// 通过 AgentExecutor 执行任务
	if o.executor != nil {
		// 从 TeamPlan 中获取 agent 的 system prompt
		agentPrompt := ""
		for _, member := range execution.TeamPlan.Members {
			if member.ID == agent.ID {
				agentPrompt = member.SystemPrompt
				break
			}
		}

		// 收集前置任务的输出作为上下文
		taskContext := make(map[string]interface{})
		if task.Dependencies != nil {
			for _, dep := range task.Dependencies {
				var depTask models.Task
				if err := o.db.First(&depTask, "id = ?", dep.DependsOn).Error; err == nil {
					if depTask.Result != nil && depTask.Result.Output != "" {
						taskContext[depTask.Title] = depTask.Result.Output
					}
				}
			}
		}

		// 获取团队目标
		teamGoal := ""
		if execution.TeamPlan != nil {
			teamGoal = execution.TeamPlan.Goal
		}

		execReq := &AgentExecRequest{
			TaskID:          task.ID,
			TaskTitle:       task.Title,
			TaskDescription: task.Description,
			AgentName:       agent.Name,
			AgentRole:       agent.Role,
			AgentPrompt:     agentPrompt,
			Context:         taskContext,
			TeamGoal:        teamGoal,
		}

		result, err := o.executor.Execute(execution.Context, execReq)
		if err != nil {
			agent.Status = "idle"
			agent.CurrentTask = nil
			return fmt.Errorf("executor failed for task %s: %w", task.Title, err)
		}

		completedAt := time.Now()
		task.Status = "completed"
		task.CompletedAt = &completedAt
		task.Result = &models.TaskResult{
			Success: result.Success,
			Output:  result.Output,
			Summary: result.Summary,
		}
		o.db.Save(task)

		agent.Status = "idle"
		agent.CurrentTask = nil
		agent.LastActivity = completedAt

		o.notifyTaskCompletion(execution, task, task.Result.Output)
		return nil
	}

	// 无 executor 时降级：标记为完成（向后兼容）
	completedAt := time.Now()
	task.Status = "completed"
	task.CompletedAt = &completedAt
	task.Result = &models.TaskResult{
		Success: true,
		Output:  fmt.Sprintf("Task %s completed by %s (no executor)", task.Title, agent.Name),
		Summary: summarizeOutput(task.Title),
	}
	o.db.Save(task)

	agent.Status = "idle"
	agent.CurrentTask = nil
	agent.LastActivity = completedAt

	o.notifyTaskCompletion(execution, task, task.Result.Output)
	return nil
}

// MonitorProgress 监控执行进度
func (o *TeamOrchestrator) MonitorProgress(teamID string) (*models.ProgressReport, error) {
	o.mu.RLock()
	execution, exists := o.runningTeams[teamID]
	o.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("team %s is not running", teamID)
	}

	// 获取所有任务
	var tasks []models.Task
	if err := o.db.Where("team_id = ?", teamID).Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("failed to load tasks: %w", err)
	}

	// 统计任务状态
	statusSummary := models.TaskStatusSummary{}
	taskDetails := make([]TaskResultDetail, 0)

	for _, task := range tasks {
		statusSummary.Total++
		switch task.Status {
		case "pending":
			statusSummary.Pending++
		case "in_progress":
			statusSummary.InProgress++
		case "completed":
			statusSummary.Completed++
		case "failed":
			statusSummary.Failed++
		case "cancelled":
			statusSummary.Cancelled++
		}

		var duration int64
		if task.StartedAt != nil && task.CompletedAt != nil {
			duration = int64(task.CompletedAt.Sub(*task.StartedAt).Seconds())
		}

		taskDetails = append(taskDetails, TaskResultDetail{
			TaskID:      task.ID,
			Title:       task.Title,
			AssignedTo:  task.AssignedTo,
			Status:      task.Status,
			Duration:    duration,
			StartedAt:   task.CreatedAt,
			CompletedAt: task.CompletedAt,
			Output:      getResultOutput(task.Result),
			Error:       getResultError(task.Result),
			RetryCount:  task.RetryCount,
		})
	}

	// 统计成员状态
	memberStats := make([]models.MemberStat, 0)
	execution.mu.RLock()
	for _, agent := range execution.ActiveAgents {
		var assigned, completed, failed int
		for _, task := range tasks {
			if task.AssignedTo == agent.ID {
				assigned++
				if task.Status == "completed" {
					completed++
				} else if task.Status == "failed" {
					failed++
				}
			}
		}

		memberStats = append(memberStats, models.MemberStat{
			MemberID:        agent.ID,
			MemberName:      agent.Name,
			TasksAssigned:   assigned,
			TasksCompleted:  completed,
			TasksFailed:     failed,
			AverageProgress: calculateAverageProgress(assigned, completed),
		})
	}
	execution.mu.RUnlock()

	// 生成报告
	report := &models.ProgressReport{
		ID:          uuid.New().String(),
		TeamID:      teamID,
		GeneratedBy: "system",
		PeriodStart: execution.StartedAt,
		PeriodEnd:   time.Now(),
		Summary:     generateProgressSummary(statusSummary),
		Statistics: map[string]interface{}{
			"total_tasks":     statusSummary.Total,
			"completion_rate": float64(statusSummary.Completed) / float64(statusSummary.Total) * 100,
			"active_members":  len(execution.ActiveAgents),
			"elapsed_time":    time.Since(execution.StartedAt).String(),
		},
		TaskStatus:  statusSummary,
		MemberStats: memberStats,
		CreatedAt:   time.Now(),
	}

	return report, nil
}

// AggregateResults 聚合执行结果
func (o *TeamOrchestrator) AggregateResults(teamID string) (*ExecutionResult, error) {
	// 获取团队信息
	var team models.Team
	if err := o.db.First(&team, "id = ?", teamID).Error; err != nil {
		return nil, fmt.Errorf("team not found: %w", err)
	}

	// 获取所有任务
	var tasks []models.Task
	if err := o.db.Where("team_id = ?", teamID).Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("failed to load tasks: %w", err)
	}

	// 获取成员
	var members []models.TeamMember
	if err := o.db.Where("team_id = ?", teamID).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to load members: %w", err)
	}

	// 构建任务摘要
	taskSummary := TaskResultSummary{
		Total:      len(tasks),
		ByStatus:   make(map[string]int),
		ByType:     make(map[string]int),
		ByMember:   make(map[string]int),
		TaskDetails: make([]TaskResultDetail, 0),
	}

	for _, task := range tasks {
		taskSummary.ByStatus[task.Status]++
		taskSummary.ByType[task.Type]++
		taskSummary.ByMember[task.AssignedTo]++

		switch task.Status {
		case "completed":
			taskSummary.Completed++
		case "failed":
			taskSummary.Failed++
		case "cancelled":
			taskSummary.Skipped++
		}

		var duration int64
		if task.StartedAt != nil && task.CompletedAt != nil {
			duration = int64(task.CompletedAt.Sub(*task.StartedAt).Seconds())
		}

		taskSummary.TaskDetails = append(taskSummary.TaskDetails, TaskResultDetail{
			TaskID:      task.ID,
			Title:       task.Title,
			AssignedTo:  task.AssignedTo,
			Status:      task.Status,
			Duration:    duration,
			StartedAt:   task.CreatedAt,
			CompletedAt: task.CompletedAt,
			Output:      getResultOutput(task.Result),
			Error:       getResultError(task.Result),
			RetryCount:  task.RetryCount,
		})
	}

	// 构建成员结果
	memberResults := make([]MemberResult, 0)
	memberOutputMap := make(map[string][]string)

	for _, member := range members {
		var assigned, completed, failed int
		var totalDuration int64

		for _, task := range tasks {
			if task.AssignedTo == member.ID {
				assigned++
				if task.Status == "completed" {
					completed++
				} else if task.Status == "failed" {
					failed++
				}
				if task.StartedAt != nil && task.CompletedAt != nil {
					totalDuration += int64(task.CompletedAt.Sub(*task.StartedAt).Seconds())
				}
				if task.Result != nil && task.Result.Output != "" {
					memberOutputMap[member.ID] = append(memberOutputMap[member.ID], task.Result.Output)
				}
			}
		}

		avgTime := 0.0
		if completed > 0 {
			avgTime = float64(totalDuration) / float64(completed)
		}

		memberResults = append(memberResults, MemberResult{
			MemberID:       member.ID,
			MemberName:     member.Name,
			Role:           member.Role,
			TasksAssigned:  assigned,
			TasksCompleted: completed,
			TasksFailed:    failed,
			TotalDuration:  totalDuration,
			AverageTaskTime: avgTime,
			Outputs:        memberOutputMap[member.ID],
		})
	}

	// 确定整体状态
	status := "completed"
	if taskSummary.Failed > 0 {
		status = "partial"
	}
	if taskSummary.Completed == 0 {
		status = "failed"
	}

	// 计算执行时长
	var duration int64
	o.mu.RLock()
	if execution, exists := o.runningTeams[teamID]; exists {
		duration = int64(time.Since(execution.StartedAt).Seconds())
	}
	o.mu.RUnlock()

	// 生成最终输出
	finalOutput := generateFinalOutput(taskSummary, memberResults)

	result := &ExecutionResult{
		TeamID:        teamID,
		TeamName:      team.Name,
		Status:        status,
		StartedAt:     team.CreatedAt,
		Duration:      duration,
		TaskSummary:   taskSummary,
		MemberResults: memberResults,
		FinalOutput:   finalOutput,
		Artifacts:     make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"total_tasks":     taskSummary.Total,
			"completion_rate": float64(taskSummary.Completed) / float64(taskSummary.Total) * 100,
		},
	}

	return result, nil
}

// HandleFailure 处理任务失败
func (o *TeamOrchestrator) HandleFailure(teamID, taskID string) error {
	// 获取任务
	var task models.Task
	if err := o.db.First(&task, "id = ?", taskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// 检查重试次数
	if task.RetryCount < task.MaxRetries {
		task.RetryCount++
		task.Status = "pending"
		task.UpdatedAt = time.Now()
		o.db.Save(task)

		// 记录活动
		o.createTeamActivity(teamID, "task_retry", fmt.Sprintf("Task %s queued for retry", task.Title), map[string]interface{}{
			"task_id":     taskID,
			"retry_count": task.RetryCount,
		})

		return nil
	}

	// 超过最大重试次数，标记为失败
	task.Status = "failed"
	task.UpdatedAt = time.Now()
	o.db.Save(task)

	// 记录活动
	o.createTeamActivity(teamID, "task_failed", fmt.Sprintf("Task %s failed after max retries", task.Title), map[string]interface{}{
		"task_id":     taskID,
		"retry_count": task.RetryCount,
	})

	return nil
}

// validatePlan 验证计划
func (o *TeamOrchestrator) validatePlan(plan *OrchestratorTeamPlan) error {
	if plan.ID == "" {
		return fmt.Errorf("plan ID is required")
	}
	if plan.Name == "" {
		return fmt.Errorf("plan name is required")
	}
	if len(plan.Members) == 0 {
		return fmt.Errorf("at least one member is required")
	}
	if len(plan.Tasks) == 0 {
		return fmt.Errorf("at least one task is required")
	}

	// 验证任务分配
	memberMap := make(map[string]string)
	for _, m := range plan.Members {
		memberMap[m.ID] = m.Name
	}

	for _, task := range plan.Tasks {
		if task.AssignedTo != "" {
			if _, ok := memberMap[task.AssignedTo]; !ok {
				return fmt.Errorf("task %s assigned to unknown member: %s", task.ID, task.AssignedTo)
			}
		}
	}

	return nil
}

// buildTaskQueue 构建任务队列（按依赖顺序）
func (o *TeamOrchestrator) buildTaskQueue(execution *TeamExecution) []*models.Task {
	tasks := make([]*models.Task, 0)
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(task *models.Task)
	visit = func(task *models.Task) {
		if visited[task.ID] {
			return
		}
		if visiting[task.ID] {
			// 循环依赖
			return
		}

		visiting[task.ID] = true

		// 访问依赖的任务
		for _, dep := range task.Dependencies {
			var depTask models.Task
			if err := o.db.First(&depTask, "id = ?", dep.DependsOn).Error; err == nil {
				visit(&depTask)
			}
		}

		visiting[task.ID] = false
		visited[task.ID] = true
		tasks = append(tasks, task)
	}

	// 从所有任务开始
	for _, taskPlan := range execution.TeamPlan.Tasks {
		var task models.Task
		if err := o.db.Where("team_id = ? AND title = ?", execution.TeamID, taskPlan.Title).First(&task).Error; err == nil {
			visit(&task)
		}
	}

	return tasks
}

// areDependenciesComplete 检查依赖是否完成
func (o *TeamOrchestrator) areDependenciesComplete(execution *TeamExecution, task *models.Task) bool {
	for _, dep := range task.Dependencies {
		execution.mu.RLock()
		completed := execution.CompletedTasks[dep.DependsOn]
		execution.mu.RUnlock()

		if !completed {
			return false
		}
	}
	return true
}

// handleTaskFailure 处理任务失败
func (o *TeamOrchestrator) handleTaskFailure(execution *TeamExecution, task *models.Task, err error) {
	task.Status = "failed"
	task.RetryCount++
	if task.Result == nil {
		task.Result = &models.TaskResult{}
	}
	task.Result.Error = err.Error()
	task.UpdatedAt = time.Now()
	o.db.Save(task)

	// 记录活动
	o.createTeamActivity(execution.TeamID, "task_failed", fmt.Sprintf("Task %s failed: %v", task.Title, err), map[string]interface{}{
		"task_id": task.ID,
		"error":   err.Error(),
	})
}

// buildTeamPlanFromDB 从数据库构建 TeamPlan
func (o *TeamOrchestrator) buildTeamPlanFromDB(team models.Team, members []models.TeamMember, tasks []models.Task) *OrchestratorTeamPlan {
	memberPlans := make([]TeamMemberPlan, len(members))
	for i, m := range members {
		capabilities := make([]string, len(m.Capabilities))
		for j, c := range m.Capabilities {
			capabilities[j] = c.Name
		}

		memberPlans[i] = TeamMemberPlan{
			ID:           m.ID,
			Name:         m.Name,
			Role:         m.Role,
			Capabilities: capabilities,
		}
	}

	taskPlans := make([]TaskPlanItem, len(tasks))
	for i, t := range tasks {
		depIDs := make([]string, len(t.Dependencies))
		for j, d := range t.Dependencies {
			depIDs[j] = d.DependsOn
		}

		taskPlans[i] = TaskPlanItem{
			ID:           t.ID,
			Title:        t.Title,
			Description:  t.Description,
			Type:         t.Type,
			Priority:     t.Priority,
			AssignedTo:   t.AssignedTo,
			Dependencies: depIDs,
			Estimated:    time.Duration(t.Estimated) * time.Minute,
		}
	}

	return &OrchestratorTeamPlan{
		ID:          uuid.New().String(),
		Name:        team.Name,
		Description: team.Description,
		Goal:        "",
		Strategy:    "sequential",
		Members:     memberPlans,
		Tasks:       taskPlans,
		Config:      team.Config,
		CreatedAt:   time.Now(),
	}
}

// notifyTaskCompletion 通知任务完成
func (o *TeamOrchestrator) notifyTaskCompletion(execution *TeamExecution, task *models.Task, output string) {
	// 发送消息到团队成员
	message := &models.TeamMessage{
		ID:       uuid.New().String(),
		TeamID:   execution.TeamID,
		From:     "system",
		ToMember: "*",
		Type:     "task_completed",
		Content: map[string]interface{}{
			"task_id":   task.ID,
			"task_title": task.Title,
			"assigned_to": task.AssignedTo,
			"output":    output,
		},
		Status:    "delivered",
		CreatedAt: time.Now(),
	}
	o.db.Create(message)
}

// generateFinalResult 生成最终结果
func (o *TeamOrchestrator) generateFinalResult(execution *TeamExecution) {
	completedAt := time.Now()

	o.createTeamActivity(execution.TeamID, "execution_completed", "Team execution completed", map[string]interface{}{
		"duration": completedAt.Sub(execution.StartedAt).String(),
	})

	// 清理执行上下文
	o.mu.Lock()
	delete(o.runningTeams, execution.TeamID)
	o.mu.Unlock()
}

// createTeamActivity 创建团队活动记录
func (o *TeamOrchestrator) createTeamActivity(teamID, action, description string, metadata map[string]interface{}) {
	activity := &models.TeamActivity{
		ID:          uuid.New().String(),
		TeamID:      teamID,
		Action:      action,
		Description: description,
		Metadata:    metadata,
		CreatedAt:   time.Now(),
	}
	o.db.Create(activity)
}

// ==================== 工具函数 ====================

func calculateAverageProgress(assigned, completed int) float64 {
	if assigned == 0 {
		return 0
	}
	return float64(completed) / float64(assigned) * 100
}

func generateProgressSummary(status models.TaskStatusSummary) string {
	return fmt.Sprintf("Total: %d, Completed: %d, In Progress: %d, Failed: %d",
		status.Total, status.Completed, status.InProgress, status.Failed)
}

func getResultOutput(result *models.TaskResult) string {
	if result == nil {
		return ""
	}
	return result.Output
}

func getResultError(result *models.TaskResult) string {
	if result == nil {
		return ""
	}
	return result.Error
}

func summarizeOutput(output string) string {
	if len(output) > 200 {
		return output[:200] + "..."
	}
	return output
}

func formatContext(ctx models.TaskContext) string {
	if ctx.Metadata == nil {
		return "No additional context"
	}
	data, _ := json.Marshal(ctx.Metadata)
	return string(data)
}

func generateFinalOutput(summary TaskResultSummary, results []MemberResult) string {
	output := fmt.Sprintf("Team Execution Summary:\n")
	output += fmt.Sprintf("- Total Tasks: %d\n", summary.Total)
	output += fmt.Sprintf("- Completed: %d\n", summary.Completed)
	output += fmt.Sprintf("- Failed: %d\n", summary.Failed)
	output += fmt.Sprintf("- Success Rate: %.1f%%\n\n", float64(summary.Completed)/float64(summary.Total)*100)

	output += "Member Performance:\n"
	for _, r := range results {
		output += fmt.Sprintf("- %s (%s): %d/%d completed\n", r.MemberName, r.Role, r.TasksCompleted, r.TasksAssigned)
	}

	return output
}
