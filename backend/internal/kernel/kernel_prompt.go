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
		}
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

	sb.WriteString(promptL0_EN())

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
	l3 := promptL3_task_EN(task)
	// Inject RepoMap for coding/debugging only (saves tokens on think/general/review)
	if task == "coding" || task == "debugging" {
		cwd, _ := os.Getwd()
		if rm := GenerateRepoMap(cwd); rm != "" {
			l3 += "\n\n[RepoMap]\n" + rm
		}
	}
	return l3
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

func defaultSystemPrompt() string {
	return promptL0_EN()
}

