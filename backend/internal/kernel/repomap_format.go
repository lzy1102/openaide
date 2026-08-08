package kernel

import (
	"fmt"
	"sort"
	"strings"
)

// formatRepoMap formats symbols as a compact markdown repomap.
// 按文件分组,每个文件显示类型计数 + 最多 15 个关键符号。
func formatRepoMap(symbols []astSymbol) string {
	return formatRepoMapFiltered(symbols, nil)
}

// repomapMaxFiles 查询感知模式下注入的文件数上限(大仓库截断,节省 token)。
const repomapMaxFiles = 60

// formatRepoMapScored 生成查询感知的符号地图:文件按与查询关键词的匹配度
// 打分排序(文件名/符号名/导出名命中),只保留得分>0 或 top-N 的文件。
// query 为空时退化为全量 formatRepoMap。
func formatRepoMapScored(symbols []astSymbol, query string) string {
	if query == "" {
		return formatRepoMap(symbols)
	}
	keywords := repoMapKeywords(query)
	if len(keywords) == 0 {
		return formatRepoMap(symbols)
	}
	return formatRepoMapFiltered(symbols, func(file string, syms []astSymbol) int {
		score := 0
		for _, s := range syms {
			if matchesAny(s.Name, keywords) {
				score += 2
			}
			for _, e := range s.Exports {
				if matchesAny(e, keywords) {
					score++
				}
			}
		}
		if matchesAny(file, keywords) {
			score += 3
		}
		return score
	})
}

// repoMapKeywords 从查询中提取匹配用关键词(≥3 字符的小写词)。
func repoMapKeywords(query string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 0x4e00 && r <= 0x9fff)
	}) {
		if len(w) >= 3 {
			out = append(out, w)
		}
	}
	return out
}

// matchesAny 报告 name 是否包含任一关键词(小写不敏感)。
func matchesAny(name string, keywords []string) bool {
	lname := strings.ToLower(name)
	for _, k := range keywords {
		if strings.Contains(lname, k) {
			return true
		}
	}
	return false
}

// formatRepoMapFiltered 是 repomap 格式化的核心:
// score 非 nil 时按分数排序文件并截断到 repomapMaxFiles;nil 时按路径排序全量输出。
func formatRepoMapFiltered(symbols []astSymbol, score func(file string, syms []astSymbol) int) string {
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
	if score != nil {
		// 查询感知:按匹配度降序,同分按路径
		type scoredFile struct {
			name  string
			score int
		}
		scored := make([]scoredFile, 0, len(files))
		for _, f := range files {
			scored = append(scored, scoredFile{f, score(f, fileSymbols[f])})
		}
		sort.Slice(scored, func(i, j int) bool {
			if scored[i].score != scored[j].score {
				return scored[i].score > scored[j].score
			}
			return scored[i].name < scored[j].name
		})
		files = files[:0]
		for _, sf := range scored {
			files = append(files, sf.name)
		}
		if len(files) > repomapMaxFiles {
			files = files[:repomapMaxFiles]
		}
	} else {
		sort.Strings(files)
	}

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
