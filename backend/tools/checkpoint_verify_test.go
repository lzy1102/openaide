package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// 验证所有编辑工具在写文件前都保存了 checkpoint（undo 保护）。
func TestAllEditToolsCreateCheckpoint(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")

	run := func(name string, fn func() string) {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644); err != nil {
				t.Fatal(err)
			}
			// 重置栈
			fileCheckpointStore.checkpoints = nil

			if msg := fn(); msg != "" {
				t.Fatalf("handler error: %s", msg)
			}
			if n := len(fileCheckpointStore.checkpoints); n != 1 {
				t.Errorf("checkpoint count = %d, want 1 — undo protection missing", n)
			}
		})
	}

	run("write_file", func() string {
		r, _ := handleWriteFile(nil, `{"path":"`+f+`","content":"new"}`)
		if r != nil {
			return r.Error
		}
		return ""
	})
	run("diff_edit", func() string {
		r, _ := handleDiffEdit(nil, `{"path":"`+f+`","search_text":"line1","replace_text":"changed"}`)
		if r != nil {
			return r.Error
		}
		return ""
	})
	run("apply_patch", func() string {
		r, _ := handleApplyPatch(nil, `{"path":"`+f+`","content":"<<<<<<< SEARCH\nline2\n=======\npatched\n>>>>>>> REPLACE"}`)
		if r != nil {
			return r.Error
		}
		return ""
	})
	run("diff_edit_lines", func() string {
		r, _ := handleDiffEditLines(nil, `{"path":"`+f+`","start_line":1,"content":"replaced"}`)
		if r != nil {
			return r.Error
		}
		return ""
	})
	run("edit_files", func() string {
		r, _ := handleEditFiles(nil, `{"edits":[{"path":"`+f+`","search_text":"line1","replace_text":"changed"}]}`)
		if r != nil {
			return r.Error
		}
		return ""
	})
}
