package orchestration

import (
	"context"
	"log"
)

// LoggerService 简单的日志服务（用于 orchestration 包）
type LoggerService struct {
	logger *log.Logger
}

// NewLoggerService 创建日志服务
func NewLoggerService() *LoggerService {
	return &LoggerService{
		logger: log.Default(),
	}
}

// Info 记录信息日志
func (l *LoggerService) Info(ctx context.Context, format string, args ...interface{}) {
	l.logger.Printf("[INFO] "+format, args...)
}

// Warn 记录警告日志
func (l *LoggerService) Warn(ctx context.Context, format string, args ...interface{}) {
	l.logger.Printf("[WARN] "+format, args...)
}

// Error 记录错误日志
func (l *LoggerService) Error(ctx context.Context, format string, args ...interface{}) {
	l.logger.Printf("[ERROR] "+format, args...)
}

// Debug 记录调试日志
func (l *LoggerService) Debug(ctx context.Context, format string, args ...interface{}) {
	l.logger.Printf("[DEBUG] "+format, args...)
}
