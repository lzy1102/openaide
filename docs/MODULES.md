# OpenAIDE 模块职责分工文档

> 版本: v3.0.0
> 更新: 2026-05-22

## 模块清单

| 模块 | 路径 | 文件数 | 核心职责 |
|------|------|--------|----------|
| **内核** | `internal/kernel/` | 22 | Agent核心：ReAct循环(process/stream/prompt)、反思、学习、模式、Skill、会话 |
| **LLM** | `internal/llm/` | 7 | 多提供商网关、OpenAI兼容、Anthropic原生、Embedding、Prompt缓存 |
| **工具** | `internal/tools/` | 10 | 22个工具：按领域拆分(filesystem/knowledge/symbol/git/web/browser/multimodal) |
| **记忆** | `internal/memory/` | 3 | L1/L2/L3 三级记忆 + 语义搜索 |
| **编排** | `internal/orchestration/` | 5 | Orchestrator、Planner、DAG执行、Team多Agent |
| **API** | `internal/api/` | 3 | REST + SSE + WebSocket + JWT认证 |
| **基础设施** | `internal/infra/` | 4 | DI容器：app + llm/kernel/channels 工厂方法 |
| **配置** | `internal/config/` | 2 | JSON/YAML配置管理 |
| **认证** | `internal/auth/` | 2 | JWT签发/验证/中间件 |
| **知识库** | `internal/knowledge/` | 1 | 文档CRUD + Embedding语义搜索 + 提示词注入 |
| **插件** | `internal/plugin/` | 2 | 插件管理 + Claude Code格式解析 (manifest/skills/MCP/hooks) |
| **MCP** | `internal/mcp/` | 2 | MCP协议：JSON-RPC stdio传输 + Server管理器 + 30s超时保护 |
| **渠道** | `internal/channel/` | 5 | 外部消息接入：Webhook、飞书、Telegram + 任务队列 |
| **压缩** | `internal/compress/` | 2 | LLM语义压缩(优先级提示词) + SimpleCompressor降级 |
| **事件** | `internal/event/` | 1 | 事件总线 (max 10k events, FIFO淘汰) + 持久化 |
| **Git** | `internal/git/` | 2 | git status/diff/log/blame 客户端封装 |
| **索引** | `internal/index/` | 3 | 代码符号索引 + 工作区扫描 |
| **认证** | `internal/auth/` | 2 | JWT签发/验证/中间件 |
| **反馈** | `internal/feedback/` | 2 | 质量门控评分 |
| **身份** | `internal/identity/` | 1 | 项目类型检测 |
| **语言** | `internal/lang/` | 2 | 国际化 (zh/en)，LANG环境变量检测 |
| **入口** | `cmd/server/`, `cmd/cli/` | 5 | API服务器 + Bubbletea TUI CLI + 首次引导 |

## 依赖关系

```
cmd/cli (TUI + onboard)  ──→ infra ──→ kernel ──→ llm / tools / memory
cmd/server                 ──→ infra ──→ api ──→ orchestration ──→ kernel
                                          ├── auth / knowledge / feedback
                                          ├── channel (Webhook/飞书/Telegram)
                                          └── plugin (Claude Code 格式)

plugin ←──→ kernel/skill  (通过 AddClaudeSkill 外部注入，避免循环导入)
mcp    ←── infra/app      (MCP server 连接 + tool 注册)
```

## 数据流

```
用户输入 → API/CLI → Orchestrator → Kernel.Process()
                                        │
                          ┌─────────────┼─────────────┐
                          ▼             ▼             ▼
                      buildMessages  LLM.Chat    executeTool
                      (提示词+记忆+   (网关路由)    (审批+权限+
                       知识库注入)                 工具执行)
                          │             │             │
                          └─────────────┴─────────────┘
                                        │
                                   doReflection
                                   autoSaveKnowledge
                                   Checkpoint.Save
                                        │
                                   Response → 用户
```

## 不再维护的旧文档

以下内容已整合到 CLAUDE.md，本文件仅作模块清单参考。
