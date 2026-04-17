package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	// LogLevelDebug 调试级别
	LogLevelDebug LogLevel = iota
	// LogLevelInfo 信息级别
	LogLevelInfo
	// LogLevelWarn 警告级别
	LogLevelWarn
	// LogLevelError 错误级别
	LogLevelError
)

// LoggerService 日志服务
type LoggerService struct {
	level  LogLevel
	logger *log.Logger
	file   *os.File
}

// NewLoggerService 创建日志服务实例
func NewLoggerService(level LogLevel, filePath string) (*LoggerService, error) {
	service := &LoggerService{
		level:  level,
		logger: log.Default(),
	}

	// 如果指定了文件路径,打开日志文件
	if filePath != "" {
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		service.file = file
		service.logger = log.New(file, "", log.LstdFlags)
	}

	return service, nil
}

// Close 关闭日志服务
func (s *LoggerService) Close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// Debug 记录调试日志
func (s *LoggerService) Debug(ctx context.Context, format string, args ...interface{}) {
	if s.level <= LogLevelDebug {
		s.log(ctx, "DEBUG", format, args...)
	}
}

// Info 记录信息日志
func (s *LoggerService) Info(ctx context.Context, format string, args ...interface{}) {
	if s.level <= LogLevelInfo {
		s.log(ctx, "INFO", format, args...)
	}
}

// Warn 记录警告日志
func (s *LoggerService) Warn(ctx context.Context, format string, args ...interface{}) {
	if s.level <= LogLevelWarn {
		s.log(ctx, "WARN", format, args...)
	}
}

// Error 记录错误日志
func (s *LoggerService) Error(ctx context.Context, format string, args ...interface{}) {
	if s.level <= LogLevelError {
		s.log(ctx, "ERROR", format, args...)
	}
}

// log 内部日志记录方法
func (s *LoggerService) log(ctx context.Context, level, format string, args ...interface{}) {
	// 从 context 中获取请求 ID
	var requestID string
	if ctx != nil {
		if id, ok := ctx.Value("request_id").(string); ok {
			requestID = id
		}
	}

	// 构建日志前缀
	prefix := fmt.Sprintf("[%s]", level)
	if requestID != "" {
		prefix = fmt.Sprintf("[%s] [req:%s]", level, requestID)
	}

	// 记录日志
	message := fmt.Sprintf(format, args...)
	s.logger.Printf("%s %s", prefix, message)
}

// LogLLMRequest 记录 LLM 请求
func (s *LoggerService) LogLLMRequest(ctx context.Context, provider, model string, messages interface{}, options map[string]interface{}) {
	s.Info(ctx, "[LLM Request] provider=%s model=%s messages_count=%d temperature=%v max_tokens=%v",
		provider,
		model,
		len(messages.([]interface{})),
		options["temperature"],
		options["max_tokens"],
	)
}

// LogLLMResponse 记录 LLM 响应
func (s *LoggerService) LogLLMResponse(ctx context.Context, provider, model string, duration time.Duration, usage interface{}, err error) {
	if err != nil {
		s.Error(ctx, "[LLM Response] provider=%s model=%s duration=%v error=%v",
			provider, model, duration, err)
	} else {
		s.Info(ctx, "[LLM Response] provider=%s model=%s duration=%v tokens=%v",
			provider, model, duration, usage)
	}
}

// LogModelExecution 记录模型执行
func (s *LoggerService) LogModelExecution(ctx context.Context, modelID, instanceID string, status string, duration time.Duration, err error) {
	if err != nil {
		s.Error(ctx, "[Model Execution] model_id=%s instance_id=%s status=%s duration=%v error=%v",
			modelID, instanceID, status, duration, err)
	} else {
		s.Info(ctx, "[Model Execution] model_id=%s instance_id=%s status=%s duration=%v",
			modelID, instanceID, status, duration)
	}
}
