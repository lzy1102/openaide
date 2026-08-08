# OpenAIDE 架构设计

> 日期: 2026-08-06
> 范围: 全栈架构 · 数据流 · 模块边界 · 扩展机制

## 技术栈

- **Go 1.26**, 纯标准库 `net/http` (非 Gin)
- **SQLite 存储** (WAL 模式，modernc.org/sqlite 纯 Go，CGO_ENABLED=0)
- **CSP Actor 模型**: 核心状态模块采用单 goroutine + channel 通信（零锁核心路径）
- **REPL**: lmorg/readline v4 + glamour (Markdown) + pterm (富文本)
- **LLM**: OpenAI 兼容协议，多 provider 网关 + 成本感知双模型路由 (reasoning/execution)
- **前端(Web)**: 纯 HTML/CSS/JS SPA，流式聊天 + 项目管理 + 配置面板
- **桌面**: Wails v2 + WebView2/WebKit，Go 后端直接复用 kernel/orchestration
- **通信**: WebSocket (双向) + SSE (服务器推送) + REST (请求-响应)

## 仓库结构

```
openaide/
├── backend/              ← Go 主后端（全部核心逻辑，约 3.6 万行）
├── frontend/             ← Web 前端（原生 JS SPA）
├── frontend-desktop/     ← 桌面端 UI（Wails + Go）
├── cmd/                  ← CLI 入口（cli/server/eval）
├── npm/                  ← npm 发布包
├── scripts/              ← 构建/发布脚本
├── docs/                 ← 架构/配置/部署文档
└── examples/             ← MCP/Plugin/Skill 扩展示例（可复制）
```

**核心形态：单一 Go 二进制**——CLI、API 服务器、桌面端共享同一套 `backend/` 内核。

## 分层架构

```
┌─────────────────────────────────────────────────────────────┐
│  接入层 (Entrypoints)                                         │
│  cmd/cli (REPL+TUI) · cmd/server (REST+SSE) · cmd/eval       │
│  frontend/ (Web SPA) · frontend-desktop/ (Wails)             │
├─────────────────────────────────────────────────────────────┤
│  API 层   internal/api         /api/v1/* + /ws WebSocket      │
│  渠道层   internal/channel     webhook/飞书/Telegram/任务队列  │
├─────────────────────────────────────────────────────────────┤
│  组装层   internal/infra       app.go 依赖注入/装配全部模块    │
│                              插件热加载 · 配置热重载 · 追踪    │
├─────────────────────────────────────────────────────────────┤
│  编排层   internal/orchestration  Team 多智能体/子代理/TOT     │
├─────────────────────────────────────────────────────────────┤
│  内核层   internal/kernel      AgentKernel: ReAct 循环         │
│                              7 层 Prompt · SkillActor         │
│                              Planner · Reflection · 检查点     │
├─────────────────────────────────────────────────────────────┤
│  能力层                                                       │
│  internal/llm    Gateway 多 provider 路由 · 成本感知           │
│  internal/tools  47 内置工具 (文件/git/web/浏览器/LSP/桌面)    │
│  internal/memory MemGPT 记忆 (向量+归档+核心事实)              │
│  internal/mcp    MCP 客户端/服务器 (外部工具生态)              │
│  internal/plugin Claude 插件/Skill/Hooks 兼容层                │
│  internal/codeindex 代码语义索引 · projectmind 项目记忆        │
├─────────────────────────────────────────────────────────────┤
│  基础层  config · event · identity · lang · lsp · compress    │
│          index · auth · git · webfront                        │
└─────────────────────────────────────────────────────────────┘
```

## 核心数据流（一次请求的完整生命周期）

```
用户输入 (CLI / HTTP / 渠道)
   │
   ▼
infra/app.go 组装 → Orchestrator
   │
   ▼
kernel_stream.go: 统一查询分析 (一次 LLM 调用)
   ├─ 任务类型 (coding/review/think/debugging)
   ├─ 复杂度评估 (complexity >= 15 → Planner 子任务分解)
   ├─ 技能预匹配 (SkillID → SkillActor.UsePreMatch)
   └─ 期望轮数 (AdaptiveRounds)
   │
   ▼
buildMessages: 分层 Prompt 组装 (L0-L6 + Intent)
   ├─ 稳定前缀: L0 安全规则 + L1 项目上下文 + L2 Skill 注入 (prompt 缓存友好)
   ├─ 会话历史 (按 token 预算截断)
   ├─ 用户查询
   ├─ Intent 层: 系统对需求的理解 (task/complexity/interpreted, 来自统一查询分析)
   └─ 动态尾部: L3 任务适配 + L4 项目知识 + L5 上轮反思 + L6 代码索引
   │
   ▼
ReAct 循环 (max_rounds 轮, 停滞检测 120s)
   ├─ LLM 调用 (Gateway 成本感知路由: execution→flash / reasoning→pro)
   ├─ 工具调用 (ToolExecutor → tools.Registry → 47 内置 + mcp_* 外部)
   ├─ SkillActor 过滤工具白名单 (allowed-tools)
   ├─ 上下文管理: 超 90% 预算 → LLM 渐进式重摘要(≤3次) + 重注入 [OriginalQuery]+[Intent]
   ├─ 方向检查: 每 5 轮 LLM 判定 on_track/off_track, 偏离注入 [Direction] 重聚焦
   ├─ 停滞检测: 重复工具/连续失败 → pivot 消息;pivot 达上限 → [Recovery] 重规划引导 (StuckDetector)
   └─ 每轮: 检查点保存 · 追踪记录 · 事件发布 (hooks/事件总线)
   │
   ▼
finalizeResponse: 保存记忆 → 更新会话 → 生成标题 → 反思
   └─ 反思沉淀: SkillActor 技能提炼 · MemGPT 归档 · 指标记录
```

## 核心模块详解

### 1. 内核层 `internal/kernel/`（最核心，50 源 + 16 测试）

- **AgentKernel**（kernel.go）：依赖注入容器——LLMProvider、ToolExecutor、Memory、SessionStore、Reflection、Planner、Checkpointer、Tracer、SkillActor、CodeIndexer、Compressor 全部可插拔（`Set*` 系列方法）
- **分层 Prompt 架构**（kernel_prompt.go）：
  | 层 | 内容 | 缓存性 |
  |----|------|--------|
  | L0 | 安全规则 + 角色 | 稳定前缀 |
  | L1 | 项目上下文 (cwd/git/规则文件带优先级/语言共享锚点表) | 稳定前缀 |
  | L2 | Skill 注入 (SkillActor.InjectPrompt) | 稳定前缀 |
  | Intent | 系统对需求的理解 (task/complexity/interpreted, 紧跟用户查询) | 动态 |
  | Clarify | 歧义查询澄清引导 (ambiguous=true 时注入, 要求先陈述理解) | 动态 |
  | L3 | 任务适配 (coding/review/think/debugging) + Active context 锚点 | 动态尾部 |
  | L4 | 跨会话学习者洞察 + ProjectMind 约定 | 动态尾部 |
  | L5 | 上一轮反思结果 | 动态尾部 |
  | L6 | 代码索引相关 chunk (codeindex) | 动态尾部 |
  - 稳定前缀（L0-L2）保持 prompt 缓存命中率；动态尾部（L3-L6）按查询生成
- **统一查询分析**（kernel_stream.go:32）：一次 LLM 调用替代三次独立判断（任务类型 + 技能检测 + 复杂度），预匹配信号零成本复用
- **任务上下文保持**（kernel.go / kernel_react.go）：buildMessages 保存原始查询 + Intent 到 TaskContext；上下文压缩后自动重注入 `[OriginalQuery]` + `[Intent]`，防止摘要丢失任务目标
- **方向检查**（direction_check.go）：每 5 轮用 LLM（execution 路由）判定最近活动是否偏离原始需求；检测到偏离（off_track）时注入 `[Direction]` 重聚焦消息
- **SkillActor**（skill_actor.go）：CSP 无锁 actor 模式，技能自动检测（LLM 匹配 + 统一分析预匹配）、prompt 注入、工具白名单、自动提炼 + 持久化（`auto_skills.json`）
- **Planner**：复杂任务（complexity ≥ 15）分解为子任务计划注入
- **AdaptiveRounds**：按任务动态调整轮数（默认 5-30）
- **Reflection**：LLM 反思 → 技能反馈 + 记忆沉淀（学习闭环核心）
- **trace/**: FileTracer + FileCheckpointer（会话恢复）

### 1.5 防跑偏体系（长任务可靠性）

内核通过三层防线确保长任务不跑偏、压缩后不变傻：

**锚定层（记得要干什么）**
- `[Intent]` 层：每轮消息携带 analyzeQuery 生成的 task/complexity/interpreted，模型始终可见系统对需求的理解
- `Active context` 锚点：L3 模式文案尾部引用原始查询（≤100 字符）
- `[Clarify]` 层：歧义查询（ambiguous=true）先陈述理解再行动
- TaskContext 重注入：上下文压缩后重注入 `[OriginalQuery]` + `[Intent]`，摘要丢失目标也能恢复

**监控层（发现跑偏）**
- 方向检查（direction_check.go）：每 5 轮 LLM 判定 on_track/off_track，偏离注入 `[Direction]` 重聚焦
- StuckDetector：连续失败/重复工具/重复错误 ≥3 次 → pivot 消息强制换策略
- 预算提示（round ≥10/20/50 分级）：显示工具用量 → 停止探索 → 强制收尾

**纠偏层（回到正轨）**
- pivot 消息：换策略 + 具体建议（重读文件/换工具/拆步骤）
- `[Recovery]` 重定向：pivot 达 3 次上限后，要求陈述已验证进展并换不同方法
- 渐进式重摘要：超限时最多重摘要 3 次，保留用户意图优先级（用户意图 > 决策 > 技术事实 > 任务状态 > 工具结果）
- 计划进度回写：每 3 轮注入子任务进度，提示下一步待办

### 2. LLM 网关 `internal/llm/`（10 源 + 3 测试）

- 多 provider 注册（OpenAI 兼容协议，DeepSeek 特化配置：thinking/reasoning_effort/json_mode/strict_tools）
- **成本感知路由**（gateway.go:206）：`options["route"]` → `execution`（flash 模型）/ `reasoning`（pro 模型）
- 3 次指数退避重试 + `FallbackChat`/`FallbackEmbed` 多 provider 故障转移
- 全局限流（阻塞式令牌桶 `RateLimiter`）、Prompt 缓存（`PromptCache`）、健康检查
- `Provider` 接口统一：Chat/ChatStream/Embed/EmbedBatch

### 3. 工具层 `internal/tools/`（47 个内置工具，40 源 + 19 测试）

| 分组 | 数量 | 代表工具 |
|------|------|----------|
| 文件系统 | 5 | read_file、write_file、list_directory、search_files |
| 文件编辑 | 3+1 | edit_files、diff_edit、apply_patch |
| 撤销 | 2 | undo 检查点管理 |
| Git | 6 | git_status、git_diff、git_blame、git_log、git_commit |
| Web | 3 | web_search、web_fetch、searxng |
| 浏览器 | 8（opt-in） | browser_navigate/extract/screenshot/click/fill… |
| 桌面 | 7 | desktop_screenshot/type/key 等 |
| 询问/记忆/多模态 | 3 | ask_user、memory_save、read_image |
| LSP/符号 | 5 | lsp_definition/references/hover/symbols |
| 待办/验证 | 4 | todo_write、verify_claim、trace_callers |

合计：39 基础 + 8 浏览器（默认关闭）= 47。

设计要点：**无删除/移动工具（有意的安全设计）**、命令黑名单、写操作 Undo 检查点、路径验证、危险命令审批（TUI approval 面板）。浏览器工具条件注册（`browserEnabled()`），关闭时不出现在 schema 中，LLM 永远不会选到会失败的工具。

### 4. 记忆系统 `internal/memory/`（MemGPT 模式，5 源 + 3 测试）

- **MemoryActor**（actor 模式）：Save/Load/Search + 向量化（Embedder 可插拔）
- **归档**：对话 → 摘要 + 重要性评分 → 向量存储（ArchiveConversation/RetrieveArchive）
- **核心事实**：StoreCoreFact/GetCoreFacts（长期稳定知识）
- 存储：SQLite + 向量 ANN（`vector.go`）

### 5. 编排层 `internal/orchestration/`（9 源 + 2 测试）

- **Orchestrator**：面向 API 的高层入口，封装 kernel
- **Team 多智能体**（team.go/team_roles.go）：LLM 动态生成角色（analyst/coder/reviewer/executor），隔离会话
- **SubAgent**（subagent.go）：隔离子代理 + 进度回调
- **TOT**（tot.go）：树状推理多路径探索 + 投票
- **Planner**（planner.go）：DeepPlan 规划管线 + PlanApprover 审批

### 6. 扩展机制（详见 examples/）

| 机制 | 模块 | 说明 |
|------|------|------|
| **MCP** | `internal/mcp/` | 外部工具服务器，stdio/SSE 传输，`mcp_` 前缀注册进 registry，config.yaml 或插件 `.mcp.json` 配置 |
| **Plugin** | `internal/plugin/` | Claude Code 兼容容器：`.claude-plugin/plugin.json` + skills/ + .mcp.json + hooks/ |
| **Skill** | `internal/plugin/` → `kernel.SkillActor` | SKILL.md 解析（YAML frontmatter + 正文），LLM 自动匹配，prompt 注入 + 工具白名单 |
| **Channel** | `internal/channel/` | Webhook/飞书/Telegram/任务队列外部消息接入 |

### 7. API 层 `internal/api/`（5 源 + 2 测试）

```
POST /api/v1/chat          聊天（同步）
POST /api/v1/chat/stream   SSE 流式
GET  /api/v1/sessions      会话管理
GET  /api/v1/memory/search 记忆检索
GET  /api/v1/tools         工具清单
GET  /api/v1/metrics       任务指标 (JSONL+Prometheus)
GET  /api/v1/projects      项目管理
GET  /ws                   WebSocket
GET  /health               健康检查
```

## 关键架构决策

1. **Actor/CSP 无锁模式**：`actor.SafeMap`、SkillActor、MemoryActor 全部用 actor 并发模型，状态变更全部进 actor 邮箱，无数据竞争（并发安全的核心手段）
2. **可插拔接口设计**：kernel 依赖 10+ 接口而非具体实现，infra 层统一装配——单测时全部可 mock
3. **提示词分层 + 缓存友好**：稳定前缀（L0-L2）+ 动态尾部（L3-L6），兼顾 prompt 缓存命中率与按需注入
4. **一次分析多用**：统一查询分析产出任务类型/复杂度/技能预匹配/轮数，避免重复 LLM 调用
5. **学习闭环**：用户反馈（good/bad）→ 反思 → 技能提炼/记忆归档 → 下次更聪明
6. **生态兼容**：不造新格式，直接兼容 Claude Code 的 plugin/skill/hooks/MCP 生态
7. **安全默认**：无删除工具、命令黑名单、浏览器工具 opt-in、写操作可 Undo

## 模块统计（2026-08-06 实测）

| 模块 | 源文件 | 测试文件 | 职责 |
|------|--------|----------|------|
| `internal/kernel/` | 50 | 16 | Agent 核心 ReAct 循环 + 7 层 Prompt + SkillActor |
| `internal/llm/` | 10 | 3 | LLM 网关 + 多 provider + 成本感知路由 |
| `internal/tools/` | 40 | 19 | 47 内置工具注册表 + handler |
| `internal/memory/` | 5 | 3 | MemGPT 记忆（向量+归档+核心事实） |
| `internal/orchestration/` | 9 | 2 | 多 Agent 编排（Team/SubAgent/TOT/Planner） |
| `internal/api/` | 5 | 2 | HTTP API + WebSocket |
| `internal/infra/` | 9 | 2 | DI 容器 + 装配 + 插件热加载 |
| `internal/plugin/` | 4 | 2 | Claude 格式插件/Skill/Hooks |
| `internal/mcp/` | 6 | 4 | MCP 协议（server/client/http） |
| `internal/channel/` | 6 | 1 | 外部渠道（webhook/飞书/Telegram/队列） |
| `internal/codeindex/` | 5 | 1 | 代码语义索引 + 检索 |
| `internal/projectmind/` | 2 | 1 | 项目记忆（约定/规则沉淀） |
| `internal/webfront/` | 2 | 1 | 嵌入 Web 前端 |
| 其余基础包 | ~30 | ~15 | config/event/identity/lang/lsp/compress/index/auth/git/eval |

> 质量门禁：gofmt + go vet + `go test -short ./internal/...`（26 包全绿）通过后才可提交。
