package kernel

import "context"

// CodeIndexer 是代码索引接口。
//
// 由 codeindex.Indexer 隐式实现(避免 kernel→codeindex 循环依赖)。
// 在 promptL3 阶段注入与查询相关的代码块,使 LLM 在编码任务中获得
// 语义相关上下文(类似 Cursor @codebase 的隐式版本)。
type CodeIndexer interface {
	// Search 返回与 query 语义最相关的 top-K 代码 chunk。
	Search(ctx context.Context, query string, limit int) ([]CodeChunk, error)
}

// CodeChunk 是 CodeIndexer 返回的代码块。
// codeindex 包直接复用此类型(避免 duck typing 类型名不匹配)。
type CodeChunk struct {
	ID        string  `json:"id"`
	Path      string  `json:"path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Content   string  `json:"content"`
	Symbol    string  `json:"symbol"`
	Score     float64 `json:"score"`
}
