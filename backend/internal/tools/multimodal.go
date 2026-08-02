package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"openaide/backend/internal/kernel"
)

func multimodalToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "read_image",
				Description: "读取图片文件，返回base64数据供多模态模型分析",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "图片路径",
						},
					},
					"required": []string{"path"},
				},
			},
		},
	}
}

// handleReadImage 读取图片文件并转为 base64
func handleReadImage(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Path == "" {
		return &kernel.ToolResult{Error: "path is required"}, nil
	}

	absPath, err := validateAndResolve(args.Path)
	if err != nil {
		return toolErrInvalidPath(err), nil
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	mt := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}[ext]
	if mt == "" {
		return toolErr("UNSUPPORTED_FORMAT", "unsupported format: %s", ext), nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if len(data) > 10*1024*1024 {
		return &kernel.ToolResult{Error: "image too large (>10MB)"}, nil
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	return &kernel.ToolResult{Content: fmt.Sprintf("data:%s;base64,%s", mt, b64)}, nil
}
