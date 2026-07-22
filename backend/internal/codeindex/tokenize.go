package codeindex

import (
	"strings"
	"unicode"
)

// tokenize 是 codeindex 包内的简化分词器(中英文混合)。
// 与 memory/vector.go 的 tokenize 行为一致,但保持独立以避免循环依赖。
func tokenize(text string) []string {
	var tokens []string
	var current []rune

	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Han, r) {
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = nil
			}
			tokens = append(tokens, string(r))
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else {
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = nil
			}
		}
	}
	if len(current) > 0 {
		tokens = append(tokens, string(current))
	}

	// 过滤 1 字母的英文 token(信息量低)
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		// 保留中文单字(1 个 rune),英文至少 2 字符
		if len([]rune(t)) == 1 && unicode.Is(unicode.Latin, []rune(t)[0]) {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}
