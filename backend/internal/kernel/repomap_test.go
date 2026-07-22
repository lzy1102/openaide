package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func hasSymbol(syms []astSymbol, name, symType string) bool {
	for _, s := range syms {
		if s.Name == name && s.Type == symType {
			return true
		}
	}
	return false
}

// ── Go ───────────────────────────────────────────────────────

func TestGoParser_FuncAndMethod(t *testing.T) {
	p := goParser{}
	syms := p.Parse("foo.go", []byte(`package foo
type S struct{}
func Hello(name string) string { return "hi" }
func (s *S) Do() {}
`))
	if !hasSymbol(syms, "Hello", "func") {
		t.Errorf("missing func Hello, got: %+v", syms)
	}
	if !hasSymbol(syms, "S.Do", "method") {
		t.Errorf("missing method S.Do, got: %+v", syms)
	}
	if !hasSymbol(syms, "S", "type") {
		t.Errorf("missing type S, got: %+v", syms)
	}
}

func TestGoParser_ConstVar(t *testing.T) {
	p := goParser{}
	syms := p.Parse("c.go", []byte(`package c
const Version = "1.0"
var Debug = false
`))
	if !hasSymbol(syms, "Version", "const") {
		t.Errorf("missing const Version")
	}
	if !hasSymbol(syms, "Debug", "var") {
		t.Errorf("missing var Debug")
	}
}

// ── Python ───────────────────────────────────────────────────

func TestPythonParser_ClassMethod(t *testing.T) {
	p := pythonParser{}
	syms := p.Parse("app.py", []byte(`import os
from typing import List

class App:
    def __init__(self):
        pass
    async def run(self):
        pass

def main():
    pass
`))
	if !hasSymbol(syms, "App", "class") {
		t.Errorf("missing class App, got: %+v", syms)
	}
	if !hasSymbol(syms, "__init__", "method") {
		t.Errorf("missing method __init__")
	}
	if !hasSymbol(syms, "run", "method") {
		t.Errorf("missing method run")
	}
	if !hasSymbol(syms, "main", "func") {
		t.Errorf("missing func main")
	}
	if !hasSymbol(syms, "os", "import") {
		t.Errorf("missing import os")
	}
}

// ── JavaScript / TypeScript ──────────────────────────────────

func TestJSParser_ClassFuncArrow(t *testing.T) {
	p := jsParser{}
	syms := p.Parse("a.js", []byte(`import React from 'react';
export class App {}
export function hello() {}
const x = () => 1;
export const y = async () => 2;
`))
	if !hasSymbol(syms, "App", "class") {
		t.Errorf("missing class App, got: %+v", syms)
	}
	if !hasSymbol(syms, "hello", "func") {
		t.Errorf("missing func hello")
	}
	if !hasSymbol(syms, "x", "func") {
		t.Errorf("missing arrow func x")
	}
	if !hasSymbol(syms, "y", "func") {
		t.Errorf("missing arrow func y")
	}
	if !hasSymbol(syms, "react", "import") {
		t.Errorf("missing import react")
	}
}

func TestJSParser_TypeScript(t *testing.T) {
	p := jsParser{}
	syms := p.Parse("a.ts", []byte(`export interface User { name: string; }
export type ID = string;
export enum Color { Red, Green }
export namespace NS { export function f() {} }
`))
	if !hasSymbol(syms, "User", "interface") {
		t.Errorf("missing interface User, got: %+v", syms)
	}
	if !hasSymbol(syms, "ID", "type") {
		t.Errorf("missing type ID")
	}
	if !hasSymbol(syms, "Color", "enum") {
		t.Errorf("missing enum Color")
	}
	if !hasSymbol(syms, "NS", "namespace") {
		t.Errorf("missing namespace NS")
	}
}

// ── Rust ─────────────────────────────────────────────────────

func TestRustParser(t *testing.T) {
	p := rustParser{}
	syms := p.Parse("lib.rs", []byte(`use std::io;
pub struct Config { field: i32 }
pub enum Kind { A, B }
pub trait Draw { fn draw(&self); }
pub fn main() {}
impl Config {
    pub fn new() -> Self { Self { field: 0 } }
}
mod inner;
macro_rules! vec_of { ($x:expr) => { vec![$x] }; }
`))
	if !hasSymbol(syms, "Config", "struct") {
		t.Errorf("missing struct Config, got: %+v", syms)
	}
	if !hasSymbol(syms, "Kind", "enum") {
		t.Errorf("missing enum Kind")
	}
	if !hasSymbol(syms, "Draw", "trait") {
		t.Errorf("missing trait Draw")
	}
	if !hasSymbol(syms, "main", "func") {
		t.Errorf("missing fn main")
	}
	if !hasSymbol(syms, "inner", "module") {
		t.Errorf("missing mod inner")
	}
	if !hasSymbol(syms, "vec_of", "macro") {
		t.Errorf("missing macro vec_of")
	}
}

// ── Java ─────────────────────────────────────────────────────

func TestJavaParser(t *testing.T) {
	p := javaParser{}
	syms := p.Parse("Foo.java", []byte(`package com.example;
import java.util.List;
public class Foo {
    public void bar() {}
    private int baz() { return 1; }
}
`))
	if !hasSymbol(syms, "com.example", "package") {
		t.Errorf("missing package, got: %+v", syms)
	}
	if !hasSymbol(syms, "java.util.List", "import") {
		t.Errorf("missing import")
	}
	if !hasSymbol(syms, "Foo", "class") {
		t.Errorf("missing class Foo")
	}
	if !hasSymbol(syms, "bar", "method") {
		t.Errorf("missing method bar")
	}
}

func TestJavaParser_Kotlin(t *testing.T) {
	p := javaParser{}
	syms := p.Parse("Foo.kt", []byte(`package com.example
object Singleton { fun hello() {} }
class Bar { fun baz() {} }
`))
	if !hasSymbol(syms, "Singleton", "object") {
		t.Errorf("missing object Singleton, got: %+v", syms)
	}
	if !hasSymbol(syms, "hello", "func") {
		t.Errorf("missing fun hello")
	}
}

// ── Ruby ─────────────────────────────────────────────────────

func TestRubyParser(t *testing.T) {
	p := rubyParser{}
	syms := p.Parse("app.rb", []byte(`require 'json'
module MyApp
  class User
    attr_accessor :name
    def initialize(name)
      @name = name
    end
  end
end
`))
	if !hasSymbol(syms, "json", "import") {
		t.Errorf("missing require json, got: %+v", syms)
	}
	if !hasSymbol(syms, "MyApp", "module") {
		t.Errorf("missing module MyApp")
	}
	if !hasSymbol(syms, "User", "class") {
		t.Errorf("missing class User")
	}
	if !hasSymbol(syms, "name", "attr") {
		t.Errorf("missing attr name")
	}
	if !hasSymbol(syms, "initialize", "func") {
		t.Errorf("missing def initialize")
	}
}

// ── C / C++ ──────────────────────────────────────────────────

func TestCParser(t *testing.T) {
	p := cParser{}
	syms := p.Parse("a.c", []byte(`#include <stdio.h>
#define MAX 100
namespace ns { class Foo { public: void bar(); }; }
typedef int Int32;
int add(int a, int b) { return a + b; }
`))
	if !hasSymbol(syms, "stdio.h", "import") {
		t.Errorf("missing include stdio.h, got: %+v", syms)
	}
	if !hasSymbol(syms, "MAX", "const") {
		t.Errorf("missing #define MAX")
	}
	if !hasSymbol(syms, "ns", "namespace") {
		t.Errorf("missing namespace ns")
	}
	if !hasSymbol(syms, "Foo", "class") {
		t.Errorf("missing class Foo")
	}
	if !hasSymbol(syms, "Int32", "type") {
		t.Errorf("missing typedef Int32")
	}
	if !hasSymbol(syms, "add", "func") {
		t.Errorf("missing func add")
	}
}

// ── PHP ──────────────────────────────────────────────────────

func TestPHPParser(t *testing.T) {
	p := phpParser{}
	syms := p.Parse("a.php", []byte(`<?php
namespace App;
use PDO;
class User {
    public function save() {}
    const TABLE = 'users';
}
`))
	if !hasSymbol(syms, "App", "namespace") {
		t.Errorf("missing namespace App, got: %+v", syms)
	}
	if !hasSymbol(syms, "PDO", "import") {
		t.Errorf("missing use PDO")
	}
	if !hasSymbol(syms, "User", "class") {
		t.Errorf("missing class User")
	}
	if !hasSymbol(syms, "save", "func") {
		t.Errorf("missing function save")
	}
	if !hasSymbol(syms, "TABLE", "const") {
		t.Errorf("missing const TABLE")
	}
}

// ── Swift ────────────────────────────────────────────────────

func TestSwiftParser(t *testing.T) {
	p := swiftParser{}
	syms := p.Parse("a.swift", []byte(`import Foundation
public struct User { let name: String }
class App {
    func run() {}
    var count: Int = 0
}
extension User { func greet() {} }
typealias ID = String
`))
	if !hasSymbol(syms, "Foundation", "import") {
		t.Errorf("missing import Foundation, got: %+v", syms)
	}
	if !hasSymbol(syms, "User", "class") {
		t.Errorf("missing struct/class User")
	}
	if !hasSymbol(syms, "App", "class") {
		t.Errorf("missing class App")
	}
	if !hasSymbol(syms, "run", "func") {
		t.Errorf("missing func run")
	}
	if !hasSymbol(syms, "count", "var") {
		t.Errorf("missing var count")
	}
	if !hasSymbol(syms, "User", "extension") {
		t.Errorf("missing extension User")
	}
	if !hasSymbol(syms, "ID", "type") {
		t.Errorf("missing typealias ID")
	}
}

// ── C# ───────────────────────────────────────────────────────

func TestCSharpParser(t *testing.T) {
	p := csharpParser{}
	syms := p.Parse("a.cs", []byte(`using System;
namespace MyApp {
    public class Foo {
        public void Bar() {}
    }
}
`))
	if !hasSymbol(syms, "System", "import") {
		t.Errorf("missing using System, got: %+v", syms)
	}
	if !hasSymbol(syms, "MyApp", "namespace") {
		t.Errorf("missing namespace MyApp")
	}
	if !hasSymbol(syms, "Foo", "class") {
		t.Errorf("missing class Foo")
	}
	if !hasSymbol(syms, "Bar", "method") {
		t.Errorf("missing method Bar")
	}
}

// ── go.mod ───────────────────────────────────────────────────

func TestGoModParser(t *testing.T) {
	p := goModParser{}
	syms := p.Parse("go.mod", []byte(`module github.com/foo/bar

go 1.21

require (
	github.com/google/uuid v1.6.0
)
`))
	if !hasSymbol(syms, "github.com/foo/bar", "module") {
		t.Errorf("missing module, got: %+v", syms)
	}
	if !hasSymbol(syms, "1.21", "go_version") {
		t.Errorf("missing go version")
	}
}

// ── 集成测试 ─────────────────────────────────────────────────

func TestGenerateRepoMap_MultiLang(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "main.go", `package main
func main() {}`)
	writeTempFile(t, dir, "app.py", `def helper():
    pass
class App:
    pass`)
	writeTempFile(t, dir, "lib.rs", `pub fn rust_fn() {}`)
	writeTempFile(t, dir, "App.java", `public class App { void m() {} }`)
	writeTempFile(t, dir, "ignored.txt", `should not be indexed`)

	// 强制清缓存,确保新扫描
	InvalidateRepoMapCache(dir)
	result := GenerateRepoMap(dir)
	if !strings.Contains(result, "RepoMap") {
		t.Errorf("expected RepoMap header, got: %s", result)
	}
	if !strings.Contains(result, "main.go") {
		t.Errorf("expected main.go in result: %s", result)
	}
	if !strings.Contains(result, "app.py") {
		t.Errorf("expected app.py in result")
	}
	if !strings.Contains(result, "lib.rs") {
		t.Errorf("expected lib.rs in result")
	}
	if !strings.Contains(result, "App.java") {
		t.Errorf("expected App.java in result")
	}
	if strings.Contains(result, "ignored.txt") {
		t.Errorf("ignored.txt should not appear: %s", result)
	}
}

func TestGenerateRepoMap_Cache(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.go", `package a
func A() {}`)

	InvalidateRepoMapCache(dir)
	r1 := GenerateRepoMap(dir)
	r2 := GenerateRepoMap(dir) // 应命中缓存
	if r1 != r2 {
		t.Errorf("cache returned different results")
	}
}

func TestInvalidateRepoMapCache(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.go", `package a`)

	GenerateRepoMap(dir)
	files, syms := CountRepoMapSymbols(dir)
	if files == 0 && syms == 0 {
		t.Errorf("expected non-empty cache after GenerateRepoMap")
	}

	InvalidateRepoMapCache(dir)
	files, syms = CountRepoMapSymbols(dir)
	if files != 0 || syms != 0 {
		t.Errorf("expected empty cache after invalidation, got files=%d syms=%d", files, syms)
	}
}
