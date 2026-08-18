package kernel

// ProjectAnchor 项目语言锚点:锚点文件路径 → 语言标识。仅供内核在 prompt 阶段
// 注入项目语言约定使用;项目类型检测由 infra 层的 identity 包独立实现。
type ProjectAnchor struct {
	Path string // 项目根下存在的文件
	Lang string // 语言标识
}

// projectAnchors 是内核自持的项目语言锚点表(单一来源),只服务于内核的 prompt 语言注入。
// 与 internal/identity 的锚点表各自独立,勿强制共享以免引入跨模块耦合。
var projectAnchors = []ProjectAnchor{
	{"go.mod", "go"},
	{"go.work", "go-workspace"},
	{"package.json", "node"},
	{"Cargo.toml", "rust"},
	{"pyproject.toml", "python"},
	{"setup.py", "python"},
	{"requirements.txt", "python"},
	{"pom.xml", "java"},
	{"build.gradle", "java"},
	{"build.gradle.kts", "kotlin"},
	{"CMakeLists.txt", "c"},
	{"Makefile", "c"},
	{"Package.swift", "swift"},
	{"composer.json", "php"},
	{"Gemfile", "ruby"},
	{"pubspec.yaml", "dart"},
	{"mix.exs", "elixir"},
	{"stack.yaml", "haskell"},
	{"rebar.config", "erlang"},
	{"dune-project", "ocaml"},
	{"cpanfile", "perl"},
}