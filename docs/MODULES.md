# OpenAIDE 模块职责分工文档

> 版本: v3.3.0
> 更新: 2026-08-06

## 模块清单

| 模块 | 路径 | 源/测试 | 核心职责 |
|------|------|---------|----------|
| **内核** | `internal/kernel/` | 50/16 | Agent核心：统一查询分析、自适应ReAct循环、7层Prompt(L0-L6)、SkillActor(技能检测/注入/白名单)、Planner、Reflection、Checkpointer、Tracer、AdaptiveRounds、Metrics |
| **LLM** | `internal/llm/` | 10/3 | 多提供商网关(OpenAI兼容)、成本感知路由(reasoning/execution双模型)、Prompt缓存、限流、重试、故障转移、Embedding |
| **工具** | `internal/tools/` | 40/19 | 47个内置工具：文件/Git/Web/浏览器(8个opt-in)/桌面/LSP/多模态/验证/记忆/待办 |
| **记忆** | `internal/memory/` | 5/3 | MemGPT三级记忆：会话记忆 + 向量搜索 + 归档 + 核心事实 |
| **编排** | `internal/orchestration/` | 9/2 | Team多Agent(LLM动态角色)、隔离子Agent+进度回调、TOT树状推理+投票、DeepPlan规划+审批 |
| **API** | `internal/api/` | 5/2 | REST + SSE + WebSocket + Prometheus /metrics + 限流 |
| **基础设施** | `internal/infra/` | 9/2 | DI容器(Application)、LLM/Kernel工厂、技能注入、Claude hooks注册、MCP注册、配置热重载、插件热加载、追踪 |
| **配置** | `internal/config/` | 1/1 | YAML配置、路径展开、MCP/浏览器/内核各段 |
| **认证** | `internal/auth/` | 2/1 | JWT签发/验证/中间件 |
| **插件** | `internal/plugin/` | 4/2 | Claude Code格式兼容：plugin.json/skills/.mcp.json/hooks 自动发现 + 热加载 |
| **MCP** | `internal/mcp/` | 6/4 | MCP协议：JSON-RPC 2024-11-05、stdio/SSE传输、Client/Server/Manager、`mcp_`前缀注册 |
| **渠道** | `internal/channel/` | 6/1 | 外部消息接入：Webhook、飞书、Telegram、任务队列 |
| **代码索引** | `internal/codeindex/` | 5/1 | 代码语义索引(chunk+向量/TF-IDF)、prompt阶段注入相关代码 |
| **项目记忆** | `internal/projectmind/` | 2/1 | 项目约定/规则沉淀与检索 |
| **压缩** | `internal/compress/` | 3/1 | LLM语义压缩 + NovelCompressor降级 |
| **事件** | `internal/event/` | 2/1 | 事件总线 + 持久化 |
| **Git** | `internal/git/` | 2/1 | git status/diff/log/blame |
| **索引** | `internal/index/` | 3/1 | 代码符号索引 + 搜索 |
| **身份** | `internal/identity/` | 2/1 | 项目类型检测 |
| **评估** | `internal/eval/` | 3/1 | LLM-as-judge评测框架 + 基准任务套件 |
| **语言** | `internal/lang/` | 2/1 | 国际化 (zh/en) |
| **LSP** | `internal/lsp/` | 3/1 | LSP协议客户端：定义/引用/悬停/诊断 |
| **Web前端** | `internal/webfront/` | 2/1 | 嵌入 Web 前端资源 |
| **入口** | `cmd/cli/`, `cmd/server/`, `cmd/eval/` | 4模块 | REPL(富终端+onboarding+setup) + API服务器 + 评测CLI |

> 文件数格式: `源文件数/测试文件数`（2026-08-06 实测，不含子包计数差异）。

## 依赖关系

```
cmd/cli (REPL)          ──→ infra ──→ kernel ──→ llm / tools / memory
openaide server            ──→ infra ──→ api ──→ orchestration ──→ kernel
                              ├── auth
                              ├── channel (Webhook/飞书/Telegram)
                              ├── plugin (Claude Code 格式兼容)
                              ├── mcp (外部工具生态)
                              └── codeindex / projectmind

orchestration/
  ├── team.go         — Team多Agent管理、LLM动态角色生成(team_roles.go)
  ├── subagent.go     — 隔离子Agent + 进度回调(ProgressCallback)
  ├── execute.go      — 并行执行
  ├── tot.go          — Tree-of-Thought多路径探索+投票
  ├── planner.go      — DeepPlan深度规划管线
  └── orchestrator.go — 主编排器

plugin ──→ kernel.SkillActor  (DiscoverClaudeSkills → AddClaudeSkill，避免循环导入)
mcp    ←── infra/app          (ConnectServer + registerMCPTools → tools.Registry)
kernel ──→ plugin             (Claude hooks 事件订阅注册)
```

## 数据流

```
用户输入 → API/CLI/Desktop → Orchestrator → Kernel.ProcessStream()
                                              │
                        统一查询分析 (一次LLM调用)
                  任务类型+复杂度+技能预匹配+期望轮数
                                              │
                        ┌─────────────────────┼─────────────────────┐
                        ▼                     ▼                     ▼
                    buildMessages          LLM.Chat           executeTool
                    (L0-L6 分层提示词       (网关路由:           (Registry→47内置
                     记忆注入+Skill注入)     reasoning/execution)  +mcp_*外部工具)
                        │                     │                     │
                        └─────────────────────┴─────────────────────┘
                                              │
                              ┌───────────────┴───────────────┐
                              ▼                               ▼
                      finalizeResponse                  Reflection
                      (保存记忆→更新会话                   (LLM反思→技能
                       →生成标题→反思)                      提炼+记忆沉淀)
                              │
                              └───────────────────────────────┘
                                              │
                                      Response → 用户

── 高级编排路径 ──

复杂任务 (complexity ≥ 15) → Kernel.Planner 分解子任务
                                              │
                     ┌────────────────────────┼────────────────────┐
                     ▼                        ▼                    ▼
              Orchestrator.Team       Orchestrator.SubAgent   Orchestrator.TOT
              (LLM生成动态角色)        (独立会话+进度回调)     (多路径探索+投票)
                     │                        │                    │
                     └────────────────────────┴────────────────────┘
                                              │
                                     汇总 → 用户
```

## 扩展机制（详见 examples/）

```
~/.openaide/
├── config.yaml          ← MCP servers 配置 (mcp.enabled + servers[])
├── data/plugins/        ← 插件目录（Claude Code 兼容）
│   └── <plugin>/
│       ├── .claude-plugin/plugin.json  ← 插件清单（必须，否则不识别）
│       ├── skills/<name>/SKILL.md      ← 技能（YAML frontmatter + 正文）
│       ├── .mcp.json                   ← 插件自带 MCP 服务器
│       └── hooks/hooks.json            ← 生命周期钩子（事件 → 命令）
└── data/skills/auto_skills.json        ← 自动提炼技能持久化
```

## 不再维护的旧文档

以下内容已整合到 OPENAIDE.md，本文件仅作模块清单参考。
