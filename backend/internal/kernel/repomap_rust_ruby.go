package kernel

import (
	"regexp"
)

// ── Rust: regex parser ───────────────────────────────────────
type rustParser struct{}

func (rustParser) Extensions() []string { return []string{".rs"} }

var (
	rustFnRe     = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:async\s+)?(?:const\s+)?(?:unsafe\s+)?fn\s+(\w+)`)
	rustImplRe   = regexp.MustCompile(`(?m)^\s*impl(?:<[^>]+>)?\s+([\w<>]+)\s*(?:for\s+([\w<>]+))?\s*\{`)
	rustStructRe = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?struct\s+(\w+)`)
	rustEnumRe   = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?enum\s+(\w+)`)
	rustTraitRe  = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?trait\s+(\w+)`)
	rustModRe    = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?mod\s+(\w+)`)
	rustMacroRe  = regexp.MustCompile(`(?m)^\s*macro_rules!\s+(\w+)`)
	rustTypeRe   = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?type\s+(\w+)\s*=`)
	rustUseRe    = regexp.MustCompile(`(?m)^\s*use\s+([\w:]+)`)
)

func (rustParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	var symbols []astSymbol

	for _, m := range rustFnRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
	}
	for _, m := range rustImplRe.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if len(m) >= 3 && m[2] != "" {
			name = m[1] + " for " + m[2]
		}
		symbols = append(symbols, astSymbol{Name: name, Type: "impl", File: path})
	}
	for _, m := range rustStructRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "struct", File: path, Exports: []string{m[1]}})
	}
	for _, m := range rustEnumRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "enum", File: path, Exports: []string{m[1]}})
	}
	for _, m := range rustTraitRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "trait", File: path, Exports: []string{m[1]}})
	}
	for _, m := range rustModRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "module", File: path})
	}
	for _, m := range rustMacroRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "macro", File: path})
	}
	for _, m := range rustTypeRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "type", File: path})
	}
	for _, m := range rustUseRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "import", File: path})
	}
	return symbols
}

// ── Ruby: regex parser ───────────────────────────────────────
type rubyParser struct{}

func (rubyParser) Extensions() []string { return []string{".rb"} }

var (
	rubyClassRe  = regexp.MustCompile(`(?m)^\s*(?:module\s+)?class\s+(?:[\w:]+(?:\s*<\s*[\w:]+)?)`)
	rubyClassNm  = regexp.MustCompile(`(?m)^\s*(?:module\s+)?class\s+([\w:]+)`)
	rubyModRe    = regexp.MustCompile(`(?m)^\s*module\s+([\w:]+)`)
	rubyDefRe    = regexp.MustCompile(`(?m)^\s*(?:def)\s+(\w+)[?!]?(?:\s*[\w=]*)?`)
	rubyAttrRe   = regexp.MustCompile(`(?m)^\s*attr_(?:accessor|reader|writer)\s*[:\s]+(\w+)`)
	rubyRequireRe = regexp.MustCompile(`(?m)^\s*require(?:_relative)?\s+['"]([^'"]+)['"]`)
)

func (rubyParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	var symbols []astSymbol

	_ = rubyClassRe // 保留用于未来扩展
	for _, m := range rubyModRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "module", File: path})
	}
	for _, m := range rubyClassNm.FindAllStringSubmatch(text, -1) {
		// 同时匹配 class 和 module class,去重
		name := m[1]
		// 跳过已被 module 抓取的
		already := false
		for _, s := range symbols {
			if s.Name == name && s.Type == "module" {
				already = true
				break
			}
		}
		if !already {
			symbols = append(symbols, astSymbol{Name: name, Type: "class", File: path, Exports: []string{name}})
		}
	}
	for _, m := range rubyDefRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
	}
	for _, m := range rubyAttrRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "attr", File: path})
	}
	for _, m := range rubyRequireRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "import", File: path})
	}
	return symbols
}
