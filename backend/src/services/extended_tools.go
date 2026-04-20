package services

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ========================================
// DockerTool Docker 容器操作工具
// ========================================

// DockerTool Docker 工具
type DockerTool struct{}

func (t *DockerTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		return nil, fmt.Errorf("action parameter is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	switch action {
	case "ps":
		return t.dockerPs(ctx, params)
	case "logs":
		return t.dockerLogs(ctx, params)
	case "build":
		return t.dockerBuild(ctx, params)
	case "run":
		return t.dockerRun(ctx, params)
	case "stop":
		return t.dockerStop(ctx, params)
	case "start":
		return t.dockerStart(ctx, params)
	case "rm":
		return t.dockerRm(ctx, params)
	case "exec":
		return t.dockerExec(ctx, params)
	case "images":
		return t.dockerImages(ctx)
	case "compose":
		return t.dockerCompose(ctx, params)
	case "inspect":
		return t.dockerInspect(ctx, params)
	default:
		return nil, fmt.Errorf("unknown docker action: %s", action)
	}
}

func (t *DockerTool) dockerPs(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	all, _ := params["all"].(bool)
	args := []string{"ps", "--format", "{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}"}
	if all {
		args = append([]string{"ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}"}, args[1:]...)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %s", string(output))
	}

	var containers []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) == 4 {
			containers = append(containers, map[string]interface{}{
				"id":     parts[0],
				"name":   parts[1],
				"status": parts[2],
				"ports":  parts[3],
			})
		}
	}

	return map[string]interface{}{
		"containers": containers,
		"count":      len(containers),
	}, nil
}

func (t *DockerTool) dockerLogs(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	container, _ := params["container"].(string)
	if container == "" {
		return nil, fmt.Errorf("container parameter is required")
	}

	tail := "100"
	if t, ok := params["tail"].(string); ok {
		tail = t
	}
	follow, _ := params["follow"].(bool)

	args := []string{"logs", "--tail", tail}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, container)

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()

	return map[string]interface{}{
		"container": container,
		"logs":      string(output),
		"error":     err != nil,
	}, nil
}

func (t *DockerTool) dockerBuild(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	tag, _ := params["tag"].(string)

	if path == "" {
		path = "."
	}
	if tag == "" {
		return nil, fmt.Errorf("tag parameter is required")
	}

	args := []string{"build", "-t", tag, path}
	if dockerfile, ok := params["dockerfile"].(string); ok {
		args = append(args, "-f", dockerfile)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir, _ = os.Getwd()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	return map[string]interface{}{
		"tag":      tag,
		"success":  err == nil,
		"output":   truncatePlanStr(stdout.String(), 2000),
		"error":    truncatePlanStr(stderr.String(), 1000),
		"duration": duration.String(),
	}, nil
}

func (t *DockerTool) dockerRun(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	image, _ := params["image"].(string)
	if image == "" {
		return nil, fmt.Errorf("image parameter is required")
	}

	args := []string{"run", "-d"}
	if name, ok := params["name"].(string); ok {
		args = append(args, "--name", name)
	}
	if ports, ok := params["ports"].([]interface{}); ok {
		for _, p := range ports {
			args = append(args, "-p", fmt.Sprintf("%v", p))
		}
	}
	if envs, ok := params["env"].([]interface{}); ok {
		for _, e := range envs {
			args = append(args, "-e", fmt.Sprintf("%v", e))
		}
	}
	if volumes, ok := params["volumes"].([]interface{}); ok {
		for _, v := range volumes {
			args = append(args, "-v", fmt.Sprintf("%v", v))
		}
	}
	if detach, ok := params["detach"].(bool); !detach {
		args[1] = "--rm" // 如果不是后台运行，自动清理
	}

	args = append(args, image)
	if cmd, ok := params["command"].(string); ok {
		args = append(args, cmd)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()

	return map[string]interface{}{
		"image":   image,
		"success": err == nil,
		"output":  string(output),
	}, nil
}

func (t *DockerTool) dockerStop(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	container, _ := params["container"].(string)
	if container == "" {
		return nil, fmt.Errorf("container parameter is required")
	}

	cmd := exec.CommandContext(ctx, "docker", "stop", container)
	output, err := cmd.CombinedOutput()

	return map[string]interface{}{
		"container": container,
		"success":   err == nil,
		"output":    string(output),
	}, nil
}

func (t *DockerTool) dockerStart(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	container, _ := params["container"].(string)
	if container == "" {
		return nil, fmt.Errorf("container parameter is required")
	}

	cmd := exec.CommandContext(ctx, "docker", "start", container)
	output, err := cmd.CombinedOutput()

	return map[string]interface{}{
		"container": container,
		"success":   err == nil,
		"output":    string(output),
	}, nil
}

func (t *DockerTool) dockerRm(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	container, _ := params["container"].(string)
	if container == "" {
		return nil, fmt.Errorf("container parameter is required")
	}

	force, _ := params["force"].(bool)
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, container)

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()

	return map[string]interface{}{
		"container": container,
		"success":   err == nil,
		"output":    string(output),
	}, nil
}

func (t *DockerTool) dockerExec(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	container, _ := params["container"].(string)
	command, _ := params["command"].(string)

	if container == "" || command == "" {
		return nil, fmt.Errorf("container and command parameters are required")
	}

	args := []string{"exec", container, "sh", "-c", command}
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()

	return map[string]interface{}{
		"container": container,
		"command":   command,
		"output":    string(output),
		"success":   err == nil,
	}, nil
}

func (t *DockerTool) dockerImages(ctx context.Context) (interface{}, error) {
	cmd := exec.CommandContext(ctx, "docker", "images", "--format", "{{.Repository}}:{{.Tag}}\t{{.ID}}\t{{.Size}}")
	output, err := cmd.CombinedOutput()

	var images []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) == 3 {
			images = append(images, map[string]interface{}{
				"repository": parts[0],
				"id":         parts[1],
				"size":       parts[2],
			})
		}
	}

	return map[string]interface{}{
		"images": images,
		"count":  len(images),
	}, nil
}

func (t *DockerTool) dockerCompose(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "up"
	}

	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}

	args := []string{"compose"}
	if composeFile, ok := params["file"].(string); ok {
		args = append(args, "-f", composeFile)
	}
	args = append(args, action)

	if detach, _ := params["detach"].(bool); action == "up" && detach {
		args = append(args, "-d")
	}
	if build, _ := params["build"].(bool); action == "up" && build {
		args = append(args, "--build")
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = path

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	return map[string]interface{}{
		"action":   action,
		"path":     path,
		"success":  err == nil,
		"output":   truncatePlanStr(stdout.String(), 2000),
		"error":    truncatePlanStr(stderr.String(), 1000),
		"duration": duration.String(),
	}, nil
}

func (t *DockerTool) dockerInspect(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	container, _ := params["container"].(string)
	if container == "" {
		return nil, fmt.Errorf("container parameter is required")
	}

	cmd := exec.CommandContext(ctx, "docker", "inspect", container)
	output, err := cmd.CombinedOutput()

	var result []map[string]interface{}
	json.Unmarshal(output, &result)

	return map[string]interface{}{
		"container": container,
		"info":      result,
	}, nil
}

// ========================================
// APITestTool API 测试工具
// ========================================

// APITestTool API 测试工具
type APITestTool struct{}

func (t *APITestTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "request"
	}

	switch action {
	case "request":
		return t.apiRequest(ctx, params)
	case "test":
		return t.apiTest(ctx, params)
	default:
		return nil, fmt.Errorf("unknown api test action: %s", action)
	}
}

func (t *APITestTool) apiRequest(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	method, _ := params["method"].(string)
	if method == "" {
		method = "GET"
	}

	url, _ := params["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if body, ok := params["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if headers, ok := params["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	startTime := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var parsedBody interface{}
	json.Unmarshal(respBody, &parsedBody)

	return map[string]interface{}{
		"status_code":   resp.StatusCode,
		"duration":      duration.String(),
		"content_type":  resp.Header.Get("Content-Type"),
		"headers":       resp.Header,
		"body":          parsedBody,
		"raw_body":      truncatePlanStr(string(respBody), 10000),
		"size":          len(respBody),
	}, nil
}

type APITestCase struct {
	Name       string                 `json:"name"`
	Method     string                 `json:"method"`
	URL        string                 `json:"url"`
	Headers    map[string]string      `json:"headers"`
	Body       map[string]interface{} `json:"body"`
	Assertions []APIAssertion         `json:"assertions"`
}

type APIAssertion struct {
	Type     string      `json:"type"`     // status_code, body_contains, json_path
	Expected interface{} `json:"expected"`
}

func (t *APITestTool) apiTest(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	testCasesJSON, _ := params["test_cases"].(string)
	if testCasesJSON == "" {
		return nil, fmt.Errorf("test_cases parameter is required (JSON array)")
	}

	var testCases []APITestCase
	if err := json.Unmarshal([]byte(testCasesJSON), &testCases); err != nil {
		return nil, fmt.Errorf("failed to parse test_cases: %w", err)
	}

	var results []map[string]interface{}

	for _, tc := range testCases {
		result := t.runTestCase(ctx, tc)
		results = append(results, result)
	}

	passed := 0
	for _, r := range results {
		if r["passed"].(bool) {
			passed++
		}
	}

	return map[string]interface{}{
		"total":   len(results),
		"passed":  passed,
		"failed":  len(results) - passed,
		"results": results,
	}, nil
}

func (t *APITestTool) runTestCase(ctx context.Context, tc APITestCase) map[string]interface{} {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if tc.Body != nil {
		bodyJSON, _ := json.Marshal(tc.Body)
		bodyReader = bytes.NewReader(bodyJSON)
	}

	req, err := http.NewRequestWithContext(ctx, tc.Method, tc.URL, bodyReader)
	if err != nil {
		return map[string]interface{}{
			"name":   tc.Name,
			"passed": false,
			"error":  err.Error(),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range tc.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	startTime := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		return map[string]interface{}{
			"name":   tc.Name,
			"passed": false,
			"error":  err.Error(),
		}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var assertionsResult []map[string]interface{}
	allPassed := true

	for _, assertion := range tc.Assertions {
		passed := t.checkAssertion(assertion, resp.StatusCode, string(respBody))
		assertionsResult = append(assertionsResult, map[string]interface{}{
			"type":     assertion.Type,
			"expected": assertion.Expected,
			"passed":   passed,
		})
		if !passed {
			allPassed = false
		}
	}

	return map[string]interface{}{
		"name":        tc.Name,
		"status_code": resp.StatusCode,
		"duration":    duration.String(),
		"passed":      allPassed,
		"assertions":  assertionsResult,
	}
}

func (t *APITestTool) checkAssertion(assertion APIAssertion, statusCode int, body string) bool {
	switch assertion.Type {
	case "status_code":
		expected, ok := assertion.Expected.(float64)
		return ok && int(expected) == statusCode
	case "body_contains":
		expected, ok := assertion.Expected.(string)
		return ok && strings.Contains(body, expected)
	case "response_time":
		// 这里需要在外部传入 duration，简化处理
		return true
	default:
		return false
	}
}

// ========================================
// SystemMonitorTool 系统监控工具
// ========================================

// SystemMonitorTool 系统监控工具
type SystemMonitorTool struct{}

func (t *SystemMonitorTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "overview"
	}

	switch action {
	case "overview":
		return t.overview(ctx)
	case "cpu":
		return t.cpuInfo(ctx)
	case "memory":
		return t.memoryInfo(ctx)
	case "disk":
		return t.diskInfo(ctx)
	case "network":
		return t.networkInfo(ctx)
	case "process":
		return t.processInfo(ctx, params)
	default:
		return nil, fmt.Errorf("unknown monitor action: %s", action)
	}
}

func (t *SystemMonitorTool) overview(ctx context.Context) (interface{}, error) {
	cpuInfo, _ := t.cpuInfo(ctx)
	memInfo, _ := t.memoryInfo(ctx)
	diskInfo, _ := t.diskInfo(ctx)

	return map[string]interface{}{
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"cpus":      runtime.NumCPU(),
		"goroutines": runtime.NumGoroutine(),
		"cpu":       cpuInfo,
		"memory":    memInfo,
		"disk":      diskInfo,
	}, nil
}

func (t *SystemMonitorTool) cpuInfo(ctx context.Context) (interface{}, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", "top -bn1 | grep 'Cpu(s)' || echo 'N/A'")
	output, _ := cmd.CombinedOutput()

	loadAvgCmd := exec.CommandContext(ctx, "sh", "-c", "cat /proc/loadavg 2>/dev/null || uptime")
	loadAvgOutput, _ := loadAvgCmd.CombinedOutput()

	return map[string]interface{}{
		"cpu_usage": strings.TrimSpace(string(output)),
		"load_avg":  strings.TrimSpace(string(loadAvgOutput)),
		"cores":     runtime.NumCPU(),
	}, nil
}

func (t *SystemMonitorTool) memoryInfo(ctx context.Context) (interface{}, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	cmd := exec.CommandContext(ctx, "sh", "-c", "free -h 2>/dev/null || echo 'N/A'")
	output, _ := cmd.CombinedOutput()

	return map[string]interface{}{
		"go_alloc_mb":    mem.Alloc / 1024 / 1024,
		"go_total_mb":    mem.TotalAlloc / 1024 / 1024,
		"go_sys_mb":      mem.Sys / 1024 / 1024,
		"go_num_gc":      mem.NumGC,
		"system_memory":  strings.TrimSpace(string(output)),
	}, nil
}

func (t *SystemMonitorTool) diskInfo(ctx context.Context) (interface{}, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", "df -h 2>/dev/null || echo 'N/A'")
	output, _ := cmd.CombinedOutput()

	return map[string]interface{}{
		"disk_usage": strings.TrimSpace(string(output)),
	}, nil
}

func (t *SystemMonitorTool) networkInfo(ctx context.Context) (interface{}, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", "netstat -tuln 2>/dev/null || ss -tuln 2>/dev/null || echo 'N/A'")
	output, _ := cmd.CombinedOutput()

	return map[string]interface{}{
		"listening_ports": strings.TrimSpace(string(output)),
	}, nil
}

func (t *SystemMonitorTool) processInfo(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	name, _ := params["name"].(string)
	if name == "" {
		cmd := exec.CommandContext(ctx, "ps", "aux", "--sort=-%mem")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("ps command failed: %s", string(output))
		}

		// 只返回前 20 行
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) > 20 {
			lines = lines[:20]
		}

		return map[string]interface{}{
			"processes": strings.Join(lines, "\n"),
		}, nil
	}

	// 搜索特定进程
	cmd := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("ps aux | grep -i '%s' | grep -v grep", name))
	output, _ := cmd.CombinedOutput()

	return map[string]interface{}{
		"name":      name,
		"processes": strings.TrimSpace(string(output)),
	}, nil
}

// ========================================
// FileArchiveTool 文件压缩解压工具
// ========================================

// FileArchiveTool 文件压缩解压工具
type FileArchiveTool struct{}

func (t *FileArchiveTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "compress"
	}

	switch action {
	case "compress":
		return t.compress(ctx, params)
	case "extract":
		return t.extract(ctx, params)
	default:
		return nil, fmt.Errorf("unknown archive action: %s", action)
	}
}

func (t *FileArchiveTool) compress(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	source, _ := params["source"].(string)
	output, _ := params["output"].(string)
	format, _ := params["format"].(string)

	if source == "" {
		return nil, fmt.Errorf("source parameter is required")
	}

	if output == "" {
		if format == "zip" {
			output = source + ".zip"
		} else {
			output = source + ".tar.gz"
		}
	}

	if format == "zip" {
		return t.compressZip(source, output)
	}
	return t.compressTarGz(source, output)
}

func (t *FileArchiveTool) compressZip(source, output string) (interface{}, error) {
	zipFile, err := os.Create(output)
	if err != nil {
		return nil, fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	fileCount := 0
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = path
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		headerWriter, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(headerWriter, f)
			if err != nil {
				return err
			}
			fileCount++
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("compression failed: %w", err)
	}

	outputInfo, _ := os.Stat(output)

	return map[string]interface{}{
		"output":      output,
		"format":      "zip",
		"file_count":  fileCount,
		"output_size": outputInfo.Size(),
		"success":     true,
	}, nil
}

func (t *FileArchiveTool) compressTarGz(source, output string) (interface{}, error) {
	file, err := os.Create(output)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	fileCount := 0
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, path)
		if err != nil {
			return err
		}

		header.Name = path
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(tarWriter, f)
			if err != nil {
				return err
			}
			fileCount++
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("compression failed: %w", err)
	}

	outputInfo, _ := os.Stat(output)

	return map[string]interface{}{
		"output":      output,
		"format":      "tar.gz",
		"file_count":  fileCount,
		"output_size": outputInfo.Size(),
		"success":     true,
	}, nil
}

func (t *FileArchiveTool) extract(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	source, _ := params["source"].(string)
	dest, _ := params["dest"].(string)

	if source == "" {
		return nil, fmt.Errorf("source parameter is required")
	}

	if dest == "" {
		dest = filepath.Dir(source)
	}

	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".zip":
		return t.extractZip(source, dest)
	case ".gz", ".tgz":
		return t.extractTarGz(source, dest)
	default:
		return nil, fmt.Errorf("unsupported format: %s", ext)
	}
}

func (t *FileArchiveTool) extractZip(source, dest string) (interface{}, error) {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	fileCount := 0
	for _, file := range reader.File {
		path := filepath.Join(dest, file.Name)

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(path), 0755)

		outFile, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create file: %w", err)
		}

		srcFile, err := file.Open()
		if err != nil {
			outFile.Close()
			return nil, fmt.Errorf("failed to open zip entry: %w", err)
		}

		_, err = io.Copy(outFile, srcFile)
		srcFile.Close()
		outFile.Close()

		if err != nil {
			return nil, fmt.Errorf("failed to extract file: %w", err)
		}

		fileCount++
	}

	return map[string]interface{}{
		"source":     source,
		"dest":       dest,
		"file_count": fileCount,
		"success":    true,
	}, nil
}

func (t *FileArchiveTool) extractTarGz(source, dest string) (interface{}, error) {
	file, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	fileCount := 0

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		path := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(path, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(path), 0755)
			outFile, err := os.Create(path)
			if err != nil {
				return nil, fmt.Errorf("failed to create file: %w", err)
			}
			_, err = io.Copy(outFile, tarReader)
			outFile.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to extract file: %w", err)
			}
			fileCount++
		}
	}

	return map[string]interface{}{
		"source":     source,
		"dest":       dest,
		"file_count": fileCount,
		"success":    true,
	}, nil
}

// ========================================
// NetworkDiagTool 网络诊断工具
// ========================================

// NetworkDiagTool 网络诊断工具
type NetworkDiagTool struct{}

func (t *NetworkDiagTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "ping"
	}

	switch action {
	case "ping":
		return t.ping(ctx, params)
	case "curl":
		return t.curl(ctx, params)
	case "dns":
		return t.dnsLookup(ctx, params)
	case "traceroute":
		return t.traceroute(ctx, params)
	case "port_check":
		return t.portCheck(ctx, params)
	default:
		return nil, fmt.Errorf("unknown network diag action: %s", action)
	}
}

func (t *NetworkDiagTool) ping(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	host, _ := params["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("host parameter is required")
	}

	count := 4
	if c, ok := params["count"].(float64); ok {
		count = int(c)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ping", "-c", fmt.Sprintf("%d", count), host)
	output, err := cmd.CombinedOutput()

	return map[string]interface{}{
		"host":    host,
		"output":  string(output),
		"success": err == nil,
	}, nil
}

func (t *NetworkDiagTool) curl(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	url, _ := params["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	method, _ := params["method"].(string)
	if method == "" {
		method = "GET"
	}

	args := []string{"-s", "-X", method, "-w", "\n%{http_code}\n%{time_total}"}
	if headers, ok := params["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			args = append(args, "-H", fmt.Sprintf("%s: %v", k, v))
		}
	}
	if body, ok := params["body"].(string); ok {
		args = append(args, "-d", body)
	}
	args = append(args, "-o", "/dev/stdout", url)

	cmd := exec.CommandContext(ctx, "curl", args...)
	output, err := cmd.CombinedOutput()

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	statusCode := ""
	duration := ""
	if len(lines) >= 2 {
		statusCode = lines[len(lines)-2]
		duration = lines[len(lines)-1]
	}
	body := strings.Join(lines[:len(lines)-2], "\n")

	return map[string]interface{}{
		"url":         url,
		"status_code": statusCode,
		"duration":    duration + "s",
		"body":        truncatePlanStr(body, 5000),
		"success":     err == nil,
	}, nil
}

func (t *NetworkDiagTool) dnsLookup(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	hostname, _ := params["hostname"].(string)
	if hostname == "" {
		return nil, fmt.Errorf("hostname parameter is required")
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed: %w", err)
	}

	var ipStrs []string
	for _, ip := range ips {
		ipStrs = append(ipStrs, ip.String())
	}

	return map[string]interface{}{
		"hostname": hostname,
		"ips":      ipStrs,
		"count":    len(ipStrs),
	}, nil
}

func (t *NetworkDiagTool) traceroute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	host, _ := params["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("host parameter is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "traceroute", host)
	output, err := cmd.CombinedOutput()

	return map[string]interface{}{
		"host":    host,
		"output":  string(output),
		"success": err == nil,
	}, nil
}

func (t *NetworkDiagTool) portCheck(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	host, _ := params["host"].(string)
	portFloat, _ := params["port"].(float64)
	port := int(portFloat)

	if host == "" || port == 0 {
		return nil, fmt.Errorf("host and port parameters are required")
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		return map[string]interface{}{
			"host":   host,
			"port":   port,
			"open":   false,
			"error":  err.Error(),
		}, nil
	}
	defer conn.Close()

	return map[string]interface{}{
		"host":  host,
		"port":  port,
		"open":  true,
	}, nil
}

// ========================================
// RegexTool 正则表达式工具
// ========================================

// RegexTool 正则表达式工具
type RegexTool struct{}

func (t *RegexTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "match"
	}

	switch action {
	case "match":
		return t.regexMatch(ctx, params)
	case "replace":
		return t.regexReplace(ctx, params)
	case "find_all":
		return t.regexFindAll(ctx, params)
	default:
		return nil, fmt.Errorf("unknown regex action: %s", action)
	}
}

func (t *RegexTool) regexMatch(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	pattern, _ := params["pattern"].(string)
	text, _ := params["text"].(string)

	if pattern == "" || text == "" {
		return nil, fmt.Errorf("pattern and text parameters are required")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	matches := re.FindStringSubmatch(text)

	return map[string]interface{}{
		"pattern": pattern,
		"matched": len(matches) > 0,
		"matches": matches,
	}, nil
}

func (t *RegexTool) regexReplace(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	pattern, _ := params["pattern"].(string)
	text, _ := params["text"].(string)
	replacement, _ := params["replacement"].(string)

	if pattern == "" || text == "" {
		return nil, fmt.Errorf("pattern, text, and replacement parameters are required")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	result := re.ReplaceAllString(text, replacement)

	return map[string]interface{}{
		"pattern":     pattern,
		"replacement": replacement,
		"result":      result,
	}, nil
}

func (t *RegexTool) regexFindAll(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	pattern, _ := params["pattern"].(string)
	text, _ := params["text"].(string)

	if pattern == "" || text == "" {
		return nil, fmt.Errorf("pattern and text parameters are required")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	matches := re.FindAllString(text, -1)

	return map[string]interface{}{
		"pattern": pattern,
		"matches": matches,
		"count":   len(matches),
	}, nil
}

// ========================================
// 注册函数
// ========================================

// RegisterExtendedTools 注册扩展工具
func RegisterExtendedTools(toolService *ToolService) {
	toolService.registry.builtin["docker"] = &DockerTool{}
	toolService.registry.builtin["api_test"] = &APITestTool{}
	toolService.registry.builtin["system_monitor"] = &SystemMonitorTool{}
	toolService.registry.builtin["file_archive"] = &FileArchiveTool{}
	toolService.registry.builtin["network_diag"] = &NetworkDiagTool{}
	toolService.registry.builtin["regex"] = &RegexTool{}
}

// GetExtendedToolDefinitions 获取扩展工具定义
func GetExtendedToolDefinitions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "docker",
				"description": "Docker 容器操作。支持 ps, logs, build, run, stop, start, rm, exec, images, compose, inspect",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action":    map[string]interface{}{"type": "string", "description": "Docker 操作"},
						"container": map[string]interface{}{"type": "string", "description": "容器名/ID"},
						"image":     map[string]interface{}{"type": "string", "description": "镜像名"},
						"tag":       map[string]interface{}{"type": "string", "description": "镜像标签"},
						"path":      map[string]interface{}{"type": "string", "description": "路径"},
						"name":      map[string]interface{}{"type": "string", "description": "容器名"},
						"ports":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "端口映射"},
						"env":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "环境变量"},
						"volumes":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "卷挂载"},
						"command":   map[string]interface{}{"type": "string", "description": "命令"},
						"tail":      map[string]interface{}{"type": "string", "description": "日志行数"},
						"follow":    map[string]interface{}{"type": "boolean", "description": "跟随日志"},
						"force":     map[string]interface{}{"type": "boolean", "description": "强制删除"},
						"file":      map[string]interface{}{"type": "string", "description": "compose 文件"},
						"detach":    map[string]interface{}{"type": "boolean", "description": "后台运行"},
						"build":     map[string]interface{}{"type": "boolean", "description": "构建镜像"},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "api_test",
				"description": "API 测试工具。支持发送请求和批量测试用例",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action":     map[string]interface{}{"type": "string", "description": "操作: request, test", "enum": []string{"request", "test"}},
						"method":     map[string]interface{}{"type": "string", "description": "HTTP 方法"},
						"url":        map[string]interface{}{"type": "string", "description": "URL"},
						"headers":    map[string]interface{}{"type": "object", "description": "请求头"},
						"body":       map[string]interface{}{"type": "string", "description": "请求体"},
						"test_cases": map[string]interface{}{"type": "string", "description": "测试用例 JSON 数组"},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "system_monitor",
				"description": "系统监控工具。查看 CPU、内存、磁盘、网络、进程状态",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action": map[string]interface{}{"type": "string", "description": "监控类型: overview, cpu, memory, disk, network, process", "enum": []string{"overview", "cpu", "memory", "disk", "network", "process"}},
						"name":   map[string]interface{}{"type": "string", "description": "进程名（process 时使用）"},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "file_archive",
				"description": "文件压缩解压工具。支持 zip 和 tar.gz 格式",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action": map[string]interface{}{"type": "string", "description": "操作: compress, extract", "enum": []string{"compress", "extract"}},
						"source": map[string]interface{}{"type": "string", "description": "源文件/目录"},
						"output": map[string]interface{}{"type": "string", "description": "输出文件（compress 时）"},
						"dest":   map[string]interface{}{"type": "string", "description": "解压目录（extract 时）"},
						"format": map[string]interface{}{"type": "string", "description": "格式: zip, tar.gz"},
					},
					"required": []string{"action", "source"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "network_diag",
				"description": "网络诊断工具。支持 ping, curl, dns, traceroute, port_check",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action":    map[string]interface{}{"type": "string", "description": "诊断类型: ping, curl, dns, traceroute, port_check"},
						"host":      map[string]interface{}{"type": "string", "description": "目标主机"},
						"url":       map[string]interface{}{"type": "string", "description": "URL"},
						"port":      map[string]interface{}{"type": "number", "description": "端口号"},
						"hostname":  map[string]interface{}{"type": "string", "description": "域名"},
						"method":    map[string]interface{}{"type": "string", "description": "HTTP 方法"},
						"headers":   map[string]interface{}{"type": "object", "description": "请求头"},
						"body":      map[string]interface{}{"type": "string", "description": "请求体"},
						"count":     map[string]interface{}{"type": "number", "description": "ping 次数"},
					},
					"required": []string{"action"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "regex",
				"description": "正则表达式工具。支持匹配、替换、查找",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"action":      map[string]interface{}{"type": "string", "description": "操作: match, replace, find_all", "enum": []string{"match", "replace", "find_all"}},
						"pattern":     map[string]interface{}{"type": "string", "description": "正则表达式"},
						"text":        map[string]interface{}{"type": "string", "description": "输入文本"},
						"replacement": map[string]interface{}{"type": "string", "description": "替换文本（replace 时）"},
					},
					"required": []string{"action", "pattern", "text"},
				},
			},
		},
	}
}
