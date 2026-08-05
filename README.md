# OpenAIDE — AI Agent Kernel

[![Build](https://github.com/lzy1102/openaide/actions/workflows/build-deploy.yml/badge.svg)](https://github.com/lzy1102/openaide/actions)
[![npm](https://img.shields.io/npm/v/@lzy1102/openaide)](https://www.npmjs.com/package/@lzy1102/openaide)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)

[English](#) | [中文](README.zh.md)

**Go-based AI agent kernel that learns from every task.** SQLite · Vector ANN · 39–47 tools · 24-language LSP

> Reflection → Skill Feedback → MemGPT Memory — gets smarter with every use.

---

## Quick Start

```bash
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
openaide setup
openaide
```

## Install

```bash
# curl
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash

# npm
npm install -g @lzy1102/openaide

# Windows
curl -o install.bat https://raw.githubusercontent.com/lzy1102/openaide/master/install.bat && install.bat
```

## Usage

```bash
openaide                # Interactive REPL
openaide "fix this bug" # One-shot query
openaide -c             # Resume last session
openaide setup          # Setup wizard
openaide server         # Start API server
```

## Architecture

```
openaide server (REST+SSE)  openaide (REPL)
         ↓                        ↓
    orchestration/          agent/kernel/
    Plan → Execute          ReAct loop
         ↓                        ↓
    llm/   tools/   memory/
    39–47 tools · Vector memory · SQLite
```

## Key Features

- **Self-Learning**: Reflects on each task, adjusts skill confidence, accumulates project memory — gets smarter over time
- **Multi-Agent Team**: Analyst → Coder → Reviewer → Executor with isolated sessions
- **DeepPlan**: Research → Propose → Select → Plan → Execute — multi-round deep thinking
- **MemGPT Memory**: Agent manages its own memory — archive, retrieve, core facts
- **Claude Plugin Compatible**: Drop-in skills, MCP servers, hooks from Claude Code ecosystem
- **39–47 Built-in Tools**: Filesystem, Git, Web, Browser, LSP, Desktop control
- **Command Safety**: Dangerous commands blocked by blacklist, safe commands run directly, write tools protected by Undo

> **Browser tools** (8 tools: `browser_navigate` / `browser_extract` / `browser_screenshot` / `browser_click` / `browser_fill` / `browser_click_at` / `browser_scroll` / `browser_type`) are **opt-in**: they require a local Chromium install (~500 MB) and are only registered when enabled. Enable via `browser.enabled: true` in `~/.openaide/config.yaml` or `OPENAIDE_BROWSER=true`. Auto-install of Chromium works only when running as root — otherwise install Chrome/Chromium yourself, e.g. `sudo apt-get install chromium-browser`.

## Configuration

`~/.openaide/config.yaml`:

```yaml
llm:
  api_key: sk-xxx
  model: deepseek-v4-pro
  execution_model: deepseek-v4-flash
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT License — see [LICENSE](LICENSE).
