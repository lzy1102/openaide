package handlers

import (
	"fmt"
	"net/http"
	"time"

	"openaide/backend/src/services"
	"github.com/gin-gonic/gin"
)

// RateLimitHandler 限流处理器
type RateLimitHandler struct {
	rateLimitService *services.RateLimitService
}

// NewRateLimitHandler 创建限流处理器
func NewRateLimitHandler(rateLimitService *services.RateLimitService) *RateLimitHandler {
	return &RateLimitHandler{
		rateLimitService: rateLimitService,
	}
}

// RateLimitMiddleware 限流中间件
func (h *RateLimitHandler) RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端标识
		ip := c.ClientIP()
		userID, _ := c.Get("user_id")
		apiKeyID, _ := c.Get("api_key_id")

		var allowed bool
		var remaining int
		var resetAt time.Time

		// 优先使用 API Key 限流
		if apiKeyID != nil {
			allowed, remaining, resetAt = h.rateLimitService.CheckAPIKey(apiKeyID.(string))
			h.setRateLimitHeaders(c, remaining, resetAt)
			if !allowed {
				h.rateLimitExceeded(c, resetAt)
				return
			}
			c.Next()
			return
		}

		// 用户限流
		if userID != nil {
			allowed, remaining, resetAt = h.rateLimitService.CheckUser(userID.(string))
			h.setRateLimitHeaders(c, remaining, resetAt)
			if !allowed {
				h.rateLimitExceeded(c, resetAt)
				return
			}
		}

		// IP 限流（作为兜底）
		ipAllowed, ipRemaining, ipResetAt := h.rateLimitService.CheckIP(ip)
		h.setRateLimitHeaders(c, ipRemaining, ipResetAt)
		if !ipAllowed {
			h.rateLimitExceeded(c, ipResetAt)
			return
		}

		c.Next()
	}
}

// IPRateLimitMiddleware IP 限流中间件
func (h *RateLimitHandler) IPRateLimitMiddleware(requestsPerMinute int) gin.HandlerFunc {
	// 创建专用的 IP 限流器
	limiter := services.NewSimpleSlidingWindowLimiter(services.SimpleRateLimitConfig{
		RequestsPerWindow: requestsPerMinute,
		WindowSize:        time.Minute,
		KeyPrefix:         "ip_custom:",
	})

	return func(c *gin.Context) {
		ip := c.ClientIP()

		remaining := limiter.Remaining(ip)
		resetAt := limiter.ResetAt(ip)

		h.setRateLimitHeaders(c, remaining, resetAt)

		if remaining <= 0 {
			h.rateLimitExceeded(c, resetAt)
			return
		}

		limiter.Increment(ip)
		c.Next()
	}
}

// UserRateLimitMiddleware 用户限流中间件
func (h *RateLimitHandler) UserRateLimitMiddleware(requestsPerMinute int) gin.HandlerFunc {
	limiter := services.NewSimpleSlidingWindowLimiter(services.SimpleRateLimitConfig{
		RequestsPerWindow: requestsPerMinute,
		WindowSize:        time.Minute,
		KeyPrefix:         "user_custom:",
	})

	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		key := userID.(string)
		remaining := limiter.Remaining(key)
		resetAt := limiter.ResetAt(key)

		h.setRateLimitHeaders(c, remaining, resetAt)

		if remaining <= 0 {
			h.rateLimitExceeded(c, resetAt)
			return
		}

		limiter.Increment(key)
		c.Next()
	}
}

// EndpointRateLimitMiddleware 端点限流中间件
func (h *RateLimitHandler) EndpointRateLimitMiddleware(endpoint string, requestsPerMinute int) gin.HandlerFunc {
	limiter := services.NewSimpleSlidingWindowLimiter(services.SimpleRateLimitConfig{
		RequestsPerWindow: requestsPerMinute,
		WindowSize:        time.Minute,
		KeyPrefix:         "endpoint:" + endpoint + ":",
	})

	return func(c *gin.Context) {
		// 使用用户ID或IP作为key
		key := c.ClientIP()
		if userID, exists := c.Get("user_id"); exists {
			key = userID.(string)
		}

		remaining := limiter.Remaining(key)
		resetAt := limiter.ResetAt(key)

		h.setRateLimitHeaders(c, remaining, resetAt)

		if remaining <= 0 {
			h.rateLimitExceeded(c, resetAt)
			return
		}

		limiter.Increment(key)
		c.Next()
	}
}

// StrictRateLimitMiddleware 严格限流中间件（用于敏感接口）
func (h *RateLimitHandler) StrictRateLimitMiddleware(requestsPerMinute int) gin.HandlerFunc {
	limiter := services.NewSimpleSlidingWindowLimiter(services.SimpleRateLimitConfig{
		RequestsPerWindow: requestsPerMinute,
		WindowSize:        time.Minute,
		KeyPrefix:         "strict:",
	})

	return func(c *gin.Context) {
		// 结合用户ID和IP
		userID, _ := c.Get("user_id")
		ip := c.ClientIP()

		key := ip
		if userID != nil {
			key = fmt.Sprintf("%s_%s", userID.(string), ip)
		}

		remaining := limiter.Remaining(key)
		resetAt := limiter.ResetAt(key)

		h.setRateLimitHeaders(c, remaining, resetAt)

		if remaining <= 0 {
			h.rateLimitExceeded(c, resetAt)
			return
		}

		limiter.Increment(key)
		c.Next()
	}
}

// setRateLimitHeaders 设置限流响应头
func (h *RateLimitHandler) setRateLimitHeaders(c *gin.Context, remaining int, resetAt time.Time) {
	c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))
	c.Header("X-RateLimit-Reset-After", fmt.Sprintf("%d", max(0, int(time.Until(resetAt).Seconds()))))
}

// rateLimitExceeded 限流超出响应
func (h *RateLimitHandler) rateLimitExceeded(c *gin.Context, resetAt time.Time) {
	retryAfter := int(time.Until(resetAt).Seconds())
	if retryAfter < 0 {
		retryAfter = 1
	}

	c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
	c.Header("X-RateLimit-Remaining", "0")
	c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))

	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":        "请求过于频繁，请稍后再试",
		"retry_after":  retryAfter,
		"reset_at":     resetAt.Unix(),
	})
}

// GetRateLimitStats 获取限流统计（管理员接口）
func (h *RateLimitHandler) GetRateLimitStats(c *gin.Context) {
	// 返回限流器状态
	c.JSON(http.StatusOK, gin.H{
		"status": "active",
		"rules": gin.H{
			"ip": gin.H{
				"requests_per_minute": 100,
				"window":              "1m",
			},
			"user": gin.H{
				"requests_per_minute": 200,
				"window":              "1m",
			},
			"api_key": gin.H{
				"requests_per_minute": 500,
				"window":              "1m",
			},
		},
	})
}

// 辅助函数
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
