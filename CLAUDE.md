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
               /      |       \
         api/    orchestration/   (HTTP handlers, SSE streaming)
                     |
              kernel/AgentKernel   ← ReAct loop, the core of the agent
              /    |    |     \
        llm/   tools/ memory/  kernel/types
```

### Entry points

- **`backend/cmd/server/main.go`** — Production server. Loads config from `~/.openaide/config.yaml`, starts the HTTP API server.
- **`backend/cmd/cli/main.go`** — Interactive CLI with raw terminal mode, streaming output, and color formatting. Forces `direct` mode (no API server).

### Layered design

1. **`backend/internal/infra/app.go`** — Application container. Creates all components in order: LLM Gateway → Tool Registry → Memory Manager → Session Store → Kernel → Orchestrator → API Server. This is the wiring diagram — start here to understand dependencies.

2. **`backend/internal/kernel/kernel.go`** — `AgentKernel` implements the ReAct loop. It takes `LLMProvider`, `ToolExecutor`, `Memory`, and `SessionStore` as interfaces (no concrete imports). Handles tool calling, context compression, event publishing, and optional reflection/learning. Two paths: `Process()` (sync) and `ProcessStream()` (streaming).

3. **`backend/internal/kernel/interfaces.go`** — All kernel-level interfaces: `Kernel`, `LLMProvider`, `ToolExecutor`, `Memory`, `SessionStore`, `ContextCompressor`, `PermissionChecker`, `Reflection`, `Learner`, `PatternDetector`. These are the contracts everything else implements against.

4. **`backend/internal/kernel/types.go`** — Shared types: `Message`, `ToolCall`, `Query`, `Response`, `StreamChunk`, `Event`, `Session`. Includes DeepSeek-specific `ThinkingConfig` and `ReasoningContent` field on messages.

5. **`backend/internal/llm/gateway.go`** — Multi-provider router. Implements `kernel.LLMProvider`. Supports fallback across enabled providers. All providers registered via `RegisterProvider()`.

6. **`backend/internal/llm/openai_provider.go`** — The only concrete provider. Handles all OpenAI-compatible APIs (OpenAI, DeepSeek, Ollama, Qwen, etc.). Includes DeepSeek-specific features: `isDeepSeek()` detection gates `thinking` and `reasoning_effort` parameters; `CompleteWithPrefix()` for prefix continuation; `FIMComplete()` for fill-in-the-middle. Also handles JSON mode and streaming via SSE.

7. **`backend/internal/tools/registry.go`** — Tool registration and execution. `BuiltinTools()` defines 6 tools (read_file, write_file, execute_command, list_directory, search_files, git_status). Tool handlers are currently stubs.

8. **`backend/internal/memory/memory.go`** — File-based JSON memory with 3 levels (L1 working, L2 short-term, L3 long-term). `FileMemory` adapts to `kernel.Memory` interface. Search is simple text matching.

9. **`backend/internal/config/config.go`** — Supports both JSON and YAML config via file extension detection. Config includes: server, LLM providers (with DeepSeek-specific fields), memory, tools, kernel, storage, and log settings.

10. **`backend/internal/api/api.go`** — HTTP REST API. Routes: `POST /api/v1/chat`, `POST /api/v1/chat/stream` (SSE), `GET /api/v1/sessions`, `GET /api/v1/sessions/{id}`, `GET /api/v1/memory/search?q=`, `GET /api/v1/tools`, `GET /api/v1/stats`, `GET /health`. CORS middleware on all routes.

11. **`backend/internal/orchestration/orchestrator.go`** — Wraps the kernel with pre/post processing: session management, permission checks, memory saving. `EnhancedOrchestrator` adds async reflection, learning, and pattern detection.

### Kernel enhancements (optional, via interfaces)

- **`kernel/reflection.go`** — `SimpleReflection`: evaluates execution quality, detects excessive tool calls, short responses, error keywords.
- **`kernel/learner.go`** — `SimpleLearner`: learns patterns, preferences, and error types; persists to `insights.json`.
- **`kernel/pattern.go`** — `SimplePatternDetector`: detects repeated queries, frequent tool usage, tool sequences, response styles, error patterns.
- **`kernel/compress.go`** — `SimpleCompressor`: keeps system prompt + last 4 messages, summarizes older messages.

### Frontend

`frontend/index.html` — Single-page vanilla JS app. ES module imports from `frontend/src/`. Pages: chat, templates, dashboard, scheduled tasks, models. Components: ThinkingVisualizer, CorrectionPanel, ToolCallDisplay. Served via `frontend/serve.py` (Python HTTP server on port 8000).

### Configuration

Config file: `~/.openaide/config.yaml` (or `.json`). Provider types: `openai` or `openai-compatible`. The `server.mode` field controls whether the API server starts: `server` starts it, `direct` skips it (for CLI usage).

### CI/CD

`.github/workflows/build-deploy.yml` — Triggers on push to `master` and `v*` tags. Builds static Go binaries with `CGO_ENABLED=0`, runs tests, packages release tarball with self-extracting installer. On `v*` tags: creates GitHub Release. Optional deploy over SSH with health check.

### Key patterns

- All inter-module communication uses interfaces defined in `kernel/interfaces.go`
- The LLM Gateway implements `kernel.LLMProvider` so it can be passed directly to the kernel
- Session store is in-memory (`SessionStoreAdapter`); not persistent across restarts
- Tool handlers in `tools/registry.go` are stubs marked with `// TODO`
- DeepSeek-specific behavior is gated by `isDeepSeek()` which checks the base URL and provider name for "deepseek"
