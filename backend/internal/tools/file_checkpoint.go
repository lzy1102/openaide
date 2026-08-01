package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"openaide/backend/internal/kernel"
)

// ── 文件级 Undo ────────────────────────────────────────────
//
// 在每次写操作(write_file / diff_edit / edit_files)之前,
// 把目标文件的当前内容备份到一个栈中。
// 提供 undo_edit 工具让 LLM 或用户回滚到上一个检查点。
//
// 设计:
//   - 栈式结构,后进先出(undo 最近一次写操作)
//   - 每个检查点记录:文件路径、原始内容、时间戳、触发工具
//   - 最多保留 50 个检查点(防止内存膨胀)
//   - 文件不存在时也记录(nil 内容),undo 时删除文件

const maxFileCheckpoints = 50

// fileCheckpoint 记录一次写操作前的文件状态。
type fileCheckpoint struct {
	Path      string    // 文件绝对路径
	Content   *string   // 原始内容(nil = 文件之前不存在)
	Tool      string    // 触发备份的工具名
	Timestamp time.Time // 备份时间
}

// fileCheckpointStore 是文件检查点的全局栈。
// 所有写工具共享同一个栈,undo 时弹出最近的检查点。
var fileCheckpointStore = &fileCheckpointStack{
	checkpoints: make([]fileCheckpoint, 0, maxFileCheckpoints),
}

type fileCheckpointStack struct {
	mu          sync.Mutex
	checkpoints []fileCheckpoint
}

// saveFileCheckpoint 在写操作前备份文件当前状态。
// 如果文件不存在,记录 nil(undo 时会删除文件)。
// 必须在持有文件锁的情况下调用(调用方负责加锁)。
func saveFileCheckpoint(absPath, toolName string) {
	fileCheckpointStore.mu.Lock()
	defer fileCheckpointStore.mu.Unlock()

	var content *string
	if data, err := os.ReadFile(absPath); err == nil {
		s := string(data)
		content = &s
	}
	// 文件不存在 → content 保持 nil,undo 时删除文件

	cp := fileCheckpoint{
		Path:      absPath,
		Content:   content,
		Tool:      toolName,
		Timestamp: time.Now(),
	}

	fileCheckpointStore.checkpoints = append(fileCheckpointStore.checkpoints, cp)
	// 裁剪:保留最近 maxFileCheckpoints 个
	if len(fileCheckpointStore.checkpoints) > maxFileCheckpoints {
		fileCheckpointStore.checkpoints = fileCheckpointStore.checkpoints[len(fileCheckpointStore.checkpoints)-maxFileCheckpoints:]
	}
}

// restoreFileCheckpoint 弹出最近的检查点并恢复文件。
// 返回恢复的文件路径和错误(如果栈空或恢复失败)。
func restoreFileCheckpoint() (path string, tool string, err error) {
	fileCheckpointStore.mu.Lock()
	defer fileCheckpointStore.mu.Unlock()

	if len(fileCheckpointStore.checkpoints) == 0 {
		return "", "", fmt.Errorf("no checkpoints to restore")
	}

	cp := fileCheckpointStore.checkpoints[len(fileCheckpointStore.checkpoints)-1]
	fileCheckpointStore.checkpoints = fileCheckpointStore.checkpoints[:len(fileCheckpointStore.checkpoints)-1]

	if cp.Content == nil {
		// 文件之前不存在 → 删除
		if rmErr := os.Remove(cp.Path); rmErr != nil && !os.IsNotExist(rmErr) {
			return cp.Path, cp.Tool, fmt.Errorf("remove failed: %w", rmErr)
		}
		return cp.Path, cp.Tool, nil
	}

	if writeErr := os.WriteFile(cp.Path, []byte(*cp.Content), 0644); writeErr != nil {
		return cp.Path, cp.Tool, fmt.Errorf("restore write failed: %w", writeErr)
	}
	return cp.Path, cp.Tool, nil
}

// listFileCheckpoints 返回当前栈中的检查点信息(不含内容)。
func listFileCheckpoints() []fileCheckpointInfo {
	fileCheckpointStore.mu.Lock()
	defer fileCheckpointStore.mu.Unlock()

	infos := make([]fileCheckpointInfo, 0, len(fileCheckpointStore.checkpoints))
	for i := len(fileCheckpointStore.checkpoints) - 1; i >= 0; i-- {
		cp := fileCheckpointStore.checkpoints[i]
		size := 0
		if cp.Content != nil {
			size = len(*cp.Content)
		}
		infos = append(infos, fileCheckpointInfo{
			Index:     i + 1,
			Path:      cp.Path,
			Tool:      cp.Tool,
			Size:      size,
			Timestamp: cp.Timestamp.Format("15:04:05"),
		})
	}
	return infos
}

type fileCheckpointInfo struct {
	Index     int    `json:"index"`
	Path      string `json:"path"`
	Tool      string `json:"tool"`
	Size      int    `json:"size"`
	Timestamp string `json:"timestamp"`
}

// ── undo_edit 工具 ─────────────────────────────────────────

func undoToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name: "undo_edit",
				Description: "撤销最近一次文件编辑,恢复到写操作前的状态。" +
					"如果文件之前不存在则删除。可连续调用撤销多次。" +
					"用于 agent 改错文件时快速回滚。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name: "list_undo_checkpoints",
				Description: "列出当前可撤销的文件检查点(最近 " +
					fmt.Sprintf("%d", maxFileCheckpoints) + " 次写操作)。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}
}

func handleUndoEdit(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	path, tool, err := restoreFileCheckpoint()
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	return &kernel.ToolResult{
		Content: fmt.Sprintf("✓ Undo: restored %s (was modified by %s)", path, tool),
	}, nil
}

func handleListUndoCheckpoints(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	infos := listFileCheckpoints()
	if len(infos) == 0 {
		return &kernel.ToolResult{Content: "No checkpoints available."}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Available checkpoints (%d):\n", len(infos)))
	for _, info := range infos {
		sizeStr := "new file"
		if info.Size > 0 {
			sizeStr = formatBytes(int64(info.Size))
		}
		sb.WriteString(fmt.Sprintf("  #%d  %s  %s  %s  (%s)\n",
			info.Index, info.Timestamp, info.Tool, info.Path, sizeStr))
	}
	sb.WriteString("\nUse undo_edit to restore the most recent checkpoint.")
	return &kernel.ToolResult{Content: sb.String()}, nil
}

// ── JSON helper for list ───────────────────────────────────

func init() {
	// 确保反序列化参数时不会因为空 arguments 报错
	_ = json.Unmarshal
}
