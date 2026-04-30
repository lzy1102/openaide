package logger

import (
	"log/slog"
	"os"
)

var defaultLogger *slog.Logger

func init() {
	defaultLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// SetLogger 设置全局日志器
func SetLogger(l *slog.Logger) {
	defaultLogger = l
}

// GetLogger 获取全局日志器
func GetLogger() *slog.Logger {
	return defaultLogger
}

// WithComponent 创建带组件标签的日志器
func WithComponent(component string) *slog.Logger {
	return defaultLogger.With("component", component)
}

// Info 记录信息日志
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Error 记录错误日志
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// Warn 记录警告日志
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Debug 记录调试日志
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}
