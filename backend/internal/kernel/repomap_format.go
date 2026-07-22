package kernel

import (
	"fmt"
	"sort"
	"strings"
)

// formatRepoMap formats symbols as a compact markdown repomap.
// 按文件分组,每个文件显示类型计数 + 最多 15 个关键符号。
func formatRepoMap(symbols []astSymbol) string {
	if len(symbols) == 0 {
		return ""
	}

	// 按文件分组
	fileSymbols := make(map[string][]astSymbol)
	for _, s := range symbols {
		fileSymbols[s.File] = append(fileSymbols[s.File], s)
	}

	// 按路径排序文件
	files := make([]string, 0, len(fileSymbols))
	for f := range fileSymbols {
		files = append(files, f)
	}
	sort.Strings(files)

	var sb strings.Builder
	sb.WriteString("## RepoMap\n")
	totalSyms := 0
	for _, file := range files {
		syms := fileSymbols[file]
		sb.WriteString(fmt.Sprintf("### %s\n", file))
		// 类型计数
		typeCount := map[string]int{}
		for _, s := range syms {
			typeCount[s.Type]++
		}
		// 按类型名排序输出
		types := make([]string, 0, len(typeCount))
		for t := range typeCount {
			types = append(types, t)
		}
		sort.Strings(types)
		for _, t := range types {
			sb.WriteString(fmt.Sprintf("  %s×%d ", t, typeCount[t]))
		}
		sb.WriteString("\n")
		// 列出关键符号(每文件最多 15 个)
		n := 0
		for _, s := range syms {
			if n >= 15 {
				sb.WriteString("  ...\n")
				break
			}
			marker := ""
			if len(s.Exports) > 0 {
				marker = " (exported)"
			}
			sb.WriteString(fmt.Sprintf("  - %s %s%s\n", s.Type, s.Name, marker))
			n++
			totalSyms++
		}
	}
	sb.WriteString(fmt.Sprintf("\n(%d files, %d symbols)\n", len(files), totalSyms))
	return sb.String()
}
