package kernel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// ── astSymbol: 通用符号结构 ──────────────────────────────────
//
// 所有语言 parser 统一产出此结构。File 字段为相对路径。
type astSymbol struct {
	Name    string   // 符号名(方法含 receiver,如 "Greeter.Greet")
	Type    string   // func/type/method/const/var/class/import 等
	File    string   // 相对路径
	Exports []string // 导出字段/方法名(可选,用于显示细节)
}

// ── Go: AST-based parser ─────────────────────────────────────
//
// 使用 Go 标准库 parser,精准提取 func/type/const/var/import。
type goParser struct{}

func (goParser) Extensions() []string { return []string{".go"} }

func (goParser) Parse(path string, content []byte) []astSymbol {
	if len(content) > 500*1024 {
		return nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil
	}

	var symbols []astSymbol

	if f.Name != nil {
		symbols = append(symbols, astSymbol{
			Name: f.Name.Name, Type: "package", File: path,
		})
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := astSymbol{Name: d.Name.Name, File: path}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Type = "method"
				if se, ok := d.Recv.List[0].Type.(*ast.StarExpr); ok {
					if ident, ok := se.X.(*ast.Ident); ok {
						sym.Name = ident.Name + "." + d.Name.Name
					}
				} else if ident, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
					sym.Name = ident.Name + "." + d.Name.Name
				}
			} else {
				sym.Type = "func"
			}
			if d.Name.IsExported() {
				sym.Exports = []string{d.Name.Name}
			}
			symbols = append(symbols, sym)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := astSymbol{Name: s.Name.Name, Type: "type", File: path}
					if s.Name.IsExported() {
						sym.Exports = []string{s.Name.Name}
					}
					if st, ok := s.Type.(*ast.StructType); ok && st.Fields != nil {
						for _, field := range st.Fields.List {
							for _, name := range field.Names {
								sym.Exports = append(sym.Exports, name.Name)
							}
						}
					}
					symbols = append(symbols, sym)

				case *ast.ValueSpec:
					for _, name := range s.Names {
						symType := "var"
						if d.Tok == token.CONST {
							symType = "const"
						}
						sym := astSymbol{Name: name.Name, Type: symType, File: path}
						if name.IsExported() {
							sym.Exports = []string{name.Name}
						}
						symbols = append(symbols, sym)
					}

				case *ast.ImportSpec:
					imp := strings.Trim(s.Path.Value, `"`)
					symbols = append(symbols, astSymbol{
						Name: imp, Type: "import", File: path,
					})
				}
			}
		}
	}

	return symbols
}

// ── go.mod: 提取模块名和 Go 版本 ─────────────────────────────
type goModParser struct{}

func (goModParser) Extensions() []string { return []string{".mod"} }

func (goModParser) Parse(path string, content []byte) []astSymbol {
	if path != "go.mod" && !strings.HasSuffix(path, "/go.mod") {
		return nil
	}
	var symbols []astSymbol
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			symbols = append(symbols, astSymbol{
				Name: strings.TrimSpace(strings.TrimPrefix(line, "module ")),
				Type: "module", File: path,
			})
		} else if strings.HasPrefix(line, "go ") {
			symbols = append(symbols, astSymbol{
				Name: strings.TrimSpace(strings.TrimPrefix(line, "go ")),
				Type: "go_version", File: path,
			})
		} else if strings.HasPrefix(line, "require ") || strings.HasPrefix(line, "require\t") {
			// require 块单行形式 require xxx v1.2.3
			rest := strings.TrimSpace(strings.TrimPrefix(line, "require"))
			fields := strings.Fields(rest)
			if len(fields) >= 1 {
				symbols = append(symbols, astSymbol{
					Name: fields[0], Type: "dependency", File: path,
				})
			}
		}
	}
	return symbols
}
