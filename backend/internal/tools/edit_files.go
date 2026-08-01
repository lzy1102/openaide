package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"openaide/backend/internal/kernel"
)

// ── edit_files: 多文件原子编辑 ────────────────────────────────
//
// 四阶段事务:
//   1. 预检(并行):验证所有 search_text 在目标文件中存在且唯一
//   2. 备份(内存):备份所有目标文件的原始内容
//   3. 应用(串行):逐文件应用替换,每个文件写完后立即读回验证
//   4. 回滚(失败时):用内存备份恢复所有已写入的文件
//
// 任意一步失败 → 整体 abort(预检阶段)或回滚(应用阶段),
// 保证"要么全部成功,要么全部不动"。

func multiFileEditToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name: "edit_files",
				Description: "原子性批量编辑多个文件。全部编辑预检通过后才会写入;" +
					"任一写入失败自动回滚已修改文件。适合跨文件重构。" +
					"同一文件可包含多个 edit,按顺序应用。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"edits": map[string]interface{}{
							"type": "array",
							"description": "编辑列表,每个 edit 修改一处",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"path": map[string]interface{}{
										"type": "string",
										"description": "文件路径(相对或绝对)",
									},
									"search_text": map[string]interface{}{
										"type": "string",
										"description": "要搜索的文本(在该文件中必须唯一)",
									},
									"replace_text": map[string]interface{}{
										"type": "string",
										"description": "替换后的文本",
									},
								},
								"required": []string{"path", "search_text", "replace_text"},
							},
							"minItems": 1,
						},
					},
					"required": []string{"edits"},
				},
			},
		},
	}
}

// editEntry 是单个编辑请求
type editEntry struct {
	Path        string `json:"path"`
	SearchText  string `json:"search_text"`
	ReplaceText string `json:"replace_text"`
}

// editPrecheckResult 是预检结果
type editPrecheckResult struct {
	index    int
	absPath  string
	ok       bool
	reason   string // 失败原因
}

// handleEditFiles 是 edit_files 工具的 handler。
func handleEditFiles(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Edits []editEntry `json:"edits"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if len(args.Edits) == 0 {
		return &kernel.ToolResult{Error: "edits is empty"}, nil
	}

	// 阶段 1:并行预检
	// 每个 edit 验证:路径合法 + 文件可读 + search_text 存在且唯一
	precheckResults := make([]editPrecheckResult, len(args.Edits))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // 并发限制 8
	for i, e := range args.Edits {
		wg.Add(1)
		go func(idx int, edit editEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := editPrecheckResult{index: idx}
			abs, err := safeAbsPath(edit.Path)
			if err != nil {
				r.reason = fmt.Sprintf("invalid path: %v", err)
				precheckResults[idx] = r
				return
			}
			r.absPath = abs

			if edit.SearchText == "" {
				r.reason = "search_text is empty"
				precheckResults[idx] = r
				return
			}

			data, err := os.ReadFile(abs)
			if err != nil {
				r.reason = fmt.Sprintf("read failed: %v", err)
				precheckResults[idx] = r
				return
			}
			content := string(data)
			count := strings.Count(content, edit.SearchText)
			switch {
			case count == 0:
				r.reason = "search_text not found (hint: read the file first to verify exact content/whitespace)"
			case count > 1:
				r.reason = fmt.Sprintf("search_text found %d times — include 2-3 lines of surrounding context for uniqueness", count)
			default:
				r.ok = true
			}
			precheckResults[idx] = r
		}(i, e)
	}
	wg.Wait()

	// 检查预检结果
	var failures []string
	for _, r := range precheckResults {
		if !r.ok {
			failures = append(failures, fmt.Sprintf("- edit %d (%s): %s", r.index+1, args.Edits[r.index].Path, r.reason))
		}
	}
	if len(failures) > 0 {
		var sb strings.Builder
		sb.WriteString("✗ Pre-check failed, no files modified\n")
		sb.WriteString(strings.Join(failures, "\n"))
		return &kernel.ToolResult{Error: sb.String()}, nil
	}

	// 阶段 2:备份所有目标文件的原始内容
	// 同一文件被多个 edit 引用时只备份一次
	// 同时保存到 Undo 检查点栈(支持 undo_edit 跨工具回滚)
	backups := make(map[string]string)
	affectedPaths := make(map[string]bool)
	for _, r := range precheckResults {
		if !affectedPaths[r.absPath] {
			data, err := os.ReadFile(r.absPath)
			if err != nil {
				return &kernel.ToolResult{Error: fmt.Sprintf("backup read failed for %s: %v", r.absPath, err)}, nil
			}
			backups[r.absPath] = string(data)
			affectedPaths[r.absPath] = true
			// Undo 检查点:每个受影响文件各存一份
			saveFileCheckpoint(r.absPath, "edit_files")
		}
	}

	// 阶段 3:串行应用替换
	// 同一文件的多个 edit 顺序应用,每次重新检查唯一性
	appliedEdits := 0
	appliedPaths := make([]string, 0)
	var appliedDetails []string

	for i, e := range args.Edits {
		abs := precheckResults[i].absPath
		data, err := os.ReadFile(abs)
		if err != nil {
			// 应用阶段失败 → 回滚
			rolledBack := rollback(backups, appliedPaths)
			return &kernel.ToolResult{
				Error: fmt.Sprintf("✗ Write phase failed at edit %d (%s): %v\n%s",
					i+1, e.Path, err, rolledBack),
			}, nil
		}
		content := string(data)
		count := strings.Count(content, e.SearchText)
		if count != 1 {
			// 唯一性被前一次替换破坏 → 回滚
			rolledBack := rollback(backups, appliedPaths)
			return &kernel.ToolResult{
				Error: fmt.Sprintf("✗ Uniqueness broken at edit %d (%s): found %d matches after previous edits\n%s",
					i+1, e.Path, count, rolledBack),
			}, nil
		}
		newContent := strings.Replace(content, e.SearchText, e.ReplaceText, 1)
		if err := os.WriteFile(abs, []byte(newContent), 0644); err != nil {
			// 写入失败 → 回滚
			rolledBack := rollback(backups, appliedPaths)
			return &kernel.ToolResult{
				Error: fmt.Sprintf("✗ Write failed at edit %d (%s): %v\n%s",
					i+1, e.Path, err, rolledBack),
			}, nil
		}
		// 读回验证
		verifyData, _ := os.ReadFile(abs)
		if !strings.Contains(string(verifyData), e.ReplaceText) {
			// 验证失败 → 回滚
			rolledBack := rollback(backups, appliedPaths)
			return &kernel.ToolResult{
				Error: fmt.Sprintf("✗ Verification failed at edit %d (%s): replacement not found in re-read\n%s",
					i+1, e.Path, rolledBack),
			}, nil
		}

		// 记录已应用
		lineNum := findLineNumber(content, e.SearchText)
		appliedDetails = append(appliedDetails, fmt.Sprintf("- %s: edit %d (line %d)", e.Path, i+1, lineNum))
		appliedEdits++
		// 记录已写入的路径(用于回滚)——只在第一次写入该文件时加入
		if !slices.Contains(appliedPaths, abs) {
			appliedPaths = append(appliedPaths, abs)
		}
	}

	// 阶段 4:成功返回
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✓ Atomic edit applied to %d files (%d edits total)\n",
		len(appliedPaths), appliedEdits))
	for _, d := range appliedDetails {
		sb.WriteString(d)
		sb.WriteString("\n")
	}
	sb.WriteString("Verified: all replacements confirmed in re-read")
	return &kernel.ToolResult{Content: sb.String()}, nil
}

// rollback 用内存备份恢复已写入的文件。
// 返回回滚报告字符串。
func rollback(backups map[string]string, appliedPaths []string) string {
	if len(appliedPaths) == 0 {
		return "No files needed rollback."
	}
	var rolled []string
	var failed []string
	for _, p := range appliedPaths {
		orig, ok := backups[p]
		if !ok {
			failed = append(failed, p+" (no backup)")
			continue
		}
		if err := os.WriteFile(p, []byte(orig), 0644); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", p, err))
		} else {
			rolled = append(rolled, p)
		}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rolled back %d file(s)", len(rolled)))
	if len(failed) > 0 {
		sb.WriteString(fmt.Sprintf(", %d failed: %s", len(failed), strings.Join(failed, ", ")))
	}
	return sb.String()
}
