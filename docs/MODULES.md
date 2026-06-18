# OpenAIDE 模块职责分工文档

> 版本: v3.1.0
> 更新: 2026-06-08

## 模块清单

| 模块 | 路径 | 文件数 | 核心职责 |
|------|------|--------|----------|
| **内核** | `internal/kernel/` | 36 | Agent核心：统一查询分析、无限ReAct循环、CSP Actor、LLM动态后处理、反思、技能蒸馏、分层提示词(L0-L5) |
| **LLM** | `internal/llm/` | 6 | 多提供商网关、OpenAI兼容、Anthropic原生、Embedding、Prompt缓存、LLM聚类 |
| **工具** | `internal/tools/` | 12 | 34个工具：文件/Git/命令/搜索/知识库/浏览器/桌面/多模态 |
| **记忆** | `internal/memory/` | 4 | SQLite存储 + 批量嵌入 + 向量缓存 |
| **编排** | `internal/orchestration/` | 5 | DeepPlan管线、LLM动态角色生成、隔离子Agent、自修复循环、ToT探索 |
| **评估** | `internal/eval/` | 2 | LLM-as-Judge评测框架：语义评判 + 基准任务套件 + 回归检测 |
| **API** | `internal/api/` | 3 | REST + SSE + WebSocket + Prometheus metrics |
| **基础设施** | `internal/infra/` | 7 | DI容器 + 热重载 + 插件热加载 + 追踪 |
| **配置** | `internal/config/` | 2 | JSON/YAML配置管理 |
| **认证** | `internal/auth/` | 2 | JWT签发/验证/中间件 |
| **知识库** | `internal/knowledge/` | 3 | SQLite + 向量索引 + 随机投影分桶ANN + 知识精炼 |
| **插件** | `internal/plugin/` | 2 | 插件管理 + Claude Code格式解析 + 热重载 |
| **MCP** | `internal/mcp/` | 3 | MCP协议：stdio + HTTP传输、完整生命周期、内容类型处理、Server |
| **渠道** | `internal/channel/` | 5 | 外部消息接入：Webhook、飞书、Telegram |
| **压缩** | `internal/compress/` | 2 | LLM语义压缩 + 降级 |
| **事件** | `internal/event/` | 1 | 事件总线 + 持久化 |
| **Git** | `internal/git/` | 2 | git status/diff/log/blame |
| **索引** | `internal/index/` | 3 | 代码符号索引 |
| **反馈** | `internal/feedback/` | 2 | LLM-first质量门控（反思优先→LLM直判→公式兜底） |
| **身份** | `internal/identity/` | 1 | 项目类型检测 |
| **评估** | `internal/eval/` | 2 | LLM-as-judge 评测框架：语义评判替代关键字匹配 |
| **语言** | `internal/lang/` | 2 | 国际化 (zh/en) |
| **入口** | `openaide server (entry)`, `cmd/cli/`, `cmd/eval/` | 8 | API服务器 + REPL + 评测CLI + 前端嵌入 |

## 依赖关系

```
cmd/cli (REPL)          ──→ infra ──→ kernel ──→ llm / tools / memory
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
                               analyzeQuery (统一分析)
                                 任务分类+技能匹配+复杂度+后处理决策
                                        │
                          ┌─────────────┼─────────────┐
                          ▼             ▼             ▼
                      buildMessages  LLM.Chat    executeTool
                      (提示词+记忆+   (网关路由)    (审批+权限+
                       知识库注入)                 工具执行)
                          │             │             │
                          └─────────────┴─────────────┘
                                        │
                              finalizeResponse
                           doReflection (LLM判断)
                           autoSaveKnowledge
                           extractSkillsFromPatterns
                                        │
                                   Response → 用户
```

## 不再维护的旧文档

以下内容已整合到 CLAUDE.md，本文件仅作模块清单参考。
