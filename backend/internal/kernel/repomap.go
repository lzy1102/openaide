package kernel

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ── RepoMap: lightweight code symbol map (Aider-style) ─────

var (
	repoMapCache     string
	repoMapCacheTime time.Time
	repoMapMu        sync.Mutex
	repoMapTTL       = 5 * time.Minute
)

var (
	// 通用符号提取：匹配常见语言的声明关键字 + 紧跟的名称
	symbolRe = regexp.MustCompile(`(?m)(?:^|\s)(?:func|type|const|var|class|def|struct|enum|interface|trait|impl|fn|let|mut|val|fun|object|public|private|protected|export|default|async|static|void|int|long|float|double|bool|String|byte|char)\s+(?:\w+\s+)?(\w+)`)

	// 二进制/生成文件扩展名（跳过）
	binaryExt = map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true,
		".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".svg": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		".o": true, ".a": true, ".class": true, ".pyc": true, ".wasm": true,
		".lock": true, ".sum": true, ".mod": true, // go.sum, package-lock.json, go.mod
	}
)

// GenerateRepoMap 扫描项目生成符号地图（带缓存）
func GenerateRepoMap(root string) string {
	repoMapMu.Lock()
	defer repoMapMu.Unlock()

	if repoMapCache != "" && time.Since(repoMapCacheTime) < repoMapTTL {
		return repoMapCache
	}

	var sb strings.Builder
	sb.WriteString("[RepoMap] 项目文件地图:\n")

	fileCount := 0
	symbolCount := 0
	maxFiles := 200 // 防止超大项目 OOM

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && (strings.HasPrefix(info.Name(), ".") ||
				info.Name() == "vendor" || info.Name() == "node_modules" ||
				info.Name() == "bin" || info.Name() == "dist" || info.Name() == "build" ||
				info.Name() == "__pycache__" || info.Name() == "target") {
				return filepath.SkipDir
			}
			return nil
		}
		if fileCount >= maxFiles { return filepath.SkipDir }

		// 跳过二进制和生成文件
		if binaryExt[filepath.Ext(info.Name())] { return nil }
		// 跳过隐藏文件
		if strings.HasPrefix(info.Name(), ".") { return nil }
		// 跳过超大文件（>500KB）
		if info.Size() > 500*1024 { return nil }

		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 { return nil }

		// 跳过明显是二进制的内容
		if len(data) > 0 && data[0] == 0 { return nil }

		relPath, _ := filepath.Rel(root, path)
		fileCount++

		// 尝试提取符号
		matches := symbolRe.FindAllStringSubmatch(string(data), -1)
		if len(matches) == 0 {
			sb.WriteString(relPath + "\n")
			return nil
		}

		sb.WriteString(relPath + ": ")
		names := make([]string, 0, len(matches))
		seen := make(map[string]bool)
		for _, m := range matches {
			name := m[1]
			if !seen[name] && len(name) > 1 { // 跳过单字母和关键字
				seen[name] = true
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			sb.WriteString(relPath + "\n")
		} else {
			sb.WriteString(strings.Join(names, ", "))
			sb.WriteString("\n")
		}
		symbolCount += len(names)
		return nil
	})

	if fileCount == 0 { return "" }

	sb.WriteString(fmt.Sprintf("\n(%d files, %d symbols)\n", fileCount, symbolCount))
	repoMapCache = sb.String()
	repoMapCacheTime = time.Now()
	return repoMapCache
}
