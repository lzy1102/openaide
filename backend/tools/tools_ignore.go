package tools

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

var ignorePatterns []string

// LoadIgnorePatterns 加载 .openaideignore 文件（gitignore 格式）
func LoadIgnorePatterns(root string) {
	ignorePatterns = nil
	f, err := os.Open(filepath.Join(root, ".openaideignore"))
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ignorePatterns = append(ignorePatterns, line)
	}
}

// isIgnored 检查路径是否匹配忽略规则
func isIgnored(path string) bool {
	for _, pattern := range ignorePatterns {
		// 支持 ** 匹配任意层级
		if strings.Contains(pattern, "**") {
			parts := strings.SplitN(pattern, "**", 2)
			if strings.HasPrefix(path, parts[0]) && strings.HasSuffix(path, parts[1]) {
				return true
			}
		}
		// 标准 glob 匹配
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
		// 前缀匹配（目录）
		if strings.HasPrefix(path, strings.TrimSuffix(pattern, "/")) {
			return true
		}
	}
	return false
}
