package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths 数据路径配置
type Paths struct {
	HomeDir     string // ~/.openaide
	DataDir     string // ~/.openaide/data
	DBDir       string // ~/.openaide/data/db
	VectorDir   string // ~/.openaide/data/vectors
	CacheDir    string // ~/.openaide/data/cache
	LedisDir    string // ~/.openaide/data/ledis
	ConfigFile  string // ~/.openaide/config.json
	LogDir      string // ~/.openaide/logs
}

// DefaultPaths 默认路径配置
var DefaultPaths *Paths

func init() {
	DefaultPaths = NewPaths("")
}

// NewPaths 创建路径配置
func NewPaths(homeDir string) *Paths {
	if homeDir == "" {
		homeDir = getHomeDir()
	}

	return &Paths{
		HomeDir:    homeDir,
		DataDir:    filepath.Join(homeDir, "data"),
		DBDir:      filepath.Join(homeDir, "data", "db"),
		VectorDir:  filepath.Join(homeDir, "data", "vectors"),
		CacheDir:   filepath.Join(homeDir, "data", "cache"),
		LedisDir:   filepath.Join(homeDir, "data", "ledis"),
		ConfigFile: filepath.Join(homeDir, "config.json"),
		LogDir:     filepath.Join(homeDir, "logs"),
	}
}

// getHomeDir 获取 openaide 主目录
func getHomeDir() string {
	// 1. 检查环境变量
	if env := os.Getenv("OPENAIDE_HOME"); env != "" {
		return env
	}

	// 2. 获取用户主目录
	userHome, err := os.UserHomeDir()
	if err != nil {
		// 回退到当前目录
		return ".openaide"
	}

	// 3. 使用 ~/.openaide
	return filepath.Join(userHome, ".openaide")
}

// EnsureDirs 确保所有目录存在
func (p *Paths) EnsureDirs() error {
	dirs := []string{
		p.HomeDir,
		p.DataDir,
		p.DBDir,
		p.VectorDir,
		p.CacheDir,
		p.LedisDir,
		p.LogDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// GetDBPath 获取数据库文件路径
func (p *Paths) GetDBPath(name string) string {
	return filepath.Join(p.DBDir, name+".db")
}

// GetVectorCollectionPath 获取向量集合路径
func (p *Paths) GetVectorCollectionPath(collection string) string {
	return filepath.Join(p.VectorDir, collection)
}

// GetLedisDataDir 获取 Ledis 数据目录
func (p *Paths) GetLedisDataDir() string {
	return p.LedisDir
}

// GetCacheDir 获取缓存目录
func (p *Paths) GetCacheDir() string {
	return p.CacheDir
}

// Platform 获取当前平台
func Platform() string {
	return runtime.GOOS
}
