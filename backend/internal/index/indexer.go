package index

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Symbol 代码符号
type Symbol struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // function, type, method, variable, constant
	Package    string `json:"package"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Signature  string `json:"signature,omitempty"`
	Doc        string `json:"doc,omitempty"`
	IsExported bool   `json:"is_exported"`
}

// FileIndex 文件索引
type FileIndex struct {
	Path        string    `json:"path"`
	Language    string    `json:"language"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mtime"`
	ContentHash string    `json:"content_hash"`
	Symbols     []Symbol  `json:"symbols"`
	Imports     []string  `json:"imports,omitempty"`
	Exports     []string  `json:"exports,omitempty"`
}

// ProjectIndex 项目索引
type ProjectIndex struct {
	Root      string                `json:"root"`
	Files     map[string]*FileIndex `json:"files"`    // path -> FileIndex
	Symbols   map[string][]*Symbol  `json:"symbols"`  // name -> symbols
	Packages  map[string][]string   `json:"packages"` // package -> files
	UpdatedAt time.Time             `json:"updated_at"`
}

// Indexer 代码索引器
type Indexer struct {
	indexDir string
	index    *ProjectIndex
	mu       sync.RWMutex
}

// NewIndexer 创建索引器
func NewIndexer(indexDir string) (*Indexer, error) {
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create index dir: %w", err)
	}

	idx := &Indexer{
		indexDir: indexDir,
		index: &ProjectIndex{
			Files:    make(map[string]*FileIndex),
			Symbols:  make(map[string][]*Symbol),
			Packages: make(map[string][]string),
		},
	}

	// 尝试加载已有索引
	idx.load()

	return idx, nil
}

// SetRoot 设置项目根目录
func (i *Indexer) SetRoot(root string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.index.Root = root
}

// IndexFile 索引单个文件
func (i *Indexer) IndexFile(path string) (*FileIndex, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// 计算内容哈希
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(content))

	// 检测语言
	lang := detectLanguage(path)

	// 解析符号
	symbols := parseSymbols(path, string(content), lang)

	fileIndex := &FileIndex{
		Path:        path,
		Language:    lang,
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		ContentHash: hash,
		Symbols:     symbols,
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	i.index.Files[path] = fileIndex

	// 更新符号索引和包索引
	for _, sym := range symbols {
		i.index.Symbols[sym.Name] = append(i.index.Symbols[sym.Name], &sym)
		if sym.Package != "" {
			i.index.Packages[sym.Package] = append(i.index.Packages[sym.Package], path)
		}
	}

	i.index.UpdatedAt = time.Now()

	return fileIndex, nil
}

// IndexDirectory 索引目录
func (i *Indexer) IndexDirectory(dir string, extensions []string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			// 跳过隐藏目录和 vendor
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// 检查扩展名
		ext := filepath.Ext(path)
		if len(extensions) > 0 {
			found := false
			for _, e := range extensions {
				if ext == e {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}

		_, err = i.IndexFile(path)
		return err
	})
}

// SearchSymbol 搜索符号
func (i *Indexer) SearchSymbol(name string) []*Symbol {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.index.Symbols[name]
}

// SearchSymbols 模糊搜索符号
func (i *Indexer) SearchSymbols(pattern string) []*Symbol {
	i.mu.RLock()
	defer i.mu.RUnlock()

	var results []*Symbol
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(pattern))
	if err != nil {
		return results
	}

	for name, symbols := range i.index.Symbols {
		if re.MatchString(name) {
			results = append(results, symbols...)
		}
	}

	return results
}

// GetFileSymbols 获取文件的所有符号
func (i *Indexer) GetFileSymbols(path string) []Symbol {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if file, ok := i.index.Files[path]; ok {
		return file.Symbols
	}
	return nil
}

// GetStats 获取索引统计
func (i *Indexer) GetStats() map[string]interface{} {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return map[string]interface{}{
		"total_files":    len(i.index.Files),
		"total_symbols":  len(i.index.Symbols),
		"total_packages": len(i.index.Packages),
		"updated_at":     i.index.UpdatedAt,
	}
}

// Save 保存索引到磁盘
func (i *Indexer) Save() error {
	i.mu.RLock()
	defer i.mu.RUnlock()
	data, err := json.MarshalIndent(i.index, "", "  ")

	if err != nil {
		return err
	}

	path := filepath.Join(i.indexDir, "project_index.json")
	return os.WriteFile(path, data, 0644)
}

// ============ 内部方法 ============

func (i *Indexer) load() error {
	path := filepath.Join(i.indexDir, "project_index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var index ProjectIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	i.index = &index
	return nil
}

func detectLanguage(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		return "go"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "unknown"
	}
}

// parseSymbols 解析文件符号（基于正则的轻量级解析）
func parseSymbols(path, content, lang string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	switch lang {
	case "go":
		symbols = parseGoSymbols(path, lines)
	case "javascript", "typescript":
		symbols = parseJSSymbols(path, lines)
	case "python":
		symbols = parsePythonSymbols(path, lines)
	case "rust":
		symbols = parseRustSymbols(path, lines)
	}

	return symbols
}

// Go 符号解析
func parseGoSymbols(path string, lines []string) []Symbol {
	var symbols []Symbol
	var currentPackage string

	// 正则表达式
	packageRe := regexp.MustCompile(`^package\s+(\w+)`)
	funcRe := regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?(\w+)`)
	typeRe := regexp.MustCompile(`^type\s+(\w+)`)
	varRe := regexp.MustCompile(`^(var|const)\s+(\w+)`)
	methodRe := regexp.MustCompile(`^func\s+\([^)]+\)\s+(\w+)`)

	for i, line := range lines {
		lineNum := i + 1

		// 包名
		if matches := packageRe.FindStringSubmatch(line); matches != nil {
			currentPackage = matches[1]
			continue
		}

		// 函数
		if matches := funcRe.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			symbols = append(symbols, Symbol{
				Name:       name,
				Type:       "function",
				Package:    currentPackage,
				File:       path,
				Line:       lineNum,
				IsExported: isExported(name),
			})
			continue
		}

		// 方法
		if matches := methodRe.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			symbols = append(symbols, Symbol{
				Name:       name,
				Type:       "method",
				Package:    currentPackage,
				File:       path,
				Line:       lineNum,
				IsExported: isExported(name),
			})
			continue
		}

		// 类型
		if matches := typeRe.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			symbols = append(symbols, Symbol{
				Name:       name,
				Type:       "type",
				Package:    currentPackage,
				File:       path,
				Line:       lineNum,
				IsExported: isExported(name),
			})
			continue
		}

		// 变量/常量
		if matches := varRe.FindStringSubmatch(line); matches != nil {
			name := matches[2]
			symType := "variable"
			if matches[1] == "const" {
				symType = "constant"
			}
			symbols = append(symbols, Symbol{
				Name:       name,
				Type:       symType,
				Package:    currentPackage,
				File:       path,
				Line:       lineNum,
				IsExported: isExported(name),
			})
		}
	}

	return symbols
}

// JS/TS 符号解析
func parseJSSymbols(path string, lines []string) []Symbol {
	var symbols []Symbol

	funcRe := regexp.MustCompile(`(?:function|const|let|var)\s+(\w+)\s*[=:]`)
	classRe := regexp.MustCompile(`class\s+(\w+)`)
	methodRe := regexp.MustCompile(`(?:async\s+)?(\w+)\s*\([^)]*\)\s*\{`)

	for i, line := range lines {
		lineNum := i + 1

		if matches := funcRe.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "function",
				File:       path,
				Line:       lineNum,
				IsExported: true,
			})
			continue
		}

		if matches := classRe.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "type",
				File:       path,
				Line:       lineNum,
				IsExported: true,
			})
			continue
		}

		if matches := methodRe.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "method",
				File:       path,
				Line:       lineNum,
				IsExported: true,
			})
		}
	}

	return symbols
}

// Python 符号解析
func parsePythonSymbols(path string, lines []string) []Symbol {
	var symbols []Symbol

	funcRe := regexp.MustCompile(`^def\s+(\w+)`)
	classRe := regexp.MustCompile(`^class\s+(\w+)`)

	for i, line := range lines {
		lineNum := i + 1

		if matches := funcRe.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "function",
				File:       path,
				Line:       lineNum,
				IsExported: !strings.HasPrefix(matches[1], "_"),
			})
			continue
		}

		if matches := classRe.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "type",
				File:       path,
				Line:       lineNum,
				IsExported: !strings.HasPrefix(matches[1], "_"),
			})
		}
	}

	return symbols
}

// Rust 符号解析
func parseRustSymbols(path string, lines []string) []Symbol {
	var symbols []Symbol

	funcRe := regexp.MustCompile(`^fn\s+(\w+)`)
	structRe := regexp.MustCompile(`^(?:struct|enum|trait)\s+(\w+)`)
	implRe := regexp.MustCompile(`^impl\s+(?:<[^>]+>\s+)?(\w+)`)

	for i, line := range lines {
		lineNum := i + 1

		if matches := funcRe.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "function",
				File:       path,
				Line:       lineNum,
				IsExported: isExported(matches[1]),
			})
			continue
		}

		if matches := structRe.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "type",
				File:       path,
				Line:       lineNum,
				IsExported: isExported(matches[1]),
			})
			continue
		}

		if matches := implRe.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:       matches[1],
				Type:       "type",
				File:       path,
				Line:       lineNum,
				IsExported: true,
			})
		}
	}

	return symbols
}

func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Go/Rust: 大写开头
	first := name[0]
	return first >= 'A' && first <= 'Z'
}
