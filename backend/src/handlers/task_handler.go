package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services"
)

// TaskHandler 任务处理器
type TaskHandler struct {
	taskService *services.TaskService
}

// NewTaskHandler 创建任务处理器
func NewTaskHandler(taskService *services.TaskService) *TaskHandler {
	return &TaskHandler{
		taskService: taskService,
	}
}

// ============ 任务管理接口 ============

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	Title       string                 `json:"title" binding:"required"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Priority    string                 `json:"priority"`
	TeamID      string                 `json:"team_id"`
	CreatedBy   string                 `json:"created_by"`
	Context     models.TaskContext     `json:"context"`
	Variables   map[string]interface{} `json:"variables"`
}

// CreateTaskHandler 创建任务处理器
func (h *TaskHandler) CreateTaskHandler(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := &models.Task{
		Title:       req.Title,
		Description: req.Description,
		Type:        req.Type,
		Priority:    req.Priority,
		TeamID:      req.TeamID,
		CreatedBy:   req.CreatedBy,
		Context:     req.Context,
		Status:      "pending",
	}

	if err := h.taskService.CreateTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// DecomposeTaskHandler 分解任务处理器
func (h *TaskHandler) DecomposeTaskHandler(c *gin.Context) {
	var req services.DecomposeTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	result, err := h.taskService.DecomposeTask(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetTaskHandler 获取任务处理器
func (h *TaskHandler) GetTaskHandler(c *gin.Context) {
	id := c.Param("id")

	task, err := h.taskService.GetTask(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, task)
}

// ListTasksHandler 列出任务处理器
func (h *TaskHandler) ListTasksHandler(c *gin.Context) {
	filter := services.TaskFilter{
		TeamID:     c.Query("team_id"),
		Status:     c.Query("status"),
		Type:       c.Query("type"),
		Priority:   c.Query("priority"),
		AssignedTo: c.Query("assigned_to"),
		Page:       1,
		PageSize:   20,
	}

	tasks, total, err := h.taskService.ListTasks(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
		"total": total,
		"page":  filter.Page,
		"page_size": filter.PageSize,
	})
}

// UpdateTaskHandler 更新任务处理器
func (h *TaskHandler) UpdateTaskHandler(c *gin.Context) {
	id := c.Param("id")

	task, err := h.taskService.GetTask(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	var req struct {
		Title       string              `json:"title"`
		Description string              `json:"description"`
		Priority    string              `json:"priority"`
		Context     models.TaskContext  `json:"context"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Title != "" {
		task.Title = req.Title
	}
	if req.Description != "" {
		task.Description = req.Description
	}
	if req.Priority != "" {
		task.Priority = req.Priority
	}
	task.Context = req.Context

	if err := h.taskService.UpdateTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// UpdateTaskStatusHandler 更新任务状态处理器
func (h *TaskHandler) UpdateTaskStatusHandler(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Status    string `json:"status" binding:"required"`
		UpdatedBy string `json:"updated_by"`
		Reason    string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.taskService.UpdateTaskStatus(id, req.Status, req.UpdatedBy, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	task, _ := h.taskService.GetTask(id)
	c.JSON(http.StatusOK, task)
}

// DeleteTaskHandler 删除任务处理器
func (h *TaskHandler) DeleteTaskHandler(c *gin.Context) {
	id := c.Param("id")

	if err := h.taskService.DeleteTask(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task deleted successfully"})
}

// ============ 子任务管理接口 ============

// UpdateSubtaskStatusHandler 更新子任务状态处理器
func (h *TaskHandler) UpdateSubtaskStatusHandler(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.taskService.UpdateSubtaskStatus(id, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subtask status updated"})
}

// ============ 进度跟踪接口 ============

// UpdateProgressHandler 更新进度处理器
func (h *TaskHandler) UpdateProgressHandler(c *gin.Context) {
	var req struct {
		TaskID    string `json:"task_id" binding:"required"`
		SubtaskID string `json:"subtask_id"`
		Stage     string `json:"stage" binding:"required"`
		Percent   int    `json:"percent" binding:"required,min=0,max=100"`
		Message   string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.taskService.UpdateProgress(req.TaskID, req.SubtaskID, req.Stage, req.Percent, req.Message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Progress updated"})
}

// GetTaskProgressHandler 获取任务进度处理器
func (h *TaskHandler) GetTaskProgressHandler(c *gin.Context) {
	taskID := c.Param("id")

	progresses, err := h.taskService.GetTaskProgress(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, progresses)
}

// ============ 团队成员管理接口 ============

// CreateMemberRequest 创建成员请求
type CreateMemberRequest struct {
	TeamID       string                `json:"team_id" binding:"required"`
	Name         string               `json:"name" binding:"required"`
	Role         string               `json:"role" binding:"required"`
	Capabilities []models.Capability  `json:"capabilities"`
	Specialization []string           `json:"specialization"`
	Experience   map[string]int       `json:"experience"`
	MaxLoad      int                  `json:"max_load"`
}

// CreateMemberHandler 创建成员处理器
func (h *TaskHandler) CreateMemberHandler(c *gin.Context) {
	var req CreateMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	member := &models.TeamMember{
		TeamID:        req.TeamID,
		Name:          req.Name,
		Role:          req.Role,
		Capabilities:  req.Capabilities,
		Specialization: req.Specialization,
		Experience:    req.Experience,
		MaxLoad:       req.MaxLoad,
		Availability:  "available",
		CurrentLoad:   0,
	}

	if err := h.taskService.CreateMember(member); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, member)
}

// ListMembersHandler 列出成员处理器
func (h *TaskHandler) ListMembersHandler(c *gin.Context) {
	teamID := c.Query("team_id")
	if teamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "team_id is required"})
		return
	}

	members, err := h.taskService.ListMembers(teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, members)
}

// UpdateMemberHandler 更新成员处理器
func (h *TaskHandler) UpdateMemberHandler(c *gin.Context) {
	id := c.Param("id")

	member, err := h.taskService.GetMember(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Member not found"})
		return
	}

	var req struct {
		Availability string               `json:"availability"`
		MaxLoad      int                  `json:"max_load"`
		Capabilities []models.Capability  `json:"capabilities"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Availability != "" {
		member.Availability = req.Availability
	}
	if req.MaxLoad > 0 {
		member.MaxLoad = req.MaxLoad
	}
	if len(req.Capabilities) > 0 {
		member.Capabilities = req.Capabilities
	}

	if err := h.taskService.UpdateMember(member); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, member)
}

// ============ 高级功能接口 ============

// GetTaskOverviewHandler 获取任务概览处理器
func (h *TaskHandler) GetTaskOverviewHandler(c *gin.Context) {
	teamID := c.Query("team_id")
	if teamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "team_id is required"})
		return
	}

	overview, err := h.taskService.GetTaskOverview(teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, overview)
}

// GetMemberTasksHandler 获取成员任务处理器
func (h *TaskHandler) GetMemberTasksHandler(c *gin.Context) {
	memberID := c.Param("id")

	tasks, subtasks, err := h.taskService.GetMemberTasks(memberID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":    tasks,
		"subtasks": subtasks,
	})
}

// RetryFailedTaskHandler 重试失败任务处理器
func (h *TaskHandler) RetryFailedTaskHandler(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.taskService.RetryFailedTask(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task retry initiated"})
}

// GenerateTaskSummaryHandler 生成任务总结处理器
func (h *TaskHandler) GenerateTaskSummaryHandler(c *gin.Context) {
	taskID := c.Param("id")

	ctx := c.Request.Context()
	summary, err := h.taskService.GenerateTaskSummary(ctx, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// CancelTaskHandler 取消任务处理器
func (h *TaskHandler) CancelTaskHandler(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.taskService.CancelTask(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task cancelled"})
}

// ReassignTaskHandler 重新分配任务处理器
func (h *TaskHandler) ReassignTaskHandler(c *gin.Context) {
	taskID := c.Param("id")

	var req struct {
		NewMemberID string `json:"new_member_id" binding:"required"`
		Reason      string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.taskService.ReassignTask(taskID, req.NewMemberID, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task reassigned"})
}

// GetTemplateHandler 获取模板处理器
func (h *TaskHandler) GetTemplateHandler(c *gin.Context) {
	taskType := c.Param("type")

	template, err := h.taskService.GetTemplate(taskType)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	c.JSON(http.StatusOK, template)
}

// ListTemplatesHandler 列出模板处理器
func (h *TaskHandler) ListTemplatesHandler(c *gin.Context) {
	templates := h.taskService.ListTemplates()
	c.JSON(http.StatusOK, templates)
}

// CanStartTaskHandler 检查任务是否可以开始处理器
func (h *TaskHandler) CanStartTaskHandler(c *gin.Context) {
	taskID := c.Param("id")

	canStart, blockedBy, err := h.taskService.CanStartTask(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"can_start":  canStart,
		"blocked_by": blockedBy,
	})
}
