package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"openaide/backend/src/response"
	"openaide/backend/src/services"
)

type ExecPolicyHandler struct {
	ExecPolicySvc *services.ExecPolicyService
}

func NewExecPolicyHandler(svc *services.ExecPolicyService) *ExecPolicyHandler {
	return &ExecPolicyHandler{ExecPolicySvc: svc}
}

func (h *ExecPolicyHandler) RegisterRoutes(r *gin.RouterGroup) {
	policy := r.Group("/exec-policy")
	{
		policy.GET("/rules", h.ListRules)
		policy.POST("/rules", h.AddRule)
		policy.DELETE("/rules/:id", h.RemoveRule)
		policy.POST("/evaluate", h.Evaluate)
		policy.POST("/evaluate/file", h.EvaluateFile)
		policy.POST("/reload", h.ReloadRules)
	}
}

func (h *ExecPolicyHandler) ListRules(c *gin.Context) {
	rules := h.ExecPolicySvc.ListRules()
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: rules})
}

func (h *ExecPolicyHandler) AddRule(c *gin.Context) {
	var rule services.ExecPolicyRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	h.ExecPolicySvc.AddRule(rule)
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "rule added"})
}

func (h *ExecPolicyHandler) RemoveRule(c *gin.Context) {
	id := c.Param("id")
	if h.ExecPolicySvc.RemoveRule(id) {
		c.JSON(http.StatusOK, response.Response{Code: 0, Message: "rule removed"})
	} else {
		c.JSON(http.StatusNotFound, response.Response{Code: 1, Message: "rule not found"})
	}
}

func (h *ExecPolicyHandler) Evaluate(c *gin.Context) {
	var req struct {
		Command string `json:"command"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	eval := h.ExecPolicySvc.Evaluate(c.Request.Context(), req.Command)
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: eval})
}

func (h *ExecPolicyHandler) EvaluateFile(c *gin.Context) {
	var req struct {
		Path  string `json:"path"`
		Write bool   `json:"write"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	eval := h.ExecPolicySvc.EvaluateFileAccess(c.Request.Context(), req.Path, req.Write)
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: eval})
}

func (h *ExecPolicyHandler) ReloadRules(c *gin.Context) {
	h.ExecPolicySvc.ReloadRules()
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "rules reloaded"})
}
