package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"openaide/backend/src/response"
	"openaide/backend/src/services"
)

type PersistentMemoryHandler struct {
	MemorySvc *services.PersistentMemoryService
}

func NewPersistentMemoryHandler(svc *services.PersistentMemoryService) *PersistentMemoryHandler {
	return &PersistentMemoryHandler{MemorySvc: svc}
}

func (h *PersistentMemoryHandler) RegisterRoutes(r *gin.RouterGroup) {
	mem := r.Group("/persistent-memories")
	{
		mem.POST("", h.Remember)
		mem.GET("", h.RecallAll)
		mem.GET("/search", h.Search)
		mem.GET("/category/:cat", h.RecallByCategory)
		mem.GET("/context", h.BuildContext)
		mem.DELETE("/:id", h.Forget)
		mem.POST("/extract", h.ExtractAndRemember)
		mem.GET("/export", h.Export)
		mem.POST("/import", h.Import)
		mem.POST("/cleanup", h.Cleanup)
	}
}

func (h *PersistentMemoryHandler) Remember(c *gin.Context) {
	var req struct {
		UserID   string `json:"user_id"`
		Category string `json:"category"`
		Key      string `json:"key"`
		Value    string `json:"value"`
		Source   string `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	mem, err := h.MemorySvc.Remember(c.Request.Context(), req.UserID, services.PersistentMemoryCategory(req.Category), req.Key, req.Value, req.Source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: mem})
}

func (h *PersistentMemoryHandler) RecallAll(c *gin.Context) {
	userID := c.Query("user_id")
	memories, err := h.MemorySvc.RecallAll(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: memories})
}

func (h *PersistentMemoryHandler) Search(c *gin.Context) {
	userID := c.Query("user_id")
	query := c.Query("q")
	limit := 20
	memories, err := h.MemorySvc.Search(c.Request.Context(), userID, query, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: memories})
}

func (h *PersistentMemoryHandler) RecallByCategory(c *gin.Context) {
	userID := c.Query("user_id")
	cat := c.Param("cat")
	memories, err := h.MemorySvc.RecallByCategory(c.Request.Context(), userID, services.PersistentMemoryCategory(cat))
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: memories})
}

func (h *PersistentMemoryHandler) BuildContext(c *gin.Context) {
	userID := c.Query("user_id")
	ctx := h.MemorySvc.BuildContextForUser(c.Request.Context(), userID)
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: map[string]string{"context": ctx}})
}

func (h *PersistentMemoryHandler) Forget(c *gin.Context) {
	userID := c.Query("user_id")
	id := c.Param("id")
	if err := h.MemorySvc.Forget(c.Request.Context(), userID, id); err != nil {
		c.JSON(http.StatusNotFound, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "forgotten"})
}

func (h *PersistentMemoryHandler) ExtractAndRemember(c *gin.Context) {
	var req struct {
		UserID        string `json:"user_id"`
		DialogueID    string `json:"dialogue_id"`
		UserMessage   string `json:"user_message"`
		AssistantMsg  string `json:"assistant_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	count := h.MemorySvc.ExtractAndRemember(c.Request.Context(), req.UserID, req.DialogueID, req.UserMessage, req.AssistantMsg)
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: map[string]int{"extracted": count}})
}

func (h *PersistentMemoryHandler) Export(c *gin.Context) {
	userID := c.Query("user_id")
	data, err := h.MemorySvc.ExportMemories(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: data})
}

func (h *PersistentMemoryHandler) Import(c *gin.Context) {
	var req struct {
		UserID   string `json:"user_id"`
		JSONData string `json:"json_data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 1, Message: err.Error()})
		return
	}
	count, err := h.MemorySvc.ImportMemories(c.Request.Context(), req.UserID, req.JSONData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 1, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: map[string]int{"imported": count}})
}

func (h *PersistentMemoryHandler) Cleanup(c *gin.Context) {
	userID := c.Query("user_id")
	count := h.MemorySvc.Cleanup(c.Request.Context(), userID)
	c.JSON(http.StatusOK, response.Response{Code: 0, Message: "ok", Data: map[string]int{"cleaned": count}})
}
