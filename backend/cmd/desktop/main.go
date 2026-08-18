package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"

	"openaide/backend/internal/config"
	"openaide/backend/internal/infra"
	"openaide/backend/core"
)

// App is the Wails application backend.
type App struct {
	ctx    context.Context
	app    *infra.Application
	kernel kernel.Kernel
	cfg    *config.Config
}

// NewApp creates the desktop application backend.
func NewApp() *App {
	return &App{}
}

// startup is called when the Wails app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	configPath := os.Getenv("HOME") + "/.openaide/config.yaml"
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		cfg = config.DefaultConfig()
	}

	app, err := infra.NewApplication(cfg)
	if err != nil {
		slog.Error("Failed to create application", "error", err)
		return
	}
	go app.Start()

	a.app = app
	a.kernel = app.Kernel
	a.cfg = cfg
}

// shutdown is called when the Wails app exits.
func (a *App) shutdown(ctx context.Context) {
	if a.app != nil {
		a.app.Stop(ctx)
	}
}

// SendMessage sends a chat message and returns streaming results via callback.
func (a *App) SendMessage(message string) (string, error) {
	if a.app == nil {
		return "", fmt.Errorf("application not initialized")
	}

	resp, err := a.app.Orchestrator.ProcessQuery(a.ctx, "desktop-user", "default", message, kernel.QueryOptions{MaxTokens: 4000})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// GetFileTree returns the project file tree starting from the given directory.
func (a *App) GetFileTree(dir string) []FileNode {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return walkDir(dir, 2)
}

// ReadFileContent reads a file and returns its content.
func (a *App) ReadFileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(data)
}

// GetSessions returns recent sessions.
func (a *App) GetSessions() []SessionInfo {
	if a.app == nil {
		return nil
	}
	sessions, err := a.app.Orchestrator.ListSessions(a.ctx, "desktop-user", "default", 20, 0)
	if err != nil {
		return nil
	}
	var result []SessionInfo
	for _, s := range sessions {
		result = append(result, SessionInfo{
			ID:        s.ID,
			UpdatedAt: s.UpdatedAt.Format("2006-01-02 15:04"),
		})
	}
	return result
}

// FileNode represents a file or directory in the project tree.
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"is_dir"`
	Children []FileNode `json:"children,omitempty"`
}

// SessionInfo is a lightweight session view for the sidebar.
type SessionInfo struct {
	ID        string `json:"id"`
	UpdatedAt string `json:"updated_at"`
}

func walkDir(dir string, depth int) []FileNode {
	if depth <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var nodes []FileNode
	for _, e := range entries {
		if e.Name()[0] == '.' {
			continue
		}
		path := filepath.Join(dir, e.Name())
		node := FileNode{Name: e.Name(), Path: path, IsDir: e.IsDir()}
		if e.IsDir() {
			node.Children = walkDir(path, depth-1)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:      "OpenAIDE",
		Width:      1200,
		Height:     800,
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
