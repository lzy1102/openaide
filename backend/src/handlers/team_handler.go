package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"openaide/backend/src/services"
)

// TeamHandler 团队处理器
type TeamHandler struct {
	coordinator *services.TeamCoordinatorService
}

// NewTeamHandler 创建团队处理器
func NewTeamHandler(coordinator *services.TeamCoordinatorService) *TeamHandler {
	return &TeamHandler{
		coordinator: coordinator,
	}
}

// RegisterRoutes 注册路由
func (h *TeamHandler) RegisterRoutes(r *gin.RouterGroup) {
	teams := r.Group("/api/teams")
	{
		// 团队管理
		teams.POST("", h.CreateTeam)
		teams.GET("", h.ListTeams)
		teams.GET("/:id", h.GetTeam)
		teams.DELETE("/:id", h.DeleteTeam)
		teams.GET("/:id/status", h.GetTeamStatus)

		// 成员管理
		teams.POST("/:id/agents", h.AddMember)
		teams.GET("/:id/agents", h.ListMembers)
		teams.DELETE("/:id/agents/:agentId", h.RemoveMember)
		teams.PUT("/:id/agents/:agentId/status", h.UpdateMemberStatus)
		teams.POST("/:id/agents/:agentId/heartbeat", h.UpdateHeartbeat)

		// 任务管理
		teams.POST("/:id/tasks", h.CreateTask)
		teams.GET("/:id/tasks", h.ListTasks)
		teams.GET("/:id/tasks/:taskId", h.GetTask)
		teams.PUT("/:id/tasks/:taskId/assign", h.AssignTask)
		teams.POST("/:id/tasks/:taskId/decompose", h.DecomposeTask)
		teams.POST("/:id/tasks/:taskId/retry", h.RetryTask)

		// Agent 管理
		teams.POST("/agents/register", h.RegisterAgent)
		teams.POST("/agents/:agentId/unregister", h.UnregisterAgent)
		teams.GET("/agents", h.ListAgents)
		teams.GET("/agents/:agentId", h.GetAgent)

		// 进度跟踪
		teams.GET("/:id/progress", h.GetProgress)
	}
}

// ============ 团队管理 ============

// CreateTeamRequest 创建团队请求
type CreateTeamRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
}

// CreateTeam 创建团队
func (h *TeamHandler) CreateTeam(c *gin.Context) {
	var req CreateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	team, err := h.coordinator.CreateTeam(req.Name, req.Description, req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, team)
}

// ListTeams 列出团队
func (h *TeamHandler) ListTeams(c *gin.Context) {
	teams, err := h.coordinator.ListTeams()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, teams)
}

// GetTeam 获取团队
func (h *TeamHandler) GetTeam(c *gin.Context) {
	id := c.Param("id")

	team, err := h.coordinator.GetTeam(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, team)
}

// DeleteTeam 删除团队
func (h *TeamHandler) DeleteTeam(c *gin.Context) {
	id := c.Param("id")
	if err := h.coordinator.DeleteTeam(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Team deleted"})
}

// GetTeamStatus 获取团队状态
func (h *TeamHandler) GetTeamStatus(c *gin.Context) {
	id := c.Param("id")

	status, err := h.coordinator.GetTeamStatus(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, status)
}

// ============ 成员管理 ============

// AddMemberRequest 添加成员请求
type AddMemberRequest struct {
	Name         string   `json:"name" binding:"required"`
	Role         string   `json:"role" binding:"required"`
	AgentType    string   `json:"agent_type"`
	Capabilities []string `json:"capabilities"`
	Config       map[string]interface{} `json:"config"`
}

// AddMember 添加成员
func (h *TeamHandler) AddMember(c *gin.Context) {
	teamID := c.Param("id")

	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	member, err := h.coordinator.AddMember(teamID, req.Name, req.Role, req.AgentType, req.Capabilities, req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, member)
}

// ListMembers 列出成员
func (h *TeamHandler) ListMembers(c *gin.Context) {
	teamID := c.Param("id")

	members, err := h.coordinator.ListMembers(teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, members)
}

// RemoveMember 移除成员
func (h *TeamHandler) RemoveMember(c *gin.Context) {
	teamID := c.Param("id")
	agentID := c.Param("agentId")

	err := h.coordinator.RemoveMember(teamID, agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member removed"})
}

// UpdateMemberStatusRequest 更新成员状态请求
type UpdateMemberStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// UpdateMemberStatus 更新成员状态
func (h *TeamHandler) UpdateMemberStatus(c *gin.Context) {
	agentID := c.Param("agentId")

	var req UpdateMemberStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.coordinator.UpdateMemberStatus(agentID, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}

// UpdateHeartbeat 更新心跳
func (h *TeamHandler) UpdateHeartbeat(c *gin.Context) {
	agentID := c.Param("agentId")

	err := h.coordinator.UpdateHeartbeat(agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Heartbeat updated"})
}

// ============ 任务管理 ============

// TeamCreateTaskRequest 创建任务请求（团队版本）
type TeamCreateTaskRequest struct {
	Title       string                 `json:"title" binding:"required"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Priority    string                 `json:"priority"`
	CreatedBy   string                 `json:"created_by" binding:"required"`
	Input       map[string]interface{} `json:"input"`
}

// CreateTask 创建任务
func (h *TeamHandler) CreateTask(c *gin.Context) {
	teamID := c.Param("id")

	var req TeamCreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.coordinator.CreateTask(teamID, "", req.Title, req.Description, req.Type, req.Priority, req.CreatedBy, req.Input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, task)
}

// ListTasksRequest 列出任务请求
type ListTasksRequest struct {
	Status     string `form:"status"`
	AssignedTo string `form:"assigned_to"`
	Page       int    `form:"page" binding:"required,min=1"`
	PageSize   int    `form:"page_size" binding:"required,min=1,max=100"`
}

// ListTasks 列出任务
func (h *TeamHandler) ListTasks(c *gin.Context) {
	var req ListTasksRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tasks, total, err := h.coordinator.ListTasks(c.Param("id"), req.Status, req.AssignedTo, req.Page, req.PageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks":     tasks,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetTask 获取任务
func (h *TeamHandler) GetTask(c *gin.Context) {
	taskID := c.Param("taskId")

	task, err := h.coordinator.GetTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// AssignTaskRequest 分配任务请求
type AssignTaskRequest struct {
	MemberID string `json:"member_id" binding:"required"`
}

// AssignTask 分配任务
func (h *TeamHandler) AssignTask(c *gin.Context) {
	taskID := c.Param("taskId")

	var req AssignTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.coordinator.AssignTask(taskID, req.MemberID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task assigned"})
}

// DecomposeTask 分解任务
func (h *TeamHandler) DecomposeTask(c *gin.Context) {
	taskID := c.Param("taskId")

	subtasks, err := h.coordinator.DecomposeTask(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subtasks": subtasks,
		"count":    len(subtasks),
	})
}

// RetryTask 重试任务
func (h *TeamHandler) RetryTask(c *gin.Context) {
	taskID := c.Param("taskId")

	err := h.coordinator.RetryTask(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task retry initiated"})
}

// ============ Agent 管理 ============

// RegisterAgentRequest 注册 Agent 请求
type RegisterAgentRequest struct {
	ID           string                 `json:"id" binding:"required"`
	Name         string                 `json:"name" binding:"required"`
	Type         string                 `json:"type" binding:"required"` // human, ai, hybrid
	Capabilities []string               `json:"capabilities"`
	Config       map[string]interface{} `json:"config"`
}

// RegisterAgent 注册 Agent
func (h *TeamHandler) RegisterAgent(c *gin.Context) {
	var req RegisterAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.coordinator.RegisterAgent(req.ID, req.Name, req.Type, req.Capabilities, req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Agent registered",
		"agent_id": req.ID,
	})
}

// UnregisterAgent 注销 Agent
func (h *TeamHandler) UnregisterAgent(c *gin.Context) {
	agentID := c.Param("agentId")

	err := h.coordinator.UnregisterAgent(agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Agent unregistered",
		"agent_id": agentID,
	})
}

// ListAgents 列出所有 Agent
func (h *TeamHandler) ListAgents(c *gin.Context) {
	agents := h.coordinator.ListAgents()

	c.JSON(http.StatusOK, agents)
}

// GetAgent 获取 Agent
func (h *TeamHandler) GetAgent(c *gin.Context) {
	agentID := c.Param("agentId")

	agent, err := h.coordinator.GetAgentStatus(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, agent)
}

// ============ 进度跟踪 ============

// GetProgress 获取进度
func (h *TeamHandler) GetProgress(c *gin.Context) {
	teamID := c.Param("id")

	progress, err := h.coordinator.TrackProgress(teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, progress)
}
