# OpenAIDE — AI Agent Kernel

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[English](#) | [中文](README.zh.md)

**TypeScript AI agent kernel — everything is a plugin.** ReAct loop · dynamic plugin loading · SQLite persistence · OpenAI-compatible gateway

> Dynamic plugin loading in-process (no subprocess) — plugins register tools, hooks and personas via a unified interface.

---

## Install

Requires Node.js ≥ 18.

```bash
# install deps + link global `openaide` command (recommended)
node scripts/install.mjs

# or dev mode only: install deps, run without a global command
npm install
```

Global install links the `openaide` command. To remove it:

```bash
npm unlink -g openaide
```

## Compile & Run

```bash
npm run dev            # run directly via tsx (no build step, default)
npm run build          # compile all packages to dist/
npm run typecheck      # typecheck all packages
npm test               # run all tests

openaide               # interactive REPL
openaide "fix this bug" # one-shot query
openaide --version
```

## Configure

Config lives at `~/.openaide/config.yaml` (auto-created; see [config.example.yaml](config.example.yaml) and `openaide setup`). Every field can be overridden by an environment variable.

```bash
openaide setup         # interactive config wizard
```

```yaml
llm:
  api_key: sk-xxx
  model: deepseek-v4-pro
  base_url: https://api.deepseek.com/v1
kernel:
  max_rounds: 10
  max_tokens: 4000
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

> Docs: [USAGE.md](docs/USAGE.md) (user + developer guide) · [ARCHITECTURE.md](docs/ARCHITECTURE.md) (kernel–plugin contract) · [config.example.yaml](config.example.yaml)

Monorepo (npm workspaces):

```
cli (REPL + commands)     api (HTTP/WS + SSE)
        ↓                      ↓
      core ── AgentKernel (ReAct loop, event bus, session)
        ↓                      ↓
   plugins (dynamic load)   memory (SQLite)   llm (OpenAI gateway)
        ↓
   tools (builtin, as a plugin)   config (yaml + env)
```

> Full contract between kernel and plugins (principles, seams, exposed info, lifecycle, extension points) → [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

## Key Concepts

- **Everything is a plugin**: kernel tools, personas and hooks all enter through the plugin system — builtin and user plugins share the same registration path
- **Dynamic loading in-process**: plugins are loaded via `import()`, no subprocess; hot-reload by busting the module cache
- **ReAct loop**: think → tool call → observe → loop, with streaming responses
- **SQLite persistence**: sessions survive restarts; REPL can resume past conversations
- **OpenAI-compatible gateway**: plain `fetch`, streaming, no heavy dependencies
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
npm run typecheck  # typecheck all packages
npm test           # run all tests
```

## License

MIT License — see [LICENSE](LICENSE).
