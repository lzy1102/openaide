# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common commands

```bash
# Build both binaries (openaide-server, openaide-cli)
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
- **`backend/cmd/cli/main.go`** — Interactive CLI. Forces `direct` mode. On first run, triggers interactive onboarding (`onboard.go`): template-guided setup → kernel start → LLM-powered interview → TUI. One-shot mode: `openaide "fix this bug"`.
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

3. **`backend/internal/tools/`** (10 files) — Tool definitions and handlers split by domain:
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

4. **`backend/internal/llm/`** (7 files):
   - `gateway.go` — Multi-provider router. `LLMProvider` interface. `Router` for task-type-based provider selection. `PromptCache`. `Embedder` adapter.
   - `openai_provider.go` — OpenAI-compatible APIs (OpenAI, DeepSeek, Ollama, Qwen, etc.)
   - `anthropic_provider.go` — Anthropic Claude API. Stream goroutine has `ctx.Done()` guard on channel sends.
   - `embedder.go` — Embedding interface + helpers
   - `cache.go` — 24h TTL prompt cache with hourly cleanup goroutine

5. **`backend/internal/memory/`** — File-based JSON memory. 3 levels (L1/L2/L3). Semantic search (cosine similarity) → TF-IDF → text match. `Embedding` persisted in JSON.

6. **`backend/internal/config/`** — JSON + YAML config. `Storage.DataDir` defaults to `./data`. All data (prompts, sessions, memory, knowledge, skills, plugins, checkpoints, traces, events, cache) stored under this directory.

7. **`backend/internal/api/`** — HTTP REST API + WebSocket. Routes: `POST /api/v1/chat`, `POST /api/v1/chat/stream` (SSE), `GET /api/v1/sessions`, etc. WebSocket heartbeat uses `done` channel for clean shutdown.

8. **`backend/internal/orchestration/`** — Full task lifecycle management:
   - `orchestrator.go` — `Orchestrator` wraps kernel with pre/post processing. `ProcessQuery` triggers LLM-based planning via `Planner`. `executePlan` runs full lifecycle (Execute→Test→Review) with context-isolated sub-agents via `RunSubAgent()`. `DeepPlan` pipeline: Research→Propose→Select→Plan. `Smart routing`: LLM auto-selects role pipeline per task. `ExecuteWithPlan` accepts pre-made plans.
   - `planner.go` — `Planner`: LLM-driven task decomposition (Function Calling → text fallback). Research phase: mini ReAct loop with read-only tools, auto-stops when LLM done. `Propose`: generates 2-3 alternatives with pros/cons/risk/effort. `PlanWithApproach`: detailed plan from chosen approach.
   - `team.go` — `Team`: 4 roles (analyst/coder/reviewer/executor) each with isolated tool sets. `RunSubAgent`: fresh session per role, prevents context pollution. `Smart routing`: `routePipeline` LLM selects needed roles. `assignRole` LLM picks right role per subtask.
   - `dag.go` — DAG execution engine with parallel execution and data-race-safe map access.

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
- **`tui.go`** — `cancelMu sync.Mutex` protects `cancelStream` from concurrent access races.
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
./data/                        ← project-local data (cfg.Storage.DataDir, default "./data")
├── prompts/
│   ├── system.zh.md / system.en.md  ← system prompt (auto-generated, user-editable)
├── sessions/                  ← file-persisted (crash-recoverable)
├── memory/                    ← L1/L2/L3 JSON memory
├── knowledge/                 ← knowledge base + embeddings
├── skills/                    ← custom skills (JSON)
├── plugins/                   ← plugin configs
├── checkpoints/               ← session checkpoints
├── traces.jsonl               ← execution traces
├── events/                    ← persisted events
└── cache/                     ← prompt cache
```

### TUI (Bubble Tea)

- **`cmd/cli/tui.go`** (1205 lines) — Full Bubble Tea TUI:
  - Streaming LLM output with thinking visualization
  - Tool call display (`⚙ read_file`, `→ result summary`)
  - Syntax highlighting (chroma/Monokai) for code blocks
  - Render throttle (50ms debounce during streaming)
  - Theme struct (`tuiTheme`) with 18 style fields
  - Emoji/ASCII dual rendering (auto-detects terminal capability)
  - Session list, model selector, help overlay, plan confirmation, proposal selection
  - Deep plan progress indicators (🔍 Research → 💡 Propose → 📋 Plan)
  - Context-isolated sub-agent execution (fresh sessions)

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
