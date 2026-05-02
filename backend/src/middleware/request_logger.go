package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		if raw != "" {
			path = path + "?" + raw
		}

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method

		attrs := []any{
			"component", "HTTP",
			"method", method,
			"path", path,
			"status", status,
			"duration", duration,
			"ip", c.ClientIP(),
		}

		if requestID, exists := c.Get("X-Request-ID"); exists {
			attrs = append(attrs, "request_id", requestID)
		}

		switch {
		case status >= 500:
			slog.Error("Request", attrs...)
		case status >= 400:
			slog.Warn("Request", attrs...)
		default:
			slog.Info("Request", attrs...)
		}
	}
}
