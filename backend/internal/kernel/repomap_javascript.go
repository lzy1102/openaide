package kernel

import (
	"regexp"
	"strings"
)

// ── JavaScript / TypeScript: 增强 regex parser ───────────────
//
// 支持:export / async / arrow function / class / interface / type /
// enum / namespace / import / decorator-less method
type jsParser struct{}

func (jsParser) Extensions() []string {
	return []string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"}
}

var (
	jsClassRe = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+(\w+)`)
	jsFuncRe  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+(\w+)\s*\(`)
	jsArrowRe = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+(\w+)\s*(?::\s*[^=]+)?=\s*(?:async\s+)?(?:\([^)]*\)|\w+)\s*=>`)
	jsMethRe  = regexp.MustCompile(`(?m)^(\s+)(?:async\s+)?(\w+)\s*\([^)]*\)\s*(?::\s*[^{]+)?\{`)
	jsImpRe   = regexp.MustCompile(`(?m)^\s*import\s+.*?(?:from\s+)?['"]([^'"]+)['"]`)
	// TypeScript 特有
	tsIfaceRe = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:abstract\s+)?interface\s+(\w+)`)
	tsTypeRe  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?type\s+(\w+)\s*=?`)
	tsEnumRe  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const\s+)?enum\s+(\w+)`)
	tsNsRe    = regexp.MustCompile(`(?m)^\s*(?:export\s+)?namespace\s+(\w+)\s*\{`)
)

func (jsParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	isTS := strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")
	var symbols []astSymbol

	for _, m := range jsClassRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{
			Name: m[1], Type: "class", File: path, Exports: []string{m[1]},
		})
	}
	for _, m := range jsFuncRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
	}
	for _, m := range jsArrowRe.FindAllStringSubmatch(text, -1) {
		// 过滤控制流关键字
		switch m[1] {
		case "if", "for", "while", "switch", "catch":
			continue
		}
		symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
	}
	for _, m := range jsMethRe.FindAllStringSubmatch(text, -1) {
		switch m[2] {
		case "if", "for", "while", "switch", "catch", "constructor":
			continue
		}
		symbols = append(symbols, astSymbol{Name: m[2], Type: "method", File: path})
	}
	for _, m := range jsImpRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "import", File: path})
	}

	if isTS {
		for _, m := range tsIfaceRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "interface", File: path})
		}
		for _, m := range tsTypeRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "type", File: path})
		}
		for _, m := range tsEnumRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "enum", File: path})
		}
		for _, m := range tsNsRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "namespace", File: path})
		}
	}

	return symbols
}
