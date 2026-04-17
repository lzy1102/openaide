package handlers

import (
	"net/http"
	"strconv"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// SchedulerHandler 调度处理器
type SchedulerHandler struct {
	schedulerService *services.SchedulerService
}

// NewSchedulerHandler 创建调度处理器
func NewSchedulerHandler(schedulerService *services.SchedulerService) *SchedulerHandler {
	return &SchedulerHandler{
		schedulerService: schedulerService,
	}
}

// CreateTask 创建定时任务
func (h *SchedulerHandler) CreateTask(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var task models.ScheduledTask
	if err := c.ShouldBindJSON(&task); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task.UserID = userID.(string)

	if err := h.schedulerService.CreateTask(&task); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, task)
}

// ListTasks 列出任务
func (h *SchedulerHandler) ListTasks(c *gin.Context) {
	userID, _ := c.Get("user_id")

	tasks, err := h.schedulerService.ListTasks(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// GetTask 获取任务详情
func (h *SchedulerHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")

	task, err := h.schedulerService.GetTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, task)
}

// PauseTask 暂停任务
func (h *SchedulerHandler) PauseTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.schedulerService.PauseTask(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "task paused",
		"task_id": taskID,
	})
}

// ResumeTask 恢复任务
func (h *SchedulerHandler) ResumeTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.schedulerService.ResumeTask(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "task resumed",
		"task_id": taskID,
	})
}

// DeleteTask 删除任务
func (h *SchedulerHandler) DeleteTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.schedulerService.DeleteTask(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "task deleted",
		"task_id": taskID,
	})
}

// ExecuteNow 立即执行任务
func (h *SchedulerHandler) ExecuteNow(c *gin.Context) {
	taskID := c.Param("id")
	userID, _ := c.Get("user_id")

	execution, err := h.schedulerService.ExecuteNow(taskID, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, execution)
}

// GetExecutions 获取执行历史
func (h *SchedulerHandler) GetExecutions(c *gin.Context) {
	taskID := c.Param("id")
	limitStr := c.DefaultQuery("limit", "20")

	limit, _ := strconv.Atoi(limitStr)

	executions, err := h.schedulerService.GetExecutions(taskID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"executions": executions,
		"count":      len(executions),
	})
}

// CreateReminder 创建提醒
func (h *SchedulerHandler) CreateReminder(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var reminder models.TaskReminder
	if err := c.ShouldBindJSON(&reminder); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reminder.UserID = userID.(string)

	if err := h.schedulerService.CreateReminder(&reminder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, reminder)
}

// ListReminders 列出提醒
func (h *SchedulerHandler) ListReminders(c *gin.Context) {
	userID, _ := c.Get("user_id")
	status := c.Query("status")

	reminders, err := h.schedulerService.GetReminders(userID.(string), status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reminders": reminders,
		"count":     len(reminders),
	})
}

// SnoozeReminder 延后提醒
func (h *SchedulerHandler) SnoozeReminder(c *gin.Context) {
	reminderID := c.Param("id")

	var req struct {
		DelayMinutes int `json:"delay_minutes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.schedulerService.SnoozeReminder(reminderID, req.DelayMinutes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "reminder snoozed",
		"reminder_id": reminderID,
	})
}

// CancelReminder 取消提醒
func (h *SchedulerHandler) CancelReminder(c *gin.Context) {
	reminderID := c.Param("id")

	if err := h.schedulerService.CancelReminder(reminderID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "reminder cancelled",
		"reminder_id": reminderID,
	})
}

// RegisterRoutes 注册路由
func (h *SchedulerHandler) RegisterRoutes(r *gin.RouterGroup) {
	tasks := r.Group("/scheduler/tasks")
	{
		tasks.GET("", h.ListTasks)
		tasks.POST("", h.CreateTask)
		tasks.GET("/:id", h.GetTask)
		tasks.POST("/:id/pause", h.PauseTask)
		tasks.POST("/:id/resume", h.ResumeTask)
		tasks.DELETE("/:id", h.DeleteTask)
		tasks.POST("/:id/execute", h.ExecuteNow)
		tasks.GET("/:id/executions", h.GetExecutions)
	}

	reminders := r.Group("/scheduler/reminders")
	{
		reminders.GET("", h.ListReminders)
		reminders.POST("", h.CreateReminder)
		reminders.POST("/:id/snooze", h.SnoozeReminder)
		reminders.DELETE("/:id", h.CancelReminder)
	}
}
