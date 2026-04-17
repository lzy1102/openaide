package services

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// Scheduler 工作流调度器
type Scheduler struct {
	workflowService *WorkflowService
	db              *gorm.DB
	ticker          *time.Ticker
	stopChan        chan struct{}
	runningJobs     map[string]*ScheduledJob
	jobMutex        sync.RWMutex
}

// ScheduledJob 调度任务
type ScheduledJob struct {
	ID          string
	WorkflowID  string
	Name        string
	CronExpr    string
	Enabled     bool
	Parameters  map[string]interface{}
	NextRunAt   time.Time
	LastRunAt   time.Time
	RunCount    int
	MissedRuns  int
	Context     context.Context
	CancelFunc  context.CancelFunc
}

// NewScheduler 创建调度器
func NewScheduler(workflowService *WorkflowService, db *gorm.DB) *Scheduler {
	return &Scheduler{
		workflowService: workflowService,
		db:              db,
		stopChan:        make(chan struct{}),
		runningJobs:     make(map[string]*ScheduledJob),
	}
}

// Start 启动调度器
func (s *Scheduler) Start(interval time.Duration) {
	s.ticker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.tick()
			case <-s.stopChan:
				return
			}
		}
	}()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	close(s.stopChan)
	if s.ticker != nil {
		s.ticker.Stop()
	}

	// 取消所有运行中的任务
	s.jobMutex.Lock()
	defer s.jobMutex.Unlock()

	for _, job := range s.runningJobs {
		if job.CancelFunc != nil {
			job.CancelFunc()
		}
	}

	s.runningJobs = make(map[string]*ScheduledJob)
}

// tick 调度器主循环
func (s *Scheduler) tick() {
	now := time.Now()

	// 获取需要执行的调度任务
	var schedules []models.WorkflowSchedule
	s.db.Where("enabled = ? AND next_run_at <= ?", true, now).Find(&schedules)

	for _, schedule := range schedules {
		// 检查任务是否已在运行
		s.jobMutex.RLock()
		_, exists := s.runningJobs[schedule.ID]
		s.jobMutex.RUnlock()

		if exists {
			continue
		}

		// 执行任务
		go s.executeScheduledJob(&schedule)
	}
}

// executeScheduledJob 执行调度任务
func (s *Scheduler) executeScheduledJob(schedule *models.WorkflowSchedule) {
	ctx, cancel := context.WithCancel(context.Background())

	// 注册运行中的任务
	job := &ScheduledJob{
		ID:         schedule.ID,
		WorkflowID: schedule.WorkflowID,
		Name:       schedule.Name,
		CronExpr:   schedule.CronExpr,
		Enabled:    schedule.Enabled,
		Parameters: schedule.Parameters,
		Context:    ctx,
		CancelFunc: cancel,
	}

	s.jobMutex.Lock()
	s.runningJobs[schedule.ID] = job
	s.jobMutex.Unlock()

	defer func() {
		s.jobMutex.Lock()
		delete(s.runningJobs, schedule.ID)
		s.jobMutex.Unlock()
	}()

	// 创建工作流实例
	instance, ok := s.workflowService.CreateWorkflowInstance(schedule.WorkflowID, schedule.Parameters)
	if !ok {
		// 记录错误
		s.logScheduleError(schedule, fmt.Errorf("failed to create workflow instance"))
		return
	}

	// 执行工作流
	_, err := s.workflowService.ExecuteWorkflowWithContext(ctx, instance.ID)
	if err != nil {
		s.logScheduleError(schedule, err)
	}

	// 更新调度记录
	now := time.Now()
	schedule.LastRunAt = &now
	schedule.RunCount++

	// 计算下次执行时间
	nextRunAt, err := s.calculateNextRunTime(schedule.CronExpr, now)
	if err == nil {
		schedule.NextRunAt = &nextRunAt
	}

	s.db.Save(schedule)
}

// AddSchedule 添加调度任务
func (s *Scheduler) AddSchedule(workflowID, name, cronExpr string, parameters map[string]interface{}) (*models.WorkflowSchedule, error) {
	// 计算下次执行时间
	nextRunAt, err := s.calculateNextRunTime(cronExpr, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	schedule := &models.WorkflowSchedule{
		ID:         uuid.New().String(),
		WorkflowID: workflowID,
		Name:       name,
		CronExpr:   cronExpr,
		Enabled:    true,
		Parameters: parameters,
		NextRunAt:  &nextRunAt,
		RunCount:   0,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := s.db.Create(schedule).Error; err != nil {
		return nil, err
	}

	return schedule, nil
}

// UpdateSchedule 更新调度任务
func (s *Scheduler) UpdateSchedule(id string, enabled *bool, cronExpr *string, parameters map[string]interface{}) error {
	var schedule models.WorkflowSchedule
	if err := s.db.First(&schedule, id).Error; err != nil {
		return err
	}

	updates := make(map[string]interface{})

	if enabled != nil {
		updates["enabled"] = *enabled
	}

	if cronExpr != nil {
		nextRunAt, err := s.calculateNextRunTime(*cronExpr, time.Now())
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		updates["cron_expr"] = *cronExpr
		updates["next_run_at"] = nextRunAt
	}

	if parameters != nil {
		updates["parameters"] = parameters
	}

	updates["updated_at"] = time.Now()

	return s.db.Model(&schedule).Updates(updates).Error
}

// RemoveSchedule 移除调度任务
func (s *Scheduler) RemoveSchedule(id string) error {
	// 取消运行中的任务
	s.jobMutex.RLock()
	if job, exists := s.runningJobs[id]; exists {
		if job.CancelFunc != nil {
			job.CancelFunc()
		}
	}
	s.jobMutex.RUnlock()

	return s.db.Where("id = ?", id).Delete(&models.WorkflowSchedule{}).Error
}

// GetSchedule 获取调度任务
func (s *Scheduler) GetSchedule(id string) (*models.WorkflowSchedule, error) {
	var schedule models.WorkflowSchedule
	err := s.db.First(&schedule, id).Error
	return &schedule, err
}

// ListSchedules 列出调度任务
func (s *Scheduler) ListSchedules(workflowID string) ([]models.WorkflowSchedule, error) {
	var schedules []models.WorkflowSchedule
	query := s.db.Order("created_at DESC")

	if workflowID != "" {
		query = query.Where("workflow_id = ?", workflowID)
	}

	err := query.Find(&schedules).Error
	return schedules, err
}

// GetRunningJobs 获取运行中的任务
func (s *Scheduler) GetRunningJobs() []*ScheduledJob {
	s.jobMutex.RLock()
	defer s.jobMutex.RUnlock()

	jobs := make([]*ScheduledJob, 0, len(s.runningJobs))
	for _, job := range s.runningJobs {
		jobs = append(jobs, job)
	}

	return jobs
}

// calculateNextRunTime 计算下次执行时间（简化版cron解析）
func (s *Scheduler) calculateNextRunTime(cronExpr string, from time.Time) (time.Time, error) {
	// 简化的cron表达式解析
	// 支持格式: "*/5 * * * *" (每5分钟)
	//           "0 9 * * *" (每天9点)
	//           "0 9 * * 1-5" (周一到周五9点)

	// 这里使用简化实现，实际应用中应该使用完整的cron库
	// 为了演示，我们返回一个默认值

	// 解析分钟间隔
	if len(cronExpr) > 0 && cronExpr[0] == '*' && len(cronExpr) > 1 && cronExpr[1] == '/' {
		// 解析 */N 格式
		var interval int
		fmt.Sscanf(cronExpr, "*/%d", &interval)
		if interval > 0 && interval <= 60 {
			return from.Add(time.Duration(interval) * time.Minute), nil
		}
	}

	// 默认返回1小时后
	return from.Add(time.Hour), nil
}

// logScheduleError 记录调度错误
func (s *Scheduler) logScheduleError(schedule *models.WorkflowSchedule, err error) {
	// 创建执行记录
	execution := &models.WorkflowExecution{
		ID:          uuid.New().String(),
		WorkflowID:  schedule.WorkflowID,
		Status:      "failed",
		TriggerType: "schedule",
		Error:       err.Error(),
		StartedAt:   time.Now(),
		CompletedAt: &[]time.Time{time.Now()}[0],
	}

	s.db.Create(execution)
}

// ============ 工作流触发器 ============

// TriggerType 触发器类型
type TriggerType string

const (
	TriggerManual  TriggerType = "manual"
	TriggerAPI     TriggerType = "api"
	TriggerSchedule TriggerType = "schedule"
	TriggerEvent   TriggerType = "event"
	TriggerWebhook TriggerType = "webhook"
)

// WorkflowTrigger 工作流触发器
type WorkflowTrigger struct {
	ID          string
	WorkflowID  string
	Type        TriggerType
	Enabled     bool
	Config      map[string]interface{}
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TriggerManager 触发器管理器
type TriggerManager struct {
	db              *gorm.DB
	workflowService *WorkflowService
	handlers        map[TriggerType]TriggerHandler
}

// TriggerHandler 触发器处理器接口
type TriggerHandler interface {
	GetType() TriggerType
	CanHandle(triggerType TriggerType) bool
	Handle(ctx context.Context, trigger *WorkflowTrigger, input map[string]interface{}) (string, error)
	Register(config map[string]interface{}) error
	Unregister(triggerID string) error
}

// NewTriggerManager 创建触发器管理器
func NewTriggerManager(db *gorm.DB, workflowService *WorkflowService) *TriggerManager {
	manager := &TriggerManager{
		db:              db,
		workflowService: workflowService,
		handlers:        make(map[TriggerType]TriggerHandler),
	}

	// 注册默认处理器
	manager.RegisterHandler(&WebhookTriggerHandler{})

	return manager
}

// RegisterHandler 注册触发器处理器
func (m *TriggerManager) RegisterHandler(handler TriggerHandler) {
	m.handlers[handler.GetType()] = handler
}

// Trigger 触发工作流
func (m *TriggerManager) Trigger(ctx context.Context, workflowID string, triggerType TriggerType, input map[string]interface{}) (string, error) {
	// 创建工作流实例
	instance, ok := m.workflowService.CreateWorkflowInstance(workflowID, input)
	if !ok {
		return "", fmt.Errorf("failed to create workflow instance")
	}

	// 执行工作流
	_, err := m.workflowService.ExecuteWorkflowWithContext(ctx, instance.ID)
	if err != nil {
		return "", fmt.Errorf("workflow execution failed: %w", err)
	}

	// 记录执行
	m.recordExecution(workflowID, instance.ID, triggerType, input)

	return instance.ID, nil
}

// recordExecution 记录执行
func (m *TriggerManager) recordExecution(workflowID, instanceID string, triggerType TriggerType, input map[string]interface{}) {
	execution := &models.WorkflowExecution{
		ID:          uuid.New().String(),
		WorkflowID:  workflowID,
		InstanceID:  instanceID,
		Status:      "running",
		TriggerType: string(triggerType),
		Input:       input,
		StartedAt:   time.Now(),
	}

	m.db.Create(execution)
}

// WebhookTriggerHandler Webhook 触发器处理器
type WebhookTriggerHandler struct {
	webhooks map[string]*WebhookConfig
}

// WebhookConfig Webhook 配置
type WebhookConfig struct {
	ID         string
	WorkflowID string
	Path       string
	Secret     string
	Enabled    bool
}

func (h *WebhookTriggerHandler) GetType() TriggerType {
	return TriggerWebhook
}

func (h *WebhookTriggerHandler) CanHandle(triggerType TriggerType) bool {
	return triggerType == TriggerWebhook
}

func (h *WebhookTriggerHandler) Handle(ctx context.Context, trigger *WorkflowTrigger, input map[string]interface{}) (string, error) {
	// Webhook 触发逻辑
	return "", nil
}

func (h *WebhookTriggerHandler) Register(config map[string]interface{}) error {
	if h.webhooks == nil {
		h.webhooks = make(map[string]*WebhookConfig)
	}

	path, _ := config["path"].(string)
	secret, _ := config["secret"].(string)
	workflowID, _ := config["workflow_id"].(string)

	h.webhooks[path] = &WebhookConfig{
		ID:         uuid.New().String(),
		WorkflowID: workflowID,
		Path:       path,
		Secret:     secret,
		Enabled:    true,
	}

	return nil
}

func (h *WebhookTriggerHandler) Unregister(triggerID string) error {
	for k, v := range h.webhooks {
		if v.ID == triggerID {
			delete(h.webhooks, k)
			return nil
		}
	}
	return fmt.Errorf("webhook not found")
}

// ============ 工作流监控 ============

// WorkflowMonitor 工作流监控
type WorkflowMonitor struct {
	db       *gorm.DB
	metrics  map[string]*WorkflowMetrics
	mu       sync.RWMutex
}

// WorkflowMetrics 工作流指标
type WorkflowMetrics struct {
	WorkflowID       string
	TotalRuns        int64
	SuccessRuns      int64
	FailedRuns       int64
	AverageDuration  int64
	LastRunAt        time.Time
	LastStatus       string
}

// NewWorkflowMonitor 创建工作流监控
func NewWorkflowMonitor(db *gorm.DB) *WorkflowMonitor {
	monitor := &WorkflowMonitor{
		db:      db,
		metrics: make(map[string]*WorkflowMetrics),
	}

	// 加载历史指标
	monitor.loadMetrics()

	return monitor
}

// loadMetrics 加载历史指标
func (m *WorkflowMonitor) loadMetrics() {
	var executions []models.WorkflowExecution
	m.db.Find(&executions)

	// 按工作流聚合指标
	for _, exec := range executions {
		m.recordExecution(&exec)
	}
}

// recordExecution 记录执行指标
func (m *WorkflowMonitor) recordExecution(execution *models.WorkflowExecution) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics, exists := m.metrics[execution.WorkflowID]
	if !exists {
		metrics = &WorkflowMetrics{
			WorkflowID: execution.WorkflowID,
		}
		m.metrics[execution.WorkflowID] = metrics
	}

	metrics.TotalRuns++
	if execution.Status == "completed" {
		metrics.SuccessRuns++
	} else {
		metrics.FailedRuns++
	}

	if execution.StartedAt.After(metrics.LastRunAt) {
		metrics.LastRunAt = execution.StartedAt
		metrics.LastStatus = execution.Status
	}

	if execution.CompletedAt != nil {
		duration := execution.CompletedAt.Sub(execution.StartedAt).Milliseconds()
		if metrics.AverageDuration == 0 {
			metrics.AverageDuration = duration
		} else {
			metrics.AverageDuration = (metrics.AverageDuration + duration) / 2
		}
	}
}

// GetMetrics 获取工作流指标
func (m *WorkflowMonitor) GetMetrics(workflowID string) *WorkflowMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if metrics, exists := m.metrics[workflowID]; exists {
		return metrics
	}

	return &WorkflowMetrics{WorkflowID: workflowID}
}

// GetAllMetrics 获取所有工作流指标
func (m *WorkflowMonitor) GetAllMetrics() map[string]*WorkflowMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*WorkflowMetrics, len(m.metrics))
	for k, v := range m.metrics {
		result[k] = v
	}

	return result
}

// CalculateSuccessRate 计算成功率
func (m *WorkflowMetrics) CalculateSuccessRate() float64 {
	if m.TotalRuns == 0 {
		return 0
	}
	return float64(m.SuccessRuns) / float64(m.TotalRuns) * 100
}

// CalculateFailureRate 计算失败率
func (m *WorkflowMetrics) CalculateFailureRate() float64 {
	if m.TotalRuns == 0 {
		return 0
	}
	return float64(m.FailedRuns) / float64(m.TotalRuns) * 100
}

// GetStatsSummary 获取统计摘要
func (m *WorkflowMonitor) GetStatsSummary() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalWorkflows := len(m.metrics)
	totalRuns := int64(0)
	totalSuccess := int64(0)
	totalFailed := int64(0)

	for _, metrics := range m.metrics {
		totalRuns += metrics.TotalRuns
		totalSuccess += metrics.SuccessRuns
		totalFailed += metrics.FailedRuns
	}

	var globalSuccessRate float64
	if totalRuns > 0 {
		globalSuccessRate = float64(totalSuccess) / float64(totalRuns) * 100
	}

	return map[string]interface{}{
		"total_workflows":  totalWorkflows,
		"total_runs":       totalRuns,
		"total_success":    totalSuccess,
		"total_failed":     totalFailed,
		"success_rate":     math.Round(globalSuccessRate*100) / 100,
		"running_workflows": len(m.getRunningWorkflows()),
	}
}

// getRunningWorkflows 获取运行中的工作流
func (m *WorkflowMonitor) getRunningWorkflows() []string {
	// 这里应该从 WorkflowService 获取实际运行中的工作流
	return []string{}
}
