# OpenAIDE — AI Agent Kernel

[![Build](https://github.com/lzy1102/openaide/actions/workflows/build-deploy.yml/badge.svg)](https://github.com/lzy1102/openaide/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)

[English](#) | [中文](README.zh.md)

**Go-based AI agent kernel that learns from every task.** 
SQLite · Vector ANN · 40 tools · 24-language LSP

> Reflection → Knowledge Refinement → Skill Extraction — gets smarter with every use.

---

## Quick Start

```bash
# One-liner install
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash

# Interactive setup wizard
openaide setup

# Start chatting
openaide
```

```bash
# Build from source
git clone https://github.com/lzy1102/openaide.git
cd openaide && make install
openaide setup
```

### Windows

```cmd
curl -o install.bat https://raw.githubusercontent.com/lzy1102/openaide/master/install.bat
install.bat
```

---

## What Makes It Different

| Feature | Claude Code | Aider | Cursor/Codex | OpenAIDE |
|----------|-------------|-------|-------------|----------|
| Knowledge Accumulation | ❌ | ❌ | ❌ | ✅ Reflect → Distill → Skill |
| Self-learning Skills | ❌ | ❌ | ❌ | ✅ Semantic cluster + LLM distill |
| Process Supervision | ❌ | ❌ | ❌ | ✅ Step-by-step evaluation |
| Tree of Thoughts | ❌ | ❌ | ❌ | ✅ Multi-path explore+select |
| Self-Rewarding | ❌ | ❌ | ❌ | ✅ Adaptive eval criteria |
| Agent Memory Mgmt | ❌ | ❌ | ❌ | ✅ MemGPT archive/retrieve |
| ACI (Agent Tool UX) | ❌ | ❌ | ❌ | ✅ Structured output + verify |
| CSP Actor (zero-lock) | ❌ | ❌ | ❌ | ✅ |
| Layered Prompts L0-L6 | ❌ | ❌ | ❌ | ✅ Task-adaptive |
| Vector ANN Search | ❌ | ❌ | ❌ | ✅ Bucketed O(n/256) |
| Desktop Control | ❌ | ❌ | ❌ | ✅ Cross-platform |
| Web Frontend | ❌ | ❌ | ❌ | ✅ | 
| Mobile Access | ❌ | ❌ | ❌ | ✅ Feishu/Telegram |
| Plugin Hot-reload | ❌ | ❌ | ❌ | ✅ Claude-compatible |
| 24-language LSP | ✅ | ✅ | ✅ | ✅ |

---

## Architecture

```
┌──────────────────────────────────────────────┐
│  openaide server (REST+SSE)  openaide (REPL) │
├──────────────────────────────────────────────┤
│  infra/Application — DI container            │
├──────────────────────────────────────────────┤
│  orchestration/    api/       channel/       │
│  Plan→Execute      REST API   Feishu/Telegram│
├──────────────────────────────────────────────┤
│  kernel/AgentKernel — ReAct loop             │
│  ├─ kernel/actor/  (CSP Actor, SafeMap)      │
│  ├─ kernel/trace/  (Tracer, Checkpointer)    │
│  └─ kernel/graph/  (DAG topo sort)           │
├──────────────────────────────────────────────┤
│  llm/      tools/     memory/   knowledge/   │
│  Gateway   40+ tools  SQLite    Vector ANN   │
└──────────────────────────────────────────────┘
```

### Core Stack

- **Language**: Go 1.26+ (pure Go, CGO_ENABLED=0)
- **Storage**: SQLite (WAL mode) — sessions, knowledge, memory, skills
- **Concurrency**: CSP Actors + SafeMap[K,V] + atomic.Value. Zero-lock core.
- **LLM**: OpenAI-compatible + Anthropic native APIs. LLM-native decisions (no rule fallback).
- **Learning**: Semantic embedding clustering + LLM knowledge distillation
- **LSP**: 24 languages via stdio JSON-RPC
- **REPL**: lmorg/readline + glamour (Markdown) + pterm

---

## Features

### Agent Capabilities
- **ReAct Loop**: Reason → Act → Observe → Repeat. Main agent with full layered prompts (L0-L6).
- **DeepPlan Pipeline**: Research → Propose → Select → Plan → Execute → Verify
- **Multi-Agent Team**: 4 roles (analyst/coder/reviewer/executor) with anti-prompt constraints, mini ReAct loops, role-filtered tools, isolated sessions. `/analyst` `/coder` `/reviewer` `/executor` `/team`
- **MemGPT Memory**: Agent actively manages its own memory — archive, retrieve, store core facts
- **Tree of Thoughts**: Multi-path exploration at decision points, LLM selects best approach
- **Self-Rewarding**: Per-task-type evaluation criteria that improve with experience
- **Process Supervision**: Step-by-step execution evaluation, identifies best/worst decisions
- **ACI Tools**: Agent-friendly tool output (before/after diffs, line numbers, verification)
- **Architect/Editor Routing**: Reasoning model for analysis, execution model for coding
- **Budget Injection**: LLM aware of remaining rounds, self-converges

### Self-Learning — Gets Smarter Every Task
- **Unified Query Analysis**: One LLM call classifies task + matches skill + estimates complexity + plans post-processing
- **LLM-Decided Learning**: LLM itself judges what's worth remembering — no fixed thresholds
- **Process Supervision**: Per-step evaluation identifies best and weakest decisions
- **Quality Gate (LLM-first)**: LLM reflection score → direct LLM ask → formula fallback
- **Knowledge Refinement**: Dedup (cosine > 0.85) → LLM structured extract → composite scored retrieval (relevance+importance+recency)
- **Skill Distillation**: Semantic/LLM clustering → evaluateAndDistill (single LLM) → auto-persist slash commands
- **ProjectMind**: Cross-session CodeMap, RiskMap, Conventions, Strategy stats
- **Config Toggles**: `distill_enabled` + `knowledge_enabled` — hot-reload without restart
- **Layered Prompts**: L0 (identity+learning) + L1 (project) + L2 (skill) + L3 (task) + L5 (reflection) + L6 (RAG)

### Developer Tools
- **40 Built-in Tools**: Filesystem, Git, Web, Knowledge, Browser, Desktop, LSP
- **verify_claim**: Pre-reporting verification tool — prevents false positives
- **Batch Embedding**: One API call for N messages
- **Embedding Cache**: LRU-cached query embeddings

### UX
- **REPL**: File-backed history, tab completion, syntax hints
- **Claude-style Approval**: Allow / Allow All / Deny
- **Interactive Setup Wizard**: Language → Provider → API Key → Model
- **Web Frontend**: Streaming chat, dashboard, settings
- **Remote Access**: Cloudflare Tunnel, frp, VPS, Tailscale

---

## Configuration

`~/.openaide/config.yaml`:

```yaml
llm:
 providers:
  - name: deepseek
   type: openai
   base_url: https://api.deepseek.com/v1
   api_key: sk-xxx
   default_model: deepseek-v4-pro
   timeout: 300
  - name: deepseek-flash
   type: openai
   base_url: https://api.deepseek.com/v1
   api_key: sk-xxx
   default_model: deepseek-v4-flash
   timeout: 120
 model_routing:
  reasoning: deepseek-v4-pro
  execution: deepseek-v4-flash

lang: zh                 # UI language: zh / en

storage:
 session_store: sqlite         # sqlite / file / memory
```

---

## Commands

```bash
openaide              # Interactive REPL
openaide "prompt"     # One-shot query
openaide -c           # Resume last session
openaide -y           # Auto-approve all tools
openaide --model <name>   # Override model
openaide --verbose    # Debug logging
openaide setup        # Interactive setup wizard
openaide sessions     # List sessions
openaide plugins      # List plugins
openaide update       # Self-update
openaide server       # Start API server (web mode)
openaide server --config /path/to/config.yaml  # Server with custom config
```

---

## API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/metrics` | GET | Prometheus metrics |
| `/api/v1/chat` | POST | Chat |
| `/api/v1/chat/stream` | POST | Streaming chat (SSE) |
| `/api/v1/sessions` | GET | List sessions |
| `/api/v1/sessions/{id}` | GET | Session history |
| `/api/v1/tools` | GET | Tool definitions |
| `/api/v1/stats` | GET | System statistics |

---

## Documentation

- **[Installation Guide](docs/INSTALL.md)** — detailed install instructions
- **[Configuration Guide](docs/CONFIG.md)** — full config reference
- **[Usage Guide](docs/USAGE.md)** — daily usage, commands, API, FAQ

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Quick areas to contribute:

- Add LSP server commands for more languages
- Improve i18n translations
- Write tests
- Improve documentation

---

## License

MIT License — see [LICENSE](LICENSE).
