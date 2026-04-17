package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"openaide/backend/src/models"
	"gorm.io/gorm"
)

// SimpleTaskCoordinator 简化的团队协调服务接口
type SimpleTaskCoordinator interface {
	// TrackProgress 跟踪所有任务进度
	TrackProgress() map[string]TaskProgressInfo
	// UpdateStatus 更新任务状态
	UpdateStatus(taskID string, status string) error
	// NotifyMember 通知成员
	NotifyMember(memberName string, message string) error
	// RetryTask 重试失败任务
	RetryTask(taskID string) error
	// GenerateReport 生成进度报告
	GenerateReport() string
}

// TaskProgressInfo 任务进度信息
type TaskProgressInfo struct {
	TaskID      string    `json:"task_id"`
	Title      string    `json:"title"`
	Status      string    `json:"status"`
	Progress    int      `json:"progress"`
	StartTime  time.Time `json:"start_time"`
	UpdatedAt  time.Time `json:"updated_at"`
	AssignedTo  string    `json:"assigned_to,omitempty"`
	Subtasks    int      `json:"subtasks"`
	Completed  int      `json:"completed"`
	Errors     int      `json:"errors"`
}
// simpleTeamCoordinatorService 简化的团队协调服务实现
type simpleTeamCoordinatorService struct {
	db       *gorm.DB
	logger   *LoggerService
	mu       sync.RWMutex
	progress map[string]TaskProgressInfo
}

// NewSimpleTeamCoordinatorService 创建简化的团队协调服务
func NewSimpleTeamCoordinatorService(db *gorm.DB, logger *LoggerService) SimpleTaskCoordinator {
	return &simpleTeamCoordinatorService{
		db:       db,
		logger:   logger,
		progress: make(map[string]TaskProgressInfo),
	}
}

// TrackProgress 跟踪所有任务进度
func (s *simpleTeamCoordinatorService) TrackProgress() map[string]TaskProgressInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 从数据库加载
	var taskRecords []models.Task
	s.db.Find(&taskRecords)

	// 转换为进度
	for _, task := range taskRecords {
		if _, exists := s.progress[task.ID]; !exists {
			s.progress[task.ID] = TaskProgressInfo{
				TaskID:     task.ID,
				Title:      task.Title,
				Status:     task.Status,
				StartTime: task.CreatedAt,
				UpdatedAt:  task.UpdatedAt,
				AssignedTo: task.AssignedTo,
			}
		}
		// 更新现有进度信息
		progress := s.progress[task.ID]
		progress.Status = task.Status
		progress.Progress = calculateProgress(task)
		progress.UpdatedAt = task.UpdatedAt
		s.progress[task.ID] = progress
	}
	return s.progress
}
// calculateProgress 计算进度
func calculateProgress(task models.Task) int {
	// 基于子任务完成情况计算进度
	if len(task.Subtasks) == 0 {
		if task.Status == "completed" {
			return 100
		}
		return 0
	}

	completed := 0
	for _, st := range task.Subtasks {
		if st.Status == "completed" {
			completed++
		}
	}

	return int(float64(completed) / float64(len(task.Subtasks)) * 100)
}
// UpdateStatus 更新任务状态
func (s *simpleTeamCoordinatorService) UpdateStatus(taskID string, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Model(&models.Task{}).Where("id = ?", taskID).Update("status", status).Error
}

// NotifyMember 通知成员
func (s *simpleTeamCoordinatorService) NotifyMember(memberName string, message string) error {
	s.logger.Info(context.Background(), "Notify %s: %s", memberName, message)
	return nil
}

// RetryTask 重试失败任务
func (s *simpleTeamCoordinatorService) RetryTask(taskID string) error {
	task, exists := s.progress[taskID]
	if !exists {
		return fmt.Errorf("task not found")
	}
	// 重置任务状态
	task.Status = "pending"
	task.Progress = 0
	s.progress[taskID] = task

	// 更新数据库
	return s.db.Model(&models.Task{}).Where("id = ?", taskID).Update("status", "pending").Error
}
// GenerateReport 生成进度报告
func (s *simpleTeamCoordinatorService) GenerateReport() string {
	report := "Task Progress Report\n"
	report += "=================\n\n"
	for _, progress := range s.progress {
		report += fmt.Sprintf("Task: %s\n", progress.Title)
		report += fmt.Sprintf("  Status: %s\n", progress.Status)
		report += fmt.Sprintf("  Progress: %d%%\n", progress.Progress)
		report += fmt.Sprintf("  Assigned: %s\n", progress.AssignedTo)
		report += "-----------------\n"
	}
	return report
}
