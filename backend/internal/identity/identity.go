package identity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Identity 项目身份
type Identity struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	ProjectType string `json:"project_type"` // go, node, python, rust, etc.
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	WorkDir     string `json:"work_dir"`
}

// ProjectAnchor 项目语言锚点:锚点文件路径 → 语言标识。
type ProjectAnchor struct {
	Path string // 项目根下存在的文件
	Lang string // 语言标识
}

// ProjectAnchors 是共享的项目语言锚点表(单一来源)。
// identity 包取首个命中作为项目类型;kernel 包取全部命中注入语言约定。
// 保持两个包列表一致,避免漂移。
var ProjectAnchors = []ProjectAnchor{
	{"go.mod", "go"},
	{"go.work", "go-workspace"},
	{"package.json", "node"},
	{"Cargo.toml", "rust"},
	{"pyproject.toml", "python"},
	{"setup.py", "python"},
	{"requirements.txt", "python"},
	{"pom.xml", "java"},
	{"build.gradle", "java"},
	{"build.gradle.kts", "kotlin"},
	{"CMakeLists.txt", "c"},
	{"Makefile", "c"},
	{"Package.swift", "swift"},
	{"composer.json", "php"},
	{"Gemfile", "ruby"},
	{"pubspec.yaml", "dart"},
	{"mix.exs", "elixir"},
	{"stack.yaml", "haskell"},
	{"rebar.config", "erlang"},
	{"dune-project", "ocaml"},
	{"cpanfile", "perl"},
}

// Detector 身份检测器
type Detector struct{}

// NewDetector 创建身份检测器
func NewDetector() *Detector {
	return &Detector{}
}

// Detect 检测项目身份
func (d *Detector) Detect(ctx context.Context, workDir string) (*Identity, error) {
	identity := &Identity{
		WorkDir: workDir,
	}

	// 检测项目类型
	identity.ProjectType = d.detectProjectType(workDir)

	// 检测项目名称
	identity.ProjectName = filepath.Base(workDir)

	// 生成项目 ID
	identity.ProjectID = d.generateProjectID(workDir)

	// 检测用户信息
	identity.UserID = d.detectUserID()
	identity.UserName = d.detectUserName()

	return identity, nil
}

func (d *Detector) detectProjectType(workDir string) string {
	for _, a := range ProjectAnchors {
		if _, err := os.Stat(filepath.Join(workDir, a.Path)); err == nil {
			return a.Lang
		}
	}
	return "unknown"
}

func (d *Detector) generateProjectID(workDir string) string {
	// 使用路径哈希作为项目 ID
	return fmt.Sprintf("proj_%x", hashString(workDir))
}

func (d *Detector) detectUserID() string {
	// 使用系统用户名
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "anonymous"
}

func (d *Detector) detectUserName() string {
	return d.detectUserID()
}

func hashString(s string) uint32 {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}

// SessionManager 会话管理器
type SessionManager struct {
	dataDir string
}

// NewSessionManager 创建会话管理器
func NewSessionManager(dataDir string) *SessionManager {
	return &SessionManager{dataDir: dataDir}
}

// GetProjectDir 获取项目数据目录
func (sm *SessionManager) GetProjectDir(identity *Identity) string {
	return filepath.Join(sm.dataDir, "projects", identity.ProjectID)
}

// GetSessionDir 获取会话数据目录
func (sm *SessionManager) GetSessionDir(identity *Identity, sessionID string) string {
	return filepath.Join(sm.GetProjectDir(identity), "sessions", sessionID)
}

// EnsureDirs 确保目录存在
func (sm *SessionManager) EnsureDirs(identity *Identity) error {
	dirs := []string{
		sm.GetProjectDir(identity),
		filepath.Join(sm.GetProjectDir(identity), "sessions"),
		filepath.Join(sm.GetProjectDir(identity), "memory"),
		filepath.Join(sm.GetProjectDir(identity), "index"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// IsSameProject 检查是否为同一项目
func IsSameProject(a, b *Identity) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ProjectID == b.ProjectID
}

// GetProjectKey 获取项目键（用于缓存等）
func GetProjectKey(identity *Identity) string {
	if identity == nil {
		return "default"
	}
	return identity.ProjectID
}
