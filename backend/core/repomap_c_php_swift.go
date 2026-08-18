package kernel

import (
	"regexp"
	"strings"
)

// ── C / C++: regex parser ────────────────────────────────────
//
// 提取函数声明、class、struct、namespace、enum、#include。
// C++ 模板和重载会带来噪音,这里只取顶层(列首)声明。
type cParser struct{}

func (cParser) Extensions() []string {
	return []string{".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hxx"}
}

var (
	cFuncRe = regexp.MustCompile(`(?m)^\s*(?:static\s+|inline\s+|extern\s+|virtual\s+|explicit\s+)*[\w:*&<>\[\],\s]+\s+(\w+)\s*\([^);]*\)\s*(?:const\s*)?(?:noexcept\s*)?(?:override\s*)?(?:final\s*)?\s*\{`)
	// class/struct 不要求行首:C++ 常见 `namespace ns { class Foo { ... }; }` 内联写法
	cClassRe = regexp.MustCompile(`(?:class|struct)\s+(\w+)\s*(?::\s*[^{]+)?\s*\{`)
	cNsRe    = regexp.MustCompile(`(?m)^\s*namespace\s+(\w+)\s*\{`)
	cEnumRe  = regexp.MustCompile(`(?m)^\s*enum\s+(?:class\s+)?(\w+)`)
	// typedef:用非贪婪 .+? 匹配类型部分,避免贪婪吃掉别名标识符
	cTypedefRe = regexp.MustCompile(`(?m)^\s*typedef\s+.+?\s+(\w+)\s*;`)
	cUsingRe   = regexp.MustCompile(`(?m)^\s*using\s+(\w+)\s*=`)
	cIncludeRe = regexp.MustCompile(`(?m)^\s*#include\s+[<"]([^>"]+)[>"]`)
	cDefineRe  = regexp.MustCompile(`(?m)^\s*#define\s+(\w+)`)
)

func (cParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	var symbols []astSymbol

	for _, m := range cFuncRe.FindAllStringSubmatch(text, -1) {
		switch m[1] {
		case "if", "for", "while", "switch", "catch", "return":
			continue
		}
		symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
	}
	for _, m := range cClassRe.FindAllStringSubmatch(text, -1) {
		symType := "struct"
		if strings.Contains(m[0], "class") {
			symType = "class"
		}
		symbols = append(symbols, astSymbol{Name: m[1], Type: symType, File: path, Exports: []string{m[1]}})
	}
	for _, m := range cNsRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "namespace", File: path})
	}
	for _, m := range cEnumRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "enum", File: path})
	}
	for _, m := range cTypedefRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "type", File: path})
	}
	for _, m := range cUsingRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "type", File: path})
	}
	for _, m := range cIncludeRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "import", File: path})
	}
	for _, m := range cDefineRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "const", File: path})
	}
	return symbols
}

// ── PHP: regex parser ────────────────────────────────────────
type phpParser struct{}

func (phpParser) Extensions() []string { return []string{".php"} }

var (
	phpClassRe = regexp.MustCompile(`(?m)^\s*(?:abstract\s+|final\s+)*(?:class|interface|trait)\s+(\w+)`)
	phpFuncRe  = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|static|abstract|final|\s)*function\s+(\w+)\s*\(`)
	phpNsRe    = regexp.MustCompile(`(?m)^\s*namespace\s+([\w\\]+);`)
	phpUseRe   = regexp.MustCompile(`(?m)^\s*use\s+([\w\\]+)(?:\s+as\s+\w+)?;`)
	phpConstRe = regexp.MustCompile(`(?m)^\s*const\s+(\w+)\s*=`)
)

func (phpParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	var symbols []astSymbol

	for _, m := range phpClassRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "class", File: path, Exports: []string{m[1]}})
	}
	for _, m := range phpFuncRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
	}
	for _, m := range phpNsRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "namespace", File: path})
	}
	for _, m := range phpUseRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "import", File: path})
	}
	for _, m := range phpConstRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "const", File: path})
	}
	return symbols
}

// ── Swift: regex parser ──────────────────────────────────────
type swiftParser struct{}

func (swiftParser) Extensions() []string { return []string{".swift"} }

var (
	swiftClassRe  = regexp.MustCompile(`(?m)^\s*(?:public|private|internal|fileprivate|open|final|abstract|\s)*(?:class|struct|enum|protocol|actor)\s+(\w+)`)
	swiftFuncRe   = regexp.MustCompile(`(?m)^\s*(?:public|private|internal|fileprivate|open|final|static|class|override|mutating|throws|async|\s)*func\s+(\w+)\s*\(`)
	swiftVarRe    = regexp.MustCompile(`(?m)^\s*(?:public|private|internal|fileprivate|open|final|static|let|var|\s)+(?:let|var)\s+(\w+)\s*:`)
	swiftImportRe = regexp.MustCompile(`(?m)^\s*import\s+([\w.]+)`)
	swiftExtRe    = regexp.MustCompile(`(?m)^\s*extension\s+([\w.]+)`)
	swiftTypeRe   = regexp.MustCompile(`(?m)^\s*typealias\s+(\w+)\s*=`)
)

func (swiftParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	var symbols []astSymbol

	for _, m := range swiftClassRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "class", File: path, Exports: []string{m[1]}})
	}
	for _, m := range swiftFuncRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
	}
	for _, m := range swiftVarRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "var", File: path})
	}
	for _, m := range swiftImportRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "import", File: path})
	}
	for _, m := range swiftExtRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "extension", File: path})
	}
	for _, m := range swiftTypeRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "type", File: path})
	}
	return symbols
}
