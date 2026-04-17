package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"openaide/backend/src/models"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// SchedulerService 定时任务调度服务
type SchedulerService struct {
	db          *gorm.DB
	logger      *LoggerService
	wsService   *WebSocketService
	workflowSvc *WorkflowService

	cron        *cron.Cron
	tasks       map[string]cron.EntryID
	onceTimers  map[string]*time.Timer // 一次性任务的定时器
	mu          sync.RWMutex

	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewSchedulerService 创建调度服务
func NewSchedulerService(db *gorm.DB, logger *LoggerService, wsService *WebSocketService, workflowSvc *WorkflowService) *SchedulerService {
	ctx, cancel := context.WithCancel(context.Background())

	s := &SchedulerService{
		db:          db,
		logger:      logger,
		wsService:   wsService,
		workflowSvc: workflowSvc,
		cron:        cron.New(cron.WithSeconds()),
		tasks:       make(map[string]cron.EntryID),
		onceTimers:  make(map[string]*time.Timer),
		ctx:         ctx,
		cancel:      cancel,
	}

	// 加载并启动所有活跃任务
	s.loadActiveTasks()

	// 启动 cron 调度器
	s.cron.Start()

	// 启动提醒检查
	s.wg.Add(1)
	go s.reminderChecker()

	s.logger.Info(ctx, "Scheduler service started")

	return s
}

// loadActiveTasks 加载活跃任务
func (s *SchedulerService) loadActiveTasks() {
	var tasks []models.ScheduledTask
	if err := s.db.Where("status = ? AND is_enabled = ?", "active", true).Find(&tasks).Error; err != nil {
		s.logger.Error(s.ctx, "Failed to load active tasks: %v", err)
		return
	}

	for _, task := range tasks {
		if err := s.scheduleTask(&task); err != nil {
			s.logger.Error(s.ctx, "Failed to schedule task %s: %v", task.ID, err)
		}
	}

	s.logger.Info(s.ctx, "Loaded %d active tasks", len(tasks))
}

// CreateTask 创建定时任务
func (s *SchedulerService) CreateTask(task *models.ScheduledTask) error {
	// 验证调度配置
	if err := s.validateSchedule(task); err != nil {
		return err
	}

	// 计算下次执行时间
	nextRun := s.calculateNextRun(task)
	task.NextRunAt = nextRun
	task.Status = "active"
	task.IsEnabled = true
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()

	if err := s.db.Create(task).Error; err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	// 调度任务
	if err := s.scheduleTask(task); err != nil {
		return fmt.Errorf("failed to schedule task: %w", err)
	}

	s.logger.Info(s.ctx, "Task created: %s - %s", task.ID, task.Name)
	return nil
}

// validateSchedule 验证调度配置
func (s *SchedulerService) validateSchedule(task *models.ScheduledTask) error {
	switch task.ScheduleType {
	case "cron":
		if task.CronExpr == "" {
			return fmt.Errorf("cron expression is required for cron schedule")
		}
		// 验证 cron 表达式 - 使用秒级解析器 (6字段)，与 cron.WithSeconds() 一致
		// 修复: 原来使用 ParseStandard (5字段) 与调度器不一致
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		_, err := parser.Parse(task.CronExpr)
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}

	case "interval":
		if task.Interval <= 0 {
			return fmt.Errorf("interval must be positive")
		}

	case "once":
		if task.ExecuteAt == nil || task.ExecuteAt.Before(time.Now()) {
			return fmt.Errorf("execute_at must be in the future")
		}

	default:
		return fmt.Errorf("unsupported schedule type: %s", task.ScheduleType)
	}

	return nil
}

// calculateNextRun 计算下次执行时间
func (s *SchedulerService) calculateNextRun(task *models.ScheduledTask) *time.Time {
	switch task.ScheduleType {
	case "cron":
		// 修复: 使用秒级解析器，与 cron.WithSeconds() 和 validateSchedule 保持一致
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		schedule, _ := parser.Parse(task.CronExpr)
		next := schedule.Next(time.Now())
		return &next

	case "interval":
		next := time.Now().Add(time.Duration(task.Interval) * time.Second)
		return &next

	case "once":
		return task.ExecuteAt
	}

	return nil
}

// scheduleTask 调度任务
func (s *SchedulerService) scheduleTask(task *models.ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果已存在，先移除
	if entryID, exists := s.tasks[task.ID]; exists {
		s.cron.Remove(entryID)
		delete(s.tasks, task.ID)
	}
	// 如果是一次性任务的定时器，也要停止并移除
	if timer, exists := s.onceTimers[task.ID]; exists {
		timer.Stop()
		delete(s.onceTimers, task.ID)
	}

	// 创建执行函数
	taskFunc := func() {
		s.executeTask(task.ID)
	}

	switch task.ScheduleType {
	case "cron":
		entryID, err := s.cron.AddFunc(task.CronExpr, taskFunc)
		if err != nil {
			return fmt.Errorf("failed to add cron job: %w", err)
		}
		s.tasks[task.ID] = entryID

	case "interval":
		spec := fmt.Sprintf("@every %ds", task.Interval)
		entryID, err := s.cron.AddFunc(spec, taskFunc)
		if err != nil {
			return fmt.Errorf("failed to add interval job: %w", err)
		}
		s.tasks[task.ID] = entryID

	case "once":
		// 使用 time.AfterFunc 实现一次性执行
		delay := time.Until(*task.ExecuteAt)
		if delay <= 0 {
			// 如果时间已过，立即执行
			go taskFunc()
			return nil
		}
		// 创建定时器，在指定时间后执行一次
		timer := time.AfterFunc(delay, taskFunc)
		s.onceTimers[task.ID] = timer
		// 一次性任务不需要更新 next_run_at
		return nil
	}

	// 更新下次执行时间（非一次性任务）
	if entryID, exists := s.tasks[task.ID]; exists {
		nextRun := s.cron.Entry(entryID).Next
		// 在锁外执行数据库操作
		go func() {
			s.db.Model(task).Update("next_run_at", &nextRun)
		}()
	}

	return nil
}

// executeTask 执行任务
func (s *SchedulerService) executeTask(taskID string) {
	// 获取任务
	var task models.ScheduledTask
	if err := s.db.First(&task, "id = ?", taskID).Error; err != nil {
		s.logger.Error(s.ctx, "Task not found: %s", taskID)
		return
	}

	// 检查是否启用
	if !task.IsEnabled || task.Status != "active" {
		return
	}

	// 检查是否过期
	if task.ExpiresAt != nil && task.ExpiresAt.Before(time.Now()) {
		s.PauseTask(taskID)
		s.logger.Info(s.ctx, "Task expired: %s", taskID)
		return
	}

	// 检查最大运行次数
	if task.MaxRuns > 0 && task.RunCount >= task.MaxRuns {
		s.PauseTask(taskID)
		s.logger.Info(s.ctx, "Task reached max runs: %s", taskID)
		return
	}

	// 创建执行记录
	execution := &models.TaskExecution{
		ID:          uuid.New().String(),
		TaskID:      task.ID,
		TaskName:    task.Name,
		StartedAt:   time.Now(),
		Status:      "running",
		TriggerType: "schedule",
	}
	s.db.Create(execution)

	// 执行任务
	startTime := time.Now()
	result, execErr := s.runTask(&task)
	duration := time.Since(startTime).Milliseconds()

	// 更新执行记录
	now := time.Now()
	execution.CompletedAt = &now
	execution.Duration = duration

	if execErr != nil {
		execution.Status = "failed"
		execution.Error = execErr.Error()

		// 更新任务统计
		s.db.Model(&task).Updates(map[string]interface{}{
			"run_count":     gorm.Expr("run_count + 1"),
			"fail_count":    gorm.Expr("fail_count + 1"),
			"last_run_at":   &now,
			"last_error":    execErr.Error(),
		})

		s.logger.Error(s.ctx, "Task execution failed: %s - %v", taskID, execErr)
	} else {
		execution.Status = "success"
		execution.Result = result

		// 更新任务统计
		s.db.Model(&task).Updates(map[string]interface{}{
			"run_count":      gorm.Expr("run_count + 1"),
			"success_count":  gorm.Expr("success_count + 1"),
			"last_run_at":    &now,
			"last_error":     "",
		})

		s.logger.Info(s.ctx, "Task executed successfully: %s (%dms)", taskID, duration)
	}

	s.db.Save(execution)

	// 更新下次执行时间
	nextRun := s.calculateNextRun(&task)
	s.db.Model(&task).Update("next_run_at", nextRun)

	// 如果是一次性任务，完成后暂停
	if task.ScheduleType == "once" {
		s.PauseTask(taskID)
		s.db.Model(&task).Update("status", "completed")
	}
}

// runTask 运行任务
func (s *SchedulerService) runTask(task *models.ScheduledTask) (map[string]interface{}, error) {
	switch task.TaskType {
	case "workflow":
		return s.runWorkflowTask(task)

	case "reminder":
		return s.runReminderTask(task)

	case "webhook":
		return s.runWebhookTask(task)

	case "script":
		return s.runScriptTask(task)

	case "notification":
		return s.runNotificationTask(task)

	default:
		return nil, fmt.Errorf("unsupported task type: %s", task.TaskType)
	}
}

// runWorkflowTask 运行工作流任务
func (s *SchedulerService) runWorkflowTask(task *models.ScheduledTask) (map[string]interface{}, error) {
	workflowID, ok := task.TaskConfig["workflow_id"].(string)
	if !ok {
		return nil, fmt.Errorf("workflow_id is required")
	}

	inputVars, _ := task.TaskConfig["input_variables"].(map[string]interface{})

	// 执行工作流
	instance, err := s.workflowSvc.ExecuteWorkflowInstance(workflowID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"workflow_id":    workflowID,
		"instance_id":    instance.ID,
		"status":         instance.Status,
		"input_variables": inputVars,
	}, nil
}

// runReminderTask 运行提醒任务
func (s *SchedulerService) runReminderTask(task *models.ScheduledTask) (map[string]interface{}, error) {
	message, _ := task.TaskConfig["message"].(string)
	userID := task.UserID

	// 发送 WebSocket 通知
	if s.wsService != nil {
		msg := WebSocketMessage{
			Type:      "reminder",
			Timestamp: time.Now().Unix(),
			Payload: map[string]interface{}{
				"title":   task.Name,
				"message": message,
				"task_id": task.ID,
			},
		}
		s.wsService.SendToUser(userID, msg)
	}

	return map[string]interface{}{
		"message": message,
		"sent":    true,
	}, nil
}

// runWebhookTask 运行 Webhook 任务
func (s *SchedulerService) runWebhookTask(task *models.ScheduledTask) (map[string]interface{}, error) {
	webhookURL, ok := task.TaskConfig["url"].(string)
	if !ok || webhookURL == "" {
		return nil, fmt.Errorf("webhook url is required in task_config")
	}

	method, _ := task.TaskConfig["method"].(string)
	if method == "" {
		method = "POST"
	}

	headers, _ := task.TaskConfig["headers"].(map[string]interface{})
	payload, _ := task.TaskConfig["payload"].(map[string]interface{})
	timeoutSec, _ := task.TaskConfig["timeout"].(float64)
	if timeoutSec <= 0 {
		timeoutSec = 30
	}

	// 构建请求体
	var bodyReader io.Reader
	if payload != nil && (method == "POST" || method == "PUT") {
		bodyJSON, _ := json.Marshal(payload)
		bodyReader = bytes.NewReader(bodyJSON)
	}

	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, webhookURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, fmt.Sprintf("%v", v))
	}

	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	s.logger.Info(s.ctx, "Webhook executed: url=%s, status=%d", webhookURL, resp.StatusCode)

	return map[string]interface{}{
		"url":         webhookURL,
		"status_code": resp.StatusCode,
		"response":    string(respBody),
		"response_size": len(respBody),
	}, nil
}

// runScriptTask 运行脚本任务
func (s *SchedulerService) runScriptTask(task *models.ScheduledTask) (map[string]interface{}, error) {
	script, ok := task.TaskConfig["script"].(string)
	if !ok || script == "" {
		return nil, fmt.Errorf("script is required in task_config")
	}

	// 参数替换
	for key, value := range task.TaskConfig {
		if key == "script" {
			continue
		}
		placeholder := "{{" + key + "}}"
		script = strings.ReplaceAll(script, placeholder, fmt.Sprintf("%v", value))
	}

	// 安全检查：只允许预定义命令
	allowedCommands := []string{"echo", "date", "pwd", "ls", "cat", "head", "tail", "wc", "grep", "find", "python3", "python", "node", "bash", "sh", "curl", "wget"}
	words := strings.Fields(script)
	if len(words) == 0 {
		return nil, fmt.Errorf("empty script")
	}

	allowed := false
	for _, cmd := range allowedCommands {
		if words[0] == cmd {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("command not allowed: %s", words[0])
	}

	// 超时控制
	timeoutSec, _ := task.TaskConfig["timeout"].(float64)
	if timeoutSec <= 0 {
		timeoutSec = 60
	}

	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	if err != nil {
		return map[string]interface{}{
			"stdout":   stdout.String(),
			"stderr":   stderr.String(),
			"duration": duration.String(),
			"success":  false,
			"error":    err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"stdout":   stdout.String(),
		"stderr":   stderr.String(),
		"duration": duration.String(),
		"success":  true,
	}, nil
}

// runNotificationTask 运行通知任务
func (s *SchedulerService) runNotificationTask(task *models.ScheduledTask) (map[string]interface{}, error) {
	title, _ := task.TaskConfig["title"].(string)
	body, _ := task.TaskConfig["body"].(string)

	// 发送 WebSocket 通知
	if s.wsService != nil {
		msg := WebSocketMessage{
			Type:      "notification",
			Timestamp: time.Now().Unix(),
			Payload: map[string]interface{}{
				"title":   title,
				"body":    body,
				"task_id": task.ID,
			},
		}
		s.wsService.Broadcast(msg)
	}

	return map[string]interface{}{
		"title":  title,
		"body":   body,
		"sent":   true,
	}, nil
}

// PauseTask 暂停任务
func (s *SchedulerService) PauseTask(taskID string) error {
	// 先获取需要删除的 entryID，然后在锁外执行数据库操作
	var entryID cron.EntryID
	var hasEntry bool
	var timer *time.Timer
	var hasTimer bool

	s.mu.Lock()
	if e, exists := s.tasks[taskID]; exists {
		entryID = e
		hasEntry = true
		delete(s.tasks, taskID)
	}
	if t, exists := s.onceTimers[taskID]; exists {
		timer = t
		hasTimer = true
		delete(s.onceTimers, taskID)
	}
	s.mu.Unlock()

	// 在锁外执行耗时操作
	if hasEntry {
		s.cron.Remove(entryID)
	}
	if hasTimer && timer != nil {
		timer.Stop()
	}

	// 数据库操作在锁外执行
	if err := s.db.Model(&models.ScheduledTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":     "paused",
			"is_enabled": false,
			"updated_at": time.Now(),
		}).Error; err != nil {
		s.logger.Error(s.ctx, "Failed to update task status: %v", err)
		return fmt.Errorf("failed to pause task: %w", err)
	}

	s.logger.Info(s.ctx, "Task paused: %s", taskID)
	return nil
}

// ResumeTask 恢复任务
func (s *SchedulerService) ResumeTask(taskID string) error {
	var task models.ScheduledTask
	if err := s.db.First(&task, "id = ?", taskID).Error; err != nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if err := s.scheduleTask(&task); err != nil {
		return err
	}

	s.db.Model(&task).Updates(map[string]interface{}{
		"status":     "active",
		"is_enabled": true,
		"updated_at": time.Now(),
	})

	s.logger.Info(s.ctx, "Task resumed: %s", taskID)
	return nil
}

// DeleteTask 删除任务
func (s *SchedulerService) DeleteTask(taskID string) error {
	s.PauseTask(taskID)
	return s.db.Delete(&models.ScheduledTask{}, "id = ?", taskID).Error
}

// GetTask 获取任务
func (s *SchedulerService) GetTask(taskID string) (*models.ScheduledTask, error) {
	var task models.ScheduledTask
	if err := s.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// ListTasks 列出任务
func (s *SchedulerService) ListTasks(userID string) ([]models.ScheduledTask, error) {
	var tasks []models.ScheduledTask
	query := s.db.Order("created_at DESC")
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// ExecuteNow 立即执行任务
func (s *SchedulerService) ExecuteNow(taskID string, userID string) (*models.TaskExecution, error) {
	var task models.ScheduledTask
	if err := s.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, fmt.Errorf("task not found")
	}

	// 创建执行记录
	execution := &models.TaskExecution{
		ID:          uuid.New().String(),
		TaskID:      task.ID,
		TaskName:    task.Name,
		StartedAt:   time.Now(),
		Status:      "running",
		TriggerType: "manual",
		TriggeredBy: userID,
	}
	s.db.Create(execution)

	// 执行任务
	startTime := time.Now()
	result, execErr := s.runTask(&task)
	duration := time.Since(startTime).Milliseconds()

	// 更新执行记录
	now := time.Now()
	execution.CompletedAt = &now
	execution.Duration = duration

	if execErr != nil {
		execution.Status = "failed"
		execution.Error = execErr.Error()
	} else {
		execution.Status = "success"
		execution.Result = result
	}

	s.db.Save(execution)

	return execution, nil
}

// GetExecutions 获取执行历史
func (s *SchedulerService) GetExecutions(taskID string, limit int) ([]models.TaskExecution, error) {
	var executions []models.TaskExecution
	query := s.db.Order("started_at DESC")

	if taskID != "" {
		query = query.Where("task_id = ?", taskID)
	}

	if err := query.Limit(limit).Find(&executions).Error; err != nil {
		return nil, err
	}

	return executions, nil
}

// ========================================
// 提醒功能
// ========================================

// CreateReminder 创建提醒
func (s *SchedulerService) CreateReminder(reminder *models.TaskReminder) error {
	reminder.Status = "pending"
	reminder.CreatedAt = time.Now()
	reminder.UpdatedAt = time.Now()

	if err := s.db.Create(reminder).Error; err != nil {
		return fmt.Errorf("failed to create reminder: %w", err)
	}

	return nil
}

// reminderChecker 提醒检查器
func (s *SchedulerService) reminderChecker() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkReminders()
		}
	}
}

// checkReminders 检查提醒
func (s *SchedulerService) checkReminders() {
	now := time.Now()

	var reminders []models.TaskReminder
	s.db.Where("status = ? AND remind_at <= ?", "pending", now).
		Find(&reminders)

	for _, reminder := range reminders {
		s.sendReminder(&reminder)
	}
}

// sendReminder 发送提醒
func (s *SchedulerService) sendReminder(reminder *models.TaskReminder) {
	// 发送 WebSocket 通知
	if s.wsService != nil {
		msg := WebSocketMessage{
			Type:      "reminder",
			Timestamp: time.Now().Unix(),
			Payload: map[string]interface{}{
				"reminder_id": reminder.ID,
				"title":       reminder.Title,
				"content":     reminder.Content,
				"dialogue_id": reminder.DialogueID,
			},
		}
		s.wsService.SendToUser(reminder.UserID, msg)
	}

	// 更新状态
	now := time.Now()
	reminder.Status = "sent"
	reminder.UpdatedAt = now

	// 如果是重复提醒，计算下次提醒时间
	if reminder.RepeatType != "none" {
		nextRemind := s.calculateNextReminder(reminder)
		if nextRemind != nil {
			reminder.RemindAt = *nextRemind
			reminder.Status = "pending"
		}
	}

	s.db.Save(reminder)

	s.logger.Info(s.ctx, "Reminder sent: %s - %s", reminder.ID, reminder.Title)
}

// calculateNextReminder 计算下次提醒时间
func (s *SchedulerService) calculateNextReminder(reminder *models.TaskReminder) *time.Time {
	var next time.Time

	switch reminder.RepeatType {
	case "daily":
		next = reminder.RemindAt.Add(24 * time.Hour)
	case "weekly":
		next = reminder.RemindAt.AddDate(0, 0, 7)
	case "monthly":
		next = reminder.RemindAt.AddDate(0, 1, 0)
	case "yearly":
		next = reminder.RemindAt.AddDate(1, 0, 0)
	default:
		return nil
	}

	return &next
}

// GetReminders 获取提醒列表
func (s *SchedulerService) GetReminders(userID string, status string) ([]models.TaskReminder, error) {
	var reminders []models.TaskReminder
	query := s.db.Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Order("remind_at ASC").Find(&reminders).Error; err != nil {
		return nil, err
	}

	return reminders, nil
}

// SnoozeReminder 延后提醒
func (s *SchedulerService) SnoozeReminder(reminderID string, delayMinutes int) error {
	var reminder models.TaskReminder
	if err := s.db.First(&reminder, "id = ?", reminderID).Error; err != nil {
		return err
	}

	newTime := time.Now().Add(time.Duration(delayMinutes) * time.Minute)
	reminder.RemindAt = newTime
	reminder.Status = "pending"
	reminder.SnoozeCount++
	now := time.Now()
	reminder.SnoozedAt = &now
	reminder.UpdatedAt = now

	return s.db.Save(&reminder).Error
}

// CancelReminder 取消提醒
func (s *SchedulerService) CancelReminder(reminderID string) error {
	return s.db.Model(&models.TaskReminder{}).
		Where("id = ?", reminderID).
		Updates(map[string]interface{}{
			"status":     "cancelled",
			"updated_at": time.Now(),
		}).Error
}

// Stop 停止调度服务
func (s *SchedulerService) Stop() {
	s.cancel()
	s.cron.Stop()
	s.wg.Wait()
	s.logger.Info(s.ctx, "Scheduler service stopped")
}
