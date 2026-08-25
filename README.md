# OpenAIDE — AI Agent Kernel

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[English](#) | [中文](README.zh.md)

**TypeScript AI agent kernel — everything is a plugin.** ReAct loop · dynamic plugin loading · SQLite persistence · OpenAI-compatible gateway

> Dynamic plugin loading in-process (no subprocess) — plugins register tools, hooks and personas via a unified interface.

---

## Quick Start

Requires Node.js ≥ 18.

```bash
# 1. install (deps + global `openaide` command)
node scripts/install.mjs

# 2. configure (interactive wizard, or copy config.example.yaml)
openaide setup

# 3. run
openaide "hello"          # one-shot query
openaide                  # interactive TUI / REPL
openaide serve            # HTTP/WS API server
```

## Install

```bash
# install deps + link global `openaide` command (recommended)
node scripts/install.mjs

# or dev mode only: install deps, run without a global command
npm install
```

Remove the global command:

```bash
npm unlink -g openaide
```

## Usage

```bash
openaide "query"             # one-shot query
openaide file.ts "query"     # inject files as context, then query
openaide --model <id> ...    # override model for any command
openaide --output json "q"   # one-shot output as JSON
openaide -c                  # continue the most recent session
openaide                     # interactive TUI (Ink) / readline REPL
openaide serve               # HTTP/WS + SSE server (default :8080) — serves built-in WebUI from frontend/dist
openaide plugins             # list all plugins (active/disabled/failed) + tools
openaide plugins search <kw>      # search the plugin registry
openaide plugins install <name>   # install from the registry (git-based)
openaide plugins disable <name>   # unload now + skip on startup (persisted)
openaide plugins enable <name>    # remove from disabled list and load again
openaide sessions            # list persisted sessions
openaide setup               # config wizard
openaide --version | --help
```

Full user guide → [docs/USAGE.md](docs/USAGE.md).

## Configure

Config lives at `~/.openaide/config.yaml` (auto-created; see [config.example.yaml](config.example.yaml) and `openaide setup`). Every field can be overridden by an environment variable.

```yaml
llm:
  api_key: sk-xxx
  model: deepseek-v4-pro
  base_url: https://api.deepseek.com/v1
kernel:
  max_rounds: 10
  max_tokens: 200000   # context budget (compression / history trim), not per-reply cap
```

Env overrides:

| Variable | Overrides |
|---|---|
| `OPENAIDE_API_KEY` | `llm.api_key` |
| `OPENAIDE_MODEL` | `llm.model` |
| `OPENAIDE_BASE_URL` | `llm.base_url` |
| `OPENAIDE_DATA_DIR` | `data_dir` (default `~/.openaide`) |
| `OPENAIDE_PLUGINS_DIR` | `plugins_dir` (default `<data_dir>/plugins`) |
| `OPENAIDE_PORT` | `serve` command port (default 8080) |

## Architecture

> Docs: [USAGE.md](docs/USAGE.md) (user + developer guide) · [ARCHITECTURE.md](docs/ARCHITECTURE.md) (kernel–plugin contract) · [examples/README.md](examples/README.md) (example plugin)

Monorepo (npm workspaces):

```
cli (TUI + REPL + commands)   api (HTTP/WS + SSE)
        ↓                        ↓
      core ── AgentKernel (ReAct loop, event bus, session)
        ↓                        ↓
   plugins (dynamic load)   memory (SQLite)   llm (OpenAI gateway)
        ↓
   tools (builtin, as a plugin)   config (yaml + env)
```

> Full contract between kernel and plugins (principles, seams, exposed info, lifecycle, extension points) → [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

## Key Concepts

- **Everything is a plugin**: kernel tools, personas and hooks all enter through the plugin system — builtin and user plugins share the same registration path
- **Ecosystem = GitHub**: tag your repo with the `openaide-plugin` topic and it's discoverable — live search, star-ranked, installed straight from git. No server, no review
- **Sessions travel with the repo (auto-synced)**: run and conversations live in `.openaide/` as readable JSON — commit them, pull on another machine, `openaide -c` continues where you left off
- **Policy as plugin**: interceptor chain can veto/rewrite LLM requests and tool calls — approval gates, PII redaction, budget guards are all just plugins
- **MCP bridge**: Claude-compatible `mcpServers` config becomes native tools (`<server>__<tool>`) via the official MCP SDK — stdio transport, `${ENV}` expansion, drop-in from claude_desktop_config.json
- **Pluggable brain**: plugins can register LLM providers (Anthropic, Ollama, …); `config.llm.provider` picks one at startup
- **Become any agent**: one `SYSTEM.md` file transforms OpenAIDE into a different domain agent (travel planner, contract analyst, …) — declarative persona plugins with optional tool allowlists, hot-switchable via `/persona <name>`
- **Dynamic loading in-process**: plugins are loaded via `import()`, no subprocess; hot-reload by busting the module cache
- **ReAct loop**: think → tool call → observe → loop, with streaming responses
- **SQLite persistence**: sessions survive restarts; REPL can resume past conversations
- **OpenAI-compatible gateway**: plain `fetch`, streaming, no heavy dependencies
- **Token-efficient by design**: stable system prefix + `cache_control` for provider prompt caching, history trimmed by token budget, empty LLM replies treated as failures
- **Plugin interface**: `OpenAIDePlugin` — `tools` (registered as `<plugin>__<tool>`), `hooks` (subscribe kernel events), `persona` (L0 system prompt)

## Plugin Example

See [examples/plugins/example-plugin](examples/plugins/example-plugin) — a TypeScript plugin exporting tools, a hook and a persona.

```ts
import type { OpenAIDePlugin } from '@openaide/plugins';

const plugin: OpenAIDePlugin = {
  name: 'example',
  version: '0.1.0',
  tools: [
    {
      name: 'upper',
      description: 'uppercase a text',
      handler: async (args) => ({ content: String(args.text).toUpperCase() }),
    },
  ],
  hooks: [{ event: 'tool.call.ended', handler: async (e) => console.log('tool done') }],
  persona: { name: 'example', description: 'example persona', systemPrompt: '...' },
};

export default plugin;
```

## Development

```bash
npm run dev            # run directly via tsx (no build step, default)
npm run build          # compile all packages to dist/
npm run typecheck      # typecheck all packages
npm test               # run all tests
```

## GitHub Search & Topics

GitHub indexes the README text and matches repos by **topics**, so keyword-rich headings plus a few topics make this project easy to find on [repo search](https://github.com/search?q=topic%3Aai-agent+language%3ATypeScript&type=repositories).

Recommended topics — add them once in the repo **Settings → Topics**, or run (requires [GitHub CLI](https://cli.github.com/)):

```bash
gh repo edit lzy1102/openaide --add-topic ai-agent --add-topic agent --add-topic llm --add-topic typescript --add-topic plugin --add-topic plugin-system --add-topic openai --add-topic nodejs --add-topic cli --add-topic monorepo
```

Also set a concise repo description (Settings → About):

> TypeScript AI agent kernel — everything is a plugin. ReAct loop, in-process plugin loading, SQLite persistence, OpenAI-compatible gateway.

## License

MIT License — see [LICENSE](LICENSE).
