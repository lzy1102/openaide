package orchestration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
	"github.com/google/uuid"
)

// TeamCoordinatorInterface 团队协调器接口 - 避免循环导入
type TeamCoordinatorInterface interface {
	ListTasks(teamID, status, assignedTo string, page, pageSize int) ([]models.Task, int64, error)
	GetTask(taskID string) (*models.Task, error)
	UpdateTaskStatus(taskID, status string, errorMsg string) error
	ListMembers(teamID string) ([]models.TeamMember, error)
}

// ProjectManagerAgent 项目经理 Agent - 统筹全局，协调各 Agent
type ProjectManagerAgent struct {
	// 基础配置
	id       string
	teamID   string
	name     string
	llmClient llm.LLMClient
	llmModel  string
	coordinator TeamCoordinatorInterface

	// 状态管理
	status        ProjectStatus
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup

	// 监控配置
	monitorInterval time.Duration
	reportInterval  time.Duration

	// 事件
	issueChan     chan Issue
	progressChan  chan ProgressUpdate
	userChan      chan UserMessage

	// 汇报历史
	reportHistory []ProgressReport
	maxHistory    int
}

// ProjectStatus 项目状态
type ProjectStatus struct {
	Phase         string                 `json:"phase"`         // planning, executing, reviewing, completed
	CurrentTask   string                 `json:"current_task"`
	OverallProgress float64             `json:"overall_progress"` // 0-100
	ActiveMembers int                    `json:"active_members"`
	BlockedIssues []string              `json:"blocked_issues"`
	Metadata      map[string]interface{} `json:"metadata"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// Issue 问题/障碍
type Issue struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // conflict, dependency, resource, error
	Severity    string                 `json:"severity"` // low, medium, high, critical
	Description string                 `json:"description"`
	RelatedTask string                 `json:"related_task,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ReportedBy  string                 `json:"reported_by"`
	ReportedAt  time.Time              `json:"reported_at"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
}

// ProgressUpdate 进度更新
type ProgressUpdate struct {
	TaskID       string    `json:"task_id"`
	AgentID      string    `json:"agent_id"`
	Progress     float64   `json:"progress"` // 0-100
	Status       string    `json:"status"`
	Message      string    `json:"message"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserMessage 用户消息
type UserMessage struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Type      string                 `json:"type"` // query, command, feedback
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// ProgressReport 进度报告
type ProgressReport struct {
	ID          string                 `json:"id"`
	TeamID      string                 `json:"team_id"`
	TeamName    string                 `json:"team_name"`
	TaskName    string                 `json:"task_name"`
	Phase       string                 `json:"phase"`
	Progress    float64                `json:"progress"`
	Completed   []TaskSummary          `json:"completed"`
	InProgress  []TaskInProgress       `json:"in_progress"`
	Pending     []TaskPending          `json:"pending"`
	Blocked     []Issue                `json:"blocked"`
	ETA         time.Duration          `json:"eta"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// TaskSummary 任务摘要
type TaskSummary struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	CompletedBy string    `json:"completed_by"`
	CompletedAt time.Time `json:"completed_at"`
}

// TaskInProgress 进行中的任务
type TaskInProgress struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	AssignedTo string    `json:"assigned_to"`
	Progress   float64   `json:"progress"`
	StartedAt  time.Time `json:"started_at"`
	ETA        time.Time `json:"eta"`
}

// TaskPending 待处理任务
type TaskPending struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Dependencies []string `json:"dependencies"`
	Priority     string   `json:"priority"`
}

// NewProjectManagerAgent 创建项目经理 Agent
func NewProjectManagerAgent(teamID string, coordinator TeamCoordinatorInterface, llmClient llm.LLMClient) *ProjectManagerAgent {
	ctx, cancel := context.WithCancel(context.Background())

	return &ProjectManagerAgent{
		id:              uuid.New().String(),
		teamID:          teamID,
		name:            "project-manager",
		llmClient:       llmClient,
		llmModel:        "gpt-4",
		coordinator:     coordinator,
		status: ProjectStatus{
			Phase:           "planning",
			OverallProgress: 0,
			ActiveMembers:   0,
			BlockedIssues:   []string{},
			Metadata:        make(map[string]interface{}),
			UpdatedAt:       time.Now(),
		},
		ctx:             ctx,
		cancel:          cancel,
		monitorInterval: 30 * time.Second,
		reportInterval:  5 * time.Minute,
		issueChan:       make(chan Issue, 100),
		progressChan:    make(chan ProgressUpdate, 100),
		userChan:        make(chan UserMessage, 50),
		reportHistory:   make([]ProgressReport, 0),
		maxHistory:      20,
	}
}

// Start 启动项目经理
func (pm *ProjectManagerAgent) Start() error {
	pm.mu.Lock()
	pm.status.Phase = "executing"
	pm.status.UpdatedAt = time.Now()
	pm.mu.Unlock()

	// 启动监控 goroutine
	pm.wg.Add(1)
	go pm.monitorLoop()

	// 启动汇报 goroutine
	pm.wg.Add(1)
	go pm.reportLoop()

	// 启动事件处理 goroutine
	pm.wg.Add(1)
	go pm.eventLoop()

	return nil
}

// Stop 停止项目经理
func (pm *ProjectManagerAgent) Stop() {
	pm.cancel()
	pm.wg.Wait()

	pm.mu.Lock()
	pm.status.Phase = "completed"
	pm.status.UpdatedAt = time.Now()
	pm.mu.Unlock()
}

// Monitor 获取当前监控状态
func (pm *ProjectManagerAgent) Monitor() ProgressReport {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 获取团队任务
	tasks, _, err := pm.coordinator.ListTasks(pm.teamID, "", "", 1, 1000)
	if err != nil {
		return pm.createErrorReport(err)
	}

	report := ProgressReport{
		ID:          uuid.New().String(),
		TeamID:      pm.teamID,
		GeneratedAt: time.Now(),
		Phase:       pm.status.Phase,
		Progress:    pm.status.OverallProgress,
		Completed:   []TaskSummary{},
		InProgress:  []TaskInProgress{},
		Pending:     []TaskPending{},
		Blocked:     []Issue{},
	}

	// 统计任务状态
	var totalProgress float64
	var taskCount int

	for _, task := range tasks {
		taskCount++

		switch task.Status {
		case "completed":
			report.Completed = append(report.Completed, TaskSummary{
				ID:          task.ID,
				Title:       task.Title,
				CompletedBy: task.AssignedTo,
				CompletedAt: *task.CompletedAt,
			})
			totalProgress += 100

		case "in_progress", "assigned":
			progress := pm.calculateTaskProgress(&task)
			report.InProgress = append(report.InProgress, TaskInProgress{
				ID:         task.ID,
				Title:      task.Title,
				AssignedTo: task.AssignedTo,
				Progress:   progress,
				StartedAt:  *task.StartedAt,
				ETA:        pm.calculateETA(&task, progress),
			})
			totalProgress += progress

		case "pending":
			report.Pending = append(report.Pending, TaskPending{
				ID:           task.ID,
				Title:        task.Title,
				Dependencies: pm.getDependencyIDs(&task),
				Priority:     task.Priority,
			})
		}
	}

	// 计算总体进度
	if taskCount > 0 {
		report.Progress = totalProgress / float64(taskCount)
		pm.status.OverallProgress = report.Progress
	}

	// 计算预计完成时间
	report.ETA = pm.calculateOverallETA(report)

	return report
}

// HandleIssue 处理问题
func (pm *ProjectManagerAgent) HandleIssue(issue Issue) error {
	pm.mu.Lock()

	// 检查严重程度
	switch issue.Severity {
	case "critical":
		// 关键问题，暂停执行
		pm.status.Phase = "blocked"
		pm.status.BlockedIssues = append(pm.status.BlockedIssues, issue.ID)
	case "high":
		// 高优先级问题，记录但继续执行
		pm.status.BlockedIssues = append(pm.status.BlockedIssues, issue.ID)
	default:
		// 低优先级问题，仅记录
	}

	pm.status.UpdatedAt = time.Now()
	pm.mu.Unlock()

	// 根据问题类型采取行动
	switch issue.Type {
	case "conflict":
		return pm.handleConflict(issue)
	case "dependency":
		return pm.handleDependencyIssue(issue)
	case "resource":
		return pm.handleResourceIssue(issue)
	case "error":
		return pm.handleError(issue)
	default:
		return pm.handleGenericIssue(issue)
	}
}

// ReportToUser 生成用户报告
func (pm *ProjectManagerAgent) ReportToUser() string {
	report := pm.Monitor()

	var sb strings.Builder

	// 标题
	sb.WriteString("📊 项目进度汇报\n\n")

	// 基本信息
	sb.WriteString(fmt.Sprintf("团队: %s\n", report.TeamID))
	if report.TaskName != "" {
		sb.WriteString(fmt.Sprintf("任务: %s\n", report.TaskName))
	}
	sb.WriteString(fmt.Sprintf("阶段: %s\n", pm.getPhaseLabel(report.Phase)))
	sb.WriteString("\n")

	// 进度条
	sb.WriteString(fmt.Sprintf("进度: %s\n", pm.renderProgressBar(report.Progress)))
	sb.WriteString(fmt.Sprintf("%.0f%%\n\n", report.Progress))

	// 已完成
	if len(report.Completed) > 0 {
		sb.WriteString("✅ 已完成:\n")
		for _, t := range report.Completed {
			sb.WriteString(fmt.Sprintf("- %s\n", t.Title))
		}
		sb.WriteString("\n")
	}

	// 进行中
	if len(report.InProgress) > 0 {
		sb.WriteString("🔄 进行中:\n")
		for _, t := range report.InProgress {
			sb.WriteString(fmt.Sprintf("- %s (%s) - %.0f%%\n", t.Title, t.AssignedTo, t.Progress))
		}
		sb.WriteString("\n")
	}

	// 待处理
	if len(report.Pending) > 0 {
		sb.WriteString("⏳ 待处理:\n")
		for _, t := range report.Pending {
			priorityLabel := pm.getPriorityLabel(t.Priority)
			sb.WriteString(fmt.Sprintf("- %s [%s]\n", t.Title, priorityLabel))
		}
		sb.WriteString("\n")
	}

	// 阻塞问题
	if len(report.Blocked) > 0 {
		sb.WriteString("⚠️ 阻塞问题:\n")
		for _, i := range report.Blocked {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", i.Severity, i.Description))
		}
		sb.WriteString("\n")
	}

	// 预计完成时间
	if report.ETA > 0 {
		sb.WriteString(fmt.Sprintf("预计完成时间: %s\n", pm.formatDuration(report.ETA)))
	}

	return sb.String()
}

// ReassignTask 重新分配任务
func (pm *ProjectManagerAgent) ReassignTask(taskID, newAgent string) error {
	// 获取任务详情
	task, err := pm.coordinator.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// 记录原分配者
	oldAgent := task.AssignedTo

	// 更新任务分配
	task.AssignedTo = newAgent
	task.Status = "assigned"
	task.UpdatedAt = time.Now()

	// 保存更新 - 使用 UpdateTaskStatus 更新状态
	if err := pm.coordinator.UpdateTaskStatus(taskID, "assigned", ""); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	// 发送通知
	pm.notifyReassignment(taskID, oldAgent, newAgent)

	return nil
}

// SubmitProgress 提交进度更新
func (pm *ProjectManagerAgent) SubmitProgress(update ProgressUpdate) error {
	select {
	case pm.progressChan <- update:
		return nil
	default:
		return fmt.Errorf("progress channel is full")
	}
}

// SubmitIssue 提交问题
func (pm *ProjectManagerAgent) SubmitIssue(issue Issue) error {
	// 确保 ID 存在
	if issue.ID == "" {
		issue.ID = uuid.New().String()
	}
	if issue.ReportedAt.IsZero() {
		issue.ReportedAt = time.Now()
	}

	select {
	case pm.issueChan <- issue:
		return nil
	default:
		return fmt.Errorf("issue channel is full")
	}
}

// SendMessage 发送消息给项目经理
func (pm *ProjectManagerAgent) SendMessage(msg UserMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	select {
	case pm.userChan <- msg:
		return nil
	default:
		return fmt.Errorf("user message channel is full")
	}
}

// monitorLoop 监控循环
func (pm *ProjectManagerAgent) monitorLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.monitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.performHealthCheck()
		}
	}
}

// reportLoop 汇报循环
func (pm *ProjectManagerAgent) reportLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			report := pm.Monitor()
			pm.saveReport(report)
		}
	}
}

// eventLoop 事件处理循环
func (pm *ProjectManagerAgent) eventLoop() {
	defer pm.wg.Done()

	for {
		select {
		case <-pm.ctx.Done():
			return

		case issue := <-pm.issueChan:
			if err := pm.HandleIssue(issue); err != nil {
				// 记录错误但继续运行
				pm.logError("failed to handle issue: %v", err)
			}

		case update := <-pm.progressChan:
			pm.handleProgressUpdate(update)

		case msg := <-pm.userChan:
			pm.handleUserMessage(msg)
		}
	}
}

// performHealthCheck 执行健康检查
func (pm *ProjectManagerAgent) performHealthCheck() {
	// 检查活跃成员
	pm.updateActiveMembers()

	// 检查超时任务
	pm.checkStaleTasks()

	// 检查阻塞状态
	pm.checkBlockedStatus()
}

// updateActiveMembers 更新活跃成员数
func (pm *ProjectManagerAgent) updateActiveMembers() {
	// 通过 coordinator 获取活跃成员
	members, err := pm.coordinator.ListMembers(pm.teamID)
	if err != nil {
		return
	}

	activeCount := 0
	for _, m := range members {
		if m.Availability == "available" || m.Availability == "busy" {
			activeCount++
		}
	}

	pm.mu.Lock()
	pm.status.ActiveMembers = activeCount
	pm.status.UpdatedAt = time.Now()
	pm.mu.Unlock()
}

// checkStaleTasks 检查超时任务
func (pm *ProjectManagerAgent) checkStaleTasks() {
	tasks, _, err := pm.coordinator.ListTasks(pm.teamID, "", "", 1, 1000)
	if err != nil {
		return
	}

	now := time.Now()
	staleThreshold := 2 * time.Hour

	for _, task := range tasks {
		if task.Status == "in_progress" && task.StartedAt != nil {
			if now.Sub(*task.StartedAt) > staleThreshold {
				// 任务超时，创建问题
				issue := Issue{
					ID:          uuid.New().String(),
					Type:        "error",
					Severity:    "medium",
					Description: fmt.Sprintf("任务 '%s' 执行超时", task.Title),
					RelatedTask: task.ID,
					ReportedBy:  pm.id,
					ReportedAt:  now,
				}
				pm.SubmitIssue(issue)
			}
		}
	}
}

// checkBlockedStatus 检查阻塞状态
func (pm *ProjectManagerAgent) checkBlockedStatus() {
	pm.mu.RLock()
	blocked := len(pm.status.BlockedIssues)
	isBlocked := pm.status.Phase == "blocked"
	pm.mu.RUnlock()

	// 如果没有阻塞问题但状态是 blocked，恢复执行
	if isBlocked && blocked == 0 {
		pm.mu.Lock()
		pm.status.Phase = "executing"
		pm.status.UpdatedAt = time.Now()
		pm.mu.Unlock()
	}
}

// handleProgressUpdate 处理进度更新
func (pm *ProjectManagerAgent) handleProgressUpdate(update ProgressUpdate) {
	// 更新任务的进度信息
	task, err := pm.coordinator.GetTask(update.TaskID)
	if err != nil {
		return
	}

	// 更新任务状态
	task.Status = update.Status
	task.UpdatedAt = update.UpdatedAt

	// 如果完成，设置完成时间
	if update.Progress >= 100 || update.Status == "completed" {
		now := time.Now()
		task.CompletedAt = &now
	}

	// 使用 UpdateTaskStatus 更新任务状态
	_ = pm.coordinator.UpdateTaskStatus(update.TaskID, update.Status, "")
}

// handleUserMessage 处理用户消息
func (pm *ProjectManagerAgent) handleUserMessage(msg UserMessage) {
	switch msg.Type {
	case "query":
		// 查询当前状态
		response := pm.ReportToUser()
		pm.sendResponseToUser(msg.ID, response)

	case "command":
		// 执行命令
		pm.executeCommand(msg)

	case "feedback":
		// 处理反馈
		pm.processFeedback(msg)
	}
}

// handleConflict 处理冲突
func (pm *ProjectManagerAgent) handleConflict(issue Issue) error {
	// 使用 LLM 分析冲突并提供建议
	return pm.resolveIssueWithLLM(issue)
}

// handleDependencyIssue 处理依赖问题
func (pm *ProjectManagerAgent) handleDependencyIssue(issue Issue) error {
	// 检查依赖任务状态
	return pm.resolveDependency(issue)
}

// handleResourceIssue 处理资源问题
func (pm *ProjectManagerAgent) handleResourceIssue(issue Issue) error {
	// 尝试重新分配任务或增加资源
	return pm.rebalanceResources()
}

// handleError 处理错误
func (pm *ProjectManagerAgent) handleError(issue Issue) error {
	// 根据错误严重程度决定是否重试
	if issue.Severity == "critical" {
		pm.mu.Lock()
		pm.status.Phase = "blocked"
		pm.status.BlockedIssues = append(pm.status.BlockedIssues, issue.ID)
		pm.status.UpdatedAt = time.Now()
		pm.mu.Unlock()
	}
	return nil
}

// handleGenericIssue 处理通用问题
func (pm *ProjectManagerAgent) handleGenericIssue(issue Issue) error {
	// 记录问题，等待人工处理
	return nil
}

// resolveIssueWithLLM 使用 LLM 解决问题
func (pm *ProjectManagerAgent) resolveIssueWithLLM(issue Issue) error {
	if pm.llmClient == nil {
		return fmt.Errorf("no LLM client configured")
	}

	prompt := fmt.Sprintf(`作为一个项目经理，请分析以下问题并提供解决方案:

问题描述: %s
相关任务: %s
严重程度: %s

请提供:
1. 问题分析
2. 可能的解决方案 (至少2个)
3. 推荐方案及理由
4. 预防措施

以 JSON 格式输出。`, issue.Description, issue.RelatedTask, issue.Severity)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "你是一个经验丰富的项目经理。"},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       pm.llmModel,
		Temperature: 0.7,
		MaxTokens:   1000,
	}

	ctx, cancel := context.WithTimeout(pm.ctx, 30*time.Second)
	defer cancel()

	resp, err := pm.llmClient.Chat(ctx, req)
	if err != nil {
		return err
	}

	// 处理响应
	_ = resp
	return nil
}

// resolveDependency 解决依赖问题
func (pm *ProjectManagerAgent) resolveDependency(issue Issue) error {
	// 检查依赖任务是否完成
	return nil
}

// rebalanceResources 重新平衡资源
func (pm *ProjectManagerAgent) rebalanceResources() error {
	// 获取所有成员的负载情况并重新分配任务
	return nil
}

// 辅助方法

func (pm *ProjectManagerAgent) calculateTaskProgress(task *models.Task) float64 {
	// 根据子任务完成情况计算进度
	if len(task.Subtasks) == 0 {
		return 50.0 // 没有子任务，返回默认进度
	}

	var totalProgress float64
	for _, st := range task.Subtasks {
		switch st.Status {
		case "completed":
			totalProgress += 100
		case "in_progress":
			totalProgress += 50
		}
	}

	return totalProgress / float64(len(task.Subtasks))
}

func (pm *ProjectManagerAgent) calculateETA(task *models.Task, progress float64) time.Time {
	if progress <= 0 || task.StartedAt == nil {
		return time.Now().Add(time.Duration(task.Estimated) * time.Minute)
	}

	elapsed := time.Since(*task.StartedAt)
	remaining := time.Duration(float64(elapsed) * (100 - progress) / progress)
	return time.Now().Add(remaining)
}

func (pm *ProjectManagerAgent) calculateOverallETA(report ProgressReport) time.Duration {
	if len(report.InProgress) == 0 {
		return 0
	}

	var totalETA time.Duration
	for _, t := range report.InProgress {
		eta := t.ETA.Sub(time.Now())
		if eta > 0 {
			totalETA += eta
		}
	}

	return totalETA / time.Duration(len(report.InProgress))
}

func (pm *ProjectManagerAgent) getDependencyIDs(task *models.Task) []string {
	ids := make([]string, len(task.Dependencies))
	for i, d := range task.Dependencies {
		ids[i] = d.DependsOn
	}
	return ids
}

func (pm *ProjectManagerAgent) renderProgressBar(progress float64) string {
	width := 10
	filled := int(progress / 100 * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func (pm *ProjectManagerAgent) getPhaseLabel(phase string) string {
	labels := map[string]string{
		"planning":   "规划中",
		"executing":  "执行中",
		"reviewing":  "审查中",
		"completed":  "已完成",
		"blocked":    "已阻塞",
	}
	if label, ok := labels[phase]; ok {
		return label
	}
	return phase
}

func (pm *ProjectManagerAgent) getPriorityLabel(priority string) string {
	labels := map[string]string{
		"low":      "低",
		"medium":   "中",
		"high":     "高",
		"urgent":   "紧急",
	}
	if label, ok := labels[priority]; ok {
		return label
	}
	return priority
}

func (pm *ProjectManagerAgent) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "不到1分钟"
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0f分钟", d.Minutes())
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%d小时%d分钟", hours, minutes)
		}
		return fmt.Sprintf("%d小时", hours)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%d天%d小时", days, hours)
	}
	return fmt.Sprintf("%d天", days)
}

func (pm *ProjectManagerAgent) saveReport(report ProgressReport) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.reportHistory = append(pm.reportHistory, report)
	if len(pm.reportHistory) > pm.maxHistory {
		pm.reportHistory = pm.reportHistory[1:]
	}
}

func (pm *ProjectManagerAgent) createErrorReport(err error) ProgressReport {
	return ProgressReport{
		ID:          uuid.New().String(),
		TeamID:      pm.teamID,
		GeneratedAt: time.Now(),
		Phase:       "error",
		Progress:    0,
		Blocked:     []Issue{{Description: err.Error()}},
	}
}

func (pm *ProjectManagerAgent) notifyReassignment(taskID, oldAgent, newAgent string) {
	// 发送重新分配通知
}

func (pm *ProjectManagerAgent) sendResponseToUser(msgID, response string) {
	// 发送响应给用户
}

func (pm *ProjectManagerAgent) executeCommand(msg UserMessage) {
	// 执行命令
}

func (pm *ProjectManagerAgent) processFeedback(msg UserMessage) {
	// 处理反馈
}

func (pm *ProjectManagerAgent) logError(format string, args ...interface{}) {
	// 记录错误
}

// GetStatus 获取当前状态
func (pm *ProjectManagerAgent) GetStatus() ProjectStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.status
}

// GetReportHistory 获取报告历史
func (pm *ProjectManagerAgent) GetReportHistory() []ProgressReport {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]ProgressReport, len(pm.reportHistory))
	copy(result, pm.reportHistory)
	return result
}

// SetMonitoringInterval 设置监控间隔
func (pm *ProjectManagerAgent) SetMonitoringInterval(interval time.Duration) {
	pm.monitorInterval = interval
}

// SetReportInterval 设置汇报间隔
func (pm *ProjectManagerAgent) SetReportInterval(interval time.Duration) {
	pm.reportInterval = interval
}
