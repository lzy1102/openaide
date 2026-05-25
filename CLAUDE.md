# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common commands

```bash
# Build both binaries (openaide-server, openaide)
make build
# or from backend/
cd backend && make build

# Run all tests
make test
# or a single package
cd backend && go test -v ./internal/kernel/...

# Run tests with coverage
cd backend && make test-coverage

# Format and lint
cd backend && make fmt
cd backend && make lint

# Run the server locally
cd backend && make run

# Run the CLI interactively
cd backend && go run ./cmd/cli
```

## Architecture

OpenAIDE is an AI Agent kernel platform in Go. The architecture is strictly layered:

```
cmd/server (API server)          cmd/cli (interactive CLI)
         \_________________________/
                     |
              infra/Application    ← DI container, wires everything
              /    |    |     \
         api/  orchestration/  channel/   (HTTP, SSE, WebSocket, webhook/Feishu/Telegram)
                     |
              kernel/AgentKernel   ← ReAct loop, the core of the agent
              /    |    |     \
        llm/   tools/ memory/  kernel/types
```

### Entry points

- **`backend/cmd/server/main.go`** — Production server. Loads config from `~/.openaide/config.yaml`, starts the HTTP API server.
- **`backend/cmd/cli/main.go`** — Interactive CLI. Defaults to **REPL mode** (rich terminal readline with markdown rendering). `--tui` flag for Bubble Tea TUI. `-c` flag resumes last session. On first run, triggers interactive onboarding (`onboard.go`). One-shot mode: `openaide "fix this bug"`.
- **`backend/cmd/cli/onboard.go`** — First-run onboarding. Template questions (role/style/language) → `NewApplication()` → LLM interview (2-round open-ended dialogue) → generates custom `system.md` → hot-reloads via `SetSystemPrompt()`.

### Layered design

1. **`backend/internal/infra/`** (4 files) — Application container, split by concern:
   - `app.go` — `Application` struct, `NewApplication` (~150 lines), `Start`, `Stop`. MCP wiring from both `config.yaml` and Claude `.mcp.json` plugins.
   - `app_llm.go` — `createLLMGateway()` — provider registration, router, prompt cache
   - `app_kernel.go` — `createKernel()` — kernel + all enhancements. Claude skills injection (`DiscoverClaudeSkills()` → `AddClaudeSkill()`). Claude hooks wiring (`DiscoverClaudeHooks()` → `agentKernel.Subscribe()` with shell execution).
   - `app_channels.go` — `setupChannels()` — webhook/Feishu/Telegram, task queue

2. **`backend/internal/kernel/`** (split from single monolithic file):
   - `kernel.go` — `AgentKernel` struct, Config, constructor, all Set* methods, state management, event system, session/message/tool helpers. Includes `SetSystemPrompt()` for hot-reloading prompts at runtime.
   - `kernel_process.go` — `Process()` sync path + `doReflection()` + `autoSaveKnowledge()`
   - `kernel_stream.go` — `ProcessStream()` streaming path (now has skill manager tool filter parity with Process)
   - `kernel_prompt.go` — `defaultSystemPrompt{EN,ZH}()` hardcoded defaults, `LoadSystemPrompt(dir)` file-based loading with language suffix (`system.zh.md`/`system.en.md`), `IsFirstRun()`, `WriteSystemPrompt()`, `IsZhEnv()`
   - `interfaces.go` — All kernel-level interfaces
   - `types.go` — Shared types: `Message`, `ToolCall`, `Query`, `Response`, `StreamChunk`, `Event`, `Session`
   - Other files: `reflection.go`, `llm_reflection.go`, `learner.go`, `pattern.go`, `compress.go`, `checkpoint.go`, `approval.go`, `adaptive.go`, `session_store.go`, `skill.go`, `skill_evolution.go`, `tracer.go`

3. **`backend/internal/tools/`** (9 files) — Tool definitions and handlers split by domain:
   - `registry.go` — `Registry` framework, `BuiltinTools()` concatenates domain-specific defs, `BuiltinHandlers()`, `RegisterBuiltins()`, `safeAbsPath()`, `formatBytes()`
   - `tools_filesystem.go` — read_file (with offset/limit), write_file, execute_command, list_directory, search_files
   - `tools_knowledge.go` — search_knowledge, add_knowledge, `KnowledgeAccessor` interface, `WithKnowledge()`
   - `tools_symbol.go` — search_symbols
   - `diff_edit.go` — diff_edit, diff_edit_lines
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

8. **`backend/internal/orchestration/`** (3 files) — Full task lifecycle management:
   - `orchestrator.go` — `ProcessQuery` → LLM planning → DeepPlan pipeline. `executePlan` with **self-correcting loop**: test→review→analyst(root cause)→coder(fix)→retry (until pass or BLOCKED). **Branch-converge model**: sub-agent signals `[DISCOVERY:]` → branch(analyze+fix) → converge back to main line → update remaining steps. `ExecuteWithPlan` accepts pre-made plans. `CleanupOldSessions` for sub-agent session GC.
   - `planner.go` — `Planner`: Research (multi-round hypothesis→verify→report, read-only tools), Propose (2-3 alternatives with reasoning/pros/cons/risk/effort), PlanWithApproach (detailed plan from chosen approach). `PreviewPlan` combines complexity classification + task splitting in single LLM call.
   - `team.go` — 4 bilingual roles (analyst/coder/reviewer/executor). `RunSubAgent`: fresh session + role-specific tools + model routing (reasoning vs execution). `routePipeline`: single LLM call assigns all subtask roles at once. Slash commands: `/analyst`, `/coder`, `/reviewer`, `/executor`, `/team`.

### LLM-driven decision making (replacing hardcoded rules)

All judgment calls now delegated to LLM, with rule-based fallbacks:
- **Role assignment** (`assignRole`): LLM picks best team role from task semantics (was 20+ keyword matches)
- **Pipeline routing** (`routePipeline`): LLM selects needed roles per task (was hardcoded coder→executor→reviewer)
- **Tool risk assessment** (`AutoApprover.assessWithLLM`): LLM evaluates tool arguments for safety (was hardcoded DangerousTools map)
- **Skill detection** (`detectWithLLM`): LLM semantic skill matching (was keyword substring scoring)
- **Round estimation** (`AdaptiveRounds.estimateWithLLM`): LLM judges task complexity (was keyword+length heuristics)
- **Session titles** (`generateSessionTitle`): LLM generates meaningful 3-5 word titles async (was 25-char truncation)
- **User preference** (`detectPreferenceWithLLM`): LLM classifies interaction type (was "代码"/"code" substring)
- **Router complexity**: removed 17-keyword indicator list; orchestrator LLM planning handles complexity
- **Reflection**: removed keyword-based quality scoring; LLMReflection handles evaluation with neutral fallback
- **Planner**: removed "≤5 subtasks" limit; LLM decides appropriate count
- **Compress**: replaced generic "error" substring with specific error markers

### Concurrency safety (audited and fixed)

- **`session_store.go`** — `SessionStoreAdapter` has `sync.RWMutex` protecting all 5 map-access methods (was missing, would fatal-panic under concurrent access).
- **`dag.go`** — `completed` map read moved inside `mu.Lock()` to prevent data race with concurrent writes.
- **`event.go`** — `events` slice capped at 10,000 via FIFO eviction; handler goroutines bounded by semaphore (max 32 concurrent).
- **`browser.go`** — `allocCancel` stored instead of discarded. `ShutdownBrowser()` added and called from `Application.Stop()` (was leaking Chrome processes).
- **`mcp.go`** — `stdout.Scan()` wrapped in goroutine with 30s timeout + `Process.Kill()` on timeout (was holding mutex across blocking I/O, could deadlock).
- **`kernel.go`** — `publishEvent()` handler goroutines have 5s timeout (was fire-and-forget, could accumulate).
- **`anthropic_provider.go`** — Stream goroutine channel sends guarded by `select { case <-resultChan: case <-ctx.Done() }` (was missing, could block forever).
- **`websocket.go`** — Heartbeat goroutine uses `done` channel for clean exit (was relying on write failure alone).
- **`indexer.go`** — All `Lock/Unlock` pairs use `defer` (3 places were manual, could leak on panic).
- **`cache.go`** — All `json.Marshal`/`os.WriteFile` errors now logged. `Shutdown()` stops cleanup goroutine. `Gateway.Shutdown()` wired to `Application.Stop()`.
- **`ratelimit.go`** — `time.Tick` replaced with `time.NewTicker` + `Shutdown()` (was unleakable goroutine).
- **`checkpoint.go`** — Max 5 checkpoints per session, auto-deletes old ones (was unbounded growth).
- **`kernel_stream.go`** — Main stream goroutine has `defer recover()` with error relay to prevent silent crashes.
- **`kernel_process.go`** — Synthesis calls use flash model (`route: "execution"`) to avoid DeepSeek thinking budget competition.
- **`api.go`** — Session authorization: verify `user_id` before GET/DELETE. `sendSSE()` handles marshal errors. `sanitizeParam()` prevents path traversal. Duplicate `/api/v1/state` endpoint removed.
- **`orchestration/orchestrator.go`** — `RunSubAgent` uses unique userID with nanosecond timestamp for true session isolation.

- **`session_store.go`** — `SessionStoreAdapter` has `sync.RWMutex` protecting all 5 map-access methods (was missing, would fatal-panic under concurrent access).
- **`dag.go`** — `completed` map read moved inside `mu.Lock()` to prevent data race with concurrent writes.
- **`event.go`** — `events` slice capped at 10,000 via FIFO eviction (was unbounded, would memory-leak on long-running servers).
- **`browser.go`** — `allocCancel` stored instead of discarded. `ShutdownBrowser()` added and called from `Application.Stop()` (was leaking Chrome processes).
- **`mcp.go`** — `stdout.Scan()` wrapped in goroutine with 30s timeout + `Process.Kill()` on timeout (was holding mutex across blocking I/O, could deadlock).
- **`kernel.go`** — `publishEvent()` handler goroutines have 5s timeout (was fire-and-forget, could accumulate).
- **`anthropic_provider.go`** — Stream goroutine channel sends guarded by `select { case <-resultChan: case <-ctx.Done() }` (was missing, could block forever).
- **`websocket.go`** — Heartbeat goroutine uses `done` channel for clean exit (was relying on write failure alone).
- **`indexer.go`** — All `Lock/Unlock` pairs use `defer` (3 places were manual, could leak on panic).

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
- **Smart routing**: `routePipeline()` LLM selects needed roles per task. `assignRole()` LLM picks best role per subtask.
- **Context-isolated sub-agents**: `RunSubAgent()` creates fresh sessions per role — main agent sees only result summaries, not intermediate tool calls.
- **Editable plans**: Foundation for user editing subtask order/add/delete in TUI overlay.

### Ecosystem compatibility

OpenAIDE reads and integrates with other agent ecosystems:

- **Claude Code plugins**: Full format support (manifest, skills, MCP, hooks). See plugin system section.
- **OpenCode config**: Auto-discovers `opencode.json` in project root — imports MCP servers + instructions.
- **MCP protocol**: Universal standard across all major coding agents (stdio transport, 30s timeout).
- **Plugin marketplace**: CLI command `openaide plugins [list|search|install <url>]`.

### Continuous learning (ProjectMind — gets smarter every task)

OpenAIDE accumulates project knowledge across sessions via `internal/projectmind/`:

- **CodeMap**: file→purpose mapping with confidence scoring. Auto-populated from Research phase discoveries. Confidence decays over time (7d: 0.8×, 30d: 0.5×), triggering re-verification.
- **RiskMap**: known fragile areas with fix status. Planner injects risks into task decomposition — high-risk files automatically get reviewer attention.
- **Conventions**: automatically learned from build/test errors. Detects naming patterns (camelCase vs snake_case), import conventions, test frameworks (testify), error handling style. Confidence builds with repeated observation (0.6→1.0). Injected into every sub-agent prompt.
- **Execution History**: records every task (success, errors, fixes, time, model). Keeps last 50. Feeds `RecentFailures()` into prompts so agents learn from past mistakes.
- **Strategy Effectiveness**: tracks which approaches succeed for which task types. Propose phase receives historical success rates — LLM makes data-informed choices ("方案C在重构场景成功率100%, 方案B在类似任务失败过").
- **Discovery signals**: sub-agent output containing `[DISCOVERY: ...]` or `[REPLAN: ...]` automatically captured and persisted.
- **KnowledgeBase sync**: `SyncToKnowledgeBase()` converts structured facts to searchable KB documents (tagged `projectmind`) — unified RAG injection via `buildMessages`.
- **Model routing**: `llm.model_routing.reasoning` / `execution` in config.yaml. Analyst/reviewer → reasoning model (pro, deep thinking). Coder/executor/classifier → execution model (flash, fast). Synthesis and small classification calls also use execution model for cost efficiency.

### Prompt system

- **Default prompt**: Bilingual (Chinese/English), auto-detected from `LANG` env var.
- **File-based loading**: `LoadSystemPrompt(dir)` → `system.{lang}.md` > `system.md` > hardcoded default.
- **First-run onboarding**: Template questions → kernel start → LLM-powered 2-round interview → profile generation → `system.md` written → `SetSystemPrompt()` hot-reload.
- **Runtime hot-reload**: `AgentKernel.SetSystemPrompt()` allows prompt changes without kernel restart.
- **Config override**: `kernel.system_prompt` in `config.yaml` has highest priority.
- **Plugin prompts**: Injected into system prompt at startup via `pluginMgr.GetPrompt()`.
- **Skill prompts**: Injected per-query when keywords match, via `skillManager.InjectPrompt()`.

### Plugin system (Claude Code compatible)

OpenAIDE is fully compatible with the [Claude Code official plugin specification](https://github.com/anthropics/claude-plugins-official). Drop a Claude plugin directory into `./data/plugins/` and it is auto-discovered.

**Supported plugin components:**

| Component | File | Phase | Description |
|-----------|------|-------|-------------|
| Manifest | `.claude-plugin/plugin.json` | Phase 1 | Plugin metadata (name, version, description) |
| Skills | `skills/*/SKILL.md` | Phase 1 | YAML frontmatter + Markdown body. Auto-discovered, registered as slash commands (`/skill-name`). Keywords auto-generated from name + description (Chinese + English). Claude tool names mapped to OpenAIDE equivalents (Read→read_file, Bash→execute_command, etc.) |
| MCP Servers | `.mcp.json` | Phase 2 | Declarative MCP server config: `{name: {type, command, args, url}}`. Stdio servers auto-connected, tools auto-registered. SSE/HTTP logged and skipped (limited to stdio transport). |
| Hooks | `hooks/hooks.json` | Phase 2 | Event-driven shell commands: `[{event, command, tools}]`. Claude events mapped to OpenAIDE: `PreToolUse→tool_call_started`, `PostToolUse→tool_call_ended`, `Stop→session_ended`, `SessionStart→session_created`. 30s timeout per hook, non-blocking goroutine. Tool name filter support with reverse mapping. |

**Code locations:**
- `plugin/plugin.go` — Manager: JSON format plugins + Claude format discovery via `loadClaudeFromDisk()`
- `plugin/plugin_claude.go` — All Claude format parsing: `DiscoverClaudePlugins()`, `DiscoverClaudeSkills()`, `DiscoverClaudeMCP()`, `DiscoverClaudeHooks()`, YAML frontmatter parsing, tool name mapping, keyword generation, event mapping
- `kernel/skill.go` — `AddClaudeSkill()` external injection, `GetSlashCommands()`, `autoKeywords()`
- `infra/app.go` — MCP wiring: `DiscoverClaudeMCP()` → `mcpManager.ConnectServer()` → register tools
- `infra/app_kernel.go` — Skills wiring: `DiscoverClaudeSkills()` → `sm.AddClaudeSkill()`; Hooks wiring: `DiscoverClaudeHooks()` → `agentKernel.Subscribe()` with shell execution

**Ecosystem compatibility:**
- Full Claude Code plugin format support
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
    │   └── review/
    │       └── SKILL.md             ← YAML frontmatter + Markdown body
    ├── .mcp.json                    ← Optional: MCP server config
    └── hooks/
        └── hooks.json               ← Optional: event-driven shell commands
```

### Configuration & data layout

```
~/.openaide/config.yaml       ← global config (LLM providers, keys, server port)
~/.openaide/data/              ← global data (cfg.Storage.DataDir, default "~/.openaide/data")
├── prompts/
│   ├── system.zh.md / system.en.md  ← system prompt (auto-generated, user-editable)
├── sessions/                  ← file-persisted (crash-recoverable, last 50 msgs)
├── memory/                    ← L1/L2/L3 JSON memory
├── knowledge/                 ← knowledge base + embeddings
├── skills/                    ← custom skills (JSON)
├── plugins/                   ← plugin configs
├── checkpoints/               ← session checkpoints (full history)
├── traces.jsonl               ← execution traces
├── events/                    ← persisted events
└── cache/                     ← prompt cache
```

**Provider config**: Two providers recommended for Architect/Editor pattern:
```yaml
providers:
  - name: deepseek             # Architect — deep reasoning
    default_model: deepseek-v4-pro
    timeout: 300
  - name: deepseek-flash       # Editor — fast execution
    default_model: deepseek-v4-flash
    timeout: 120
model_routing:
  reasoning: deepseek-v4-pro   # analyst, reviewer
  execution: deepseek-v4-flash # coder, executor, synthesis, classification
```

### CLI (REPL + TUI)

**REPL mode (default)** — `openaide`:
- `cmd/cli/repl.go` — Rich terminal REPL with lmorg/readline:
  - Readline with history, tab completion, hints
  - Markdown rendering via glamour (headers, code blocks with Chroma highlighting, tables)
  - Semantic color palette (15 aliases: cToolName/yellow, cError/red+bold, cThink/dim, etc.)
  - Smart routing: PreviewPlan → direct ReAct or team execution
  - Streaming display: tools summarized on one line, answer rendered after completion
  - Slash commands: `/analyst`, `/coder`, `/reviewer`, `/executor`, `/team`
  - Session resume: `-c` flag loads last non-empty session
- `cmd/cli/repl_output.go` — ANSI color system, glamour markdown renderer, output helpers

**TUI mode** — `openaide --tui`:
- `cmd/cli/tui_app.go` — `AppModel` root coordinator
- `cmd/cli/tui_chat.go` — `ChatArea` component (viewport + messages + streaming buffer)
- `cmd/cli/tui_status.go` — `StatusBar` component (spinner, session, tokens, tools)
- `cmd/cli/tui_input.go` — `InputBar` component (text input, history, queued query)
- `cmd/cli/tui_theme.go` — Icons, lipgloss styles, logRing, syntax highlighting, utilities
- `cmd/cli/tui_types.go` — Message types, enums (StreamPhase, OverlayType, WorkKind)
- Smart scroll: only auto-scrolls when user is at bottom (`viewport.AtBottom()`)
- Thinking content collapsed to one line

### Budget injection (Claude Code style)

Instead of hard-capping ReAct rounds and forcing synthesis, the LLM sees remaining budget:

- Round >= maxRounds/2: `"[系统] 已使用 N/M 轮，剩余 X 轮。如信息足够请直接给出结论。"`
- Round >= maxRounds-1: `"[系统] 最后一轮，必须给出最终结论，禁止调用工具。"`
- Only if budget exhausted: synthesis via flash model (no thinking, `route: "execution"`)

### Architect/Editor model routing

- **Architect** (analyst, reviewer) → reasoning model (pro, with thinking)
- **Editor** (coder, executor, classifier) → execution model (flash, no thinking)
- Synthesis/summarization → flash
- Skill detect, adaptive rounds estimation → flash (`route: "execution"`)
- `pickModel()` in orchestrator; `findProviderForModel()` in gateway auto-matches provider

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
- All 22 tool handlers implemented across 9 source files; no stubs
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
