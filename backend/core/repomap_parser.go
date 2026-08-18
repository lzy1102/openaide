package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SymbolParser 提取某种编程语言的符号清单。
// 实现需要纯 Go(无 CGO 依赖),支持按扩展名匹配。
type SymbolParser interface {
	// Extensions 返回该 parser 处理的文件扩展名(含点,如 ".go"、".py")。
	Extensions() []string
	// Parse 解析单个文件,返回提取到的符号。
	// path 为相对路径(用于符号显示),content 为文件字节内容。
	Parse(path string, content []byte) []astSymbol
}

// ── Parser 注册表 ─────────────────────────────────────────────

var (
	parserRegistry     = map[string]SymbolParser{}
	parserRegistryOnce sync.Once
)

// RegisterParser 注册一个 SymbolParser。重复注册时后注册的覆盖前者。
// 通常在 init() 中调用。
func RegisterParser(p SymbolParser) {
	for _, ext := range p.Extensions() {
		parserRegistry[ext] = p
	}
}

// lookupParser 按扩展名查找 parser,未注册返回 nil。
func lookupParser(ext string) SymbolParser {
	return parserRegistry[ext]
}

// initDefaultParsers 注册内置 parser。只执行一次。
func initDefaultParsers() {
	parserRegistryOnce.Do(func() {
		RegisterParser(&goParser{})
		RegisterParser(&pythonParser{})
		RegisterParser(&jsParser{})
		RegisterParser(&rustParser{})
		RegisterParser(&javaParser{})
		RegisterParser(&rubyParser{})
		RegisterParser(&cParser{})
		RegisterParser(&phpParser{})
		RegisterParser(&swiftParser{})
		RegisterParser(&csharpParser{})
		RegisterParser(&goModParser{}) // go.mod 提取模块名
	})
}

// ── 通用扫描与缓存 ────────────────────────────────────────────
//
// 只保留 root-level TTL 缓存(5 分钟),不做 file-level hash 缓存。
// 理由:file-level hash 命中时若不返回符号会丢失数据,若返回则需要额外
// 维护符号缓存,复杂度上升而收益有限。root-level TTL 已能覆盖连续对话场景。

type repomapEntry struct {
	symbols   []astSymbol
	updatedAt time.Time
}

var (
	repomapCache    = map[string]*repomapEntry{} // key = root
	repomapCacheMu  sync.RWMutex
	repomapCacheTTL = 5 * time.Minute
)

// shouldSkipDir 判断目录是否应跳过(vendor/node_modules 等)。
func shouldSkipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "vendor", "node_modules", "bin", "dist", "build",
		"__pycache__", "target", ".git", ".svn", ".hg",
		"out", "Debug", "Release", "obj":
		return true
	}
	return false
}

// isBinaryExt 判断是否二进制/生成文件扩展名(应跳过)。
// 注意:.mod 不在此列,因为 go.mod 由 goModParser 处理。
var binaryExtMap = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".svg": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
	".o": true, ".a": true, ".class": true, ".pyc": true, ".wasm": true,
	".lock": true, ".sum": true,
}

// scanRepoWithParsers 用 parser registry 扫描整个项目,返回符号列表。
// maxFiles 限制扫描文件数,<= 0 表示不限制。
func scanRepoWithParsers(root string, maxFiles int) []astSymbol {
	initDefaultParsers()

	var symbols []astSymbol
	fileCount := 0

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		// 隐藏文件跳过
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		ext := filepath.Ext(path)
		if binaryExtMap[ext] {
			return nil
		}
		parser := lookupParser(ext)
		if parser == nil {
			return nil
		}
		if maxFiles > 0 && fileCount >= maxFiles {
			return filepath.SkipDir
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > 500*1024 {
			return nil // 跳过 >500KB 文件
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			return nil
		}
		// 跳过明显二进制(NUL 字节)
		if data[0] == 0 {
			return nil
		}

		relPath, _ := filepath.Rel(root, path)
		// 用 forward slash 风格显示路径
		relPath = filepath.ToSlash(relPath)

		syms := parser.Parse(relPath, data)
		symbols = append(symbols, syms...)
		fileCount++
		return nil
	})

	return symbols
}
