# OPENAIDE.md

项目规则文件——OpenAIDE 每次查询自动加载到系统提示词。也是 Claude Code 的 CLAUDE.md 兼容文件。

## 作用

LLM 在处理任何请求前自动获得此文件内容，无需手动探索项目结构。省 2-3 轮探索。
维护：LLM 犯错→加规则；重复说话→加约定；新功能→更新架构。

## Output Quality Rules

- 分析代码后必须给可执行的改进方案（具体文件名、改动内容、预估工作量）
- 被问"怎么改进"时输出优先级排序的 action items，不是泛泛的"建议拆分大文件"
- 审查时说清楚：哪个文件、改什么、为什么。分析 + 建议才是完整输出
- 不要说"可以优化"而不说怎么优化

## Common commands

```bash
# Build unified binary → bin/openaide
make build

# Build desktop app (requires CGO + WebKit/WebView2)
make build-desktop

# Run all tests
make test
# or a single package
cd backend && go test -v ./internal/kernel/...

# Format and lint
make fmt
make lint

# Run the server
openaide server

# Run the CLI
openaide

# Run evaluation suite
cd backend && go run ./cmd/eval          # all builtin tasks
cd backend && go run ./cmd/eval -full    # full capability acceptance
cd backend && go run ./cmd/eval -quick   # smoke test (easy only)
```

## Architecture

OpenAIDE is an AI Agent kernel platform in Go. Strictly layered, CSP actor concurrency, LLM-native decisions.

```
┌──────────────────────────────────────────────────────────┐
│                    Entry Points                          │
│  openaide server  (REST + SSE + WebSocket)              │
│  openaide         (REPL: readline + glamour + pterm)    │
│  cmd/desktop      (Wails v2: Go + WebView)              │
├──────────────────────────────────────────────────────────┤
│                 infra/Application                        │
│  DI container — wires kernel, tools, plugins, channels   │
├──────────────────────────────────────────────────────────┤
│  orchestration/          api/            channel/        │
│  Research→Propose→Plan   HTTP handlers   Feishu/Telegram │
│  Multi-Agent Team        CORS/middleware Webhook/TaskQ   │
├──────────────────────────────────────────────────────────┤
│               kernel/AgentKernel                         │
│  ┌─ ReAct Loop (Process / ProcessStream)              ─┐ │
│  │                                                    │ │
│  │  Think → Act → Observe → Repeat                    │ │
│  │    │       │        │                               │ │
│  │    LLM   Tools   Reflection                        │ │
│  │                                                    │ │
│  │  Before each loop:                                  │ │
│  │    analyzeQuery (unified: task+skill+complexity)    │ │
│  │                                                      │ │
│  │  After each loop:                                  │ │
│  │    doReflection (LLM post_process) → QualityGate   │ │
│  └────────────────────────────────────────────────────┘ │
│                                                          │
│  Sub-packages:                                           │
│    kernel/actor/  — Actor, ActorStore, SafeMap          │
│    kernel/trace/  — FileTracer, FileCheckpointer         │
│    kernel/graph/  — DAG topological sort                 │
├──────────────────────────────────────────────────────────┤
│  llm/          tools/         memory/                     │
│  Multi-provider 43 tools     Session mem (MemGPT-style)   │
│  Gateway+Rtr   Filesystem     Embed cache                 │
│               Browser/Desktop/LSP/MCP                    │
└──────────────────────────────────────────────────────────┘
```

### Design Principles

- **LLM is the brain.** No rule-based fallbacks. Skill matching, risk assessment, round estimation, reflection — all LLM-native. LLM unavailable = agent dead. No degradation.
- **CSP Actors.** Stateful modules own their data in one goroutine. External access via channels. Zero locks for core data paths. Per-request state passed as parameters — never stored on shared structs.
- **Goroutines are cheap.** No artificial semaphores. Direct goroutine dispatch for event handlers. `context.WithoutCancel` for async tasks.
- **Prompt is layered.** Stable prefix (L0+L1+L2) cached. Dynamic tail (L3+L5+L6) per-query. Analysis format only in review/research modes.

### Entry points

- **`openaide server`** — Production server. Loads config from `~/.openaide/config.yaml`, starts the HTTP API server.
- **`openaide`** — Interactive CLI. **REPL mode** (rich terminal readline with markdown rendering, pterm styling). `-c` flag resumes last session. On first run, triggers interactive onboarding (`onboard.go`). One-shot mode: `openaide "fix this bug"`.
- **`backend/cmd/desktop/main.go`** — Desktop application (Wails v2). Go backend reuses kernel/orchestration directly — no HTTP layer. HTML/CSS/JS frontend with 3-panel layout. Build: `make build-desktop`.
- **`backend/cmd/cli/setup.go`** — `openaide setup` interactive configuration wizard. Step-by-step: language → provider → API key → model → SearXNG. Auto-generates valid `config.yaml` with dual-model setup and tests connection.
- **`backend/cmd/cli/onboard.go`** — First-run onboarding. Template questions (role/style/language) → `NewApplication()` → LLM interview (2-round open-ended dialogue) → generates custom `system.md` → hot-reloads via `SetSystemPrompt()`.

### Layered design

1. **`backend/internal/infra/`** (7 files) — Application container, split by concern:
   - `app.go` — `Application` struct, `NewApplication` (~150 lines), `Start`, `Stop`. MCP wiring from both `config.yaml` and Claude `.mcp.json` plugins.
   - `app_llm.go` — `createLLMGateway()` — provider registration, router, prompt cache
   - `app_kernel.go` — `createKernel()` — kernel + all enhancements. Claude skills injection (`DiscoverClaudeSkills()` → `AddClaudeSkill()`). Claude hooks wiring (`DiscoverClaudeHooks()` → `agentKernel.Subscribe()` with shell execution).
   - `app_channels.go` — `setupChannels()` — webhook/Feishu/Telegram, task queue

2. **`backend/internal/kernel/`** — Core agent kernel with CSP actor architecture:
   - `kernel.go` — `AgentKernel` struct, Config, constructor, state management, event system.
   - `kernel_process.go` — `Process()` sync path + `doReflection()` (async reflection after ReAct loop)
    - `kernel_stream.go` — `ProcessStream()` streaming path. Tool partitioning with parallel-safe batching. **Auto-verification**: after coding, detects project test command (go test/npm test/make test) and runs it; failures injected as user messages back into ReAct loop.
   - `kernel_prompt.go` — Layered prompt system (L0-L5), LLM task classification, file overrides, L3 dynamic tail.
   - `kernel_react.go` — Shared ReAct helpers: prepareReActRound, partitionToolCalls, executeToolBatch
   - **Sub-packages:**
     - `kernel/actor/` — Generic Actor, ActorStore[K,V], SafeMap[K,V] (zero kernel deps, used by 12+ packages)
     - `kernel/trace/` — FileTracer (JSONL buffered), FileCheckpointer (disk crash recovery)
     - `kernel/graph/` — DAG engine with topological sort (Kahn's algorithm)
   - **CSP Actors** (zero-lock, single-goroutine ownership):
     - `session_actor.go` — SessionActor (SQLite-backed, WAL mode)
     - `skill_actor.go` — SkillActor (LLM detection outside actor, auto-persist)
     - `embedder.go` — Embedder interface + CosineSimilarity (canonical, avoids circular import)
   - `saga.go` — RunSaga() for cross-actor transactional compensation
   - `interfaces.go` — All kernel-level interfaces (LLMProvider, ModelSwitcher, SessionStore, etc.)
   - `types.go` — Shared types: `Message`, `ToolCall`, `Query`, `Response`, `StreamChunk`, `Event`, `Session`
   - `llm_reflection.go` — Process supervision: evaluates each ReAct step individually, identifies best/worst decisions
   - Other files: `compress.go`, `checkpoint.go`, `approval.go`, `adaptive.go`, `tracer.go`
   - Legacy (not default): `session_store.go`

3. **`backend/internal/memory/`** — MemGPT-style memory store:
   - `memory_actor.go` — MemoryActor (working memory + archival storage + core facts). Agent-driven memory management: archive conversations, retrieve from archive, store core facts that survive all sessions.
   - `memory.go` — Legacy file-based memory (not default)

3. **`backend/internal/tools/`** (18 files) — Tool definitions and handlers split by domain:
   - `registry.go` — `Registry` framework, `BuiltinTools()` concatenates domain-specific defs, `BuiltinHandlers()`, `RegisterBuiltins()`, `safeAbsPath()`, `formatBytes()`
   - `tools_filesystem.go` — read_file (with offset/limit), write_file, execute_command, list_directory, search_files
   - `tools_symbol.go` — search_symbols
   - `diff_edit.go` — diff_edit (ACI-verified: before/after comparison + write verification), diff_edit_lines
   - `tools_memory.go` — manage_memory (MemGPT: archive, retrieve, remember, recall actions)
   - `git_deep.go` — git_status, git_diff, git_log, git_blame
   - `web.go` — web_search, web_fetch, ai_search
   - `browser.go` — 5 browser tools + `ShutdownBrowser()` + `SetBrowserEnabled()`
   - `multimodal.go` — read_image
   - `registry_test.go`

4. **`backend/internal/llm/`** (6 files):
   - `gateway.go` — Multi-provider router. `LLMProvider` interface. `Router` for task-type-based provider selection. `PromptCache`. `Embedder` adapter.
   - `openai_provider.go` — OpenAI-compatible APIs (OpenAI, DeepSeek, Ollama, Qwen, etc.)
   - `anthropic_provider.go` — Anthropic Claude API. Stream goroutine has `ctx.Done()` guard on channel sends.
   - `embedder.go` — Embedding interface + helpers
   - `cache.go` — 24h TTL prompt cache with hourly cleanup goroutine

5. **`backend/internal/memory/`** — File-based JSON memory. 3 levels (L1/L2/L3). Semantic search (cosine similarity) → TF-IDF → text match. `Embedding` persisted in JSON.

6. **`backend/internal/config/`** — JSON + YAML config. `Storage.DataDir` defaults to `./data`. All data (prompts, sessions, memory, knowledge, skills, plugins, checkpoints, traces, events, cache) stored under this directory.

7. **`backend/internal/api/`** — HTTP REST API + WebSocket. Routes: `POST /api/v1/chat`, `POST /api/v1/chat/stream` (SSE), `GET /api/v1/sessions`, etc. WebSocket heartbeat uses `done` channel for clean shutdown.

8. **`backend/internal/orchestration/`** (7 files) — Full task lifecycle management:
   - `orchestrator.go` — `ProcessQuery` → LLM planning → DeepPlan pipeline. `executePlan` with **self-correcting loop**: test→review→analyst(root cause)→coder(fix)→retry (until pass or BLOCKED). **Branch-converge model**: sub-agent signals `[DISCOVERY:]` → branch(analyze+fix) → converge back to main line → update remaining steps. `CleanupOldSessions` for sub-agent session GC.
   - `planner.go` — `Planner`: Research (multi-round hypothesis→verify→report, read-only tools), Propose (2-3 alternatives with reasoning/pros/cons/risk/effort), PlanWithApproach (detailed plan from chosen approach).
   - `team.go` — Team with LLM-defined dynamic roles. `GenerateRoles`: LLM analyzes the task and outputs custom roles (name, description, prompt, tools) as JSON. Falls back to 4 default roles (analyst/coder/reviewer/executor). `routePipeline`: LLM assigns all subtask roles at once.
   - `team_roles.go` — `GenerateRoles()` prompts LLM to define task-specific roles from scratch. Available tools list injected so LLM assigns minimal tool sets. `AddRole()` for manual role injection.
   - `subagent.go` — `RunSubAgent`: isolated session + mini ReAct loop (10 rounds) + role-filtered tools + budget injection + core rules injection + model routing (reasoning vs execution). Sub-agents can actually read, write, and execute — not just return text.

### Unified Query Analysis (single-pass pre-execution)

Before the ReAct loop, `analyzeQuery()` (in `kernel_prompt.go`) makes ONE holistic LLM call that replaces what was previously 3-4 fragmented micro-judgments:

```
analyzeQuery(query, available_skills):
  → TaskType (coding/review/think/debugging/general)
  → SkillID (matched skill, or empty)
  → Complexity (estimated rounds)
  → Strategy (one-sentence approach hint)
```

**Before**: `detectTaskType()` + `SkillActor.DetectSkill()` + `AdaptiveRounds.estimateWithLLM()` — three independent 30-token calls, each blind to the others' context.

**After**: One structured call sees the full picture (query + all skills + their tools) and makes a coherent decision. The result (`*QueryAnalysis`) is stored as a local variable in `ProcessStream` and passed explicitly through the call chain:
- `detectTaskType()` → always calls LLM (lightweight classification, ~30 tokens)
- `SkillActor.UsePreMatch(skillID)` → `DetectSkill` returns pre-matched skill without LLM
- `determineMaxRounds()` → receives `analysis.Complexity` as parameter
- `doReflection()` → receives `analysis` as parameter for post-processing decisions
- Falls back gracefully to individual LLM calls when analysis fails.

### LLM-driven decision making (zero hardcoded rules)

All judgment calls delegated to LLM. No keyword matching, no regex routing, no substring heuristics:
- **Task type classification** (`detectTaskType`): LLM classifies query into coding/review/teaching/research/general (was 45-keyword scoring)
- **Role generation** (`GenerateRoles`): LLM defines custom roles for each task — not picking from a preset list, but designing role names, prompts, and tool sets from scratch based on the task at hand. Skills, plugins, and user config can inject additional roles via `AddRole()`.
- **Pipeline routing** (`routePipeline`): LLM selects needed roles per task from available roles (dynamic or default)
- **Model routing**: LLM `route:"execution"/"reasoning"` options (was regex patterns on query text)
- **Tool risk assessment** (`assessWithLLM`): LLM evaluates tool arguments; exact match parsing (was `Contains("safe")` vulnerable to "unsafe")
- **Skill detection** (`detectWithLLM`): LLM semantic skill matching (was keyword substring scoring + 18-entry keyword map)
- **Round estimation** (`AdaptiveRounds.estimateWithLLM`): LLM judges task complexity (was keyword+length heuristics)
- **Session titles** (`generateSessionTitle`): LLM generates meaningful 3-5 word titles async (was 25-char truncation)
- **Eval judging**: LLM-as-judge evaluates response quality against natural-language criteria (was `MustContain` keyword matching)
- **Build error analysis**: LLM reflection extracts conventions (was 7 Go-specific substring patterns)
- **Plugin keywords**: auto-derived from name + description words (was 18-entry enKeywordMap + 40-term hardcoded list)
- **User preference** (`detectPreferenceWithLLM`): LLM classifies interaction type (was "代码"/"code" substring)

### Concurrency architecture

**CSP Actor pattern** (zero-lock, single-goroutine ownership): Stateful modules own their data in a single goroutine — external access via channels. Generic Actor + ActorStore[K,V] + SafeMap[K,V] live in `kernel/actor/` (used by 12+ packages). Lock count: 50→19. Per-request state passed as parameters — no shared mutable state on AgentKernel.

**Channel-first design**: "Share memory by communicating" — data flows through channels, not shared maps with locks. Event persistence uses a single worker goroutine with buffered channel (no per-event goroutine spawn). Tool registries own their channels. Locks reserved for the few cases where they're the right tool (LSP ID routing, file I/O).

**Goroutine philosophy**: Every goroutine has a clear lifecycle — started, stopped, or tied to a context. No fire-and-forget goroutines without panic recovery. No global goroutine pools. `context.WithTimeout` for safety, `context.WithoutCancel` for async cleanup.

Key fixes from audit:
- **Deadlock**: SkillManager RLock held during LLM call → fixed by moving LLM call outside actor
- **Deadlock**: RepoMap Write Lock → RLock reentrancy on same mutex → fixed by removing inner lock
- **Goroutine leaks**: browser Chrome processes, rate limiter ticker, prompt cache cleanup → all fixed
- **Event dispatch**: time.After timer leak → context.WithTimeout for automatic cleanup
- **Dead code removed**: traceMu (unused sync.Mutex), eventSem (unnecessary channel semaphore)

### LLM Context Compression

- **`kernel/compress.go`** — `SimpleCompressor`: separates system messages from history, keeps last 4 messages verbatim, compresses older messages into a `[历史对话摘要]` by truncating each to 50 chars + joining with `;`. Falls back when LLM compression fails.
- **`compress/llm_compressor.go`** — `LLMCompressor`: triggered each ReAct round when `EstimateTokens(messages) > maxTokens`. Sends old messages to LLM for semantic compression with a structured prompt:
  - **Retention priorities** (ordered): user intents → decisions/agreements → technical facts (file paths, errors, code patterns) → current task state → tool call results
  - **Explicit discard rules**: greetings, boilerplate tool output, redundant confirmations, already-corrected failed attempts
  - **Output format**: structured `[用户意图] [关键事实] [当前状态] [注意事项]`, under 200 words, matching user's language
  - **Parameters**: `max_tokens=400`, `temperature=0.2` for deterministic summaries
  - `extractPendingQuestions()`: detects unanswered user queries in compressed messages, re-injects as `[待解决问题]` to prevent information loss
  - **Fallback chain**: LLM call fails → `SimpleCompressor` → returns original messages if everything fails

### Deep thinking pipeline (Research → Propose → Select → Plan)

OpenAIDE's unique value: deep pre-execution analysis before any code changes.

- **Multi-round Research**: Hypothesis-driven — scan code → form hypothesis → read deeply → verify → report. Only read-only tools allowed.
- **Propose alternatives**: LLM generates 2-3 approaches with pros/cons/risk/effort/reasoning. User selects from TUI overlay.
- **Decision rationale**: Each proposal includes a `reasoning` field explaining *why* this option exists and when to choose it.
- **Self-reflection loop**: After review, if `[需要返工]` is detected, auto-loops back to coder for fixes (max 2 retries).
- **Smart role generation**: `GenerateRoles()` LLM defines custom roles per task. `routePipeline()` LLM assigns subtasks to roles. Fallback to 4 defaults.
- **Context-isolated sub-agents**: `RunSubAgent()` creates fresh sessions per role — main agent sees only result summaries, not intermediate tool calls.
- **Editable plans**: Foundation for user editing subtask order/add/delete in TUI overlay.

### Ecosystem compatibility

OpenAIDE reads and integrates with other agent ecosystems:

- **Rule files**: Auto-loads OPENAIDE.md, CLAUDE.md, CODEBUDDY.md, CONVENTIONS.md, `.github/copilot-instructions.md`, `.cursor/rules/*.md` — write once, works across OpenAIDE + Claude Code + Cursor + Aider + OpenCode + Copilot.
- **Claude Code plugins**: Full format support (manifest, skills, MCP, hooks). See plugin system section.
- **OpenCode config**: Auto-discovers `opencode.json` in project root — imports MCP servers + instructions.
- **MCP protocol**: Universal standard across all major coding agents. Supports stdio (subprocess, 30s timeout) and HTTP/SSE (POST to /message). Full MCP lifecycle: initialize → initialized notification → tools/list → tools/call → shutdown. Handles text, image, and resource content types.

### Continuous learning (ProjectMind — gets smarter every task)

OpenAIDE accumulates project knowledge across sessions via `internal/projectmind/`:

- **CodeMap**: file→purpose mapping with confidence scoring. Auto-populated from Research phase discoveries. Confidence decays over time (7d: 0.8×, 30d: 0.5×), triggering re-verification.
- **RiskMap**: known fragile areas with fix status. Planner injects risks into task decomposition — high-risk files automatically get reviewer attention.
- **Conventions**: automatically learned from build/test errors. Detects naming patterns (camelCase vs snake_case), import conventions, test frameworks (testify), error handling style. Confidence builds with repeated observation (0.6→1.0). Injected into every sub-agent prompt.
- **Execution History**: records every task (success, errors, fixes, time, model). Keeps last 50. Feeds `RecentFailures()` into prompts so agents learn from past mistakes.
- **Strategy Effectiveness**: tracks which approaches succeed for which task types. Propose phase receives historical success rates — LLM makes data-informed choices ("方案C在重构场景成功率100%, 方案B在类似任务失败过").
- **Discovery signals**: sub-agent output containing `[DISCOVERY: ...]` or `[REPLAN: ...]` automatically captured and persisted.
- **Model routing**: `llm.model_routing.reasoning` / `execution` in config.yaml. Analyst/reviewer → reasoning model (pro, deep thinking). Coder/executor/classifier → execution model (flash, fast). Synthesis and small classification calls also use execution model for cost efficiency.

### Process Supervision (Let's Verify Step by Step, 2023)

After each ReAct loop, `doReflection` sends the FULL message history to LLMReflection for per-step evaluation:

- **Step-by-step trace**: Each ReAct round (LLM thought → tool called → tool result) is included
- **Per-step scoring**: Identifies BEST decision (what to reinforce) and WEAKEST decision (what to fix)
- **Precision**: "Round 2's tool choice was suboptimal" vs old "7/10 overall"

### Tree of Thoughts (2023)

`ExploreAlternatives` forks multiple solution approaches at decision points:

- **Branch**: 2-4 parallel sub-agent explorations, each guided by a different approach prompt
- **Evaluate**: LLM compares branch findings and selects the best approach
- **Learn**: Failed branches recorded in ProjectMind for future strategy effectiveness tracking
- Called from DeepPlan when Proposals phase generates alternative approaches

### Self-Rewarding (2024)

LLMReflection learns per-task-type evaluation criteria:

- **Criteria per task type**: coding, review, teaching, research each have their own evaluation standards
- **Self-improvement**: After each reflection, LLM can update criteria via `CRITERIA:` prefix in suggestions
- **Adaptive scoring**: Later reflections use previously learned criteria, making evaluations more precise over time

### Evaluation Framework (LLM-as-Judge)

`backend/internal/eval/` — Benchmark evaluation with LLM semantic judging:

- **LLM-as-Judge**: Uses flash model to judge response quality against natural-language `EvalCriteria` — no keyword matching.
- **Fallback chain**: Execution model → default model → error (single-model configs work).
- **Quick pre-checks**: `MustNotContain` (disqualifying patterns) and `MinToolCalls` (tool usage) run before judge.
- **Task suites**: `BuiltinTasks()` (10 tasks, 3 difficulties), `FullCapabilityTasks()` (7 tasks covering all agent capabilities: read, search, write, diff edit, knowledge RAG, memory, architecture synthesis).
- **CLI**: `go run ./cmd/eval` with `-full`, `-quick`, `-category`, `-output` flags.
- **Compare**: `eval.Compare(before, after)` detects regressions and fixes across runs.

### Memory Management (MemGPT, 2023)

Agent actively manages its own memory via the `manage_memory` tool:

- **archive**: Store completed conversation summaries in archival storage with embeddings
- **retrieve**: Search archived conversations by embedding similarity
- **remember**: Persist important facts to core memory (survives all sessions)
- **recall**: Retrieve core facts by importance and recency

Budget injection now hints: "If you've completed subtasks, use manage_memory(action='archive')".

### Agent-Computer Interface (SWE-Agent, 2024)

All tools return structured, agent-friendly output:

- **diff_edit**: Shows before/after comparison with line numbers + write verification
- **write_file**: Line-numbered preview of written content + read-back verification
- **read_file**: Already ACI-compatible — file path, line count, numbered lines
- **search_files**: `file:line: matched_content` format with match count
- **execute_command**: `[exit=N]` prefix with stdout/stderr separation

### Prompt system

- **Layered architecture**: Stable prefix (L0 Identity + L1 Project + L2 Skill) cached in system message. Dynamic tail (L3 Mode Signal + L5 Reflection + L6 Knowledge RAG) appended per-query.
- **L0**: Core rules — Hard Blocks + Grounding Protocol + Certainty Labels + **Coding Workflow — Simplicity First** (with Resource Lifecycle rule) + Engineering Checklist (with Simplicity + Resource Lifecycle checks) + Review Mode + Debugging Mode + Error Recovery + Interaction + Learning (~30 rules, ~600 tokens).
- **L1**: Project context — working directory, git branch, OPENAIDE.md loading, RepoMap. Auto-detects 21 languages (go/java/python/rust/c/c++/c#/swift/kotlin/node/php/ruby/scala/dart/elixir/haskell/erlang/ocaml/r/lua/julia/perl) and injects language-specific conventions.
- **L3**: Mode activation signal injected per-query (dynamic tail). `detectTaskType()` uses LLM to classify into 5 categories: coding/review/think/debugging/general. Each mode is a 2-3 line activation signal — all real rules live in L0. No duplication between layers.
- **Engineering Checklist** (L0): When modifying code, agent proactively checks simplicity (did I add new files? was that necessary?), resource lifecycle (every Thread/Handler/AsyncTask has cleanup), test runnability, config strictness, CI/CD, data backward compatibility, and test coverage gaps.
- **Auto-verification** (kernel): After coding and before "done", kernel auto-detects project test command (go.mod → `go test ./...`, package.json → `npm test`, etc.), runs it, and injects failures back into the ReAct loop for automatic fixing (max 3 retry rounds).
- **User customization**: Add `.md` files to `~/.openaide/data/prompts/user/` — auto-appended after system layers. Never overwritten on upgrade. Onboarding creates commented templates.
- **System prompts**: Always built-in (compiled Go code). Upgrades apply immediately — no disk files to conflict.
- **Default prompt**: Bilingual (Chinese/English), auto-detected from `LANG` env var.
- **Runtime hot-reload**: `AgentKernel.SetSystemPrompt()` allows prompt changes without kernel restart. `ConfigReloader` hot-reloads all settings including `MaxRounds`, `MaxTokens`, `MinRounds`, `MaxRoundsCap` without restart.
- **Skill prompts**: LLM semantic skill detection per-query via `skillActor.DetectSkill()`, then injected via `skillActor.InjectPrompt()`.

### Plugin system (Claude Code compatible)

OpenAIDE is fully compatible with the [Claude Code official plugin specification](https://github.com/anthropics/claude-plugins-official). Drop a Claude plugin directory into `./data/plugins/` and it is auto-discovered.

**Supported plugin components:**

| Component | File | Phase | Description |
|-----------|------|-------|-------------|
| Manifest | `.claude-plugin/plugin.json` | Phase 1 | Plugin metadata (name, version, description) |
| Skills | `skills/*/SKILL.md` | Phase 1 | YAML frontmatter + Markdown body. Auto-discovered, registered as slash commands (`/skill-name`). Keywords auto-generated from name + description (Chinese + English). Claude tool names mapped to OpenAIDE equivalents (Read→read_file, Bash→execute_command, etc.) |
| MCP Servers | `.mcp.json` | Phase 2 | Declarative MCP server config: `{name: {type, command, args, url, env}}`. Both stdio (subprocess) and SSE (HTTP POST to /message) transports supported. Tools auto-registered with `mcp_` prefix. Env vars passed to stdio subprocesses. |
| Hooks | `hooks/hooks.json` | Phase 2 | Event-driven shell commands: `[{event, command, tools}]`. Claude events mapped to OpenAIDE: `PreToolUse→tool_call_started`, `PostToolUse→tool_call_ended`, `Stop→session_ended`, `SessionStart→session_created`. 30s timeout per hook, non-blocking goroutine. Tool name filter support with reverse mapping. |

**Code locations:**
- `plugin/plugin.go` — Manager: JSON format plugins + Claude format discovery via `loadClaudeFromDisk()`. `Reload()` with `Enabled` state preservation (no overwrite). `RunMessageHooks()` and `RunEventHooks()` for hook execution pipeline.
- **`backend/internal/mcp/mcp.go`** — MCP client: `Transport` interface (Call/Notify/Close), `stdioTransport` (subprocess, 30s timeout), `httpTransport` (MCP Streamable HTTP POST /message). Full lifecycle + content type handling (text/image/resource). `Manager` + `EnvMap` helper.
- `plugin/plugin_claude.go` — All Claude format parsing: `DiscoverClaudePlugins()`, `DiscoverClaudeSkills()`, `DiscoverClaudeMCP()`, `DiscoverClaudeHooks()`, YAML frontmatter parsing, tool name mapping, auto keyword generation from name+description, event mapping, script discovery
- `kernel/skill_actor.go` — `AddClaudeSkill()` preserves runtime stats on re-add (Confidence/Usage/Success/Enabled survive hot-reload). `UsePreMatch()` for unified query analysis integration. `ListEnabled()` and `GetSkill()` for external skill inspection.
- `infra/app.go` — MCP wiring: `DiscoverClaudeMCP()` → `mcpManager.ConnectServer()` → `registerMCPTools()`. PluginWatcher with unified reload callback.
- `infra/app_kernel.go` — Skills + Hooks wiring at startup. Hook execution injects `OPENAIDE_EVENT`, `OPENAIDE_TOOL_NAME`, `OPENAIDE_SESSION_ID` as env vars.
- `infra/plugin_watcher.go` — fsnotify-based directory watcher with debounce. Triggers unified reload (plugin manager + skills) on new directory creation.

**Hot-reload**: `PluginWatcher` in `infra/app.go` watches the plugins directory via fsnotify. New directories trigger a single unified reload callback that rescans both plugin metadata AND Claude skills. `AddClaudeSkill` preserves accumulated stats (Confidence, UsageCount) on re-add — no 5-minute stats reset.

**Hook environment variables**: Hook commands receive:
- `OPENAIDE_EVENT` — mapped event type (e.g., `tool_call_started`)
- `OPENAIDE_TOOL_NAME` — name of the tool being called/applied
- `OPENAIDE_SESSION_ID` — current session identifier

**Ecosystem compatibility:**
- Full Claude Code plugin format support (manifests, skills, MCP, hooks)
- Bridge to OpenCode/Codex/Cursor via `acplugin` conversion tool
- MCP: universal standard across all 5 major coding agents

### Installing plugins

**Method 1: Claude official plugins**

```bash
# Clone the official repository
git clone https://github.com/anthropics/claude-plugins-official.git /tmp/claude-plugins

# Copy any plugin you want into your project's plugins directory
cp -r /tmp/claude-plugins/plugins/code-review ./data/plugins/

# Or install globally (shared across all projects)
cp -r /tmp/claude-plugins/plugins/code-review ~/.openaide/plugins/
```

**Method 2: Any Claude-compatible plugin from the community**

```bash
# Plugins from GitHub, npm, or any source — just copy the directory
cp -r ~/Downloads/some-plugin ./data/plugins/
```

**Method 3: One-liner install from GitHub**

```bash
# Clone a plugin repo directly into the plugins directory
git clone https://github.com/user/some-plugin.git ./data/plugins/some-plugin
```

**Where to find plugins:**
- [Claude Plugins Official](https://github.com/anthropics/claude-plugins-official) — 30+ internal + 15+ partner plugins (LSP, Git, PR review, Firebase, Linear, Terraform, etc.)
- [Superpowers](https://github.com/obra/superpowers) — most popular cross-agent skill framework (15k+ stars), supports Claude Code + OpenCode + Codex
- npm: `npx acplugin convert` — converts Claude plugins to OpenCode/Codex/Cursor formats (and vice versa)
- Search GitHub: [`claude-plugin` topic](https://github.com/topics/claude-plugin)

**Verification:**

```bash
# Start OpenAIDE, check logs for plugin discovery
openaide --verbose 2>&1 | grep -i "claude\|plugin\|hook"

# Skills appear as slash commands: type / in the TUI to see registered skills
```

**Plugin directory structure recognized by OpenAIDE:**

```
./data/plugins/
├── my-json-plugin.json              ← Legacy JSON format
└── code-review/                     ← Claude format (directory)
    ├── .claude-plugin/
    │   └── plugin.json              ← Required: {name, version, description}
    ├── skills/
├── project_mind.json          ← cross-session facts (CodeMap, RiskMap, Conventions)
    │   └── review/
    │       └── SKILL.md             ← YAML frontmatter + Markdown body
    ├── .mcp.json                    ← Optional: MCP server config
    └── hooks/
        └── hooks.json               ← Optional: event-driven shell commands
```

### Configuration & data layout

```
~/.openaide/config.yaml       ← global config (LLM providers, keys, server port)
~/.openaide/data/              ← global data (default ~/.openaide/data)
├── sessions.db                ← SessionActor (SQLite, WAL mode)
├── memory.db                  ← MemoryActor (SQLite + vector cache)
├── prompts/
│   └── user/                  ← user custom prompts (template on first install)
├── plugins/                   ← Claude-format plugins (hot-reload via fsnotify, preserves stats)
├── skills/
├── project_mind.json          ← cross-session facts (CodeMap, RiskMap, Conventions)
│   └── auto_skills.json       ← persisted skills (Claude skills + manual)
├── checkpoints/               ← session checkpoints (JSON files)
├── traces.jsonl               ← execution traces (append-only)
├── events/                    ← persisted events (optional)
└── cache/                     ← prompt cache (in-memory LRU, 500 entries)
```

**Provider config**: Two providers recommended for Architect/Editor pattern:
```yaml
providers:
  - name: deepseek             # Architect — deep reasoning
    default_model: deepseek-v4-pro
    timeout: 300
    # embedding_model: text-embedding-3-small  # optional: for memory semantic search
  - name: deepseek-flash       # Editor — fast execution
    default_model: deepseek-v4-flash
    timeout: 120
model_routing:
  reasoning: deepseek-v4-pro   # analyst, reviewer, reflection
  execution: deepseek-v4-flash # coder, executor, synthesis, classification

# MCP: Model Context Protocol servers (optional)
mcp:
  enabled: true
  servers:
    # Stdio transport — spawn a local subprocess
    - id: filesystem
      command: npx
      args: ["-y", "@anthropic/mcp-server-filesystem", "/path"]
      env:
        HOME: /home/user
    # SSE/HTTP transport — connect to a remote MCP server
    - id: tavily
      type: sse
      url: https://mcp.tavily.com/sse

# Kernel: ReAct loop + reflection + skill management
kernel:
  reflection_enabled: true     # LLM post-execution reflection (default true)

# Storage: default is SQLite
storage:
  data_dir: ~/.openaide/data
  session_store: sqlite        # "sqlite" (default), "file", "memory"
```

### CLI (REPL)

**REPL mode** — `openaide`:
- `cmd/cli/repl.go` — Rich terminal REPL with lmorg/readline:
  - Readline with file-backed history (~/.openaide/history), tab completion, hints
  - Markdown rendering via glamour (headers, code blocks with Chroma, tables)
  - pterm: progress bars, spinners, colored messages
  - Claude-style approval: 3-option select (Allow/Allow All/Deny)
  - ESC: cancel active request; idle → undo last user+assistant message pair
  - Ctrl+C: first press cancels request, second press exits
  - Smart routing: PreviewPlan → direct ReAct or team execution
  - Streaming display: deduplicated tool names, max 2 think lines
  - Visual separator between response and next prompt
  - Simplified prompt (session ID + ❯, model in banner)
  - /clear deletes session + starts fresh (no confirmation)
  - Slash commands: `/analyst`, `/coder`, `/reviewer`, `/executor`, `/team`, `/auto`
  - Session resume: `-c` flag loads last non-empty session
  - Version display: `openaide --version` shows build info from ldflags
- `cmd/cli/repl_output.go` — pterm/ANSI styling, glamour renderer, output helpers
- `cmd/cli/setup.go` — interactive setup wizard (language→provider→API key→model)

### Adaptive ReAct loop (Claude Code style)

No artificial round limits. The LLM decides when to stop. Budget hints at rounds 10, 20, 50 — gentle reminders, not hard limits. 200-round safety net. Sync path (`Process`) is a thin wrapper over `ProcessStream` — both share one canonical ReAct implementation in `kernel_stream.go`. `finalizeResponse` unifies post-loop work (save memory, update session, generate title, reflection). **Auto-verification** runs before `finalizeResponse`: kernel auto-detects project test command, executes it, and injects failures back into the ReAct loop (max 3 rounds).

### User Experience

- **LLM auto-approval**: Dangerous tools (e.g., `execute_command`) trigger LLM risk assessment. `AutoApprover.assessWithLLM()` evaluates the command — safe commands (ls, pwd, cat) auto-approved, risky commands (rm, format) prompt user, dangerous commands blocked. No more tedious manual approval for safe operations.
- **Error messages**: `humanizeError` wraps API errors with human-readable tips (429→quota, 401→bad key, timeout→network, deadline→slow model).
- **Tool visibility**: One-shot streaming mode shows tool calls on stderr as they execute.
- **Session continuity**: `getOrCreateSession` auto-resumes the most recent session. Use `-c` flag for explicit continuation.
- **Config validation**: `validate()` warns on missing API keys and placeholder values at startup.
- **Cross-session learning**: `SeedFromHistory` pre-loads the pattern detector with query history from past sessions on startup. Pattern detection survives restarts.

### Architect/Editor model routing

- **Architect** (analyst, reviewer) → reasoning model (pro, with thinking)
- **Editor** (coder, executor, classifier) → execution model (flash, no thinking)
- Synthesis/summarization → flash
- Skill detect, adaptive rounds estimation → flash (`route: "execution"`)
- `pickModel()` in orchestrator; `findProviderForModel()` in gateway auto-matches provider

### CSP Actor Architecture (Go-native concurrency)

All stateful modules use Go's CSP model: each module owns its data in a single goroutine, and external access goes through channels. **Zero locks** for core data paths. Lock count reduced from 50→19.

| Module | Actor | Backend | Notes |
|--------|-------|---------|-------|
| Sessions | `SessionActor` | SQLite (`sessions.db`) | WAL mode, search, pagination |
| Skills | `SkillActor` | In-memory | LLM detection outside actor |
| Memory | `MemoryActor` | SQLite (`memory.db`) | Batch embedding, vector cache |
| Checkpoint | `FileCheckpointer` | File JSONL | Actor serializes file access |
| Tracer | `FileTracer` | File JSONL | Actor serializes file access |
| Rate Limit | `RateLimiter` | In-memory | Token bucket via channel |

**Pattern**: `NewActor(bufSize)` → `Send(fn)` / `SendAsync(fn)` → `Stop()`. Each actor is a `for { select { case cmd := <-ch } }` loop.

**SafeMap[K,V]**: Generic wrapper around `RWMutex + map[K]V`. Used for write-once-read-many maps (gateway providers, plugin registry, channel handlers, MCP clients, auth tokens).

**atomic.Value**: Used for read-heavy single-value state (systemPrompt, kernel state, language, tool registry).

### Storage Layout

```
~/.openaide/data/
├── sessions.db        ← SessionActor (SQLite, default)
├── memory.db          ← MemoryActor (SQLite)
├── prompts/user/      ← user custom prompts (editable templates)
├── plugins/           ← Claude-format plugins (hot-reload via fsnotify, preserves stats)
├── skills/
├── project_mind.json          ← cross-session facts (CodeMap, RiskMap, Conventions)
│   └── auto_skills.json       ← persisted skills (Claude skills + manual)
├── skills/            ← custom skills
├── project_mind.json          ← cross-session facts (CodeMap, RiskMap, Conventions)
├── traces.jsonl       ← execution traces
├── checkpoints/       ← session checkpoints
├── events/            ← persisted events (optional)
└── cache/             ← prompt cache (in-memory LRU, 500 entries)
```

### Tool concurrency safety

- `parallelSafeTools` map: read-only tools (read_file, search_files, git_*, etc.) can run in parallel
- Write tools (write_file, diff_edit, execute_command) serialized between batches
- Partitioning in both sync (`kernel_process.go`) and stream (`kernel_stream.go`) paths

### Tool output snipping (Claude Code style)

- `snipOldToolOutputs()`: keep last 4 tool results intact, older ones snipped to head(500) + tail(500)

### Lint/Repair loop (Aider-style)

- After coder fixes, auto-runs linter: golangci-lint (Go), eslint/ruff/pylint (JS/Python)
- New errors fed back to coder for fixing (max 3 retries, error deduplication)
- `lintRepairLoop()` in `orchestrator.go`, `runLint()` auto-detects project language

### RepoMap (Aider-style code symbol map)

- `repomap.go`: regex-based symbol extraction from Go files (func, type, const, var)
- Injected into system prompt as `[RepoMap] 项目符号地图: file: symbol1, symbol2, ...`
- 5-min TTL cache; LLM sees code structure without exploration rounds
- Skips dot-dirs, vendor, node_modules, bin

### Key patterns

- All inter-module communication uses interfaces defined in `kernel/interfaces.go`
- LLM Gateway implements `kernel.LLMProvider` — passed directly to kernel, planner, skill manager
- File session store default (crash-recoverable); `SessionStoreAdapter` is fallback
- All 43 tool handlers implemented across 18 source files; no stubs
- DeepSeek behavior gated by `isDeepSeek()` checking base URL + provider name
- Prompt: file-based with bilingual fallback; `SetSystemPrompt()` hot-reloads without restart
- First-run onboarding: template → LLM interview → TUI
- LLM as decision engine: removed all keyword/rule-based judgment; LLM handles role assignment, risk assessment, skill matching, complexity estimation, task planning
- Context isolation: sub-agents use unique session IDs; main agent sees only result summaries
- Read-only Plan Agent: Research phase restricted to 8 read-only tools (OpenCode pattern)
- Plugin compatibility: full Claude Code official format (skills, MCP, hooks)

### Frontend

`frontend/index.html` — Single-page vanilla JS app. ES module imports from `frontend/src/`. Pages: chat, templates, dashboard, scheduled tasks, models. Components: ThinkingVisualizer, CorrectionPanel, ToolCallDisplay. Served via `frontend/serve.py` (Python HTTP server on port 8000).

### CI/CD

`.github/workflows/build-deploy.yml` — Triggers on push to `master` and `v*` tags. Builds static Go binaries with `CGO_ENABLED=0`, runs tests, packages release tarball with self-extracting installer. On `v*` tags: creates GitHub Release. Optional deploy over SSH with health check.
