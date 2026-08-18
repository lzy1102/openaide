package kernel

import "regexp"

// ── Python: 增强 regex parser ────────────────────────────────
//
// 支持:class / def / async def / @decorator / import / from import
// 自动识别嵌套类方法(def 在 class 内的缩进)
type pythonParser struct{}

func (pythonParser) Extensions() []string { return []string{".py", ".pyi"} }

var (
	// 注意:缩进捕获组用 [ \t]* 而非 \s*,避免 \s 跨行吃掉空行换行符
	// (否则 def main() 前的空行会让 m[1] = "\n",被误判为 method)
	pyClassRe  = regexp.MustCompile(`(?m)^([ \t]*)(?:@[\w.]+\s*\n\s*)*class\s+(\w+)\s*(?:\([^)]*\))?\s*:`)
	pyFuncRe   = regexp.MustCompile(`(?m)^([ \t]*)(?:@[\w.]+(?:\([^)]*\))?\s*\n\s*)*(?:async\s+)?def\s+(\w+)\s*\(`)
	pyImportRe = regexp.MustCompile(`(?m)^\s*(?:from\s+([\w.]+)\s+import|import\s+([\w.]+))`)
)

func (pythonParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	text := string(content)
	var symbols []astSymbol

	// class
	for _, m := range pyClassRe.FindAllStringSubmatch(text, -1) {
		if len(m) >= 3 {
			sym := astSymbol{Name: m[2], Type: "class", File: path}
			if len(m[1]) == 0 {
				sym.Exports = []string{m[2]} // 顶层 class 视为导出
			}
			symbols = append(symbols, sym)
		}
	}

	// def / async def
	for _, m := range pyFuncRe.FindAllStringSubmatch(text, -1) {
		if len(m) >= 3 {
			symType := "func"
			if len(m[1]) > 0 {
				symType = "method" // 缩进的 def 视为方法
			}
			symbols = append(symbols, astSymbol{
				Name: m[2], Type: symType, File: path,
			})
		}
	}

	// import / from import
	for _, m := range pyImportRe.FindAllStringSubmatch(text, -1) {
		if len(m) >= 3 {
			imp := m[1]
			if imp == "" {
				imp = m[2]
			}
			if imp != "" {
				symbols = append(symbols, astSymbol{
					Name: imp, Type: "import", File: path,
				})
			}
		}
	}

	return symbols
}
