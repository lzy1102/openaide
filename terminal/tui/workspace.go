package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const workspaceDirName = ".openaide"
const workspaceFileName = "workspace.json"

// WorkspaceState 保存工作区状态
type WorkspaceState struct {
	ProjectID  string `json:"project_id"`
	DialogueID string `json:"dialogue_id,omitempty"`
	WorkingDir string `json:"working_dir"`
}

// GetWorkspacePath 获取当前目录下的 .openaide 路径
func GetWorkspacePath() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(wd, workspaceDirName)
}

// HasWorkspace 检查当前目录是否有 .openaide 标记
func HasWorkspace() bool {
	path := GetWorkspacePath()
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// LoadWorkspaceState 从 .openaide/workspace.json 加载状态
func LoadWorkspaceState() (*WorkspaceState, error) {
	path := GetWorkspacePath()
	if path == "" {
		return nil, fmt.Errorf("cannot get working directory")
	}

	stateFile := filepath.Join(path, workspaceFileName)
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, err
	}

	var state WorkspaceState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	// 验证 working_dir 匹配当前目录
	wd, _ := os.Getwd()
	if state.WorkingDir != wd {
		state.WorkingDir = wd
	}

	return &state, nil
}

// SaveWorkspaceState 保存状态到 .openaide/workspace.json
func SaveWorkspaceState(state *WorkspaceState) error {
	path := GetWorkspacePath()
	if path == "" {
		return fmt.Errorf("cannot get working directory")
	}

	// 确保目录存在
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	// 更新 working_dir
	wd, _ := os.Getwd()
	state.WorkingDir = wd

	stateFile := filepath.Join(path, workspaceFileName)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(stateFile, data, 0644)
}

// InitWorkspace 初始化 .openaide 目录
func InitWorkspace(projectID string) error {
	state := &WorkspaceState{
		ProjectID: projectID,
	}
	return SaveWorkspaceState(state)
}

// ClearWorkspace 清除 .openaide 标记
func ClearWorkspace() error {
	path := GetWorkspacePath()
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}

// UpdateWorkspaceDialogue 更新工作区中的对话 ID
func UpdateWorkspaceDialogue(dialogueID string) error {
	state, err := LoadWorkspaceState()
	if err != nil {
		return err
	}
	state.DialogueID = dialogueID
	return SaveWorkspaceState(state)
}
