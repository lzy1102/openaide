package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleWriteFile_ACIVerification(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	result, err := handleWriteFile(context.Background(), `{"path":"`+file+`","content":"line1\nline2\nline3"}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	content := result.Content.(string)
	if !strings.Contains(content, "Verified: content matches") {
		t.Errorf("write response should include verification, got: %s", content)
	}
	if !strings.Contains(content, "1 | line1") {
		t.Errorf("write response should include line-numbered preview, got: %s", content)
	}

	// Verify file on disk
	data, _ := os.ReadFile(file)
	if string(data) != "line1\nline2\nline3" {
		t.Errorf("file content mismatch: got %q", string(data))
	}
}

func TestHandleWriteFile_LargeContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "large.txt")
	largeContent := ""
	for i := 0; i < 200; i++ {
		largeContent += "this is a line of text that repeats many times\n"
	}
	result, _ := handleWriteFile(context.Background(), `{"path":"`+file+`","content":"`+strings.ReplaceAll(largeContent, "\n", "\\n")+`"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	content := result.Content.(string)
	if !strings.Contains(content, "more lines") {
		t.Errorf("large write should show truncated preview, got: %s", content[:100])
	}
}

func TestHandleDiffEdit_ACIVerification(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "edit.go")
	os.WriteFile(file, []byte("old code here\nmore old code"), 0644)

	result, _ := handleDiffEdit(context.Background(), `{"path":"`+file+`","search_text":"old code here","replace_text":"new code here"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	content := result.Content.(string)
	t.Logf("diff_edit response: %s", content)
	if !strings.Contains(content, "--- before") {
		t.Error("diff_edit should show before context")
	}
	if !strings.Contains(content, "+++ after") {
		t.Error("diff_edit should show after context")
	}
	if !strings.Contains(content, "Modified") {
		t.Error("diff_edit should show modification summary")
	}

	// Verify file was actually changed
	data, _ := os.ReadFile(file)
	if string(data) != "new code here\nmore old code" {
		t.Errorf("file not modified correctly: %q", string(data))
	}
}

func TestHandleReadFile_LineNumbers(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "read.txt")
	os.WriteFile(file, []byte("line one\nline two\nline three"), 0644)

	result, _ := handleReadFile(context.Background(), `{"path":"`+file+`"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	content := result.Content.(string)
	if !strings.Contains(content, "1 | line one") {
		t.Errorf("read_file should include line numbers, got: %s", content)
	}
	if !strings.Contains(content, "2 | line two") {
		t.Errorf("read_file should include line two")
	}
}

func TestHandleReadFile_OffsetLimit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "range.txt")
	os.WriteFile(file, []byte("line1\nline2\nline3\nline4\nline5"), 0644)

	result, _ := handleReadFile(context.Background(), `{"path":"`+file+`","offset":1,"limit":2}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	content := result.Content.(string)
	if strings.Contains(content, "line1") {
		t.Error("offset=1 should skip line1")
	}
	if !strings.Contains(content, "line2") && !strings.Contains(content, "line3") {
		t.Errorf("should contain requested range, got: %s", content)
	}
}

func TestHandleExecuteCommand_ExitCode(t *testing.T) {
	result, _ := handleExecuteCommand(context.Background(), `{"command":"echo hello world"}`)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	content := result.Content.(string)
	if !strings.Contains(content, "[exit=0]") {
		t.Errorf("execute_command should show exit code, got: %s", content)
	}
	if !strings.Contains(content, "hello world") {
		t.Errorf("should show command output")
	}
}

func TestHandleExecuteCommand_FailureExitCode(t *testing.T) {
	result, _ := handleExecuteCommand(context.Background(), `{"command":"cat /nonexistent_file_xyz 2>&1; exit 1"}`)
	// This may not error — exit 1 is a non-zero exit
	content := result.Content.(string)
	t.Logf("failure result: %s", content)
}
