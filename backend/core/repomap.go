package kernel

import (
	"strings"
	"time"
)

// ── GenerateRepoMap: 项目符号地图入口 ────────────────────────────
//
// 使用 parser registry 扫描整个项目,产出符号清单。
// 缓存策略:root-level TTL 5 分钟,缓存命中直接返回。
// 文件上限:2000(超大项目截断)。

const repoMapMaxFiles = 2000

// GenerateRepoMap 扫描项目生成符号地图(带缓存)。
// 返回 markdown 格式字符串,可直接注入 system prompt。
func GenerateRepoMap(root string) string {
	return generateRepoMap(root, "")
}

// GenerateRepoMapForQuery 生成查询感知的符号地图:文件按与查询关键词的
// 匹配度排序,只注入 top-N 相关文件(大仓库节省 token)。
// 与 GenerateRepoMap 共享同一扫描缓存,仅格式化阶段不同。
func GenerateRepoMapForQuery(root, query string) string {
	return generateRepoMap(root, query)
}

func generateRepoMap(root, query string) string {
	// 检查 root-level 缓存
	repomapCacheMu.RLock()
	if entry, ok := repomapCache[root]; ok && time.Since(entry.updatedAt) < repomapCacheTTL {
		repomapCacheMu.RUnlock()
		if query != "" {
			return formatRepoMapScored(entry.symbols, query)
		}
		return formatRepoMap(entry.symbols)
	}
	repomapCacheMu.RUnlock()

	// 扫描
	symbols := scanRepoWithParsers(root, repoMapMaxFiles)

	// 写入缓存
	repomapCacheMu.Lock()
	repomapCache[root] = &repomapEntry{
		symbols:   symbols,
		updatedAt: time.Now(),
	}
	repomapCacheMu.Unlock()

	if query != "" {
		return formatRepoMapScored(symbols, query)
	}
	return formatRepoMap(symbols)
}

// InvalidateRepoMapCache 清除指定 root 的 repomap 缓存。
// 文件变更后调用以强制重新扫描。
func InvalidateRepoMapCache(root string) {
	repomapCacheMu.Lock()
	delete(repomapCache, root)
	repomapCacheMu.Unlock()
}

// InvalidateAllRepoMapCache 清除所有 repomap 缓存。
func InvalidateAllRepoMapCache() {
	repomapCacheMu.Lock()
	repomapCache = make(map[string]*repomapEntry)
	repomapCacheMu.Unlock()
}

// CountRepoMapSymbols 统计符号总数(用于诊断)
func CountRepoMapSymbols(root string) (files, symbols int) {
	repomapCacheMu.RLock()
	defer repomapCacheMu.RUnlock()
	if entry, ok := repomapCache[root]; ok {
		fileSet := make(map[string]bool)
		for _, s := range entry.symbols {
			fileSet[s.File] = true
		}
		return len(fileSet), len(entry.symbols)
	}
	return 0, 0
}

// RepoMapFilesByExt 统计各扩展名文件数(用于诊断)
func RepoMapFilesByExt(root string) map[string]int {
	repomapCacheMu.RLock()
	defer repomapCacheMu.RUnlock()
	if entry, ok := repomapCache[root]; ok {
		result := make(map[string]int)
		for _, s := range entry.symbols {
			if idx := strings.LastIndex(s.File, "."); idx >= 0 {
				result[s.File[idx:]]++
			}
		}
		return result
	}
	return nil
}
