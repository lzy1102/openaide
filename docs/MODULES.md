# OpenAIDE 模块职责分工文档

> 版本: v3.2.0
> 更新: 2026-06-23

## 模块清单

| 模块 | 路径 | 文件数 | 核心职责 |
|------|------|--------|----------|
| **内核** | `internal/kernel/` | 22+14T | Agent核心：LLM统一查询分析、自适应ReAct循环（5-50轮）、CSP Actor模型、LLM动态后处理（反思）、分层提示词(L0-L5)、会话管理 |
| **LLM** | `internal/llm/` | 6+2T | 多提供商网关（OpenAI兼容+Anthropic原生）、成本感知路由（reasoning/execution双模型）、Prompt缓存、Embedding、Router |
| **工具** | `internal/tools/` | 18+6T | 43个内置工具：文件系统/Git/命令/搜索/浏览器/桌面/多模态/验证/LSP/记忆管理 |
| **记忆** | `internal/memory/` | 2+3T | SQLite WAL存储 + 批量嵌入 + 向量缓存 + 跨会话记忆 |
| **编排** | `internal/orchestration/` | 7+2T | DeepPlan管线、LLM动态Team角色生成、隔离子Agent+进度回调、Branch并行分解、ToT探索、DAG拓扑排序 |
| **API** | `internal/api/` | 3+2T | REST + SSE + WebSocket + Prometheus /metrics + JWT鉴权中间件 + 限流 |
| **基础设施** | `internal/infra/` | 7+1T | DI容器(Application)、LLM工厂、Kernel工厂+增强(技能注入/Claude hooks)、渠道装配、配置热重载(hotreload)、插件热加载(plugin_watcher)、追踪 |
| **配置** | `internal/config/` | 1+1T | JSON/YAML双格式、扁平展开、自动Provider识别、模型名→上下文推断 |
| **认证** | `internal/auth/` | 1+1T | JWT签发/验证/中间件 |
| **插件** | `internal/plugin/` | 2+2T | 插件管理 + Claude Code格式解析(skills/MCP/hooks) + fsnotify热重载 |
| **MCP** | `internal/mcp/` | 2+1T | MCP协议：stdio+SSE传输、完整生命周期、内容类型处理、Server管理 |
| **渠道** | `internal/channel/` | 5+1T | 外部消息接入：Webhook、飞书、Telegram、任务队列 |
| **压缩** | `internal/compress/` | 2+1T | LLM语义压缩 + 简单降级 |
| **事件** | `internal/event/` | 1+1T | 事件总线 + 持久化 |
| **Git** | `internal/git/` | 1+1T | git status/diff/log/blame |
| **索引** | `internal/index/` | 2+1T | 代码符号索引 + 搜索 |
| **反馈** | `internal/feedback/` | 1+1T | LLM-first质量门控（反思优先→LLM直判） |
| **身份** | `internal/identity/` | 1+1T | 项目类型检测 |
| **评估** | `internal/eval/` | 2+1T | LLM-as-judge评测框架：语义评判 + 基准任务套件 + 回归检测 |
| **语言** | `internal/lang/` | 1+1T | 国际化 (zh/en) |
| **LSP** | `internal/lsp/` | 2+1T | LSP协议客户端：定义跳转、引用查找、悬停信息、诊断 |
| **入口** | `cmd/cli/`, `cmd/server/`, `cmd/desktop/`, `cmd/eval/` | 4模块 | REPL(富终端+onboarding+setup) + API服务器 + 桌面应用(Wails) + 评测CLI |

> 文件数格式: `源文件数+测试文件数T`。sub-package（如 kernel/actor/）已计入父模块。

## 依赖关系

```
cmd/cli (REPL)          ──→ infra ──→ kernel ──→ llm / tools / memory
openaide server            ──→ infra ──→ api ──→ orchestration ──→ kernel
cmd/desktop (Wails)        ──→ infra ──→ kernel (direct, no HTTP)
                              ├── auth / feedback
                              ├── channel (Webhook/飞书/Telegram)
                              ├── plugin (Claude Code 格式兼容)
                              └── mcp (外部工具生态)

orchestration/
  ├── team.go         — Team多Agent管理、LLM动态角色生成(team_roles.go)
  ├── subagent.go     — 隔离子Agent + 进度回调(ProgressCallback)
  ├── execute.go      — Branch并行分解 + errgroup并发执行
  ├── tot.go          — Tree-of-Thought多路径探索+投票
  ├── planner.go      — DeepPlan深度规划管线
  └── orchestrator.go — 主编排器

plugin ←──→ kernel/skill  (通过 AddClaudeSkill 外部注入，避免循环导入)
mcp    ←── infra/app      (MCP server 连接 + tool 注册)
```

## 数据流

```
用户输入 → API/CLI/Desktop → Orchestrator → Kernel.Process()
                                              │
                              analyzeQuery (统一LLM分析)
                           任务分类+技能匹配+复杂度+后处理决策
                                              │
                        ┌─────────────────────┼─────────────────────┐
                        ▼                     ▼                     ▼
                    buildMessages          LLM.Chat           executeTool
                    (L0-L5分层提示词        (网关路由:          (命令安全+
                     记忆注入)               reasoning/execution) 工具执行)
                        │                     │                     │
                        └─────────────────────┴─────────────────────┘
                                              │
                              ┌───────────────┴───────────────┐
                              ▼                               ▼
                      finalizeResponse                  doReflection
                           (LLM判断是否执行后处理)
                              │                               │
                              └───────────────────────────────┘
                                              │
                                      Response → 用户

── 高级编排路径 ──

复杂任务 → Orchestrator.Plan()
              │
     ┌───────┼───────┐
     ▼       ▼       ▼
  Research Propose  Select  → DeepPlan
                              │
                    LLM生成动态Team角色
                    (GenerateRoles → 自定义analyst/coder/reviewer/...)
                              │
                    ┌─────────┼─────────┐
                    ▼         ▼         ▼
               SubAgent   SubAgent   SubAgent
               (独立会话   (进度回调   (隔离子Agent
               隔离上下文)  实时推送)   LLM分配角色)
                              │
                    ToT探索 (可选)
                    多路径→并行执行→投票
                              │
                    Branch分解 (可选)
                    发现子任务→并行分解→汇总
```

## 不再维护的旧文档

以下内容已整合到 OPENAIDE.md，本文件仅作模块清单参考。
