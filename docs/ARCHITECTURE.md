# OpenAIDE 架构设计

> 日期: 2026-06-23

## 技术栈

- **Go 1.25**, 纯标准库 `net/http` (非 Gin)
- **SQLite 存储** (WAL 模式，modernc.org/sqlite 纯 Go，CGO_ENABLED=0)
- **CSP Actor 模型**: 核心状态模块采用单 goroutine + channel 通信
- **REPL**: lmorg/readline v4 + glamour (Markdown) + pterm (富文本)
- **LLM**: OpenAI 兼容协议 + Anthropic Native，双模型路由 (Architect/Editor)
- **前端(Web)**: 纯 HTML/CSS/JS，SPA 流式聊天 + 项目管理 + 配置面板
- **桌面**: Wails v2 + WebView2/WebKit，Go 后端直接复用 kernel/orchestration
- **通信**: WebSocket (双向) + SSE (服务器推送) + REST (请求-响应)

## 分层架构

```
┌──────────────────────────────────────────────────┐
│                   Entry Points                    │
│  openaide server  (REST + SSE + WebSocket)       │
│  openaide         (REPL: readline+glamour+pterm) │
│  cmd/desktop      (Wails v2: Go+WebView)         │
│  cmd/eval         (评测框架 LLM-as-judge)         │
├──────────────────────────────────────────────────┤
│              infra/Application                    │
│  DI容器 — 装配 kernel/tools/plugins/channels     │
│  LLM工厂 + Kernel工厂 + 渠道装配                │
│  热重载(hotreload) + 插件热加载(plugin_watcher)  │
├──────────────────────────────────────────────────┤
│   orchestration/           api/       channel/   │
│   DeepPlan+动态Team       HTTP API   飞书/Telegram│
│   隔离子Agent+进度回调    JWT鉴权     Webhook/TaskQ│
│   Branch并行+ToT探索      CORS/限流               │
├──────────────────────────────────────────────────┤
│              kernel/AgentKernel                   │
│  ┌─ ReAct Loop (Process / ProcessStream)      ─┐ │
│  │  Think → Act → Observe → Repeat            │ │
│  │    │       │        │                       │ │
│  │   LLM   Tools   Reflection                 │ │
│  │  (自适应5-50轮，上下文窗口收敛)               │ │
│  │                                              │ │
│  │  Before each loop:                          │ │
│  │    analyzeQuery (LLM统一分析)               │ │
│  │    → 任务分类+技能匹配+复杂度+后处理决策      │ │
│  │                                              │ │
│  │  After each loop:                           │ │
│  │    LLM-doReflection → QualityGate           │ │
│  └────────────────────────────────────────────┘ │
│                                                  │
│  Sub-packages:                                   │
│    kernel/actor/  — Actor, ActorStore, SafeMap  │
│    kernel/trace/  — FileTracer, Checkpointer     │
│    kernel/graph/  — DAG topological sort         │
├──────────────────────────────────────────────────┤
│  llm/      tools/       memory/                  │
│  多提供商   43内置工具   会话记忆                  │
│  网关+路由  文件/Git/    嵌入缓存                  │
│  双模型     命令/搜索/                           │
│             浏览器/桌面/                          │
│             LSP/MCP/桌面                              │
└──────────────────────────────────────────────────┘
```

## 设计原则

- **LLM is the brain.** 不做规则降级。技能匹配、风险评估、轮次预估、反思——全部 LLM-native。LLM 不可用 = Agent 停摆。
- **CSP Actors.** 有状态模块单 goroutine 持有数据，外部通过 channel 通信。核心数据路径零锁。
- **Prompt is layered.** 稳定前缀 (L0+L1+L2) 缓存。动态尾部 (L3+L5+L6) 按查询生成。分析格式仅审查/研究模式注入。

## 模块统计

| 模块 | 文件 | 职责 |
|------|------|------|
| `internal/kernel/` | 22+14T | Agent核心 ReAct 循环 |
| `internal/llm/` | 6+2T | LLM 网关 + 路由 + 缓存 |
| `internal/tools/` | 18+6T | 43 内置工具注册表 |
| `internal/memory/` | 2+3T | 会话记忆 |
| `internal/orchestration/` | 7+2T | 多Agent编排 |
| `internal/api/` | 3+2T | HTTP API |
| `internal/infra/` | 7+1T | DI容器 + 装配 |
| `internal/plugin/` | 2+2T | Claude格式插件 |
| `internal/mcp/` | 2+1T | MCP协议 |
| `internal/channel/` | 5+1T | 外部渠道 |
| `internal/auth/` | 1+1T | JWT 认证 |
| `internal/lsp/` | 2+1T | LSP 客户端 |
| `internal/eval/` | 2+1T | LLM评测框架 |
| 入口 (4) | 4模块 | CLI/Server/Desktop/Eval |

## 关键能力

| 能力 | 说明 |
|------|------|
| **LLM 原生决策** | 全自动化：任务分析、技能匹配、风险判断、轮次预估、反射评估、后处理决策——全部 LLM 执行 |
| **43 个内置工具** | 文件系统/Git/命令/Web搜索/浏览器/桌面操作/多模态/LSP/MCP/验证/记忆管理 |
| **CSP Actor 并发** | 有状态模块单 goroutine + channel 通信，无锁化核心路径 |
| **双模型路由** | 成本感知：reasoning 模型处理分析/审查/反思，execution 模型处理代码/命令/分类 |
| **分层提示词 L0-L5** | L0(角色)+L1(规则)+L2(语言)+L3(动态)+L5(辅助)，L0按场景特殊化(server/repl/subagent) |
| **自适应 ReAct** | 5-50 轮动态收敛，上下文窗口驱动而非固定上限，round≥10 时注入预算提示 |
| **LLM 动态后处理** | 根据查询复杂度决定是否执行反思，跳过简单查询以避免不必要开销 |
| **质量门控** | LLM-first 评判（不再使用固定公式），自动评估输出质量并决定补交或交付 |
| **自动语言注入** | 自动检测 21 种语言项目规范，注入语言特定约定 |
| **多Agent编排** | LLM动态生成Team角色（非硬编码）、隔离子Agent（独立会话+进度回调）、Branch并行分解、ToT多路径探索投票 |
| **插件系统** | Claude Code 官方格式兼容 (skills/MCP/hooks)，fsnotify热加载，支持命令定义钩子 |
| **MCP 协议** | JSON-RPC stdio+SSE，外部工具生态，完整生命周期管理 |
| **配置热重载** | hotreload.go 监听配置文件变化，无需重启 |
| **Web 前端** | 流式聊天 + 项目管理 + 配置编辑 + 多语言 + 暗色模式 |
| **API Server** | REST + SSE + WebSocket + Prometheus /metrics |
| **桌面应用** | Wails v2: Go backend 直接复用 kernel/orchestration，无 HTTP 层 |
