# OpenAIDE — AI Agent Kernel

[![Build](https://github.com/lzy1102/openaide/actions/workflows/build-deploy.yml/badge.svg)](https://github.com/lzy1102/openaide/actions)
[![npm](https://img.shields.io/npm/v/@lzy1102/openaide)](https://www.npmjs.com/package/@lzy1102/openaide)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)

[English](#) | [中文](README.zh.md)

**Go-based AI agent kernel that learns from every task.** SQLite · Vector ANN · 40 tools · 24-language LSP

> Reflection → Knowledge Refinement → Skill Extraction — gets smarter with every use.

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
    llm/   tools/   memory/   knowledge/
    40+ tools · Vector ANN · SQLite
```

## Key Features

- **Self-Learning**: Reflects on each task, extracts skills, accumulates knowledge — gets smarter over time
- **Multi-Agent Team**: Analyst → Coder → Reviewer → Executor with isolated sessions
- **DeepPlan**: Research → Propose → Select → Plan → Execute — multi-round deep thinking
- **MemGPT Memory**: Agent manages its own memory — archive, retrieve, core facts
- **Claude Plugin Compatible**: Drop-in skills, MCP servers, hooks from Claude Code ecosystem
- **40+ Built-in Tools**: Filesystem, Git, Web, Browser, LSP, Desktop control
- **Smart Approval**: Safe commands auto-approve, dangerous commands blocked, no LLM overhead

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
