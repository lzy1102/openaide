# OpenAIDE — Open-Source AI Coding Agent Kernel (Claude Code Alternative)

<p align="center">
  <strong>Everything is a plugin.</strong> A TypeScript AI agent harness with a ReAct loop,
  in-process plugin loading, MCP support, multi-agent subagents, git-synced sessions
  and an OpenAI-compatible model gateway. MIT licensed.
</p>

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
![Node.js](https://img.shields.io/badge/Node.js-%E2%89%A518-green)
![TypeScript](https://img.shields.io/badge/TypeScript-5.x-blue)
![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen)

[English](#) | [中文](README.zh.md)

---

## ✨ Highlights

- 🧩 **Everything is a plugin** — tools, personas, policy interceptors, LLM providers, even the TUI itself register through one unified interface. Builtin and community plugins share the same path
- 🔌 **MCP support (Model Context Protocol)** — reuse your **Claude Desktop / Claude Code `mcpServers` config as-is**: stdio transport via the official SDK, `${ENV}` expansion, tools appear as `<server>__<tool>`
- 🤖 **Any model, any provider** — DeepSeek, GLM, Kimi, Qwen, MiniMax, OpenAI-compatible relays, or local models via Ollama/vLLM. Custom providers are plugins too (`config.llm.provider`)
- 🪆 **Multi-agent subagents** — declare `(persona, tool allowlist)` pairs in config and every persona becomes a delegatable tool (`subagent__travel`) running its own isolated ReAct loop
- 💾 **Sessions travel with your repo** — conversations live in `.openaide/` as readable JSON per developer, auto-committed, so `git pull && openaide -c` continues on any machine
- 🛡️ **Policy-as-plugin interceptor chain** — approve dangerous tools, redact PII, enforce budgets: veto or rewrite any LLM request or tool call
- 🎨 **Pluggable UI** — built-in Ink TUI (Claude Code style) and readline REPL are themselves plugins; ship your own interface in ~40 lines
- 🛒 **GitHub-powered plugin marketplace** — no server, no review: tag your repo `openaide-plugin` and it's searchable/installable (`openaide plugins search|install`)
- ⚡ **Token-efficient by design** — stable system prefix + `cache_control` prompt caching, token-budgeted history trim, empty replies treated as failures

## 🆚 How it compares

| | OpenAIDE | Claude Code | DeepSeek Harness (dsh) | Gemini CLI |
|---|---|---|---|---|
| Open source | ✅ MIT | ❌ closed | ✅ MIT (dev preview) | ✅ Apache-2.0 |
| Any model (BYOK) | ✅ plugins + OpenAI-compatible | ❌ Claude only | ✅ adapters | ✅ Google-first |
| In-process TS plugins | ✅ hot-reloadable | ❌ | ⚠️ Cordis-bound | ⚠️ extensions |
| MCP client | ✅ Claude-config compatible | ✅ native | ✅ native | ✅ |
| Sessions in your git repo | ✅ per-dev JSON, auto-commit | ❌ local | ⚠️ | ❌ local |
| UI itself pluggable | ✅ | ❌ | ✅ | ❌ |

> OpenAIDE keeps the whole agent surface swappable while staying a single lightweight Node.js process — a good fit if you want a **self-hosted, model-agnostic, hackable coding agent** you can fully customize.

## 🚀 Quick Start

Requires Node.js ≥ 18.

```bash
# 1. install (deps + global `openaide` command)
node scripts/install.mjs

# 2. configure (interactive wizard, or copy config.example.yaml)
openaide setup

# 3. run
openaide "hello"          # one-shot query
openaide                  # interactive TUI / REPL
openaide serve            # HTTP/WS API server + WebUI
```

## 📖 Usage

```bash
openaide "query"             # one-shot query
openaide file.ts "query"     # inject files as context, then query
openaide --model <id> ...    # override model for any command
openaide --output json "q"   # one-shot output as JSON
openaide -c                  # continue the most recent session
openaide                     # interactive TUI (Ink) / readline REPL
openaide serve               # HTTP/WS + SSE server (default :8080) — serves built-in WebUI from frontend/dist
openaide workspace           # workspace status: identity, sessions, sync state
openaide plugins             # list all plugins (active/disabled/failed) + tools
openaide plugins search <kw>      # search GitHub ecosystem + seed registry
openaide plugins install <name>   # install from the registry (git-based)
openaide plugins disable <name>   # unload now + skip on startup (persisted)
openaide plugins enable <name>    # remove from disabled list and load again
openaide sessions            # list persisted sessions
openaide setup               # config wizard
openaide --version | --help
```

Inside a session, slash commands manage everything live: `/plugins`, `/persona`, `/model`,
`/undo`, `/sessions`, `/help`. Full user guide → [docs/USAGE.md](docs/USAGE.md).

## ⚙️ Configure

Config lives at `~/.openaide/config.yaml` (auto-created; see [config.example.yaml](config.example.yaml) and `openaide setup`). Every field can be overridden by an environment variable.

```yaml
llm:
  api_key: sk-xxx
  model: deepseek-v4-pro            # any model your endpoint serves
  base_url: https://api.deepseek.com/v1
kernel:
  max_rounds: 10
  max_tokens: 200000   # context budget (compression / history trim), not per-reply cap
  approval: dangerous  # ask before dangerous tool calls (off | dangerous | always)
  subagents:
    - name: travel
      persona: voyager # any plugin persona becomes a delegatable tool
mcp_servers:           # Claude-compatible schema
  filesystem:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "."]
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
| `OPENAIDE_UI` | interactive UI (plugin `uis` name: ink / readline / custom) |
| `OPENAIDE_PROVIDER` | `llm.provider` (plugin-registered backend) |

## 🏗️ Architecture

> Docs: [USAGE.md](docs/USAGE.md) (user + developer guide) · [ARCHITECTURE.md](docs/ARCHITECTURE.md) (kernel–plugin contract) · [examples/README.md](examples/README.md) (example plugin)

Monorepo (npm workspaces):

```
cli (TUI + REPL + commands)   api (HTTP/WS + SSE + static WebUI)
        ↓                        ↓
      core ── AgentKernel (ReAct loop, event bus, session, interceptor chain)
         ↓                        ↓
   plugins (dynamic load)   memory (file/sqlite)   llm (provider registry)   mcp (bridge)
        ↓
   tools (builtin, as a plugin)   config (yaml + env)
```

> Full contract between kernel and plugins (principles, seams, exposed info, lifecycle, extension points) → [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

## 🔑 Key Concepts

- **Everything is a plugin**: kernel tools, personas and hooks all enter through the plugin system — builtin and user plugins share the same registration path
- **Ecosystem = GitHub**: tag your repo with the `openaide-plugin` topic and it's discoverable — live search, star-ranked, installed straight from git. No server, no review
- **Sessions travel with the repo (auto-synced)**: conversations live in `.openaide/` as readable JSON — commit them, pull on another machine, `openaide -c` continues where you left off
- **Policy as plugin**: interceptor chain can veto/rewrite LLM requests and tool calls — approval gates, PII redaction, budget guards are all just plugins
- **MCP bridge**: Claude-compatible `mcpServers` config becomes native tools (`<server>__<tool>`) via the official MCP SDK — stdio transport, `${ENV}` expansion, drop-in from claude_desktop_config.json
- **Pluggable brain**: plugins can register LLM providers (Anthropic, Ollama, …); `config.llm.provider` picks one at startup
- **Become any agent**: one `SYSTEM.md` file transforms OpenAIDE into a different domain agent (travel planner, contract analyst, …) — declarative persona plugins with optional tool allowlists, hot-switchable via `/persona <name>`
- **Multi-agent orchestration**: `config.kernel.subagents` packs (persona, allowlist) into delegatable tools — isolated child ReAct loops, cascading abort, zero session pollution
- **SKILL.md compatible**: drop an agent-skills/dsh-style SKILL.md folder into the plugins dir and it just works
- **Dynamic loading in-process**: plugins are loaded via `import()`, no subprocess; hot-reload by busting the module cache
- **ReAct loop**: think → tool call → observe → loop, with streaming responses
- **OpenAI-compatible gateway**: plain `fetch`, streaming, no heavy dependencies
- **Plugin interface**: `OpenAIDePlugin` — `tools` (registered as `<plugin>__<tool>`), `hooks` (subscribe kernel events), `interceptors` (veto/rewrite), `providers` (LLM backends), `uis` (interfaces), `personas` (multi-persona packs)

## 🧩 Plugin Example

See [examples/plugins/example-plugin](examples/plugins/example-plugin) — a TypeScript plugin exporting tools, a hook and a persona.

```ts
import type { OpenAIDePlugin } from '@openaide/plugins';

const plugin: OpenAIDePlugin = {
  name: 'example',
  version: '0.1.0',
  category: 'capability',
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

## ❓ FAQ

**Is OpenAIDE a Claude Code alternative?**
It's an open-source, self-hosted agent harness in the same category — ReAct coding agent, terminal TUI, tool approvals — but model-agnostic (bring DeepSeek/GLM/Kimi/Ollama/OpenAI), fully pluggable, and stores sessions in your own git repo.

**Is it built for vibe coding?**
Yes — that's the intended workflow: describe what you want in plain language, let the agent inspect files, run commands and ship changes, with approval gates on dangerous operations and every session synced to your repo so you can always roll back or continue elsewhere.

**Does it support MCP (Model Context Protocol)?**
Yes. Point it at any stdio MCP server with the same `mcpServers` schema Claude Desktop uses; tools show up automatically.

**Which models work?**
Anything OpenAI-compatible: DeepSeek, GLM, Kimi, Qwen, MiniMax, OpenRouter relays, plus local models through Ollama/vLLM. Custom backends = a small provider plugin.

**Can I build my own UI?**
Yes — the built-in Ink TUI and readline REPL are themselves UI plugins; implement `uis.start(host)` and select it with `OPENAIDE_UI`.

**Single binary?**
`make binary` produces a standalone executable via Bun compile (no Node required on target).

## 🛠️ Development

```bash
npm run dev            # run directly via tsx (no build step, default)
npm run build          # compile all packages to dist/
npm run typecheck      # typecheck all packages
npm test               # node --test (packages) + vitest (frontend)
make binary            # single-file executable (bun compile)
```

## 🔎 GitHub Topics

Help others find this project — set topics once in **Settings → Topics**, or run (requires [GitHub CLI](https://cli.github.com/)):

```bash
gh repo edit lzy1102/openaide \
  --add-topic ai-agent --add-topic ai-agents --add-topic agent-framework \
  --add-topic ai-harness --add-topic coding-agent --add-topic cli-agent \
  --add-topic claude-code-alternative --add-topic mcp --add-topic mcp-server \
  --add-topic multi-agent --add-topic react-agent --add-topic llm \
  --add-topic deepseek --add-topic openai --add-topic ollama \
  --add-topic typescript --add-topic nodejs --add-topic plugin-system \
  --add-topic tui --add-topic monorepo --add-topic mit-license
```

And a keyword-rich repo description (**Settings → About**) — this is the single highest-impact field for GitHub search:

> Open-source AI coding agent & harness in TypeScript — everything is a plugin. ReAct loop, MCP support (Claude-compatible config), multi-agent subagents, pluggable TUI/providers/policies, git-synced sessions. Works with DeepSeek, GLM, Kimi, Qwen, OpenAI, Ollama. MIT.

## License

MIT License — see [LICENSE](LICENSE).
