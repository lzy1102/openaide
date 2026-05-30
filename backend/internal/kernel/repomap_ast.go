package kernel

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ── Enhanced RepoMap with AST-based parsing ─────────────────
// Uses Go's standard library parser for Go files (tree-sitter quality).
// Falls back to regex for Python, JavaScript, TypeScript, Rust.

type astSymbol struct {
	Name    string
	Type    string // func, type, method, const, var, class, import
	File    string
	Exports []string
}

type astRepoMap struct {
	symbols   []astSymbol
	files     map[string]string // file → content hash
	updatedAt time.Time
	mu        sync.RWMutex
}

var globalASTRepoMap = &astRepoMap{files: make(map[string]string)}

// GenerateASTRepoMap builds a symbol map using AST parsing for Go files
// and enhanced regex for other languages. Returns a markdown-formatted string.
func GenerateASTRepoMap(root string) string {
	globalASTRepoMap.mu.RLock()
	if time.Since(globalASTRepoMap.updatedAt) < 5*time.Minute && len(globalASTRepoMap.symbols) > 0 {
		globalASTRepoMap.mu.RUnlock()
		return formatRepoMap(globalASTRepoMap.symbols)
	}
	globalASTRepoMap.mu.RUnlock()

	globalASTRepoMap.mu.Lock()
	defer globalASTRepoMap.mu.Unlock()

	// Re-check after acquiring write lock
	if time.Since(globalASTRepoMap.updatedAt) < 5*time.Minute && len(globalASTRepoMap.symbols) > 0 {
		return formatRepoMap(globalASTRepoMap.symbols)
	}

	var symbols []astSymbol
	fileCount := 0

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && (strings.HasPrefix(d.Name(), ".") ||
				d.Name() == "vendor" || d.Name() == "node_modules" ||
				d.Name() == "bin" || d.Name() == "dist" || d.Name() == "__pycache__") {
				return filepath.SkipDir
			}
			return nil
		}
		if fileCount >= 200 { return filepath.SkipDir }
		ext := filepath.Ext(path)
		if ext == ".go" {
			syms := parseGoAST(path)
			symbols = append(symbols, syms...)
			fileCount++
		} else if ext == ".py" {
			syms := parsePythonRegex(path)
			symbols = append(symbols, syms...)
			fileCount++
		} else if ext == ".js" || ext == ".ts" || ext == ".tsx" {
			syms := parseJSRegex(path)
			symbols = append(symbols, syms...)
			fileCount++
		}
		return nil
	})

	globalASTRepoMap.symbols = symbols
	globalASTRepoMap.updatedAt = time.Now()
	return formatRepoMap(symbols)
}

// parseGoAST uses Go's standard library parser for precise symbol extraction.
func parseGoAST(path string) []astSymbol {
	data, err := os.ReadFile(path)
	if err != nil { return nil }
	content := string(data)
	if len(content) > 500*1024 { return nil } // skip files >500KB

	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	// Check cache: skip if file unchanged since last parse.
	// Called under GenerateASTRepoMap's write-lock; standalone callers (tests) are single-goroutine.
	if h, ok := globalASTRepoMap.files[path]; ok && h == hash {
		return nil // unchanged
	}
	globalASTRepoMap.files[path] = hash

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil { return nil }

	relPath, _ := filepath.Rel(".", path)
	var symbols []astSymbol

	// Package declaration
	if f.Name != nil {
		symbols = append(symbols, astSymbol{
			Name: f.Name.Name, Type: "package", File: relPath,
		})
	}

	// Walk the AST
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := astSymbol{Name: d.Name.Name, File: relPath}
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
					sym := astSymbol{Name: s.Name.Name, Type: "type", File: relPath}
					if s.Name.IsExported() {
						sym.Exports = []string{s.Name.Name}
					}
					// Extract struct fields as sub-info
					if st, ok := s.Type.(*ast.StructType); ok && st.Fields != nil {
						for _, field := range st.Fields.List {
							for _, name := range field.Names {
								sym.Exports = append(sym.Exports, name.Name)
							}
						}
					}
					symbols = append(symbols, sym)

				case *ast.ValueSpec:
					for i, name := range s.Names {
						symType := "var"
						if d.Tok == token.CONST { symType = "const" }
						sym := astSymbol{Name: name.Name, Type: symType, File: relPath}
						if name.IsExported() {
							sym.Exports = []string{name.Name}
						}
						_ = i
						symbols = append(symbols, sym)
					}

				case *ast.ImportSpec:
					imp := strings.Trim(s.Path.Value, `"`)
					symbols = append(symbols, astSymbol{
						Name: imp, Type: "import", File: relPath,
					})
				}
			}
		}
	}

	return symbols
}

// parsePythonRegex extracts Python symbols using regex patterns.
func parsePythonRegex(path string) []astSymbol {
	data, err := os.ReadFile(path)
	if err != nil { return nil }
	content := string(data)
	if len(content) > 500*1024 { return nil }

	relPath, _ := filepath.Rel(".", path)
	var symbols []astSymbol

	patterns := map[string]string{
		"class":  `^\s*class\s+(\w+)\s*[(:]`,
		"func":   `^\s*def\s+(\w+)\s*\(`,
		"import": `^\s*(?:from|import)\s+([\w.]+)`,
	}

	for typ, pat := range patterns {
		re := regexp.MustCompile(`(?m)` + pat) // multiline mode: ^ matches line start
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				symbols = append(symbols, astSymbol{
					Name: m[1], Type: typ, File: relPath,
				})
			}
		}
	}
	return symbols
}

// parseJSRegex extracts JavaScript/TypeScript symbols using regex patterns.
func parseJSRegex(path string) []astSymbol {
	data, err := os.ReadFile(path)
	if err != nil { return nil }
	content := string(data)
	if len(content) > 500*1024 { return nil }

	relPath, _ := filepath.Rel(".", path)
	var symbols []astSymbol

	patterns := map[string]string{
		"class":    `(?m)^\s*(?:export\s+)?(?:abstract\s+)?class\s+(\w+)`,
		"func":     `(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`,
		"arrow":    `(?m)^\s*(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\(`,
		"method":   `(?m)^\s+(?:async\s+)?(\w+)\s*\([^)]*\)\s*{`,
		"import":   `(?m)^\s*import\s+.*?(?:from\s+)?['"]([^'"]+)['"]`,
		"export":   `(?m)^\s*export\s+(?:const|let|var|function|class)\s+(\w+)`,
	}

	for typ, pat := range patterns {
		re := regexp.MustCompile(pat)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 && m[1] != "if" && m[1] != "for" && m[1] != "while" {
				symbols = append(symbols, astSymbol{
					Name: m[1], Type: typ, File: relPath,
				})
			}
		}
	}
	return symbols
}

// formatRepoMap formats symbols as a compact markdown repomap.
func formatRepoMap(symbols []astSymbol) string {
	if len(symbols) == 0 { return "" }

	// Group by file
	fileSymbols := make(map[string][]astSymbol)
	for _, s := range symbols {
		fileSymbols[s.File] = append(fileSymbols[s.File], s)
	}

	var sb strings.Builder
	sb.WriteString("## RepoMap (AST)\n")
	for file, syms := range fileSymbols {
		sb.WriteString(fmt.Sprintf("### %s\n", file))
		// Group by type
		typeCount := map[string]int{}
		for _, s := range syms { typeCount[s.Type]++ }
		for typ, count := range typeCount {
			sb.WriteString(fmt.Sprintf("  %s×%d ", typ, count))
		}
		sb.WriteString("\n")
		// List key symbols (limit to 15 per file)
		n := 0
		for _, s := range syms {
			if n >= 15 { sb.WriteString("  ...\n"); break }
			marker := ""
			if len(s.Exports) > 0 { marker = " (exported)" }
			sb.WriteString(fmt.Sprintf("  - %s %s%s\n", s.Type, s.Name, marker))
			n++
		}
	}
	return sb.String()
}
