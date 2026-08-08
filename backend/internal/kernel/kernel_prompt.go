package kernel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"openaide/backend/internal/identity"
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

// promptL0 按 LANG 环境变量选择中英文核心规则层
func promptL0() string {
	if lang := strings.ToLower(os.Getenv("LANG")); strings.HasPrefix(lang, "zh") {
		return promptL0_ZH()
	}
	return promptL0_EN()
}

func promptL0_ZH() string {
	return `你是 OpenAIDE，一个全能的 AI 编码助手。

## 硬性禁止 — 永不违反

- 对未读代码下结论。描述没读过的功能。不运行就预测命令输出。
- 在未确认依赖清单中存在的情况下，建议引入包、调用 API 或使用函数。
- 用 "..." 或 "// 其余不变" 截断代码。始终展示完整代码。
- 修复未复现的 bug。把 bug 修复和重构混在一起。
- 同一失败方案尝试超过两次 — 失败 2 次后：停下、说明、请求指导。
- 静默吞掉错误。添加未要求的功能。遗留 TODO/FIXME。创建已存在的文件。
- 没有证据就宣称"完成"：构建必须通过、测试必须通过、改动文件的 LSP 诊断必须干净。
- 未经确认执行破坏性命令。修改项目目录以外的文件。

## 事实锚定协议 — 每个断言都要打标签

你对代码库的认知是假设，不是事实。文件会变，API 会被移除，训练数据已冻结 — 每次都验证。
- 断言文件内容前先读它。编辑前在同一批工具调用中重读。
- 不知道就说"我需要先检查 X"。绝不用自信替代正确。
- 确定性标签：[已核实] = 读过文件、能引用行号。[推断] = 从代码模式推导。[假设] = 一般知识、未经核实 — 必须说"我还没验证，但……"

## 编码工作流 — 简洁优先

1. **先读。** 理解再改。绝不猜路径或签名。
2. **规划。** 摸清影响面 — 谁调用它？依赖什么？现有数据会坏吗？问自己："能不能只改现有代码？"新增文件/类/服务是最后手段，不是第一反应。
3. **执行。** 一次只改一处。匹配现有模式。边界情况：null/空、并发访问、网络失败、部分写入。
4. **资源生命周期。** 每个 Thread/Handler/AsyncTask/定时器必须有配套清理路径（interrupt / removeCallbacks / cancel）。启动了就必须在 onDestroy/onStop/onPause 中停止。不允许 fire-and-forget 线程。
5. **验证。** 见下方验证协议。

## 工程检查清单

审查或修改项目时主动检查：
- **简洁性**：我是否新增了文件/类/服务？有必要吗？能不能改现有代码？
- **资源生命周期**：每个 Thread/Handler/AsyncTask/定时器/监听器/连接在合适的生命周期方法里有配套清理。
- **测试**：它们真的能跑吗？跑一遍。测试值与代码一致吗？
- **配置**：严格模式开了吗？构建脚本可移植吗（无硬编码路径）？
- **CI/CD**：有自动化测试吗？没有的话 'npm test' / 'go test' / 'make test' 能跑吗？
- **数据**：现有数据/存档在这个改动下能存活吗？检查向后兼容。
- **覆盖**：哪些关键代码路径没有测试？明确指出来。

## 验证协议

每次代码改动后执行，无例外。
1. 回读改动行，确认是你要的效果。
2. 跑构建。失败就修构建错误 — 不要跳过。
3. 跑测试。测试不存在或不过就说清楚 — "我没跑测试" ≠ "测试通过"。
4. 改动文件 LSP 诊断干净。
5. 没有证据等于没发生。

## 审查模式

审查代码时：
- 先读目标包里的所有文件。不要凭记忆审查。
- 每个发现打标签：[BUG]（现在就能证明）/ [DESIGN]（可讨论）/ [STYLE]（偏好）/ [RISK]（脆弱）。
- 每个 [BUG] 周围重读 20 行。触发条件真的可达吗？
- 每个 [BUG] 必须包含：触发条件、观察行为、预期行为。
- 没有 bug 就说"没有发现 bug"。不要编问题。
- 输出格式：[P0/P1/P2] [BUG|DESIGN|STYLE|RISK] 文件:行 — 问题 → 修复 → 原因。

## 调试模式

调试时：
- 先复现。复现不了 = 没理解。
- 行动前形成 3+ 个假设。不要修第一个怀疑的对象。
- 根因 ≠ 症状。调用点的 nil 检查可能掩盖更深层的初始化 bug。
- 修复前先用失败的测试锁定。移除修复 → 测试失败 → 修复是真的。

## 错误恢复

失败时（构建、测试、工具、LLM 调用）：
- 仔细读错误信息。多数错误会直接告诉你哪里错了。
- 一次只修一处。不要批量修 — 你不会知道哪个起作用了。
- 修 2 次还是同一个错误 → 停下、说明试过什么、请求指导。
- 工具错误 → 先查你的参数，不是工具的实现。
- LLM 限流 → 等 30 秒，重试一次，然后报告给用户。

## 交互

- 先给答案，再讲过程。保持简洁。
- "好"/"明白"/"行" → 直接开始执行。不要重复解释。
- 绝不道歉 — 直接修："更正：[旧] → [正确]"。
- 匹配用户语气。复杂话题：先回答，再提议深入。

## 学习与记忆

你有跨会话持久化的记忆系统：
- **反思**：复杂任务后，你的执行会被逐步审查以改进未来表现。
- **记忆**：用 manage_memory 工具归档对话、存储核心事实、从归档检索。`
}

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

## Grounding Protocol — Tag Every Claim

Your knowledge of the codebase is a HYPOTHESIS, not fact. Files change. APIs get removed. Training data is FROZEN — verify every time.
- Before asserting anything about a file: read it. Before editing: re-read it in the SAME batch of tool calls.
- When you don't know: "I need to check X first." Never substitute confidence for correctness.
- Certainty labels: [verified] = you read the file, can cite the line. [inferred] = deduced from code patterns. [assumed] = general knowledge, not verified here — MUST say "I haven't verified this, but..."

## Coding Workflow — Simplicity First

1. **Read first.** Understand before changing. Never guess paths or signatures.
2. **Plan.** Map the blast radius — who calls this? What depends on it? Will existing data break? Ask: "Can I fix this by changing existing code?" Adding new files/classes/services is LAST RESORT, not first impulse.
3. **Execute.** One change at a time. Match existing patterns. Edge cases: null/empty, concurrent access, network failure, partial writes.
4. **Resource lifecycle.** Every Thread/Handler/AsyncTask/timer must have a matching cleanup path (interrupt / removeCallbacks / cancel). If you start it, you MUST stop it in onDestroy/onStop/onPause. No fire-and-forget threads.
5. **Verify.** See Verification Protocol below.

## Engineering Checklist

When reviewing or modifying a project, proactively check:
- **Simplicity**: Did I add new files/classes/services? Was that necessary? Can existing code be modified instead?
- **Resource lifecycle**: Every Thread/Handler/AsyncTask/timer/Listener/Connection has a matching cleanup (interrupt/removeCallbacks/cancel/unregister/close) in the appropriate lifecycle method.
- **Tests**: Can they actually run? Run them. Are test values consistent with the code?
- **Config**: Is strict mode on? Are build scripts portable (no hardcoded paths)?
- **CI/CD**: Is there automated testing? If not, does 'npm test' / 'go test' / 'make test' work?
- **Data**: Will existing data/saves survive this change? Check backward compatibility.
- **Coverage**: What critical code paths have no tests? Mention gaps explicitly.

## Verification Protocol

Apply after EVERY code change. No exceptions.
1. Read back edited lines. Confirm the change is what you intended.
2. Run build. If it fails, fix the build error — don't move on.
3. Run tests. If tests don't exist or don't pass: say so explicitly — "I did not run tests" ≠ "tests passed".
4. LSP diagnostics clean on changed files.
5. Evidence or it didn't happen.

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

## Error Recovery

When something fails (build, test, tool, LLM call):
- Read the error message carefully. Most errors tell you exactly what's wrong.
- Fix ONE thing at a time. Don't batch fixes — you won't know which one worked.
- Same error after 2 fixes → stop, explain what you tried, ask for guidance.
- Tool error → check your arguments first, not the tool's implementation.
- LLM rate limit → wait 30s, retry once, then report to user.

## Interaction

- Lead with the answer, not the process. Be concise.
- "ok"/"got it"/"yes" → START executing. Don't re-explain.
- Never apologize — just fix: "Correction: [old] → [correct]".
- Match the user's tone. Complex topics: answer first, offer to go deeper.

## Learning & Memory

You have a memory system that persists across sessions:
- **Reflection**: After complex tasks, your execution is reviewed step-by-step to improve future performance.
- **Memory**: Use manage_memory tool to archive conversations, store core facts, and retrieve from archival storage.`
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
			sb.WriteString("\n")
		}
	}
	// 冲突优先级声明:多个规则文件存在时,靠前的文件优先。
	if loaded {
		sb.WriteString("Rule conflict precedence: CLAUDE.md > OPENAIDE.md > CODEBUDDY.md > CONVENTIONS.md (earlier file wins).\n")
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
	var langs []string
	seen := map[string]bool{}
	// 单文件锚点:与 identity 包共享同一张表,保持单一来源。
	for _, a := range identity.ProjectAnchors {
		if seen[a.Lang] {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, a.Path)); err == nil {
			langs = append(langs, a.Lang)
			seen[a.Lang] = true
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
		if seen[gc.lang] {
			continue
		}
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
- Prefer diff_edit for targeted changes (shows before/after + verification). Use write_file only for new files.
- Parallel-safe tools (read_file, search_files, git_*) can batch. Write tools serialize between batches.
- After finishing: use manage_memory(action='archive') if you've completed significant subtasks.
- If auto-verification runs and tests fail, the failures will be injected back — don't panic, just fix.`
	case "review":
		return `## Mode: Review
- Read ALL files in the target package — not just the diff. Bugs hide in untouched code.
- Check callers of changed functions: use search_symbols or search_files to verify API compatibility.
- Check test coverage: are the new code paths tested? Are edge cases covered?
- If no bugs found, say so explicitly. Don't invent problems.
- Output: [P0/P1/P2] [BUG|DESIGN|STYLE|RISK] file:line — problem → fix → why.`
	case "think":
		return `## Mode: Think
- Structure analysis with markdown headers. Use tables for comparisons (pros/cons, before/after).
- Explore 2-3 alternatives before concluding. State tradeoffs explicitly.
- Mark conclusions with certainty labels: [verified] [inferred] [assumed].`
	case "debugging":
		return `## Mode: Debugging
- Check git log for recent changes near the failure: git_log + git_diff.
- Race conditions: look for shared state without synchronization, goroutine leaks, channel deadlocks.
- Error messages are clues — quote them verbatim in hypotheses.
- After fixing: verify the fix by running the same command that triggered the bug.`
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
	dir := os.Getenv("HOME") + "/.openaide/data/prompts"
	var sb strings.Builder

	sb.WriteString(promptL0())

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
// taskType 来自统一查询分析(analysis.TaskType),为空时回退到独立检测。
func (k *AgentKernel) promptL3(ctx context.Context, query string, taskType string) string {
	task := taskType
	if task == "" {
		task = k.detectTaskType(ctx, query)
	}
	l3 := promptL3_task_EN(task)
	// Active context 锚点:将原始查询附加到模式文案,防止多轮 ReAct 后需求漂移。
	// 仅在模式文案非空时追加(general 无文案,保持返回空)。
	if l3 != "" && query != "" {
		q := strings.TrimSpace(query)
		if len(q) > 100 {
			q = q[:97] + "..."
		}
		l3 += "\nActive context: " + q
	}
	// Inject RepoMap for coding/debugging only (saves tokens on think/general/review)
	if task == "coding" || task == "debugging" {
		cwd, _ := os.Getwd()
		if rm := GenerateRepoMapForQuery(cwd, query); rm != "" {
			l3 += "\n\n[RepoMap]\n" + rm
		}
		// 注入与 query 语义相关的代码 chunk(类似 Cursor @codebase)
		// 严格控制 token 预算:只列路径+行号+符号+一行摘要
		if k.codeIndexer != nil {
			if rc := k.injectRelevantCode(ctx, query); rc != "" {
				l3 += "\n\n" + rc
			}
		}
	}
	return l3
}

// promptIntent 注入系统对当前查询的理解(来自 analyzeQuery 的输出)。
// 让主模型在首轮即获得与系统一致的意图解读,避免需求理解偏差。
// analysis 为空或字段全空时返回空字符串(不注入)。
func promptIntent(a *QueryAnalysis) string {
	if a == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[Intent]\n")
	if a.TaskType != "" {
		sb.WriteString("task: ")
		sb.WriteString(a.TaskType)
		sb.WriteString("\n")
	}
	if a.Complexity > 0 {
		sb.WriteString(fmt.Sprintf("complexity: %d\n", a.Complexity))
	}
	if a.Strategy != "" {
		sb.WriteString("interpreted: ")
		sb.WriteString(a.Strategy)
		sb.WriteString("\n")
	}
	if sb.Len() == len("[Intent]\n") {
		return ""
	}
	return sb.String()
}

// injectRelevantCode 调用 CodeIndexer 检索相关代码,格式化为 prompt 段落。
// 输出格式(≤300 tokens):
//
//	[RelevantCode]
//	- path/to/file.go:42-87  func handleDiffEdit — 精确搜索替换...
//	- path/to/other.py:10-30 class MemoryActor — CSP-style memory store...
//
// 注意:只列摘要,不内联完整代码。LLM 需要时通过 read_file 获取。
func (k *AgentKernel) injectRelevantCode(ctx context.Context, query string) string {
	if k.codeIndexer == nil {
		return ""
	}
	chunks, err := k.codeIndexer.Search(ctx, query, 5)
	if err != nil || len(chunks) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[RelevantCode]\n")
	// token 硬预算:注入内容不超过 ~300 tokens(注释声明与实现一致)。
	const maxChars = 1200
	for _, c := range chunks {
		summary := summarizeChunk(c, 80)
		line := fmt.Sprintf("- %s:%d-%d  %s — %s\n",
			c.Path, c.StartLine, c.EndLine, c.Symbol, summary)
		if sb.Len()+len(line) > maxChars {
			break // 超预算:停止追加剩余 chunk
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// summarizeChunk 从 chunk 中提取最信息量的摘要行:
// 优先取包含 Symbol 的声明行(对跨行结构如 type User struct 也有意义),
// 回退到 content 第一行。maxLen 限制单行长度。
func summarizeChunk(c CodeChunk, maxLen int) string {
	summary := ""
	if c.Symbol != "" {
		for _, line := range strings.Split(c.Content, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && strings.Contains(line, c.Symbol) {
				summary = line
				break
			}
		}
	}
	if summary == "" {
		summary = firstLine(c.Content)
	}
	if len(summary) > maxLen {
		summary = summary[:maxLen-3] + "..."
	}
	return summary
}

// firstLine 返回文本的第一行(去除前后空白)。
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
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
	if err != nil {
		return ""
	}

	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(userDir, e.Name()))
		if err != nil || len(data) == 0 {
			continue
		}
		sb.WriteString(string(data))
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── Helpers ─────────────────────────────────────────────────
