package kernel

import (
	"os"
	"strings"
	"testing"
)

func TestParseGoAST_Func(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/test.go"
	os.WriteFile(f, []byte("package main\n\nfunc Hello(name string) string { return \"hi\" }\n"), 0644)

	syms := parseGoAST(f)
	found := false
	for _, s := range syms {
		if s.Name == "Hello" && s.Type == "func" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Hello' function in parsed symbols, got %d symbols", len(syms))
	}
}

func TestParseGoAST_Type(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/types.go"
	os.WriteFile(f, []byte("package main\n\ntype Greeter struct {\n\tName string\n}\n"), 0644)

	syms := parseGoAST(f)
	found := false
	for _, s := range syms {
		if s.Name == "Greeter" && s.Type == "type" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Greeter' type in symbols")
	}
}

func TestParseGoAST_Method(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/method.go"
	os.WriteFile(f, []byte("package main\n\ntype S struct{}\nfunc (s *S) Do() {}"), 0644)

	syms := parseGoAST(f)
	found := false
	for _, s := range syms {
		if strings.Contains(s.Name, ".Do") && s.Type == "method" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected method 'Do' on S in symbols")
	}
}

func TestParseGoAST_Const(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/const.go"
	os.WriteFile(f, []byte("package main\n\nconst Version = \"1.0\"\n"), 0644)

	syms := parseGoAST(f)
	found := false
	for _, s := range syms {
		if s.Name == "Version" && s.Type == "const" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Version' const in symbols")
	}
}

func TestParseGoAST_Import(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/import.go"
	os.WriteFile(f, []byte("package main\n\nimport \"fmt\"\nimport \"strings\"\n"), 0644)

	syms := parseGoAST(f)
	count := 0
	for _, s := range syms {
		if s.Type == "import" { count++ }
	}
	if count < 2 {
		t.Errorf("expected >=2 imports, got %d", count)
	}
}

func TestParsePythonRegex(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/app.py"
	os.WriteFile(f, []byte("class App:\n    def run(self):\n        pass\n"), 0644)

	syms := parsePythonRegex(f)
	if len(syms) < 2 {
		t.Errorf("expected >=2 symbols (class + func), got %d", len(syms))
	}
}

func TestParseJSRegex(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/app.js"
	os.WriteFile(f, []byte("export function hello() {}\nclass App {}\nconst x = 1;\n"), 0644)

	syms := parseJSRegex(f)
	if len(syms) < 2 {
		t.Errorf("expected >=2 symbols, got %d", len(syms))
	}
}

func TestGenerateASTRepoMap_Integration(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/main.go", []byte("package main\n\nfunc main() {}\n"), 0644)
	os.WriteFile(dir+"/util.py", []byte("def helper():\n    pass\n"), 0644)

	result := GenerateASTRepoMap(dir)
	if !strings.Contains(result, "RepoMap (AST)") {
		t.Error("expected AST repo map header")
	}
	if !strings.Contains(result, "main") {
		t.Error("expected main function in repo map")
	}
}
