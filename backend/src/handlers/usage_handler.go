package handlers

import (
	"net/http"
	"strconv"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
)

// UsageHandler 使用量处理器
type UsageHandler struct {
	usageService *services.UsageService
}

// NewUsageHandler 创建使用量处理器
func NewUsageHandler(usageService *services.UsageService) *UsageHandler {
	return &UsageHandler{
		usageService: usageService,
	}
}

// GetStats 获取使用统计
func (h *UsageHandler) GetStats(c *gin.Context) {
	userID, _ := c.Get("user_id")
	period := c.DefaultQuery("period", "month")

	stats, err := h.usageService.GetUsageStats(userID.(string), period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetDailyUsage 获取每日使用量
func (h *UsageHandler) GetDailyUsage(c *gin.Context) {
	userID, _ := c.Get("user_id")
	dateStr := c.DefaultQuery("date", time.Now().Format("2006-01-02"))

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format"})
		return
	}

	usage, err := h.usageService.GetDailyUsage(userID.(string), date)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"date":         dateStr,
			"total_tokens": 0,
			"total_cost":   0,
		})
		return
	}

	c.JSON(http.StatusOK, usage)
}

// GetMonthlyUsage 获取每月使用量
func (h *UsageHandler) GetMonthlyUsage(c *gin.Context) {
	userID, _ := c.Get("user_id")
	yearStr := c.DefaultQuery("year", strconv.Itoa(time.Now().Year()))
	monthStr := c.DefaultQuery("month", strconv.Itoa(int(time.Now().Month())))

	year, _ := strconv.Atoi(yearStr)
	month, _ := strconv.Atoi(monthStr)

	usage, err := h.usageService.GetMonthlyUsage(userID.(string), year, month)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"year":         year,
			"month":        month,
			"total_tokens": 0,
			"total_cost":   0,
		})
		return
	}

	c.JSON(http.StatusOK, usage)
}

// GetHistory 获取使用历史
func (h *UsageHandler) GetHistory(c *gin.Context) {
	userID, _ := c.Get("user_id")

	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	limitStr := c.DefaultQuery("limit", "100")

	var startDate, endDate time.Time
	var err error

	if startDateStr != "" {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date format"})
			return
		}
	}

	if endDateStr != "" {
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_date format"})
			return
		}
	}

	limit, _ := strconv.Atoi(limitStr)

	records, err := h.usageService.GetUsageHistory(userID.(string), startDate, endDate, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"records": records,
		"count":   len(records),
	})
}

// GetBudget 获取用户预算
func (h *UsageHandler) GetBudget(c *gin.Context) {
	userID, _ := c.Get("user_id")

	budget, err := h.usageService.GetUserBudget(userID.(string))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"monthly_budget": 0,
			"daily_budget":   0,
			"message":        "no budget set",
		})
		return
	}

	c.JSON(http.StatusOK, budget)
}

// SetBudget 设置用户预算
func (h *UsageHandler) SetBudget(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req struct {
		MonthlyBudget float64 `json:"monthly_budget"`
		DailyBudget   float64 `json:"daily_budget"`
		Thresholds    []int   `json:"thresholds"`
		AlertEmail    string  `json:"alert_email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Thresholds) == 0 {
		req.Thresholds = []int{50, 80, 100}
	}

	if err := h.usageService.SetUserBudget(
		userID.(string),
		req.MonthlyBudget,
		req.DailyBudget,
		req.Thresholds,
		req.AlertEmail,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "budget set successfully",
		"monthly_budget": req.MonthlyBudget,
		"daily_budget":   req.DailyBudget,
	})
}

// GetPricing 获取模型定价
func (h *UsageHandler) GetPricing(c *gin.Context) {
	pricings, err := h.usageService.ListModelPricing()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"pricing": pricings,
		"count":   len(pricings),
	})
}

// UpdatePricing 更新模型定价
func (h *UsageHandler) UpdatePricing(c *gin.Context) {
	var pricing struct {
		Provider         string  `json:"provider" binding:"required"`
		ModelName        string  `json:"model_name" binding:"required"`
		InputPricePer1K  float64 `json:"input_price_per_1k"`
		OutputPricePer1K float64 `json:"output_price_per_1k"`
	}

	if err := c.ShouldBindJSON(&pricing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	modelPricing := &models.ModelPricing{
		Provider:         pricing.Provider,
		ModelName:        pricing.ModelName,
		InputPricePer1K:  pricing.InputPricePer1K,
		OutputPricePer1K: pricing.OutputPricePer1K,
	}

	if err := h.usageService.UpdateModelPricing(modelPricing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "pricing updated successfully",
		"pricing": modelPricing,
	})
}

// RegisterRoutes 注册路由
func (h *UsageHandler) RegisterRoutes(r *gin.RouterGroup) {
	usage := r.Group("/usage")
	{
		usage.GET("/stats", h.GetStats)
		usage.GET("/daily", h.GetDailyUsage)
		usage.GET("/monthly", h.GetMonthlyUsage)
		usage.GET("/history", h.GetHistory)

		// 预算管理
		usage.GET("/budget", h.GetBudget)
		usage.POST("/budget", h.SetBudget)

		// 定价管理 (管理员)
		usage.GET("/pricing", h.GetPricing)
		usage.POST("/pricing", h.UpdatePricing)
	}
}
