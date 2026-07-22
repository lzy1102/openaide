package codeindex

import (
	"strings"
)

// Chunker 把文件内容切成可被检索的小块。
//
// 切块策略:
//  1. 优先按"顶层声明边界"切——每个 func/class/method 是独立 chunk
//  2. 没有声明结构的文件(配置、脚本)按 ChunkSize 字符数切
//  3. 超长声明(>ChunkSize)二次切分,保留 ChunkOverlap 行重叠
//  4. 文件级 summary(文件头 N 行 + 顶层符号清单)作为一个额外 chunk
type Chunker struct {
	cfg Config
}

func NewChunker(cfg Config) *Chunker {
	return &Chunker{cfg: cfg}
}

// Chunk 切分文件,返回 chunk 列表(不含 embedding)。
// path 为相对路径,用于 chunk ID 和显示。
func (ch *Chunker) Chunk(path string, content []byte) []Chunk {
	if len(content) == 0 {
		return nil
	}
	text := string(content)
	lines := strings.Split(text, "\n")

	// 第一步:识别顶层声明边界
	boundaries := ch.detectBoundaries(text, lines)

	var chunks []Chunk

	// 文件级 summary chunk
	if summary := ch.fileSummary(path, lines); summary != nil {
		chunks = append(chunks, *summary)
	}

	// 按声明边界切块
	if len(boundaries) > 0 {
		for i, b := range boundaries {
			start := b.startLine
			end := b.endLine
			if i == len(boundaries)-1 && end < len(lines) {
				end = len(lines)
			} else if i < len(boundaries)-1 {
				end = boundaries[i+1].startLine - 1
			}
			if end > len(lines) {
				end = len(lines)
			}
			if end < start {
				continue
			}
			chunkText := strings.Join(lines[start-1:end], "\n")
			// 超长 chunk 二次切分
			if len(chunkText) > ch.cfg.ChunkSize {
				subs := ch.splitLongChunk(path, start, chunkText, b.symbol)
				chunks = append(chunks, subs...)
			} else {
				chunks = append(chunks, Chunk{
					Path:      path,
					StartLine: start,
					EndLine:   end,
					Content:   chunkText,
					Symbol:    b.symbol,
				})
			}
		}
	} else {
		// 无声明结构:按字符数切
		chunks = append(chunks, ch.sliceByChars(path, text)...)
	}

	// 上限保护
	if len(chunks) > ch.cfg.MaxChunks {
		chunks = chunks[:ch.cfg.MaxChunks]
	}

	return chunks
}

// boundary 是顶层声明边界
type boundary struct {
	startLine int    // 1-based
	endLine   int    // 1-based,0 表示"到下一个边界"
	symbol    string // 符号名
}

// detectBoundaries 用行首关键字检测顶层声明。
// 这是简化版的"AST":按行匹配声明关键字,以行为单位切。
func (ch *Chunker) detectBoundaries(text string, lines []string) []boundary {
	// 通用声明模式:行首(允许缩进 0-4 空格)出现关键字
	// 注意:Go/Java/C 用 {,Python 用 :,Ruby 用 end/Lustre 等
	patterns := []string{
		"func ", "func\t", // Go
		"type ", "type\t",
		"package ",
		"class ", "interface ", "enum ", "struct ", "record ",
		"def ", "def\t", "async def ", // Python
		"function ", "function\t", // JS
		"export function", "export default function",
		"export class", "export default class",
		"export const", "export let", "export var",
		"pub fn ", "pub struct ", "pub enum ", "pub trait ", "pub mod ", // Rust
		"fn ", "struct ", "enum ", "trait ", "mod ", "impl ",
		"public class", "public final class", "public abstract class",
		"public interface", "public enum",
		"private class", "protected class",
		"class ", "interface ", "enum ", // Java/C#
		"namespace ",
		"module ", "defmodule ", // Elixir
		"object ", "case object", "case class", // Scala
	}

	var boundaries []boundary
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		// 只识别顶层(缩进 <= 4 空格)
		if indent := len(line) - len(trimmed); indent > 4 {
			continue
		}
		for _, p := range patterns {
			if strings.HasPrefix(trimmed, p) {
				sym := extractSymbolName(trimmed, p)
				boundaries = append(boundaries, boundary{
					startLine: i + 1,
					endLine:   0,
					symbol:    sym,
				})
				break
			}
		}
	}

	// 填充 endLine:每个边界的 end 是下一个边界的前一行
	for i := range boundaries {
		if i+1 < len(boundaries) {
			boundaries[i].endLine = boundaries[i+1].startLine - 1
		} else {
			boundaries[i].endLine = len(lines)
		}
	}

	return boundaries
}

// extractSymbolName 从声明行提取符号名。
// 简化:取关键字后第一个 word。
func extractSymbolName(line, prefix string) string {
	rest := strings.TrimPrefix(line, prefix)
	rest = strings.TrimLeft(rest, " \t")
	// 取第一个 word(字母/数字/下划线)
	var name []rune
	for _, r := range rest {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' {
			name = append(name, r)
		} else {
			break
		}
	}
	if len(name) == 0 {
		return ""
	}
	// 加上声明类型前缀
	return strings.TrimSpace(prefix) + " " + string(name)
}

// fileSummary 生成文件级 summary chunk(头 20 行 + 顶层符号清单)。
// 受 ChunkSize 约束:超过时按字符数截断,避免 summary 自身超过 chunk 上限。
func (ch *Chunker) fileSummary(path string, lines []string) *Chunk {
	head := 20
	if len(lines) < head {
		head = len(lines)
	}
	var sb strings.Builder
	sb.WriteString("// File: " + path + "\n")
	sb.WriteString("// Lines: " + itoa(len(lines)) + "\n\n")
	// 按字符数添加头部行,不超过 ChunkSize
	charCount := sb.Len()
	added := 0
	for i := 0; i < head; i++ {
		lineLen := len(lines[i]) + 1 // +1 for newline
		if charCount+lineLen > ch.cfg.ChunkSize && added > 0 {
			break
		}
		sb.WriteString(lines[i])
		sb.WriteString("\n")
		charCount += lineLen
		added++
	}
	return &Chunk{
		Path:      path,
		StartLine: 1,
		EndLine:   added,
		Content:   sb.String(),
		Symbol:    "file_summary",
	}
}

// splitLongChunk 把超长 chunk 按 ChunkSize 字符切分,保留 ChunkOverlap 行重叠。
func (ch *Chunker) splitLongChunk(path string, startLine int, text string, symbol string) []Chunk {
	lines := strings.Split(text, "\n")
	var chunks []Chunk

	i := 0
	for i < len(lines) {
		var sub []string
		charCount := 0
		j := i
		for j < len(lines) {
			line := lines[j]
			if charCount+len(line)+1 > ch.cfg.ChunkSize && j > i {
				break
			}
			sub = append(sub, line)
			charCount += len(line) + 1
			j++
		}
		if len(sub) == 0 {
			break
		}
		chunks = append(chunks, Chunk{
			Path:      path,
			StartLine: startLine + i,
			EndLine:   startLine + j - 1,
			Content:   strings.Join(sub, "\n"),
			Symbol:    symbol,
		})
		// 重叠
		next := j - ch.cfg.ChunkOverlap
		if next <= i {
			next = i + 1
		}
		i = next
	}
	return chunks
}

// sliceByChars 无声明结构的文件按字符数切。
func (ch *Chunker) sliceByChars(path, text string) []Chunk {
	lines := strings.Split(text, "\n")
	var chunks []Chunk
	i := 0
	for i < len(lines) {
		var sub []string
		charCount := 0
		j := i
		for j < len(lines) {
			line := lines[j]
			if charCount+len(line)+1 > ch.cfg.ChunkSize && j > i {
				break
			}
			sub = append(sub, line)
			charCount += len(line) + 1
			j++
		}
		if len(sub) == 0 {
			break
		}
		chunks = append(chunks, Chunk{
			Path:      path,
			StartLine: i + 1,
			EndLine:   j,
			Content:   strings.Join(sub, "\n"),
			Symbol:    "",
		})
		if j == i {
			j = i + 1
		}
		i = j
	}
	return chunks
}

// itoa 是简化的 int→string,避免引入 strconv(保持文件简洁)
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
