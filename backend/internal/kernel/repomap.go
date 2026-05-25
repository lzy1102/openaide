package kernel

import (
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
	// 关键字后紧跟的名称（函数/类/类型/变量/方法等）
	symbolRe = regexp.MustCompile(`(?m)(?:^|\s)(?:` +
		`func|type|const|var|class|def|struct|enum|interface|trait|impl|` +
		`void|int|long|float|double|bool|String|byte|char|` +
		`public|private|protected|export|default|async|static|` +
		`fn|let|mut|val|var|fun|object|data\s+class|sealed\s+class|` +
		`@interface|@implementation|@protocol` +
		`)\s+(?:\w+\s+)?(\w+)`)
	// 支持常见源码文件扩展名
	fileRe = regexp.MustCompile(`\.(go|tsx?|py|rs|jsx?|java|kt|kts|[ch](?:pp|\+\+)?|rb|swift|scala|lua|zig|dart|m|mm|r|R|exs?|hs|clj|erl|ex|cr|nim|v)$`)
)

// GenerateRepoMap 扫描项目生成符号地图（带缓存）
func GenerateRepoMap(root string) string {
	repoMapMu.Lock()
	defer repoMapMu.Unlock()

	if repoMapCache != "" && time.Since(repoMapCacheTime) < repoMapTTL {
		return repoMapCache
	}

	var sb strings.Builder
	sb.WriteString("[RepoMap] 项目符号地图:\n")

	fileCount := 0
	symbolCount := 0

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !fileRe.MatchString(info.Name()) {
			if info != nil && info.IsDir() && (strings.HasPrefix(info.Name(), ".") ||
				info.Name() == "vendor" || info.Name() == "node_modules" || info.Name() == "bin") {
				return filepath.SkipDir
			}
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil { return nil }
		if len(data) == 0 { return nil }

		relPath, _ := filepath.Rel(root, path)
		matches := symbolRe.FindAllStringSubmatch(string(data), -1)
		fileCount++

		if len(matches) == 0 {
			// 无符号文件仍列出（防止 LLM 幻觉不存在的文件）
			sb.WriteString(relPath + "\n")
			return nil
		}

		sb.WriteString(relPath + ": ")
		names := make([]string, 0, len(matches))
		seen := make(map[string]bool)
		for _, m := range matches {
			name := m[1]
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
		sb.WriteString(strings.Join(names, ", "))
		sb.WriteString("\n")
		symbolCount += len(names)
		return nil
	})

	if fileCount == 0 {
		return ""
	}

	sb.WriteString("\n---\n")
	repoMapCache = sb.String()
	repoMapCacheTime = time.Now()
	return repoMapCache
}
