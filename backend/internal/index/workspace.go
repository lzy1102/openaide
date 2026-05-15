package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace 工作区
type Workspace struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Root     string    `json:"root"`
	Type     string    `json:"type"` // go-workspace, node-monorepo, rust-workspace, custom
	Modules  []Module  `json:"modules"`
	Config   WorkspaceConfig `json:"config"`
}

// Module 子模块
type Module struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Language string `json:"language"`
}

// WorkspaceConfig 工作区配置
type WorkspaceConfig struct {
	IndexGranularity string `json:"index_granularity"` // coarse, standard, fine
	MaxMemoryMB      int    `json:"max_memory_mb"`
	Workers          int    `json:"workers"`
}

// WorkspaceDetector 工作区检测器
type WorkspaceDetector struct{}

// NewWorkspaceDetector 创建工作区检测器
func NewWorkspaceDetector() *WorkspaceDetector {
	return &WorkspaceDetector{}
}

// Detect 检测工作区
func (d *WorkspaceDetector) Detect(root string) (*Workspace, error) {
	workspace := &Workspace{
		ID:   d.generateID(root),
		Name: filepath.Base(root),
		Root: root,
		Config: WorkspaceConfig{
			IndexGranularity: "standard",
			MaxMemoryMB:      256,
			Workers:          4,
		},
	}

	// 检测工作区类型
	if d.isGoWorkspace(root) {
		workspace.Type = "go-workspace"
		workspace.Modules = d.detectGoModules(root)
	} else if d.isNodeMonorepo(root) {
		workspace.Type = "node-monorepo"
		workspace.Modules = d.detectNodePackages(root)
	} else if d.isRustWorkspace(root) {
		workspace.Type = "rust-workspace"
		workspace.Modules = d.detectRustPackages(root)
	} else {
		workspace.Type = "single"
		workspace.Modules = []Module{
			{
				ID:       d.generateID(root),
				Name:     workspace.Name,
				Path:     root,
				Language: detectLanguage(root),
			},
		}
	}

	return workspace, nil
}

func (d *WorkspaceDetector) isGoWorkspace(root string) bool {
	_, err := os.Stat(filepath.Join(root, "go.work"))
	return err == nil
}

func (d *WorkspaceDetector) isNodeMonorepo(root string) bool {
	files := []string{"pnpm-workspace.yaml", "lerna.json", "nx.json"}
	for _, f := range files {
		_, err := os.Stat(filepath.Join(root, f))
		if err == nil {
			return true
		}
	}

	// 检查 package.json 是否有 workspaces
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return false
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}

	_, hasWorkspaces := pkg["workspaces"]
	return hasWorkspaces
}

func (d *WorkspaceDetector) isRustWorkspace(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "Cargo.toml"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "[workspace]")
}

func (d *WorkspaceDetector) detectGoModules(root string) []Module {
	var modules []Module

	// 读取 go.work
	data, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "use ") {
				path := strings.Trim(strings.TrimPrefix(line, "use "), "\"'")
				modPath := filepath.Join(root, path)
				name := filepath.Base(modPath)
				modules = append(modules, Module{
					ID:       d.generateID(modPath),
					Name:     name,
					Path:     modPath,
					Language: "go",
				})
			}
		}
	}

	// 如果没有找到，扫描子目录
	if len(modules) == 0 {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			modPath := filepath.Join(root, entry.Name())
			if _, err := os.Stat(filepath.Join(modPath, "go.mod")); err == nil {
				modules = append(modules, Module{
					ID:       d.generateID(modPath),
					Name:     entry.Name(),
					Path:     modPath,
					Language: "go",
				})
			}
		}
	}

	return modules
}

func (d *WorkspaceDetector) detectNodePackages(root string) []Module {
	var modules []Module

	// 读取 pnpm-workspace.yaml
	data, err := os.ReadFile(filepath.Join(root, "pnpm-workspace.yaml"))
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				pattern := strings.Trim(strings.TrimPrefix(line, "- "), "\"'")
				// 解析 glob 模式
				if strings.Contains(pattern, "*") {
					// 简单处理：取目录前缀
					prefix := strings.Split(pattern, "/*")[0]
					dir := filepath.Join(root, prefix)
					entries, _ := os.ReadDir(dir)
					for _, entry := range entries {
						if !entry.IsDir() {
							continue
						}
						modPath := filepath.Join(dir, entry.Name())
						if _, err := os.Stat(filepath.Join(modPath, "package.json")); err == nil {
							modules = append(modules, Module{
								ID:       d.generateID(modPath),
								Name:     entry.Name(),
								Path:     modPath,
								Language: "javascript",
							})
						}
					}
				}
			}
		}
	}

	return modules
}

func (d *WorkspaceDetector) detectRustPackages(root string) []Module {
	var modules []Module

	// 读取 Cargo.toml
	data, err := os.ReadFile(filepath.Join(root, "Cargo.toml"))
	if err != nil {
		return modules
	}

	content := string(data)
	inWorkspace := false
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "[workspace]" {
			inWorkspace = true
			continue
		}
		if inWorkspace && strings.HasPrefix(line, "[") && line != "[workspace]" {
			break
		}
		if inWorkspace && strings.HasPrefix(line, "members") {
			// 解析 members 数组
			start := strings.Index(line, "[")
			end := strings.Index(line, "]")
			if start >= 0 && end > start {
				members := line[start+1 : end]
				for _, m := range strings.Split(members, ",") {
					m = strings.Trim(strings.TrimSpace(m), "\"'")
					if m == "" {
						continue
					}
					modPath := filepath.Join(root, m)
					name := filepath.Base(modPath)
					modules = append(modules, Module{
						ID:       d.generateID(modPath),
						Name:     name,
						Path:     modPath,
						Language: "rust",
					})
				}
			}
		}
	}

	return modules
}

func (d *WorkspaceDetector) generateID(path string) string {
	var h uint32
	for _, c := range path {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("mod_%x", h)
}
