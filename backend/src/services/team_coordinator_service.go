package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// TeamCoordinatorService 团队协调服务
type TeamCoordinatorService struct {
	db            *gorm.DB
	taskMutex     sync.RWMutex
	memberMutex   sync.RWMutex
	messageMutex  sync.RWMutex
	activeTasks   map[string]*models.Task
	memberStatus  map[string]string
	eventChannels map[string][]chan models.TeamEvent
	stopChannels  map[string]context.CancelFunc
	configPath    string
	defaultRetry  models.RetryConfig

	// Agent 管理增强
	agentRegistry map[string]*AgentInfo      // agent_id -> AgentInfo
	capabilityIndex map[string][]string       // capability -> [agent_ids]
	heartbeats     map[string]time.Time       // agent_id -> last_heartbeat
	heartbeatMutex sync.RWMutex

	// 任务协调增强
	taskScheduler  *TaskScheduler
	dependencyGraph *TaskDependencyManager
	resultAggregator *ResultAggregator

	// LLM 集成
	llmClients     map[string]LLMClient       // agent_id -> LLM client
	rolePrompts    map[string]string          // role -> prompt template

	// 消息总线
	messageBus     *MessageBus

	// 上下文
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// AgentInfo Agent 信息
type AgentInfo struct {
	ID            string
	Name          string
	Type          string    // human, ai, hybrid
	Capabilities  []string
	Status        string
	CurrentLoad   int
	MaxLoad       int
	Specialization []string
	LastHeartbeat time.Time
	Config        map[string]interface{}
	LLMProvider   string // LLM 提供商
	LLMModel      string // LLM 模型
}

// TaskScheduler 任务调度器
type TaskScheduler struct {
	strategies map[string]AssignmentStrategy
}

// AssignmentStrategy 分配策略接口
type AssignmentStrategy interface {
	Assign(task *models.Task, members []models.TeamMember) (string, error)
	Name() string
}

// TaskDependencyManager 任务依赖管理器
type TaskDependencyManager struct {
	graph  map[string][]string // task_id -> [dependent_task_ids]
	mutex  sync.RWMutex
}

// ResultAggregator 结果聚合器
type ResultAggregator struct {
	results map[string]*TaskResult // task_id -> result
	mutex   sync.RWMutex
}

// TaskResult 任务结果
type TaskResult struct {
	TaskID      string
	Success     bool
	Output      interface{}
	Error       string
	Metrics     map[string]interface{}
	CompletedBy string
	CompletedAt time.Time
}

// MessageBus 消息总线
type MessageBus struct {
	topics    map[string][]chan Message
	mu        sync.RWMutex
}

// Message 消息
type Message struct {
	ID        string
	Topic     string
	From      string
	To        string // "" 表示广播
	Type      string
	Content   interface{}
	Timestamp time.Time
}

// LLMClient LLM 客户端接口
type LLMClient interface {
	Generate(ctx context.Context, prompt string, options map[string]interface{}) (string, error)
	GenerateStream(ctx context.Context, prompt string, options map[string]interface{}) (<-chan string, error)
}

// ProgressData 进度数据
type ProgressData struct {
	TotalTasks      int                       `json:"total_tasks"`
	CompletedTasks  int                       `json:"completed_tasks"`
	FailedTasks     int                       `json:"failed_tasks"`
	PendingTasks    int                       `json:"pending_tasks"`
	InProgressTasks int                       `json:"in_progress_tasks"`
	OverallProgress float64                   `json:"overall_progress"`
	TasksByStatus   map[string]int            `json:"tasks_by_status"`
	TasksByMember   map[string]*MemberProgress `json:"tasks_by_member"`
	TasksByPriority map[string]int            `json:"tasks_by_priority"`
	EstimatedTime   time.Duration             `json:"estimated_time"`
	StartedAt       time.Time                 `json:"started_at"`
}

// MemberProgress 成员进度
type MemberProgress struct {
	MemberID        string    `json:"member_id"`
	MemberName      string    `json:"member_name"`
	AssignedTasks   int       `json:"assigned_tasks"`
	CompletedTasks  int       `json:"completed_tasks"`
	FailedTasks     int       `json:"failed_tasks"`
	AverageProgress float64   `json:"average_progress"`
	CurrentTask     string    `json:"current_task,omitempty"`
	Status          string    `json:"status"`
	LastActive      time.Time `json:"last_active"`
}

// NewTeamCoordinatorService 创建团队协调服务实例
func NewTeamCoordinatorService(db *gorm.DB, configPath string) *TeamCoordinatorService {
	ctx, cancel := context.WithCancel(context.Background())

	service := &TeamCoordinatorService{
		db:            db,
		activeTasks:   make(map[string]*models.Task),
		memberStatus:  make(map[string]string),
		eventChannels: make(map[string][]chan models.TeamEvent),
		stopChannels:  make(map[string]context.CancelFunc),
		configPath:    configPath,
		defaultRetry: models.RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    1000,
			MaxDelay:        30000,
			BackoffFactor:   2.0,
			RetryableErrors: []string{"timeout", "connection", "temporary"},
		},

		// Agent 管理增强
		agentRegistry:   make(map[string]*AgentInfo),
		capabilityIndex: make(map[string][]string),
		heartbeats:      make(map[string]time.Time),

		// 任务协调增强
		taskScheduler:      &TaskScheduler{strategies: make(map[string]AssignmentStrategy)},
		dependencyGraph:    &TaskDependencyManager{graph: make(map[string][]string)},
		resultAggregator:   &ResultAggregator{results: make(map[string]*TaskResult)},

		// LLM 集成
		llmClients:  make(map[string]LLMClient),
		rolePrompts: make(map[string]string),

		// 消息总线
		messageBus: &MessageBus{topics: make(map[string][]chan Message)},

		// 上下文
		ctx:    ctx,
		cancel: cancel,
	}

	// 初始化默认分配策略
	service.taskScheduler.strategies["load_balanced"] = &LoadBalancedStrategy{service: service}
	service.taskScheduler.strategies["capability_based"] = &CapabilityBasedStrategy{service: service}
	service.taskScheduler.strategies["priority_first"] = &PriorityFirstStrategy{service: service}

	// 初始化角色提示词
	service.initRolePrompts()

	// 启动事件处理器
	service.startEventHandler()

	// 启动心跳监控
	service.wg.Add(1)
	go service.heartbeatMonitor()

	// 启动依赖解析器
	service.wg.Add(1)
	go service.dependencyResolver()

	return service
}

// initRolePrompts 初始化角色提示词
func (s *TeamCoordinatorService) initRolePrompts() {
	s.rolePrompts["researcher"] = `你是一个专业的研究员，擅长：
- 深入研究和分析问题
- 收集和整理信息
- 提供详细的报告和建议
- 保持客观和批判性思维`

	s.rolePrompts["developer"] = `你是一个经验丰富的开发者，擅长：
- 编写高质量、可维护的代码
- 进行代码审查和重构
- 解决技术问题和挑战
- 遵循最佳实践和设计模式`

	s.rolePrompts["reviewer"] = `你是一个严格的代码审查员，擅长：
- 发现代码中的问题和潜在风险
- 提供改进建议和最佳实践
- 确保代码符合规范和标准
- 提供清晰、具体的反馈`

	s.rolePrompts["planner"] = `你是一个项目规划专家，擅长：
- 任务分解和规划
- 资源分配和时间估算
- 风险识别和管理
- 制定详细的执行计划`

	s.rolePrompts["tester"] = `你是一个专业的测试工程师，擅长：
- 设计全面的测试用例
- 发现和报告缺陷
- 自动化测试开发
- 质量保证和持续改进`
}

// ============ 团队管理 ============

// CreateTeam 创建新团队
func (s *TeamCoordinatorService) CreateTeam(name, description string, config map[string]interface{}) (*models.Team, error) {
	team := &models.Team{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Enabled:     true,
		Config:      config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.Create(team).Error; err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	return team, nil
}

// GetTeam 获取团队信息
func (s *TeamCoordinatorService) GetTeam(teamID string) (*models.Team, error) {
	var team models.Team
	err := s.db.Where("id = ?", teamID).First(&team).Error
	if err != nil {
		return nil, fmt.Errorf("team not found: %w", err)
	}
	return &team, nil
}

// ListTeams 列出所有团队
func (s *TeamCoordinatorService) ListTeams() ([]models.Team, error) {
	var teams []models.Team
	err := s.db.Find(&teams).Error
	return teams, err
}

// DeleteTeam 删除团队
func (s *TeamCoordinatorService) DeleteTeam(teamID string) error {
	var team models.Team
	if err := s.db.First(&team, teamID).Error; err != nil {
		return fmt.Errorf("team not found: %w", err)
	}
	s.db.Where("team_id = ?", teamID).Delete(&models.TeamMember{})
	s.db.Where("team_id = ?", teamID).Delete(&models.Task{})
	return s.db.Delete(&team).Error
}

// ============ 成员管理 ============

// AddMember 添加团队成员
func (s *TeamCoordinatorService) AddMember(teamID, name, role, agentType string, capabilities []string, config map[string]interface{}) (*models.TeamMember, error) {
	// 验证团队存在
	_, err := s.GetTeam(teamID)
	if err != nil {
		return nil, err
	}

	member := &models.TeamMember{
		ID:       uuid.New().String(),
		TeamID:   teamID,
		Name:     name,
		Role:     role,
		// 转换 capabilities 到 Capability 结构
		Capabilities: make([]models.Capability, 0),
		Availability: "active",
		CurrentLoad:  0,
		MaxLoad:      3,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// 添加能力
	for _, cap := range capabilities {
		member.Capabilities = append(member.Capabilities, models.Capability{
			Name:        cap,
			Level:       1.0,
			ConfirmedAt: time.Now(),
		})
	}

	if err := s.db.Create(member).Error; err != nil {
		return nil, fmt.Errorf("failed to add member: %w", err)
	}

	// 更新内存中的成员状态
	s.memberMutex.Lock()
	s.memberStatus[member.ID] = "active"
	s.memberMutex.Unlock()

	// 发布成员添加事件
	s.publishEvent(teamID, models.TeamEvent{
		ID:        uuid.New().String(),
		TeamID:    teamID,
		Type:      "member_added",
		Source:    "system",
		Data:      map[string]interface{}{"member_id": member.ID, "member_name": name},
		CreatedAt: time.Now(),
	})

	return member, nil
}

// RemoveMember 移除团队成员
func (s *TeamCoordinatorService) RemoveMember(teamID, memberID string) error {
	// 检查是否有任务正在执行
	if s.hasActiveTasks(memberID) {
		return fmt.Errorf("cannot remove member with active tasks")
	}

	err := s.db.Delete(&models.TeamMember{}, "id = ? AND team_id = ?", memberID, teamID).Error
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	// 更新内存中的成员状态
	s.memberMutex.Lock()
	delete(s.memberStatus, memberID)
	s.memberMutex.Unlock()

	// 发布成员移除事件
	s.publishEvent(teamID, models.TeamEvent{
		ID:        uuid.New().String(),
		TeamID:    teamID,
		Type:      "member_removed",
		Source:    "system",
		Data:      map[string]interface{}{"member_id": memberID},
		CreatedAt: time.Now(),
	})

	return nil
}

// GetMember 获取成员信息
func (s *TeamCoordinatorService) GetMember(memberID string) (*models.TeamMember, error) {
	var member models.TeamMember
	err := s.db.Where("id = ?", memberID).First(&member).Error
	if err != nil {
		return nil, fmt.Errorf("member not found: %w", err)
	}
	return &member, nil
}

// ListMembers 列出团队成员
func (s *TeamCoordinatorService) ListMembers(teamID string) ([]models.TeamMember, error) {
	var members []models.TeamMember
	err := s.db.Where("team_id = ?", teamID).Find(&members).Error
	return members, err
}

// UpdateMemberStatus 更新成员状态
func (s *TeamCoordinatorService) UpdateMemberStatus(memberID, status string) error {
	err := s.db.Model(&models.TeamMember{}).Where("id = ?", memberID).Update("availability", status).Error
	if err != nil {
		return fmt.Errorf("failed to update member status: %w", err)
	}

	s.memberMutex.Lock()
	s.memberStatus[memberID] = status
	s.memberMutex.Unlock()

	return nil
}

// GetMemberStatus 获取成员状态
func (s *TeamCoordinatorService) GetMemberStatus(memberID string) string {
	s.memberMutex.RLock()
	defer s.memberMutex.RUnlock()
	if status, ok := s.memberStatus[memberID]; ok {
		return status
	}
	return "unknown"
}

// ============ 任务管理 ============

// CreateTask 创建新任务
func (s *TeamCoordinatorService) CreateTask(teamID, workflowID, title, description, taskType, priority, createdBy string, input map[string]interface{}) (*models.Task, error) {
	taskID := uuid.New().String()
	now := time.Now()

	// 使用原始 SQL 插入以避免 GORM 处理切片字段的问题
	var metadataJSON string
	if input != nil {
		if data, err := json.Marshal(input); err == nil {
			metadataJSON = string(data)
		}
	}

	err := s.db.Exec(`
		INSERT INTO tasks (id, team_id, title, description, type, priority, status, created_by, context_metadata, retry_count, max_retries, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, taskID, teamID, title, description, taskType, priority, "pending", createdBy, metadataJSON, 0, s.defaultRetry.MaxAttempts, now, now).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	task := &models.Task{
		ID:        taskID,
		TeamID:    teamID,
		Title:     title,
		Description: description,
		Type:      taskType,
		Priority:  priority,
		Status:    "pending",
		CreatedBy: createdBy,
		Context:   models.TaskContext{Metadata: input},
		RetryCount: 0,
		MaxRetries: s.defaultRetry.MaxAttempts,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 添加到活跃任务
	s.taskMutex.Lock()
	s.activeTasks[task.ID] = task
	s.taskMutex.Unlock()

	// 发布任务创建事件
	s.publishEvent(teamID, models.TeamEvent{
		ID:        uuid.New().String(),
		TeamID:    teamID,
		Type:      "task_created",
		Source:    createdBy,
		Data:      map[string]interface{}{"task_id": task.ID, "title": title},
		CreatedAt: time.Now(),
	})

	return task, nil
}

// AssignTask 分配任务给成员
func (s *TeamCoordinatorService) AssignTask(taskID, memberID string) error {
	s.taskMutex.Lock()
	task, exists := s.activeTasks[taskID]
	if !exists {
		s.taskMutex.Unlock()
		return fmt.Errorf("task not found")
	}
	task.AssignedTo = memberID
	task.Status = "in_progress"
	task.UpdatedAt = time.Now()
	s.taskMutex.Unlock()

	// 更新数据库（使用原始 SQL）
	now := time.Now()
	updateNow := time.Now()
	err := s.db.Exec(`
		UPDATE tasks SET assigned_to = ?, status = ?, started_at = ?, updated_at = ? WHERE id = ?
	`, memberID, "in_progress", now, updateNow, taskID).Error
	if err != nil {
		return fmt.Errorf("failed to assign task: %w", err)
	}

	// 发送通知给成员
	go s.notifyMember(task.TeamID, memberID, "task_assigned", map[string]interface{}{
		"task_id":  taskID,
		"title":    task.Title,
		"priority": task.Priority,
		"type":     task.Type,
	})

	return nil
}

// UpdateTaskStatus 更新任务状态
func (s *TeamCoordinatorService) UpdateTaskStatus(taskID, status string, errorMsg string) error {
	s.taskMutex.Lock()
	task, exists := s.activeTasks[taskID]
	if !exists {
		s.taskMutex.Unlock()
		return fmt.Errorf("task not found")
	}

	oldStatus := task.Status
	task.Status = status
	task.UpdatedAt = time.Now()

	if errorMsg != "" {
		if task.Result == nil {
			task.Result = &models.TaskResult{}
		}
		task.Result.Error = errorMsg
		task.Result.Success = false
	} else if status == "completed" {
		if task.Result == nil {
			task.Result = &models.TaskResult{}
		}
		task.Result.Success = true
	}

	// 根据状态更新时间戳
	now := time.Now()
	if status == "in_progress" && task.StartedAt == nil {
		task.StartedAt = &now
	} else if status == "completed" || status == "failed" || status == "cancelled" {
		task.CompletedAt = &now
	}
	s.taskMutex.Unlock()

	// 更新数据库（使用原始 SQL）
	updateNow := time.Now()
	updateSQL := "UPDATE tasks SET status = ?, updated_at = ?"
	updateArgs := []interface{}{status, updateNow}

	if status == "completed" || status == "failed" || status == "cancelled" {
		updateSQL += ", completed_at = ?"
		updateArgs = append(updateArgs, &now)
	}

	updateSQL += " WHERE id = ?"
	updateArgs = append(updateArgs, taskID)

	err := s.db.Exec(updateSQL, updateArgs...).Error
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// 发布状态变更事件
	eventType := "task_status_changed"
	if status == "completed" {
		eventType = "task_completed"
	} else if status == "failed" {
		eventType = "task_failed"
	}

	s.publishEvent(task.TeamID, models.TeamEvent{
		ID:        uuid.New().String(),
		TeamID:    task.TeamID,
		Type:      eventType,
		Source:    task.AssignedTo,
		Data: map[string]interface{}{
			"task_id":    taskID,
			"old_status": oldStatus,
			"new_status": status,
		},
		CreatedAt: time.Now(),
	})

	// 如果任务完成或失败，通知相关成员
	if status == "completed" || status == "failed" {
		if task.AssignedTo != "" {
			go s.notifyMember(task.TeamID, task.AssignedTo, eventType, map[string]interface{}{
				"task_id": taskID,
				"status":  status,
			})
		}
	}

	return nil
}

// UpdateTaskProgress 更新任务进度
func (s *TeamCoordinatorService) UpdateTaskProgress(taskID string, progress int) error {
	s.taskMutex.Lock()
	_, exists := s.activeTasks[taskID]
	s.taskMutex.Unlock()

	if !exists {
		return fmt.Errorf("task not found")
	}

	// 保存进度记录
	progressRecord := &models.TaskProgress{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Percent:   progress,
		CreatedAt: time.Now(),
	}
	return s.db.Create(progressRecord).Error
}

// GetTask 获取任务详情
func (s *TeamCoordinatorService) GetTask(taskID string) (*models.Task, error) {
	s.taskMutex.RLock()
	task, exists := s.activeTasks[taskID]
	s.taskMutex.RUnlock()

	if exists {
		return task, nil
	}

	// 使用原始 SQL 查询
	type TaskResult struct {
		ID          string
		TeamID      string
		Title       string
		Description string
		Type        string
		Priority    string
		Status      string
		AssignedTo  string
		CreatedBy   string
		RetryCount  int
		MaxRetries  int
		CreatedAt   time.Time
		UpdatedAt   time.Time
		StartedAt   *time.Time
		CompletedAt *time.Time
		MetadataJSON string
	}

	var result TaskResult
	err := s.db.Raw(`
		SELECT id, team_id, title, description, type, priority, status, assigned_to, created_by,
		       retry_count, max_retries, created_at, updated_at, started_at, completed_at, context_metadata as metadata_json
		FROM tasks WHERE id = ?
	`, taskID).Scan(&result).Error
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}

	taskFromDB := &models.Task{
		ID:          result.ID,
		TeamID:      result.TeamID,
		Title:       result.Title,
		Description: result.Description,
		Type:        result.Type,
		Priority:    result.Priority,
		Status:      result.Status,
		AssignedTo:  result.AssignedTo,
		CreatedBy:   result.CreatedBy,
		RetryCount:  result.RetryCount,
		MaxRetries:  result.MaxRetries,
		CreatedAt:   result.CreatedAt,
		UpdatedAt:   result.UpdatedAt,
		StartedAt:   result.StartedAt,
		CompletedAt: result.CompletedAt,
		Context:     models.TaskContext{},
	}

	// 解析 metadata
	if result.MetadataJSON != "" {
		json.Unmarshal([]byte(result.MetadataJSON), &taskFromDB.Context.Metadata)
	}

	// 添加到活跃任务
	s.taskMutex.Lock()
	s.activeTasks[taskID] = taskFromDB
	s.taskMutex.Unlock()

	return taskFromDB, nil
}

// ListTasks 列出任务
func (s *TeamCoordinatorService) ListTasks(teamID, status, assignedTo string, page, pageSize int) ([]models.Task, int64, error) {
	type TaskResult struct {
		ID         string
		TeamID     string
		Title      string
		Type       string
		Priority   string
		Status     string
		AssignedTo string
		CreatedAt  time.Time
	}

	// 构建查询条件
	whereClause := "WHERE team_id = ?"
	args := []interface{}{teamID}

	if status != "" {
		whereClause += " AND status = ?"
		args = append(args, status)
	}
	if assignedTo != "" {
		whereClause += " AND assigned_to = ?"
		args = append(args, assignedTo)
	}

	// 获取总数
	var total int64
	s.db.Raw("SELECT COUNT(*) FROM tasks "+whereClause, args...).Scan(&total)

	// 获取任务列表
	offset := (page - 1) * pageSize
	var results []TaskResult
	err := s.db.Raw(`
		SELECT id, team_id, title, type, priority, status, assigned_to, created_at
		FROM tasks `+whereClause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, offset)...).Scan(&results).Error
	if err != nil {
		return nil, 0, err
	}

	tasks := make([]models.Task, len(results))
	for i, r := range results {
		tasks[i] = models.Task{
			ID:         r.ID,
			TeamID:     r.TeamID,
			Title:      r.Title,
			Type:       r.Type,
			Priority:   r.Priority,
			Status:     r.Status,
			AssignedTo: r.AssignedTo,
			CreatedAt:  r.CreatedAt,
		}
	}

	return tasks, total, nil
}

// ============ 任务重试 ============

// RetryTask 重试失败的任务
func (s *TeamCoordinatorService) RetryTask(taskID string) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return err
	}

	if task.Status != "failed" {
		return fmt.Errorf("can only retry failed tasks")
	}

	if task.RetryCount >= task.MaxRetries {
		return fmt.Errorf("max retry attempts reached")
	}

	// 计算延迟
	delay := time.Duration(s.defaultRetry.InitialDelay)
	for i := 0; i < task.RetryCount; i++ {
		delay = time.Duration(float64(delay) * s.defaultRetry.BackoffFactor)
		if delay > time.Duration(s.defaultRetry.MaxDelay)*time.Millisecond {
			delay = time.Duration(s.defaultRetry.MaxDelay) * time.Millisecond
		}
	}

	// 异步重试
	go func() {
		time.Sleep(delay)
		s.executeTask(task.ID)
	}()

	// 更新任务状态
	return s.UpdateTaskStatus(taskID, "pending", "")
}

// executeTask 执行任务（内部方法）
func (s *TeamCoordinatorService) executeTask(taskID string) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return err
	}

	task.RetryCount++
	now := time.Now()
	task.StartedAt = &now

	// 更新为进行中
	s.UpdateTaskStatus(taskID, "in_progress", "")

	// 这里应该调用实际的执行器
	// 简化处理：假设执行成功
	success := true

	if success {
		return s.UpdateTaskStatus(taskID, "completed", "")
	} else {
		return s.UpdateTaskStatus(taskID, "failed", "execution failed")
	}
}

// ============ 进度跟踪 ============

// getTasksByTeam 获取团队的所有任务（内部方法）
func (s *TeamCoordinatorService) getTasksByTeam(teamID string) ([]models.Task, error) {
	type TaskResult struct {
		ID          string
		TeamID      string
		Title       string
		Type        string
		Priority    string
		Status      string
		AssignedTo  string
		RetryCount  int
		MaxRetries  int
	}

	var results []TaskResult
	err := s.db.Raw(`
		SELECT id, team_id, title, type, priority, status, assigned_to, retry_count, max_retries
		FROM tasks WHERE team_id = ?
	`, teamID).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	tasks := make([]models.Task, len(results))
	for i, r := range results {
		tasks[i] = models.Task{
			ID:         r.ID,
			TeamID:     r.TeamID,
			Title:      r.Title,
			Type:       r.Type,
			Priority:   r.Priority,
			Status:     r.Status,
			AssignedTo: r.AssignedTo,
			RetryCount: r.RetryCount,
			MaxRetries: r.MaxRetries,
		}
	}

	return tasks, nil
}

// TrackProgress 跟踪所有任务进度
func (s *TeamCoordinatorService) TrackProgress(teamID string) (*ProgressData, error) {
	// 获取所有任务
	tasks, err := s.getTasksByTeam(teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tasks: %w", err)
	}

	// 获取成员信息
	members, err := s.ListMembers(teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch members: %w", err)
	}

	progress := &ProgressData{
		TasksByStatus:   make(map[string]int),
		TasksByMember:   make(map[string]*MemberProgress),
		TasksByPriority: make(map[string]int),
		StartedAt:       time.Now(),
	}

	// 初始化成员进度
	for _, member := range members {
		progress.TasksByMember[member.ID] = &MemberProgress{
			MemberID:   member.ID,
			MemberName: member.Name,
			Status:     s.GetMemberStatus(member.ID),
		}
	}

	// 获取任务进度记录
	taskProgressMap := make(map[string][]models.TaskProgress)
	for _, task := range tasks {
		var progresses []models.TaskProgress
		s.db.Where("task_id = ?", task.ID).Order("created_at DESC").Find(&progresses)
		taskProgressMap[task.ID] = progresses
	}

	// 统计任务
	for _, task := range tasks {
		progress.TotalTasks++
		progress.TasksByStatus[task.Status]++
		progress.TasksByPriority[task.Priority]++

		if task.Status == "completed" {
			progress.CompletedTasks++
		} else if task.Status == "failed" {
			progress.FailedTasks++
		} else if task.Status == "pending" {
			progress.PendingTasks++
		} else if task.Status == "in_progress" {
			progress.InProgressTasks++
		}

		// 成员统计
		if task.AssignedTo != "" {
			if memberProgress, ok := progress.TasksByMember[task.AssignedTo]; ok {
				memberProgress.AssignedTasks++
				if task.Status == "completed" {
					memberProgress.CompletedTasks++
				} else if task.Status == "failed" {
					memberProgress.FailedTasks++
				}
				if task.Status == "in_progress" {
					memberProgress.CurrentTask = task.Title
				}
			}
		}

		// 计算总体进度（从任务进度记录）
		if progresses, ok := taskProgressMap[task.ID]; ok && len(progresses) > 0 {
			progress.OverallProgress += float64(progresses[0].Percent)
		}
	}

	if progress.TotalTasks > 0 {
		progress.OverallProgress /= float64(progress.TotalTasks)
	}

	// 计算成员平均进度
	for _, memberProgress := range progress.TasksByMember {
		if memberProgress.AssignedTasks > 0 {
			var totalProgress int
			var count int
			for _, task := range tasks {
				if task.AssignedTo == memberProgress.MemberID {
					if progresses, ok := taskProgressMap[task.ID]; ok && len(progresses) > 0 {
						totalProgress += progresses[0].Percent
						count++
					}
				}
			}
			if count > 0 {
				memberProgress.AverageProgress = float64(totalProgress) / float64(count)
			}
		}
	}

	return progress, nil
}

// GenerateReport 生成进度报告
func (s *TeamCoordinatorService) GenerateReport(teamID, generatedBy string, periodStart, periodEnd time.Time) (*models.ProgressReport, error) {
	// 获取团队信息
	team, err := s.GetTeam(teamID)
	if err != nil {
		return nil, err
	}

	// 获取进度数据
	progress, err := s.TrackProgress(teamID)
	if err != nil {
		return nil, err
	}

	// 获取时间范围内的任务
	tasks, err := s.getTasksByTeam(teamID)
	if err != nil {
		return nil, err
	}

	// 过滤时间范围
	var periodTasks []models.Task
	for _, task := range tasks {
		if (task.CreatedAt.Equal(periodStart) || task.CreatedAt.After(periodStart)) &&
		   (task.CreatedAt.Equal(periodEnd) || task.CreatedAt.Before(periodEnd)) {
			periodTasks = append(periodTasks, task)
		}
	}

	// 构建任务状态摘要
	taskStatus := models.TaskStatusSummary{
		Total:      progress.TotalTasks,
		Pending:    progress.PendingTasks,
		InProgress: progress.InProgressTasks,
		Completed:  progress.CompletedTasks,
		Failed:     progress.FailedTasks,
		Cancelled:  progress.TasksByStatus["cancelled"],
	}

	// 构建成员统计
	var memberStats []models.MemberStat
	for _, memberProgress := range progress.TasksByMember {
		memberStats = append(memberStats, models.MemberStat{
			MemberID:        memberProgress.MemberID,
			MemberName:      memberProgress.MemberName,
			TasksAssigned:   memberProgress.AssignedTasks,
			TasksCompleted:  memberProgress.CompletedTasks,
			TasksFailed:     memberProgress.FailedTasks,
			AverageProgress: memberProgress.AverageProgress,
		})
	}

	// 构建统计信息
	var completionRate float64
	if progress.TotalTasks > 0 {
		completionRate = float64(progress.CompletedTasks) / float64(progress.TotalTasks) * 100
	}

	statistics := map[string]interface{}{
		"total_duration":    time.Since(periodStart).String(),
		"tasks_by_status":   progress.TasksByStatus,
		"tasks_by_priority": progress.TasksByPriority,
		"overall_progress":  progress.OverallProgress,
		"completion_rate":   completionRate,
	}

	// 生成摘要
	summary := fmt.Sprintf("团队 '%s' 进度报告 (%s 至 %s)\n",
		team.Name,
		periodStart.Format("2006-01-02"),
		periodEnd.Format("2006-01-02"))
	summary += fmt.Sprintf("总任务数: %d, 已完成: %d, 进行中: %d, 失败: %d\n",
		progress.TotalTasks, progress.CompletedTasks,
		progress.InProgressTasks, progress.FailedTasks)
	summary += fmt.Sprintf("总体进度: %.2f%%, 完成率: %.2f%%\n", progress.OverallProgress, completionRate)

	report := &models.ProgressReport{
		ID:          uuid.New().String(),
		TeamID:      teamID,
		GeneratedBy: generatedBy,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Summary:     summary,
		Statistics:  statistics,
		TaskStatus:  taskStatus,
		MemberStats: memberStats,
		CreatedAt:   time.Now(),
	}

	// 保存报告
	if err := s.db.Create(report).Error; err != nil {
		return nil, fmt.Errorf("failed to save report: %w", err)
	}

	return report, nil
}

// GetReport 获取报告
func (s *TeamCoordinatorService) GetReport(reportID string) (*models.ProgressReport, error) {
	var report models.ProgressReport
	err := s.db.Where("id = ?", reportID).First(&report).Error
	if err != nil {
		return nil, fmt.Errorf("report not found: %w", err)
	}
	return &report, nil
}

// ListReports 列出报告
func (s *TeamCoordinatorService) ListReports(teamID string, page, pageSize int) ([]models.ProgressReport, int64, error) {
	var reports []models.ProgressReport
	var total int64

	query := s.db.Model(&models.ProgressReport{}).Where("team_id = ?", teamID)
	query.Count(&total)

	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&reports).Error

	return reports, total, err
}

// ============ 成员通知 ============

// NotifyMember 通知成员
func (s *TeamCoordinatorService) NotifyMember(teamID, memberID, notificationType string, data map[string]interface{}) error {
	return s.notifyMember(teamID, memberID, notificationType, data)
}

// notifyMember 内部通知方法
func (s *TeamCoordinatorService) notifyMember(teamID, memberID, notificationType string, data map[string]interface{}) error {
	message := &models.TeamMessage{
		ID:        uuid.New().String(),
		TeamID:    teamID,
		From:      "coordinator",
		ToMember:  memberID,
		Type:      notificationType,
		Content:   data,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(message).Error; err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}

	// 发布消息事件
	s.publishEvent(teamID, models.TeamEvent{
		ID:        uuid.New().String(),
		TeamID:    teamID,
		Type:      "message_sent",
		Source:    "coordinator",
		Target:    memberID,
		Data:      map[string]interface{}{"message_id": message.ID, "type": notificationType},
		CreatedAt: time.Now(),
	})

	return nil
}

// BroadcastMessage 广播消息给所有成员
func (s *TeamCoordinatorService) BroadcastMessage(teamID, from string, messageType string, content map[string]interface{}) error {
	members, err := s.ListMembers(teamID)
	if err != nil {
		return err
	}

	for _, member := range members {
		message := &models.TeamMessage{
			ID:        uuid.New().String(),
			TeamID:    teamID,
			From:      from,
			ToMember:  member.ID,
			Type:      messageType,
			Content:   content,
			Status:    "pending",
			CreatedAt: time.Now(),
		}
		s.db.Create(message)
	}

	return nil
}

// GetMessage 获取消息
func (s *TeamCoordinatorService) GetMessage(messageID string) (*models.TeamMessage, error) {
	var message models.TeamMessage
	err := s.db.Where("id = ?", messageID).First(&message).Error
	if err != nil {
		return nil, fmt.Errorf("message not found: %w", err)
	}
	return &message, nil
}

// ListMessages 列出消息
func (s *TeamCoordinatorService) ListMessages(teamID, memberID string, page, pageSize int) ([]models.TeamMessage, int64, error) {
	var messages []models.TeamMessage
	var total int64

	query := s.db.Model(&models.TeamMessage{}).Where("team_id = ?", teamID)
	if memberID != "" {
		query = query.Where("to_member = ?", memberID)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&messages).Error

	return messages, total, err
}

// MarkMessageRead 标记消息为已读
func (s *TeamCoordinatorService) MarkMessageRead(messageID string) error {
	now := time.Now()
	return s.db.Model(&models.TeamMessage{}).
		Where("id = ?", messageID).
		Updates(map[string]interface{}{
			"status":  "read",
			"read_at": &now,
		}).Error
}

// ============ 团队配置保存/恢复 ============

// SaveTeamConfig 保存团队配置（用于恢复）
func (s *TeamCoordinatorService) SaveTeamConfig(teamID string) (*models.TeamConfig, error) {
	// 获取团队信息
	team, err := s.GetTeam(teamID)
	if err != nil {
		return nil, err
	}

	// 获取成员
	members, err := s.ListMembers(teamID)
	if err != nil {
		return nil, err
	}

	// 获取活动任务
	tasks, _ := s.getTasksByTeam(teamID)
	var activeTasks []models.Task
	for _, task := range tasks {
		if task.Status == "pending" || task.Status == "in_progress" {
			activeTasks = append(activeTasks, task)
		}
	}

	config := &models.TeamConfig{
		Team:        *team,
		Members:     members,
		Tasks:       activeTasks,
		RetryConfig: s.defaultRetry,
		Version:     "1.0",
		ExportedAt:  time.Now(),
	}

	return config, nil
}

// LoadTeamConfig 从配置恢复团队
func (s *TeamCoordinatorService) LoadTeamConfig(config *models.TeamConfig) (*models.Team, error) {
	// 创建团队
	team := &models.Team{
		ID:          uuid.New().String(),
		Name:        config.Team.Name,
		Description: config.Team.Description,
		Enabled:     config.Team.Enabled,
		Config:      config.Team.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.db.Create(team).Error; err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	// 恢复成员
	for _, member := range config.Members {
		newMember := member
		newMember.ID = uuid.New().String()
		newMember.TeamID = team.ID
		newMember.CreatedAt = time.Now()
		newMember.UpdatedAt = time.Now()

		if err := s.db.Create(&newMember).Error; err != nil {
			return nil, fmt.Errorf("failed to restore member: %w", err)
		}

		// 更新内存状态
		s.memberStatus[newMember.ID] = newMember.Availability
	}

	// 恢复任务
	for _, task := range config.Tasks {
		newTaskID := uuid.New().String()
		now := time.Now()

		// 使用原始 SQL 插入
		err := s.db.Exec(`
			INSERT INTO tasks (id, team_id, title, description, type, priority, status,
			    retry_count, max_retries, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, newTaskID, team.ID, task.Title, task.Description, task.Type, task.Priority,
		   "pending", 0, task.MaxRetries, now, now).Error
		if err != nil {
			return nil, fmt.Errorf("failed to restore task: %w", err)
		}

		// 创建任务对象并添加到活跃任务
		newTask := &models.Task{
			ID:         newTaskID,
			TeamID:     team.ID,
			Title:      task.Title,
			Type:       task.Type,
			Priority:   task.Priority,
			Status:     "pending",
			RetryCount: 0,
			MaxRetries: task.MaxRetries,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		s.activeTasks[newTask.ID] = newTask
	}

	return team, nil
}

// ExportTeamConfig 导出团队配置为JSON
func (s *TeamCoordinatorService) ExportTeamConfig(teamID string) ([]byte, error) {
	config, err := s.SaveTeamConfig(teamID)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(config, "", "  ")
}

// ImportTeamConfig 从JSON导入团队配置
func (s *TeamCoordinatorService) ImportTeamConfig(data []byte) (*models.Team, error) {
	var config models.TeamConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return s.LoadTeamConfig(&config)
}

// ============ 事件处理 ============

// SubscribeToEvents 订阅团队事件
func (s *TeamCoordinatorService) SubscribeToEvents(teamID string) chan models.TeamEvent {
	s.messageMutex.Lock()
	defer s.messageMutex.Unlock()

	ch := make(chan models.TeamEvent, 100)
	if s.eventChannels[teamID] == nil {
		s.eventChannels[teamID] = make([]chan models.TeamEvent, 0)
	}
	s.eventChannels[teamID] = append(s.eventChannels[teamID], ch)

	return ch
}

// UnsubscribeFromEvents 取消订阅事件
func (s *TeamCoordinatorService) UnsubscribeFromEvents(teamID string, ch chan models.TeamEvent) {
	s.messageMutex.Lock()
	defer s.messageMutex.Unlock()

	channels := s.eventChannels[teamID]
	for i, c := range channels {
		if c == ch {
			s.eventChannels[teamID] = append(channels[:i], channels[i+1:]...)
			close(ch)
			break
		}
	}
}

// publishEvent 发布事件
func (s *TeamCoordinatorService) publishEvent(teamID string, event models.TeamEvent) {
	s.messageMutex.RLock()
	channels := s.eventChannels[teamID]
	s.messageMutex.RUnlock()

	for _, ch := range channels {
		select {
		case ch <- event:
		default:
			// 通道满了，跳过
		}
	}

	// 保存事件到数据库
	s.db.Create(&event)
}

// GetEvents 获取事件历史
func (s *TeamCoordinatorService) GetEvents(teamID string, eventType string, limit int) ([]models.TeamEvent, error) {
	var events []models.TeamEvent

	query := s.db.Where("team_id = ?", teamID)
	if eventType != "" {
		query = query.Where("type = ?", eventType)
	}

	err := query.Order("created_at DESC").Limit(limit).Find(&events).Error
	return events, err
}

// startEventHandler 启动事件处理器
func (s *TeamCoordinatorService) startEventHandler() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			s.processEventHandlers()
		}
	}()
}

// processEventHandlers 处理事件处理器
func (s *TeamCoordinatorService) processEventHandlers() {
	// 检查活跃任务中的失败任务
	s.taskMutex.RLock()
	var failedTaskIDs []string
	for _, task := range s.activeTasks {
		if task.Status == "failed" && task.RetryCount < task.MaxRetries {
			failedTaskIDs = append(failedTaskIDs, task.ID)
		}
	}
	s.taskMutex.RUnlock()

	// 重试失败的任务
	for _, taskID := range failedTaskIDs {
		if task, exists := s.activeTasks[taskID]; exists {
			if s.shouldRetryTask(task) {
				s.RetryTask(taskID)
			}
		}
	}
}

// ============ 辅助方法 ============

// hasActiveTasks 检查成员是否有活动任务
func (s *TeamCoordinatorService) hasActiveTasks(memberID string) bool {
	// 使用原始 SQL 来避免 GORM 处理切片字段的问题
	var count int64
	s.db.Raw(`
		SELECT COUNT(*) FROM tasks
		WHERE assigned_to = ? AND status IN ('pending', 'in_progress')
	`, memberID).Scan(&count)
	return count > 0
}

// shouldRetryTask 判断任务是否应该重试
func (s *TeamCoordinatorService) shouldRetryTask(task *models.Task) bool {
	if task.RetryCount >= task.MaxRetries {
		return false
	}

	// 检查错误是否可重试
	if task.Result != nil && task.Result.Error != "" {
		for _, retryableError := range s.defaultRetry.RetryableErrors {
			if containsCoord(task.Result.Error, retryableError) {
				return true
			}
		}
	}

	return false
}

// containsCoord 检查字符串是否包含子串
func containsCoord(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && indexOfCoord(s, substr) >= 0)
}

// indexOfCoord 查找子串位置
func indexOfCoord(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ============ 服务控制 ============

// Start 启动服务
func (s *TeamCoordinatorService) Start(ctx context.Context) error {
	// 加载活跃任务
	s.loadActiveTasks()

	// 启动事件监听
	go s.monitorEvents(ctx)

	return nil
}

// loadActiveTasks 加载活跃任务
func (s *TeamCoordinatorService) loadActiveTasks() {
	// 从数据库获取所有团队
	var teams []models.Team
	s.db.Find(&teams)

	// 加载每个团队的活跃任务
	for _, team := range teams {
		tasks, _ := s.getTasksByTeam(team.ID)
		for i := range tasks {
			if tasks[i].Status == "pending" || tasks[i].Status == "in_progress" {
				s.activeTasks[tasks[i].ID] = &tasks[i]
			}
		}
	}
}

// monitorEvents 监控事件
func (s *TeamCoordinatorService) monitorEvents(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAndUpdateMemberStatus()
		}
	}
}

// checkAndUpdateMemberStatus 检查并更新成员状态
func (s *TeamCoordinatorService) checkAndUpdateMemberStatus() {
	s.taskMutex.RLock()
	defer s.taskMutex.RUnlock()

	// 更新成员状态基于任务分配
	memberTaskCount := make(map[string]int)
	for _, task := range s.activeTasks {
		if task.AssignedTo != "" && task.Status == "in_progress" {
			memberTaskCount[task.AssignedTo]++
		}
	}

	for memberID, count := range memberTaskCount {
		status := "available"
		if count > 0 {
			status = "busy"
		}
		s.UpdateMemberStatus(memberID, status)
	}
}

// ============ Agent 管理增强 ============

// RegisterAgent 注册 Agent
func (s *TeamCoordinatorService) RegisterAgent(agentID, name, agentType string, capabilities []string, config map[string]interface{}) error {
	s.memberMutex.Lock()
	defer s.memberMutex.Unlock()

	// 检查是否已注册
	if _, exists := s.agentRegistry[agentID]; exists {
		return fmt.Errorf("agent already registered: %s", agentID)
	}

	// 获取 LLM 配置
	llmProvider, _ := config["llm_provider"].(string)
	if llmProvider == "" {
		llmProvider = "openai" // 默认
	}
	llmModel, _ := config["llm_model"].(string)
	if llmModel == "" {
		llmModel = "gpt-4"
	}

	agent := &AgentInfo{
		ID:             agentID,
		Name:           name,
		Type:           agentType,
		Capabilities:   capabilities,
		Status:         "online",
		CurrentLoad:    0,
		MaxLoad:        3,
		Specialization: []string{},
		LastHeartbeat:  time.Now(),
		Config:         config,
		LLMProvider:    llmProvider,
		LLMModel:       llmModel,
	}

	s.agentRegistry[agentID] = agent

	// 更新能力索引
	for _, cap := range capabilities {
		s.capabilityIndex[cap] = append(s.capabilityIndex[cap], agentID)
	}

	// 初始化心跳
	s.heartbeats[agentID] = time.Now()

	// 发布 Agent 注册事件
	s.publishEvent("system", models.TeamEvent{
		ID:        uuid.New().String(),
		TeamID:    "system",
		Type:      "agent_registered",
		Source:    "coordinator",
		Data:      map[string]interface{}{"agent_id": agentID, "name": name},
		CreatedAt: time.Now(),
	})

	return nil
}

// UnregisterAgent 注销 Agent
func (s *TeamCoordinatorService) UnregisterAgent(agentID string) error {
	s.memberMutex.Lock()
	defer s.memberMutex.Unlock()

	agent, exists := s.agentRegistry[agentID]
	if !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	// 检查是否有活跃任务
	if agent.CurrentLoad > 0 {
		return fmt.Errorf("cannot unregister agent with active tasks")
	}

	// 从能力索引中移除
	for _, cap := range agent.Capabilities {
		agents := s.capabilityIndex[cap]
		for i, id := range agents {
			if id == agentID {
				s.capabilityIndex[cap] = append(agents[:i], agents[i+1:]...)
				break
			}
		}
	}

	delete(s.agentRegistry, agentID)
	delete(s.heartbeats, agentID)

	// 发布 Agent 注销事件
	s.publishEvent("system", models.TeamEvent{
		ID:        uuid.New().String(),
		TeamID:    "system",
		Type:      "agent_unregistered",
		Source:    "coordinator",
		Data:      map[string]interface{}{"agent_id": agentID},
		CreatedAt: time.Now(),
	})

	return nil
}

// UpdateHeartbeat 更新心跳
func (s *TeamCoordinatorService) UpdateHeartbeat(agentID string) error {
	s.heartbeatMutex.Lock()
	defer s.heartbeatMutex.Unlock()

	if _, exists := s.agentRegistry[agentID]; !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	s.heartbeats[agentID] = time.Now()

	// 更新 Agent 最后活跃时间
	if agent, ok := s.agentRegistry[agentID]; ok {
		agent.LastHeartbeat = time.Now()
	}

	return nil
}

// heartbeatMonitor 心跳监控
func (s *TeamCoordinatorService) heartbeatMonitor() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkHeartbeats()
		}
	}
}

// checkHeartbeats 检查心跳超时
func (s *TeamCoordinatorService) checkHeartbeats() {
	s.heartbeatMutex.RLock()
	defer s.heartbeatMutex.RUnlock()

	timeout := 2 * time.Minute // 2分钟超时
	now := time.Now()

	for agentID, lastHeartbeat := range s.heartbeats {
		if now.Sub(lastHeartbeat) > timeout {
			// Agent 心跳超时，标记为离线
			if agent, ok := s.agentRegistry[agentID]; ok {
				agent.Status = "offline"
				s.memberStatus[agentID] = "offline"

				// 发布离线事件
				s.publishEvent("system", models.TeamEvent{
					ID:        uuid.New().String(),
					TeamID:    "system",
					Type:      "agent_offline",
					Source:    "coordinator",
					Data:      map[string]interface{}{"agent_id": agentID},
					CreatedAt: time.Now(),
				})
			}
		}
	}
}

// GetAgentsByCapability 根据能力查找 Agent
func (s *TeamCoordinatorService) GetAgentsByCapability(capability string) []*AgentInfo {
	s.memberMutex.RLock()
	defer s.memberMutex.RUnlock()

	var agents []*AgentInfo
	for _, agentID := range s.capabilityIndex[capability] {
		if agent, ok := s.agentRegistry[agentID]; ok {
			if agent.Status == "online" && agent.CurrentLoad < agent.MaxLoad {
				agents = append(agents, agent)
			}
		}
	}

	return agents
}

// GetAgentStatus 获取 Agent 状态
func (s *TeamCoordinatorService) GetAgentStatus(agentID string) (*AgentInfo, error) {
	s.memberMutex.RLock()
	defer s.memberMutex.RUnlock()

	agent, ok := s.agentRegistry[agentID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	return agent, nil
}

// ListAgents 列出所有 Agent
func (s *TeamCoordinatorService) ListAgents() []*AgentInfo {
	s.memberMutex.RLock()
	defer s.memberMutex.RUnlock()

	agents := make([]*AgentInfo, 0, len(s.agentRegistry))
	for _, agent := range s.agentRegistry {
		agents = append(agents, agent)
	}

	return agents
}

// ============ 任务协调增强 ============

// DecomposeTask 分解任务
func (s *TeamCoordinatorService) DecomposeTask(taskID string) ([]*models.Subtask, error) {
	s.taskMutex.Lock()
	task, exists := s.activeTasks[taskID]
	s.taskMutex.Unlock()

	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	// 根据任务类型获取分解模板
	var subtasks []*models.Subtask

	switch task.Type {
	case "coding":
		subtasks = s.decomposeCodingTask(task)
	case "research":
		subtasks = s.decomposeResearchTask(task)
	case "testing":
		subtasks = s.decomposeTestingTask(task)
	default:
		subtasks = s.decomposeGenericTask(task)
	}

	// 保存子任务到数据库
	for _, st := range subtasks {
		if err := s.db.Create(st).Error; err != nil {
			return nil, fmt.Errorf("failed to save subtask: %w", err)
		}
	}

	// 发布任务分解事件
	s.publishEvent(task.TeamID, models.TeamEvent{
		ID:        uuid.New().String(),
		TeamID:    task.TeamID,
		Type:      "task_decomposed",
		Source:    "coordinator",
		Data:      map[string]interface{}{"task_id": taskID, "subtask_count": len(subtasks)},
		CreatedAt: time.Now(),
	})

	return subtasks, nil
}

// decomposeCodingTask 分解编码任务
func (s *TeamCoordinatorService) decomposeCodingTask(task *models.Task) []*models.Subtask {
	return []*models.Subtask{
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "需求分析",
			Description:  "分析任务需求和约束条件",
			Type:         "research",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        1,
			Estimated:    30,
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "设计方案",
			Description:  "设计技术方案和实现路径",
			Type:         "plan",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        2,
			Estimated:    60,
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "实现开发",
			Description:  "编写代码实现功能",
			Type:         "coding",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        3,
			Estimated:    120,
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "代码审查",
			Description:  "审查代码质量和规范性",
			Type:         "review",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        4,
			Estimated:    30,
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "测试验证",
			Description:  "编写测试并验证功能",
			Type:         "testing",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        5,
			Estimated:    60,
			CreatedAt:    time.Now(),
		},
	}
}

// decomposeResearchTask 分解研究任务
func (s *TeamCoordinatorService) decomposeResearchTask(task *models.Task) []*models.Subtask {
	return []*models.Subtask{
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "资料收集",
			Description:  "收集相关资料和背景信息",
			Type:         "research",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        1,
			Estimated:    60,
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "信息分析",
			Description:  "分析收集的信息",
			Type:         "research",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        2,
			Estimated:    90,
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "报告撰写",
			Description:  "撰写研究报告",
			Type:         "research",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        3,
			Estimated:    60,
			CreatedAt:    time.Now(),
		},
	}
}

// decomposeTestingTask 分解测试任务
func (s *TeamCoordinatorService) decomposeTestingTask(task *models.Task) []*models.Subtask {
	return []*models.Subtask{
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "测试计划",
			Description:  "制定测试计划和策略",
			Type:         "plan",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        1,
			Estimated:    30,
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "用例设计",
			Description:  "设计测试用例",
			Type:         "testing",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        2,
			Estimated:    60,
			CreatedAt:    time.Now(),
		},
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "测试执行",
			Description:  "执行测试用例",
			Type:         "testing",
			Priority:     task.Priority,
			Status:       "pending",
			Order:        3,
			Estimated:    120,
			CreatedAt:    time.Now(),
		},
	}
}

// decomposeGenericTask 分解通用任务
func (s *TeamCoordinatorService) decomposeGenericTask(task *models.Task) []*models.Subtask {
	return []*models.Subtask{
		{
			ID:           uuid.New().String(),
			ParentTaskID: task.ID,
			Title:        "任务执行",
			Description:  task.Description,
			Type:         task.Type,
			Priority:     task.Priority,
			Status:       "pending",
			Order:        1,
			Estimated:    task.Estimated,
			CreatedAt:    time.Now(),
		},
	}
}

// AssignTaskWithStrategy 使用策略分配任务
func (s *TeamCoordinatorService) AssignTaskWithStrategy(taskID, strategy string) error {
	s.taskMutex.Lock()
	task, exists := s.activeTasks[taskID]
	if !exists {
		s.taskMutex.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	s.taskMutex.Unlock()

	// 获取团队成员
	members, err := s.ListMembers(task.TeamID)
	if err != nil {
		return err
	}

	// 获取分配策略
	assignmentStrategy, ok := s.taskScheduler.strategies[strategy]
	if !ok {
		// 默认使用负载均衡策略
		assignmentStrategy = s.taskScheduler.strategies["load_balanced"]
	}

	// 分配任务
	assignedMemberID, err := assignmentStrategy.Assign(task, members)
	if err != nil {
		return err
	}

	// 更新任务分配
	return s.AssignTask(taskID, assignedMemberID)
}

// AddDependency 添加任务依赖
func (s *TeamCoordinatorService) AddDependency(taskID, dependsOnTaskID, depType string) error {
	s.dependencyGraph.mutex.Lock()
	defer s.dependencyGraph.mutex.Unlock()

	// 检查循环依赖
	if s.hasCycle(taskID, dependsOnTaskID) {
		return fmt.Errorf("adding dependency would create a cycle")
	}

	// 添加依赖
	s.dependencyGraph.graph[dependsOnTaskID] = append(s.dependencyGraph.graph[dependsOnTaskID], taskID)

	// 保存到数据库
	dependency := &models.TaskDependency{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		DependsOn: dependsOnTaskID,
		Type:      depType,
		CreatedAt: time.Now(),
	}
	return s.db.Create(dependency).Error
}

// hasCycle 检查是否会形成循环
func (s *TeamCoordinatorService) hasCycle(from, to string) bool {
	visited := make(map[string]bool)
	return s.hasCycleDFS(to, from, visited)
}

// hasCycleDFS DFS 检查循环
func (s *TeamCoordinatorService) hasCycleDFS(current, target string, visited map[string]bool) bool {
	if current == target {
		return true
	}
	if visited[current] {
		return false
	}
	visited[current] = true

	for _, dependent := range s.dependencyGraph.graph[current] {
		if s.hasCycleDFS(dependent, target, visited) {
			return true
		}
	}

	return false
}

// GetReadyTasks 获取可以执行的任务（依赖已满足）
func (s *TeamCoordinatorService) GetReadyTasks(teamID string) ([]*models.Task, error) {
	s.dependencyGraph.mutex.RLock()
	defer s.dependencyGraph.mutex.RUnlock()

	s.taskMutex.RLock()
	defer s.taskMutex.RUnlock()

	var readyTasks []*models.Task

	for _, task := range s.activeTasks {
		if task.Status != "pending" {
			continue
		}

		// 检查所有依赖是否已完成
		if s.areDependenciesComplete(task.ID) {
			readyTasks = append(readyTasks, task)
		}
	}

	return readyTasks, nil
}

// areDependenciesComplete 检查依赖是否完成
func (s *TeamCoordinatorService) areDependenciesComplete(taskID string) bool {
	// 获取任务的依赖
	var dependencies []models.TaskDependency
	s.db.Where("task_id = ?", taskID).Find(&dependencies)

	for _, dep := range dependencies {
		// 检查依赖任务是否完成
		var depTask models.Task
		if err := s.db.Where("id = ?", dep.DependsOn).First(&depTask).Error; err == nil {
			if depTask.Status != "completed" {
				return false
			}
		}
	}

	return true
}

// dependencyResolver 依赖解析器
func (s *TeamCoordinatorService) dependencyResolver() {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.resolvePendingTasks()
		}
	}
}

// resolvePendingTasks 解析待处理任务
func (s *TeamCoordinatorService) resolvePendingTasks() {
	// 获取所有团队
	teams, err := s.ListTeams()
	if err != nil {
		return
	}

	for _, team := range teams {
		// 获取就绪任务
		readyTasks, _ := s.GetReadyTasks(team.ID)

		// 自动分配就绪任务
		for _, task := range readyTasks {
			if task.AssignedTo == "" {
				s.AssignTaskWithStrategy(task.ID, "load_balanced")
			}
		}
	}
}

// AggregateResults 聚合任务结果
func (s *TeamCoordinatorService) AggregateResults(taskID string) (*TaskResult, error) {
	s.resultAggregator.mutex.RLock()
	defer s.resultAggregator.mutex.RUnlock()

	// 获取子任务结果
	var subtasks []models.Subtask
	s.db.Where("parent_task_id = ?", taskID).Find(&subtasks)

	aggregated := &TaskResult{
		TaskID:      taskID,
		Success:     true,
		Output:      map[string]interface{}{},
		Metrics:     make(map[string]interface{}),
		CompletedAt: time.Now(),
	}

	successCount := 0
	failCount := 0

	for _, st := range subtasks {
		if st.Status == "completed" {
			successCount++
		} else if st.Status == "failed" {
			failCount++
			aggregated.Success = false
		}
	}

	aggregated.Metrics["total_subtasks"] = len(subtasks)
	aggregated.Metrics["completed"] = successCount
	aggregated.Metrics["failed"] = failCount
	aggregated.Metrics["success_rate"] = float64(successCount) / float64(len(subtasks)) * 100

	return aggregated, nil
}

// ============ 通信机制增强 ============

// Subscribe 订阅消息主题
func (s *TeamCoordinatorService) Subscribe(topic string) chan Message {
	s.messageBus.mu.Lock()
	defer s.messageBus.mu.Unlock()

	ch := make(chan Message, 100)
	s.messageBus.topics[topic] = append(s.messageBus.topics[topic], ch)

	return ch
}

// Unsubscribe 取消订阅
func (s *TeamCoordinatorService) Unsubscribe(topic string, ch chan Message) {
	s.messageBus.mu.Lock()
	defer s.messageBus.mu.Unlock()

	channels := s.messageBus.topics[topic]
	for i, c := range channels {
		if c == ch {
			s.messageBus.topics[topic] = append(channels[:i], channels[i+1:]...)
			close(ch)
			break
		}
	}
}

// Publish 发布消息
func (s *TeamCoordinatorService) Publish(topic string, msg Message) {
	s.messageBus.mu.RLock()
	defer s.messageBus.mu.RUnlock()

	msg.ID = uuid.New().String()
	msg.Timestamp = time.Now()

	channels := s.messageBus.topics[topic]
	for _, ch := range channels {
		select {
		case ch <- msg:
		default:
			// 通道满，跳过
		}
	}

	// 广播到通配符订阅
	if wildcardChannels, ok := s.messageBus.topics["*"]; ok {
		for _, ch := range wildcardChannels {
			select {
			case ch <- msg:
			default:
			}
		}
	}
}

// SendMessage 发送消息给 Agent
func (s *TeamCoordinatorService) SendMessage(from, to, messageType string, content interface{}) error {
	msg := Message{
		Topic:   fmt.Sprintf("agent.%s", to),
		From:    from,
		To:      to,
		Type:    messageType,
		Content: content,
	}

	s.Publish(msg.Topic, msg)

	return nil
}

// Broadcast 广播消息
func (s *TeamCoordinatorService) Broadcast(from, topic, messageType string, content interface{}) error {
	msg := Message{
		Topic:   topic,
		From:    from,
		Type:    messageType,
		Content: content,
	}

	s.Publish(topic, msg)

	return nil
}

// ============ LLM 集成 ============

// SetLLMClient 设置 Agent 的 LLM 客户端
func (s *TeamCoordinatorService) SetLLMClient(agentID string, client LLMClient) error {
	s.memberMutex.Lock()
	defer s.memberMutex.Unlock()

	if _, exists := s.agentRegistry[agentID]; !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	s.llmClients[agentID] = client
	return nil
}

// GenerateWithLLM 使用 LLM 生成内容
func (s *TeamCoordinatorService) GenerateWithLLM(agentID, taskID string, prompt string, options map[string]interface{}) (string, error) {
	s.memberMutex.RLock()
	_, ok := s.agentRegistry[agentID]
	s.memberMutex.RUnlock()

	if !ok {
		return "", fmt.Errorf("agent not found: %s", agentID)
	}

	// 获取 LLM 客户端
	client, ok := s.llmClients[agentID]
	if !ok {
		return "", fmt.Errorf("LLM client not configured for agent: %s", agentID)
	}

	// 添加角色提示词
	role, _ := s.getMemberRole(agentID)
	if rolePrompt, exists := s.rolePrompts[role]; exists {
		prompt = rolePrompt + "\n\n" + prompt
	}

	// 生成内容
	return client.Generate(context.Background(), prompt, options)
}

// GenerateWithLLMStream 使用 LLM 流式生成
func (s *TeamCoordinatorService) GenerateWithLLMStream(agentID, taskID string, prompt string, options map[string]interface{}) (<-chan string, error) {
	s.memberMutex.RLock()
	_, ok := s.agentRegistry[agentID]
	s.memberMutex.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	client, ok := s.llmClients[agentID]
	if !ok {
		return nil, fmt.Errorf("LLM client not configured for agent: %s", agentID)
	}

	// 添加角色提示词
	role, _ := s.getMemberRole(agentID)
	if rolePrompt, exists := s.rolePrompts[role]; exists {
		prompt = rolePrompt + "\n\n" + prompt
	}

	return client.GenerateStream(context.Background(), prompt, options)
}

// getMemberRole 获取成员角色
func (s *TeamCoordinatorService) getMemberRole(memberID string) (string, error) {
	var member models.TeamMember
	err := s.db.Where("id = ?", memberID).First(&member).Error
	if err != nil {
		return "", err
	}
	return member.Role, nil
}

// SetRolePrompt 设置角色提示词
func (s *TeamCoordinatorService) SetRolePrompt(role, prompt string) {
	s.rolePrompts[role] = prompt
}

// ============ 任务分配策略实现 ============

// LoadBalancedStrategy 负载均衡策略
type LoadBalancedStrategy struct {
	service *TeamCoordinatorService
}

func (s *LoadBalancedStrategy) Assign(task *models.Task, members []models.TeamMember) (string, error) {
	var selectedMember *models.TeamMember
	minLoad := int(^uint(0) >> 1)

	for _, member := range members {
		// 从 Agent 注册表获取负载
		s.service.memberMutex.RLock()
		agent, exists := s.service.agentRegistry[member.ID]
		s.service.memberMutex.RUnlock()

		load := member.CurrentLoad
		if exists {
			load = agent.CurrentLoad
		}

		if load < minLoad && load < (member.MaxLoad) {
			minLoad = load
			selectedMember = &member
		}
	}

	if selectedMember == nil {
		return "", fmt.Errorf("no available member")
	}

	return selectedMember.ID, nil
}

func (s *LoadBalancedStrategy) Name() string {
	return "load_balanced"
}

// CapabilityBasedStrategy 基于能力的策略
type CapabilityBasedStrategy struct {
	service *TeamCoordinatorService
}

func (s *CapabilityBasedStrategy) Assign(task *models.Task, members []models.TeamMember) (string, error) {
	// 根据任务类型选择最合适的成员
	taskType := task.Type

	var bestMember *models.TeamMember
	bestScore := 0.0

	for _, member := range members {
		score := s.calculateCapabilityScore(member.ID, taskType)
		if score > bestScore {
			bestScore = score
			bestMember = &member
		}
	}

	if bestMember == nil {
		return "", fmt.Errorf("no suitable member found")
	}

	return bestMember.ID, nil
}

func (s *CapabilityBasedStrategy) calculateCapabilityScore(memberID, taskType string) float64 {
	s.service.memberMutex.RLock()
	agent, exists := s.service.agentRegistry[memberID]
	s.service.memberMutex.RUnlock()

	if !exists {
		return 0.0
	}

	// 检查是否有相关能力
	for _, cap := range agent.Capabilities {
		if cap == taskType {
			return 1.0
		}
	}

	// 检查专业化
	for _, spec := range agent.Specialization {
		if spec == taskType {
			return 0.8
		}
	}

	return 0.5 // 默认分数
}

func (s *CapabilityBasedStrategy) Name() string {
	return "capability_based"
}

// PriorityFirstStrategy 优先级优先策略
type PriorityFirstStrategy struct {
	service *TeamCoordinatorService
}

func (s *PriorityFirstStrategy) Assign(task *models.Task, members []models.TeamMember) (string, error) {
	// 高优先级任务分配给最空闲的高级成员
	priority := map[string]int{
		"urgent":   4,
		"high":     3,
		"medium":   2,
		"low":      1,
	}

	taskPriority := priority[task.Priority]
	if taskPriority == 0 {
		taskPriority = 1 // 默认低优先级
	}

	var selectedMember *models.TeamMember
	minLoad := int(^uint(0) >> 1)

	for _, member := range members {
		// 根据成员角色选择
		if task.Priority == "urgent" || task.Priority == "high" {
			// 高优先级任务倾向于分配给经验丰富的成员
			if member.Role != "lead" && member.Role != "senior" {
				continue
			}
		}

		s.service.memberMutex.RLock()
		agent, exists := s.service.agentRegistry[member.ID]
		s.service.memberMutex.RUnlock()

		load := member.CurrentLoad
		if exists {
			load = agent.CurrentLoad
		}

		if load < minLoad {
			minLoad = load
			selectedMember = &member
		}
	}

	if selectedMember == nil {
		return "", fmt.Errorf("no available member")
	}

	return selectedMember.ID, nil
}

func (s *PriorityFirstStrategy) Name() string {
	return "priority_first"
}

// ============ 团队状态增强 ============

// GetTeamStatus 获取团队状态
func (s *TeamCoordinatorService) GetTeamStatus(teamID string) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	// 获取团队信息
	team, err := s.GetTeam(teamID)
	if err != nil {
		return nil, err
	}
	status["team"] = team

	// 获取成员状态
	members, err := s.ListMembers(teamID)
	if err != nil {
		return nil, err
	}

	var memberStatus []map[string]interface{}
	for _, member := range members {
		s.memberMutex.RLock()
		agent, exists := s.agentRegistry[member.ID]
		s.memberMutex.RUnlock()

		info := map[string]interface{}{
			"id":         member.ID,
			"name":       member.Name,
			"role":       member.Role,
			"status":     s.GetMemberStatus(member.ID),
			"load":       member.CurrentLoad,
			"max_load":   member.MaxLoad,
		}

		if exists {
			info["agent_type"] = agent.Type
			info["capabilities"] = agent.Capabilities
			info["last_heartbeat"] = agent.LastHeartbeat
		}

		memberStatus = append(memberStatus, info)
	}
	status["members"] = memberStatus

	// 获取任务统计
	tasks, _ := s.getTasksByTeam(teamID)
	taskStats := map[string]int{
		"total":       len(tasks),
		"pending":     0,
		"in_progress": 0,
		"completed":   0,
		"failed":      0,
	}

	for _, task := range tasks {
		taskStats[task.Status]++
	}
	status["tasks"] = taskStats

	// 获取活跃 Agent
	var activeAgents []string
	s.memberMutex.RLock()
	for id, agent := range s.agentRegistry {
		if agent.Status == "online" {
			activeAgents = append(activeAgents, id)
		}
	}
	s.memberMutex.RUnlock()
	status["active_agents"] = activeAgents

	return status, nil
}

// ============ 服务控制增强 ============

// Stop 停止服务
func (s *TeamCoordinatorService) Stop() error {
	// 取消上下文
	s.cancel()

	// 等待所有 goroutine 完成
	s.wg.Wait()

	// 保存当前状态
	// 可以在这里实现快照逻辑
	return nil
}
