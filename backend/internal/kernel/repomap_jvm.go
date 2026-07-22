package kernel

import (
	"regexp"
)

// ── Java / Kotlin: regex parser ──────────────────────────────
//
// 共享一个 parser,因为语法关键字相似(package/import/class/interface/enum)。
// Kotlin 特有的 fun/object 也覆盖。
type javaParser struct{}

func (javaParser) Extensions() []string {
	return []string{".java", ".kt", ".kts", ".scala"}
}

var (
	javaClassRe = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|abstract|final|static|\s)*(?:class|interface|enum|record|@interface)\s+(\w+)`)
	javaMethRe  = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|abstract|final|static|synchronized|native|default|\s)+[\w<>\[\],?\s]+\s+(\w+)\s*\([^)]*\)\s*(?:throws[\s\w,]+)?\s*\{?`)
	javaPkgRe   = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+);`)
	javaImpRe   = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([\w.*]+);`)
	// Kotlin 特有
	// fun 不要求行首:Kotlin 常见 `object X { fun y() {} }` 内联写法
	kotlinFunRe   = regexp.MustCompile(`\bfun\s+(\w+)\s*\(`)
	kotlinObjRe   = regexp.MustCompile(`(?m)^\s*(?:private|public|protected|internal|abstract|open|\s)*object\s+(\w+)`)
	// Scala 特有
	scalaObjRe    = regexp.MustCompile(`(?m)^\s*(?:private|protected|abstract|final|override|implicit|\s)*(?:case\s+)?object\s+(\w+)`)
	scalaTraitRe  = regexp.MustCompile(`(?m)^\s*(?:private|protected|abstract|\s)*trait\s+(\w+)`)
	scalaDefRe    = regexp.MustCompile(`(?m)^\s*(?:private|protected|abstract|final|override|implicit|\s)*def\s+(\w+)`)
)

func (javaParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	var symbols []astSymbol
	isKotlin := hasSuffix(path, ".kt") || hasSuffix(path, ".kts")
	isScala := hasSuffix(path, ".scala")

	for _, m := range javaPkgRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "package", File: path})
	}
	for _, m := range javaImpRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "import", File: path})
	}
	for _, m := range javaClassRe.FindAllStringSubmatch(text, -1) {
		symType := "class"
		if matched := regexp.MustCompile(`\binterface\b`).FindString(text); matched != "" {
			// 上面正则抓到的可能是 interface,这里粗略判断
		}
		symbols = append(symbols, astSymbol{Name: m[1], Type: symType, File: path, Exports: []string{m[1]}})
	}
	for _, m := range javaMethRe.FindAllStringSubmatch(text, -1) {
		// 过滤关键字
		switch m[1] {
		case "if", "for", "while", "switch", "catch", "return", "new":
			continue
		}
		symbols = append(symbols, astSymbol{Name: m[1], Type: "method", File: path})
	}

	if isKotlin {
		for _, m := range kotlinFunRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
		}
		for _, m := range kotlinObjRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "object", File: path, Exports: []string{m[1]}})
		}
	}

	if isScala {
		for _, m := range scalaObjRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "object", File: path, Exports: []string{m[1]}})
		}
		for _, m := range scalaTraitRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "trait", File: path, Exports: []string{m[1]}})
		}
		for _, m := range scalaDefRe.FindAllStringSubmatch(text, -1) {
			symbols = append(symbols, astSymbol{Name: m[1], Type: "func", File: path})
		}
	}

	return symbols
}

// ── C# (与 Java 类似,但扩展名不同) ──────────────────────────
type csharpParser struct{}

func (csharpParser) Extensions() []string { return []string{".cs"} }

var (
	csharpNsRe    = regexp.MustCompile(`(?m)^\s*namespace\s+([\w.]+)`)
	csharpClassRe = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal|abstract|sealed|static|partial|\s)*(?:class|interface|enum|struct|record)\s+(\w+)`)
	csharpMethRe  = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal|abstract|sealed|static|virtual|override|async|\s)+[\w<>\[\],?\s]+\s+(\w+)\s*\([^)]*\)\s*\{?`)
	csharpUsingRe = regexp.MustCompile(`(?m)^\s*using\s+([\w.=]+);`)
)

func (csharpParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	var symbols []astSymbol

	for _, m := range csharpNsRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "namespace", File: path})
	}
	for _, m := range csharpUsingRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "import", File: path})
	}
	for _, m := range csharpClassRe.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, astSymbol{Name: m[1], Type: "class", File: path, Exports: []string{m[1]}})
	}
	for _, m := range csharpMethRe.FindAllStringSubmatch(text, -1) {
		switch m[1] {
		case "if", "for", "while", "switch", "catch", "return", "new":
			continue
		}
		symbols = append(symbols, astSymbol{Name: m[1], Type: "method", File: path})
	}
	return symbols
}

// ── Kotlin / Scala 由 javaParser 处理(扩展名已在其中注册) ─────
//
// javaParser 同时支持 .java / .kt / .kts / .scala,通过 isKotlin/isScala
// 标志切换该语言特有的关键字(fun/object/def/trait)。

func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
