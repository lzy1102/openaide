package kernel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ── Layered Prompt System ──────────────────────────────────
// Layers are assembled dynamically based on task context.
// Simple queries get L0+L1 (~500 tokens). Complex tasks get all layers.
//
//   L0: Core Rules (always, ~600 tok)
//       Hard Blocks + Grounding Protocol + Certainty Labels +
//       Coding Workflow + Review Mode + Debugging Mode +
//       Interaction + Learning & Memory
//   L1: Project Context         (OPENAIDE.md etc., ~100 tok)
//   L2: Skill Prompt            (skill match, ~100-500 tok)
//   L3: Mode Signal (2-3 lines) (coding/review/think/debugging/general)
//   L4: Learned Experience      (learner insights, ~100 tok)
//   L5: Reflection Improvement  (last reflection, ~100 tok)

// ── L0: Identity + Safety (always loaded) ─────────────────

func promptL0_EN() string {
	return `You are OpenAIDE, a versatile AI coding assistant.

## Hard Blocks — Never Violate These

- Claim facts about unread code. Describe features you haven't read. Predict command output without running it.
- Suggest importing a package, calling an API, or using a function without verifying it exists in the dependency manifest.
- Truncate code with "..." or "// rest remains the same". Always show complete code.
- Fix a bug you haven't reproduced. Mix a bug fix with a refactor.
- Try the same failing approach more than twice — after 2 failures: stop, explain, ask for guidance.
- Silently swallow errors. Add features not requested. Leave TODO/FIXME. Create files that already exist.
- Claim "done" without evidence: build must pass, tests must pass, LSP diagnostics must be clean on changed files.
- Execute destructive commands without confirmation. Modify files outside the project directory.

## Grounding Protocol

- Your knowledge of the codebase is a HYPOTHESIS, not fact. Files change. APIs get removed.
- Your training data is FROZEN — package versions, function signatures, dependency manifests: verify every time.
- Before asserting anything about a file: read it. Before editing: re-read it in the SAME batch of tool calls.
- When you don't know: "I need to check X first." Never substitute confidence for correctness.

## Certainty Labels — Tag Every Claim

[verified] = you read the file, can cite the line.
[inferred] = deduced from code patterns you read.
[assumed] = from general knowledge, NOT verified here. MUST say "I haven't verified this, but..."

## Coding Workflow

1. **Read first.** Understand before changing. Never guess paths or signatures.
2. **Plan.** Map the blast radius — who calls this? What depends on it? Minimal change >50 lines → over-engineering.
3. **Execute.** One change at a time. Match existing patterns. Edge cases: null/empty, concurrent access, network failure, partial writes.
4. **Verify.** After every edit: read back changed lines. Run build + tests. "Should work" ≠ verified.

## Review Mode

When reviewing code:
- Read ALL files in the target package first. Don't review from memory.
- Label every finding: [BUG] (provable NOW) / [DESIGN] (debatable) / [STYLE] (preference) / [RISK] (fragile).
- Re-read 20 lines around each [BUG]. Is the trigger actually reachable?
- Every [BUG] must include: trigger condition, observed behavior, expected behavior.
- No bugs found → say "No bugs found". Don't invent problems.
- Output format: [P0/P1/P2] [BUG|DESIGN|STYLE|RISK] file:line — problem → fix → why.

## Debugging Mode

When debugging:
- Reproduce first. Can't reproduce = don't understand.
- Form 3+ hypotheses before acting. Don't fix the first thing you suspect.
- Root cause ≠ symptom. A nil check at the call site may mask a deeper initialization bug.
- Lock with a failing test BEFORE fixing. Remove the fix → test fails → fix is real.

## Interaction

- Lead with the answer, not the process. Be concise.
- "ok"/"got it"/"yes" to your plan → START executing. Don't re-explain.
- Never apologize — just fix: "Correction: [old] → [correct]".
- Match the user's tone. Complex topics: answer first, offer to go deeper.

## Learning & Memory

You have a self-improving memory system that persists across sessions:
- **Knowledge Base**: Facts and patterns you discover are saved and retrieved in future sessions.
- **Skill Distillation**: Recurring successful patterns become reusable slash commands.
- **Reflection**: After complex tasks, your execution is reviewed step-by-step.
- **Auto-trigger**: The system asks whether the result is worth remembering. Answer honestly.`
}

func promptL0_ZH() string {
	return `你是 OpenAIDE，一个多功能的 AI 编程助手。

## 硬约束 — 绝对禁止

- 声称未读代码的事实。描述未读的功能。不运行就预测命令输出。
- 未验证依赖是否存在就建议引入包、调用 API。未验证就建议使用某个函数。
- 用 "..." 或 "// 其余代码不变" 截断代码。始终展示完整代码。
- 修复未复现的 bug。在同一改动中混入重构。
- 同一失败方法尝试超过两次——2 次失败后：停止，解释经过，请求指导。
- 吞掉错误不解释。加用户没要的功能。留 TODO/FIXME。创建已存在的文件。
- 没有证据就说"做完了"：构建必须通过，测试必须通过，修改文件的 LSP 诊断必须干净。
- 未确认就执行破坏性命令。修改项目目录外的文件。

## 接地协议

- 你对代码库的了解是假说，不是事实。文件会变。API 会被移除。
- 你的训练数据已冻结——包版本、函数签名、依赖清单：每次都要验证。
- 断言任何文件内容前：先读。编辑前：在同一次工具调用中重读。
- 不知道就说"我需要先检查 X"。不要用自信替代准确。

## 确定性标注 — 标记每个声明

[verified]  = 你读了文件，能引用具体行号。
[inferred] = 从读到的代码模式推断。
[assumed]  = 来自通用知识，未在此项目中验证。必须说"我还没有验证，但..."

## 编码工作流

1. **先读。** 理解再修改。不猜测路径或签名。
2. **计划。** 摸清影响范围——谁调用了？什么依赖它？最小改动超过 50 行 → 过度设计。
3. **执行。** 一次只改一个东西。匹配现有模式。边界情况：空值、并发、网络失败、部分写入。
4. **验证。** 每次编辑后：回读修改行。跑构建 + 测试。"应该没问题" ≠ 已验证。

## 审查模式

审查代码时：
- 先通读目标包所有文件。不要凭记忆审查。
- 每个发现标注：[BUG]（当前可复现）/ [DESIGN]（设计争议）/ [STYLE]（风格偏好）/ [RISK]（当前正确但脆弱）。
- 每个 [BUG] 后重读周围 20 行。触发条件真的可达吗？
- 每个 [BUG] 必须包含：触发条件、实际行为、预期行为。
- 没找到 bug → 说"未发现 bug"。不要编造问题。
- 输出格式：[P0/P1/P2] [BUG|DESIGN|STYLE|RISK] 文件:行号 — 问题 → 修复 → 原因。

## 调试模式

调试时：
- 先复现。不能复现 = 不理解。
- 动手前至少形成 3 个假说。不要修复你第一个怀疑的东西。
- 根因 ≠ 表象。调用方加 nil 检查可能掩盖了更深的初始化 bug。
- 修复前先用一个失败的测试锁定。移除修复 → 测试失败 → 修复是真的。

## 交互

- 先给答案，再讲过程。简洁。
- "好"/"行"/"做" → 立即执行。不要重新解释。
- 不道歉，只修正："更正：[旧] → [正确]"。
- 匹配用户语气。复杂话题：先答核心，再问要不要深入。

## 学习与记忆

你有一套跨会话的自我改进记忆系统：
- **知识库**：你发现的事实和模式会被保存，下次会话自动注入。
- **技能蒸馏**：反复出现的成功模式会变成可复用的命令。
- **反思**：复杂任务后，你的执行会被逐步评估。
- **自动触发**：系统询问结果是否值得记忆。诚实回答。`
}

// ── L1: Project Context ────────────────────────────────────

func promptL1() string {
	cwd, _ := os.Getwd()
	var sb strings.Builder
	sb.WriteString("\n[WorkingDir] ")
	sb.WriteString(cwd)

	// Git branch
	if out, err := runGitCmd("rev-parse", "--abbrev-ref", "HEAD"); err == nil && out != "" {
		sb.WriteString("\n[Git] branch: ")
		sb.WriteString(out)
	}

	// Project rules — load all found rule files, merge into one context
	ruleFiles := []string{"CLAUDE.md", "OPENAIDE.md", "CODEBUDDY.md", "CONVENTIONS.md"}
	loaded := false
	for _, f := range ruleFiles {
		if data, err := os.ReadFile(filepath.Join(cwd, f)); err == nil && len(data) > 0 {
			content := string(data)
		if len(content) > 8000 {
				content = content[:8000]
				if lastPeriod := strings.LastIndex(content, "."); lastPeriod > 6000 {
					content = content[:lastPeriod+1]
				}
				content += "\n(Full content available via read_file)"
			}
			if !loaded {
				sb.WriteString("\n[Rules] ")
				loaded = true
			}
			sb.WriteString(f)
			sb.WriteString(":\n")
			sb.WriteString(content)
		}
	}

	// RepoMap symbol map
	if rm := GenerateRepoMap(cwd); rm != "" {
		sb.WriteString("\n[RepoMap]\n")
		sb.WriteString(rm)
	}

	// Language-specific conventions
	if langs := detectProjectLangs(cwd); len(langs) > 0 {
		sb.WriteString("\n[Language] ")
		sb.WriteString(strings.Join(langs, ", "))
		for _, lang := range langs {
			sb.WriteString("\n")
			sb.WriteString(langConventions(lang))
		}
	}

	return sb.String()
}

func detectProjectLangs(dir string) []string {
	checks := []struct{ file, lang string }{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"pyproject.toml", "python"},
		{"Cargo.toml", "rust"},
		{"pom.xml", "java"},
		{"build.gradle", "java"},
		{"build.gradle.kts", "kotlin"},
		{"CMakeLists.txt", "c"},
		{"Makefile", "c"},
		{"Package.swift", "swift"},
		{"composer.json", "php"},
		{"Gemfile", "ruby"},
		{"build.sbt", "scala"},
		{"pubspec.yaml", "dart"},
		{"mix.exs", "elixir"},
		{"stack.yaml", "haskell"},
		{"rebar.config", "erlang"},
		{"dune-project", "ocaml"},
		{"cpanfile", "perl"},
		// Additional: glob-based checks
	}
	var langs []string
	seen := map[string]bool{}
	for _, c := range checks {
		if seen[c.lang] { continue }
		if _, err := os.Stat(filepath.Join(dir, c.file)); err == nil {
			langs = append(langs, c.lang)
			seen[c.lang] = true
		}
	}
	// Glob-based detection (patterns, not single files)
	globChecks := []struct{ pattern, lang string }{
		{"*.csproj", "csharp"},
		{"*.sln", "csharp"},
		{"*.cabal", "haskell"},
		{"*.c", "c"},
		{"*.cpp", "c"},
		{"*.r", "r"},
		{"*.lua", "lua"},
		{"*.jl", "julia"},
		{"*.dart", "dart"},
	}
	for _, gc := range globChecks {
		if seen[gc.lang] { continue }
		if matches, _ := filepath.Glob(filepath.Join(dir, gc.pattern)); len(matches) > 0 {
			langs = append(langs, gc.lang)
			seen[gc.lang] = true
		}
	}
	return langs
}

func langConventions(lang string) string {
	switch lang {
	case "go":
		return "- Go: explicit error handling, short names, defer for cleanup. Tests: table-driven with testing.T. Format: gofmt. Prefer composition, small interfaces. Use context.Context."
	case "java":
		return "- Java: camelCase naming, checked exceptions or unchecked with documentation. Tests: JUnit 5. Build: Maven or Gradle. Use try-with-resources for AutoCloseable. Prefer immutable objects, dependency injection."
	case "kotlin":
		return "- Kotlin: camelCase naming, prefer val over var, nullable types explicitly (?.). Tests: JUnit 5 or Kotest. Build: Gradle with kotlin DSL. Use coroutines for async, avoid GlobalScope."
	case "c":
		return "- C/C++: follow existing clang-format config if present. Check memory: malloc/free paired, no use-after-free, no buffer overflows. C++: RAII, rule of five, smart pointers, no raw new/delete. Tests: CUnit/GTest/Catch2."
	case "csharp":
		return "- C#: follow Microsoft conventions. Use async/await with Task. Tests: xUnit or NUnit. LINQ for collections, avoid multiple enumeration. 'using' for IDisposable. Properties, not public fields."
	case "swift":
		return "- Swift: follow Swift API Design Guidelines. let over var, optionals over force-unwrap. Tests: XCTest. Prefer structs over classes. Protocol-oriented design. MainActor for UI."
	case "php":
		return "- PHP: follow PSR-12. Use strict types (declare(strict_types=1)). Use type hints for parameters and returns. Tests: PHPUnit. Prefer dependency injection. Avoid global state. Use Composer for packages."
	case "ruby":
		return "- Ruby: follow community style guide (RuboCop). Use symbols for hash keys when appropriate. Tests: RSpec or Minitest. Prefer enumerable methods over loops. Use Bundler for dependencies. Favor composition."
	case "scala":
		return "- Scala: prefer immutable data structures, val over var. Use pattern matching, not isInstanceOf. Tests: ScalaTest. Functional style: map/flatMap/fold over loops. Implicit parameters documented. Use sbt or Mill."
	case "dart":
		return "- Dart: follow Effective Dart guidelines. Use final over var where possible. Null safety: explicit ? and !. Tests: test package. Async: Future/Stream with async/await. Widget testing with flutter_test if Flutter."
	case "haskell":
		return "- Haskell: follow existing style (stylish-haskell or hindent). Pure functions over IO where possible. Use types to encode invariants. Tests: Hspec or Tasty. Avoid partial functions (head, fromJust). Use stack or cabal."
	case "elixir":
		return "- Elixir: follow community conventions. Pattern matching over conditionals. Tests: ExUnit. Use with for chaining. Supervisor trees for fault tolerance. Pipe operator for data transformation. Avoid {:ok, _} ignoring errors."
	case "r":
		return "- R: follow tidyverse style guide. Use <- for assignment. Vectorized operations over explicit loops. Tests: testthat. Prefer dplyr/purrr over base apply when in tidyverse project. Document functions with roxygen2."
	case "lua":
		return "- Lua: follow existing project style. Use local variables by default. Tests: busted or luaunit. Prefer table-based modules. Avoid global namespace pollution. Keep it simple — Lua philosophy."
	case "julia":
		return "- Julia: follow SciML style guide. Type-stable functions for performance. Tests: Test standard library. Multiple dispatch over class hierarchies. Use @assert for invariants. Avoid global variables in loops."
	case "erlang":
		return "- Erlang: follow Erlang Programming Rules. Let it crash philosophy with supervisor trees. Pattern matching in function heads. Tests: EUnit or Common Test. Use OTP behaviours (gen_server, gen_statem). Immutable data."
	case "ocaml":
		return "- OCaml: follow OCaml Programming Guidelines. Prefer pattern matching over if/else. Use types to prevent errors. Tests: Alcotest or OUnit. Use dune for builds. Prefer tail recursion for performance."
	case "perl":
		return "- Perl: use strict and warnings. Tests: Test::More or Test2. Use Perl::Tidy or perltidy for formatting. Prefer lexical filehandles and three-arg open. Use CPAN modules over reinventing. Keep regex readable with /x modifier."
	case "node":
		return "- Node/JS/TS: follow project's ESLint/Prettier config. async/await, not raw promises. Handle rejections. Tests: the framework in the project (Jest/Mocha/Vitest). One component per file."
	case "python":
		return "- Python: follow PEP 8, type hints. Tests: pytest or project's framework. Virtual environments. Explicit over implicit, handle exceptions specifically."
	case "rust":
		return "- Rust: use Result/Option, match exhaustively, derive common traits. Format: cargo fmt, lint: cargo clippy. Tests: #[cfg(test)] with #[test]. Own your data."
	}
	return ""
}

// ── L2: Skill Prompt ───────────────────────────────────────

// L2 is injected by SkillActor.InjectPrompt. Defined here for completeness.

// ── L3: Task Adapter ────────────────────────────────────────

func promptL3_task_EN(task string) string {
	switch task {
	case "coding":
		return `## Mode: Coding
You are writing or modifying code. Follow the Coding Workflow: Read → Plan → Execute → Verify.
After changes: read back edited lines, run build + tests, check LSP diagnostics. No shortcuts.`
	case "review":
		return `## Mode: Review
You are auditing code. Follow the Review Mode rules: read all files first, label every finding, verify each [BUG] is reachable. Output findings in [P0/P1/P2] [BUG|DESIGN|STYLE|RISK] format.`
	case "think":
		return `## Mode: Think
You are analyzing, explaining, or exploring. Use [verified]/[inferred]/[assumed] labels. Be specific: "We could use Redis" → "We could use Redis as session store replacing SQLite. [assumed]"`
	case "debugging":
		return `## Mode: Debugging
You are investigating a bug. Follow the Debugging Mode rules: reproduce first, form 3+ hypotheses, lock with a failing test before fixing, verify root cause ≠ symptom.`
	default:
		return ""
	}
}

func promptL3_task_ZH(task string) string {
	switch task {
	case "coding":
		return `## 模式：编码
你正在编写或修改代码。遵循编码工作流：先读 → 计划 → 执行 → 验证。
改动后：回读修改行，跑构建 + 测试，检查 LSP 诊断。不走捷径。`
	case "review":
		return `## 模式：审查
你正在审计代码。遵循审查模式规则：先通读所有文件，标注每个发现，验证每个 [BUG] 确实可达。输出格式：[P0/P1/P2] [BUG|DESIGN|STYLE|RISK]。`
	case "think":
		return `## 模式：思考
你正在分析、解释或探索。使用 [verified]/[inferred]/[assumed] 标签。要具体："可以用 Redis" → "可以用 Redis 做 session 存储替代 SQLite。[assumed]"`
	case "debugging":
		return `## 模式：调试
你正在调查 bug。遵循调试模式规则：先复现，形成 3+ 假说，修复前用失败测试锁定，验证根因 ≠ 表象。`
	default:
		return ""
	}
}

// ── L4: Learned Experience ─────────────────────────────────

func promptL4(insights []string) string {
	if len(insights) == 0 {
		return ""
	}
	sb := new(strings.Builder)
	sb.WriteString("\n## Project Knowledge\n")
	for _, insight := range insights {
		sb.WriteString("- ")
		sb.WriteString(insight)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── L5: Reflection Improvement ─────────────────────────────

func promptL5(reflection *ReflectionResult) string {
	if reflection == nil || reflection.Quality == 0 {
		return ""
	}
	sb := new(strings.Builder)
	sb.WriteString("\n## Lessons from Last Execution — Apply These")
	// Include quality score so the LLM knows how much to adjust
	sb.WriteString(fmt.Sprintf("\nLast execution quality: %d/10", reflection.Quality))
	if reflection.Quality <= 4 {
		sb.WriteString(" — SIGNIFICANT change needed: different approach, fewer tools, earlier synthesis.")
	} else if reflection.Quality <= 7 {
		sb.WriteString(" — decent. Refine but don't overhaul.")
	} else {
		sb.WriteString(" — good. Minor adjustments only.")
	}
	if len(reflection.Issues) > 0 {
		sb.WriteString("\nAvoid these mistakes from last time:")
		for _, issue := range reflection.Issues {
			sb.WriteString("\n- ❌ ")
			sb.WriteString(issue)
		}
	}
	if len(reflection.Suggestions) > 0 {
		sb.WriteString("\n## Behavioral Rules for This Round — FOLLOW THESE")
		for _, s := range reflection.Suggestions {
			sb.WriteString("\n- ")
			sb.WriteString(s)
		}
	}
	if reflection.Learned != "" {
		sb.WriteString("\nKey lesson: ")
		sb.WriteString(reflection.Learned)
	}
	return sb.String()
}

// ── Builders ────────────────────────────────────────────────

// buildSystemPrompt assembles the stable prompt prefix (L0+L1+L2+user).
// Built-in prompts are always used; user customizations are loaded from
// ~/.openaide/data/prompts/user/*.md and appended (never override system).
func (k *AgentKernel) buildSystemPrompt(query *Query) string {
	zh := isZhEnv()
	dir := os.Getenv("HOME") + "/.openaide/data/prompts"
	var sb strings.Builder

	// L0: Identity + Safety (always built-in)
	if zh {
		sb.WriteString(promptL0_ZH())
	} else {
		sb.WriteString(promptL0_EN())
	}

	// User custom prompts: appended after system layers, never overwritten on upgrade
	if userPrompt := loadUserPrompts(dir); userPrompt != "" {
		sb.WriteString("\n\n")
		sb.WriteString(userPrompt)
	}

	// L1: Project context (always auto-generated)
	if l1 := promptL1(); l1 != "" {
		sb.WriteString(l1)
	}

	return sb.String()
}
// promptL3 returns the task adapter for the current query.
func (k *AgentKernel) promptL3(ctx context.Context, query string) string {
	task := k.detectTaskType(ctx, query)
	if isZhEnv() {
		return promptL3_task_ZH(task)
	}
	return promptL3_task_EN(task)
}

// detectTaskType classifies the query via LLM into one of five task types.
func (k *AgentKernel) detectTaskType(ctx context.Context, query string) string {
	prompt := `Classify this user query into exactly one category: coding, review, think, general.

Definitions:
- coding: writing, fixing, refactoring, or implementing code
- review: auditing, checking security, reviewing changes
- think: explaining, analyzing, designing, researching, exploring possibilities
- debugging: investigating a bug, crash, unexpected behavior, or error (user reports something broken)
- general: greetings, chitchat, or none of the above

Query: ` + query + `

Answer with ONLY the category name (one word).`

	resp, err := k.llmProvider.Chat(ctx, []Message{
		{Role: "user", Content: prompt},
	}, nil, map[string]interface{}{
		"route":       "execution",
		"no_thinking": true,
	})
	if err != nil {
		return "general"
	}

	result := strings.TrimSpace(strings.ToLower(resp.Content))
	for _, cat := range []string{"debugging", "coding", "review", "think", "general"} {
		if strings.HasPrefix(result, cat) {
			return cat
		}
	}
	return "general"
}

// loadUserPrompts reads all .md files from the user prompts directory and
// concatenates them. User prompts are additive — appended after system layers,
// never override built-in prompts. Upgrades never touch user files.
// Directory: ~/.openaide/data/prompts/user/*.md
func loadUserPrompts(dir string) string {
	userDir := filepath.Join(dir, "user")
	entries, err := os.ReadDir(userDir)
	if err != nil { return "" }

	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") { continue }
		data, err := os.ReadFile(filepath.Join(userDir, e.Name()))
		if err != nil || len(data) == 0 { continue }
		sb.WriteString(string(data))
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── Helpers ─────────────────────────────────────────────────

// defaultSystemPrompt returns the default system prompt based on locale.
func defaultSystemPrompt() string {
	if isZhEnv() {
		return promptL0_ZH()
	}
	return promptL0_EN()
}



func isZhEnv() bool {
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = os.Getenv("LC_ALL")
	}
	return strings.HasPrefix(lang, "zh")
}

