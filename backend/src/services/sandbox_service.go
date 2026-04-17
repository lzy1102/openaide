package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// SandboxService 沙箱代码执行服务
type SandboxService struct {
	config     SandboxConfig
	httpClient *http.Client
}

// SandboxConfig 沙箱配置
type SandboxConfig struct {
	Enabled       bool   `json:"enabled"`
	DockerImage   string `json:"docker_image"`    // Docker 镜像名
	Timeout       int    `json:"timeout"`          // 默认超时（秒）
	MaxMemoryMB   int    `json:"max_memory_mb"`    // 最大内存（MB）
	NetworkAccess bool   `json:"network_access"`   // 是否允许网络访问
}

// SupportedLanguage 支持的执行语言
type SupportedLanguage struct {
	Name        string `json:"name"`
	Extension   string `json:"extension"`
	Image       string `json:"image"`
	Description string `json:"description"`
}

// ExecutionResult 代码执行结果
type ExecutionResult struct {
	Success    bool   `json:"success"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	Duration   string `json:"duration"`
	Language   string `json:"language"`
	MemoryUsed string `json:"memory_used,omitempty"`
}

// NewSandboxService 创建沙箱服务
func NewSandboxService(config SandboxConfig) *SandboxService {
	return &SandboxService{
		config:     config,
		httpClient: &http.Client{Timeout: time.Duration(config.Timeout+5) * time.Second},
	}
}

// IsEnabled 检查沙箱是否可用
func (s *SandboxService) IsEnabled() bool {
	return s.config.Enabled
}

// IsDockerAvailable 检查 Docker 是否可用
func (s *SandboxService) IsDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// SupportedLanguages 返回支持的语言列表
func (s *SandboxService) SupportedLanguages() []SupportedLanguage {
	return []SupportedLanguage{
		{Name: "python", Extension: "py", Image: "python:3.11-slim", Description: "Python 3.11"},
		{Name: "javascript", Extension: "js", Image: "node:20-slim", Description: "Node.js 20"},
		{Name: "go", Extension: "go", Image: "golang:1.21-alpine", Description: "Go 1.21"},
		{Name: "bash", Extension: "sh", Image: "bash:5.2", Description: "Bash 5.2"},
	}
}

// Execute 执行代码
func (s *SandboxService) Execute(ctx context.Context, language, code string) (*ExecutionResult, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("sandbox is not enabled")
	}

	if s.IsDockerAvailable() {
		return s.executeDocker(ctx, language, code)
	}
	return s.executeLocal(ctx, language, code)
}

// executeDocker 使用 Docker 执行代码
func (s *SandboxService) executeDocker(ctx context.Context, language, code string) (*ExecutionResult, error) {
	image := s.getDockerImage(language)
	timeout := time.Duration(s.config.Timeout) * time.Second

	containerID, err := s.createContainer(ctx, image, language, code, timeout)
	if err != nil {
		return nil, err
	}

	// 确保容器被清理
	defer func() {
		cleanupCmd := exec.CommandContext(context.Background(), "docker", "rm", "-f", containerID)
		cleanupCmd.Run()
	}()

	return s.startAndWait(ctx, containerID, language, timeout)
}

// createContainer 创建 Docker 容器
func (s *SandboxService) createContainer(ctx context.Context, image, language, code string, timeout time.Duration) (string, error) {
	var cmd *exec.Cmd

	switch language {
	case "python", "python3":
		cmd = exec.CommandContext(ctx, "docker", "create",
			"--rm",
			"--memory", fmt.Sprintf("%dm", s.config.MaxMemoryMB),
			"--network", boolStr(s.config.NetworkAccess),
			image,
			"python", "-c", code,
		)
	case "javascript", "js", "node":
		cmd = exec.CommandContext(ctx, "docker", "create",
			"--rm",
			"--memory", fmt.Sprintf("%dm", s.config.MaxMemoryMB),
			"--network", boolStr(s.config.NetworkAccess),
			image,
			"node", "-e", code,
		)
	case "go":
		// Go 需要先写入文件再运行
		tmpFile := fmt.Sprintf("/tmp/main.%s", language)
		cmd = exec.CommandContext(ctx, "docker", "create",
			"--rm",
			"--memory", fmt.Sprintf("%dm", s.config.MaxMemoryMB),
			"--network", boolStr(s.config.NetworkAccess),
			image,
			"sh", "-c", fmt.Sprintf("echo '%s' > %s && go run %s", escapeSingleQuotes(code), tmpFile, tmpFile),
		)
	case "bash", "sh":
		cmd = exec.CommandContext(ctx, "docker", "create",
			"--rm",
			"--memory", fmt.Sprintf("%dm", s.config.MaxMemoryMB),
			"--network", boolStr(s.config.NetworkAccess),
			image,
			"bash", "-c", code,
		)
	default:
		return "", fmt.Errorf("unsupported language: %s", language)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w, output: %s", err, string(output))
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		return "", fmt.Errorf("empty container ID returned")
	}

	return containerID, nil
}

// startAndWait 启动容器并等待结果
func (s *SandboxService) startAndWait(ctx context.Context, containerID, language string, timeout time.Duration) (*ExecutionResult, error) {
	startTime := time.Now()

	startCmd := exec.CommandContext(ctx, "docker", "start", "-a", containerID)
	var stdout, stderr bytes.Buffer
	startCmd.Stdout = &stdout
	startCmd.Stderr = &stderr

	err := startCmd.Run()
	duration := time.Since(startTime)

	result := &ExecutionResult{
		Success:  err == nil,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration.String(),
		Language: language,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
	}

	return result, nil
}

// executeLocal 本地执行（无 Docker 时的降级方案）
func (s *SandboxService) executeLocal(ctx context.Context, language, code string) (*ExecutionResult, error) {
	startTime := time.Now()

	var cmd *exec.Cmd
	timeout := time.Duration(s.config.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch language {
	case "python", "python3":
		cmd = exec.CommandContext(ctx, "python3", "-c", code)
	case "javascript", "js", "node":
		cmd = exec.CommandContext(ctx, "node", "-e", code)
	case "bash", "sh":
		cmd = exec.CommandContext(ctx, "bash", "-c", code)
	case "go":
		// 本地 go run
		tmpFile := "/tmp/sandbox_main.go"
		writeCmd := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo '%s' > %s", escapeSingleQuotes(code), tmpFile))
		writeCmd.Run()
		cmd = exec.CommandContext(ctx, "go", "run", tmpFile)
	default:
		return nil, fmt.Errorf("unsupported language: %s (no Docker available)", language)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(startTime)

	result := &ExecutionResult{
		Success:  err == nil,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration.String(),
		Language: language,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		result.Stderr += "\n[Warning: Running without Docker sandbox]"
	}

	return result, nil
}

// ExecuteViaAPI 通过远程 API 执行代码（如 E2B/Jina)
func (s *SandboxService) ExecuteViaAPI(ctx context.Context, apiURL, apiKey, language, code string) (*ExecutionResult, error) {
	reqBody := map[string]interface{}{
		"language": language,
		"code":     code,
		"timeout":  s.config.Timeout,
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sandbox API call failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sandbox API returned %d: %s", resp.StatusCode, string(body))
	}

	var result ExecutionResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse sandbox response: %w", err)
	}

	return &result, nil
}

// GetStatus 获取沙箱状态
func (s *SandboxService) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"enabled":         s.config.Enabled,
		"docker_available": s.IsDockerAvailable(),
		"languages":       s.SupportedLanguages(),
		"timeout":         s.config.Timeout,
		"max_memory_mb":   s.config.MaxMemoryMB,
		"network_access":  s.config.NetworkAccess,
	}
}

// ==================== 辅助函数 ====================

func getDockerImage(language string) string {
	images := map[string]string{
		"python":     "python:3.11-slim",
		"python3":    "python:3.11-slim",
		"javascript": "node:20-slim",
		"js":         "node:20-slim",
		"go":         "golang:1.21-alpine",
		"bash":       "bash:5.2",
		"sh":         "bash:5.2",
	}
	if img, ok := images[language]; ok {
		return img
	}
	return "python:3.11-slim"
}

func (s *SandboxService) getDockerImage(language string) string {
	if s.config.DockerImage != "" {
		return s.config.DockerImage
	}
	return getDockerImage(language)
}

func boolStr(b bool) string {
	if b {
		return "host"
	}
	return "none"
}

func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// ValidateCodeSafety 验证代码安全性（基础检查）
func ValidateCodeSafety(code string) error {
	dangerous := []string{
		"rm -rf /",
		"mkfs.",
		"dd if=",
		":(){ :|:& };:",
	}
	codeLower := strings.ToLower(code)
	for _, d := range dangerous {
		if strings.Contains(codeLower, strings.ToLower(d)) {
			return fmt.Errorf("code contains potentially dangerous pattern: %s", d)
		}
	}
	return nil
}
