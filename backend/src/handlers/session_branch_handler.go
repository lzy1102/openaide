package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"openaide/backend/src/response"
	"openaide/backend/src/services"
)

type SessionBranchHandler struct {
	BranchSvc *services.SessionBranchService
}

func NewSessionBranchHandler(svc *services.SessionBranchService) *SessionBranchHandler {
	return &SessionBranchHandler{BranchSvc: svc}
}

func (h *SessionBranchHandler) RegisterRoutes(r *gin.RouterGroup) {
	branch := r.Group("/session-branches")
	{
		branch.POST("/fork", h.Fork)
		branch.GET("/list/:dialogueID", h.ListBranches)
		branch.GET("/:id", h.GetBranch)
		branch.POST("/switch", h.SwitchBranch)
		branch.DELETE("/:id", h.DeleteBranch)
		branch.PUT("/:id/rename", h.RenameBranch)
		branch.GET("/timeline/:dialogueID", h.GetTimeline)
	}
}

func (h *SessionBranchHandler) Fork(c *gin.Context) {
	var req struct {
		DialogueID  string `json:"dialogue_id"`
		UserID      string `json:"user_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		BranchPoint int    `json:"branch_point"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	branch, err := h.BranchSvc.Fork(c.Request.Context(), req.DialogueID, req.UserID, req.Name, req.Description, req.BranchPoint)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: branch})
}

func (h *SessionBranchHandler) ListBranches(c *gin.Context) {
	dialogueID := c.Param("dialogueID")
	branches, err := h.BranchSvc.ListBranches(c.Request.Context(), dialogueID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: branches})
}

func (h *SessionBranchHandler) GetBranch(c *gin.Context) {
	id := c.Param("id")
	branch, err := h.BranchSvc.GetBranch(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: branch})
}

func (h *SessionBranchHandler) SwitchBranch(c *gin.Context) {
	var req struct {
		DialogueID string `json:"dialogue_id"`
		BranchID   string `json:"branch_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	if err := h.BranchSvc.SwitchToBranch(c.Request.Context(), req.DialogueID, req.BranchID); err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "switched"})
}

func (h *SessionBranchHandler) DeleteBranch(c *gin.Context) {
	id := c.Param("id")
	if err := h.BranchSvc.DeleteBranch(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "deleted"})
}

func (h *SessionBranchHandler) RenameBranch(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	if err := h.BranchSvc.RenameBranch(c.Request.Context(), id, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "renamed"})
}

func (h *SessionBranchHandler) GetTimeline(c *gin.Context) {
	dialogueID := c.Param("dialogueID")
	branches, err := h.BranchSvc.GetBranchTimeline(c.Request.Context(), dialogueID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: branches})
}
