# OpenAIDE 使用指南

本文档分「使用者」和「开发者」两部分，覆盖安装、配置、交互式/一次性使用、API 服务与插件开发。

> 版本：v0.3.0（TypeScript monorepo）｜插件同进程动态加载，无多进程。

## 目录

- [一、使用者（User）](#一使用者user)
  - [1.1 安装](#11-安装)
  - [1.2 配置](#12-配置)
  - [1.3 交互式使用](#13-交互式使用)
  - [1.4 一次性问答](#14-一次性问答)
  - [1.5 API 服务](#15-api-服务)
  - [1.6 插件的使用](#16-插件的使用)
- [二、开发者（Developer）](#二开发者developer)
  - [2.1 在仓库内开发](#21-在仓库内开发)
  - [2.2 写一个插件](#22-写一个插件)
  - [2.3 目录速览](#23-目录速览)
  - [2.4 发布与分发](#24-发布与分发)

---

## 一、使用者（User）

### 1.1 安装

前提：Node.js ≥ 18。

```bash
# 一键安装：装依赖 + 全局 link openaide 命令（推荐）
node scripts/install.mjs

# 或仅装依赖，配合 npm run dev 使用（不生成全局命令）
npm install
```

卸载全局命令：

```bash
npm unlink -g openaide
```

### 1.2 配置

```bash
openaide setup          # 交互式配置向导
```

或复制模板 [config.example.yaml](../config.example.yaml) 到 `~/.openaide/config.yaml` 后编辑：

```yaml
llm:
  api_key: sk-xxx                 # 也可是环境变量 OPENAIDE_API_KEY
  model: deepseek-v4-pro
  base_url: https://api.deepseek.com/v1
  # provider: acme               # 插件注册的 LLM 后端（缺省内置 openai-compatible）
kernel:
  max_rounds: 10
  max_tokens: 200000    # 上下文 token 预算(压缩/历史裁剪阈值),非单次输出上限
  approval: dangerous   # 工具审批：off(默认)/dangerous(危险工具需确认)/always(全部确认)
```

环境变量覆盖（优先级高于配置文件）：

| 变量 | 覆盖项 |
|---|---|
| `OPENAIDE_API_KEY` | `llm.api_key` |
| `OPENAIDE_MODEL` | `llm.model` |
| `OPENAIDE_BASE_URL` | `llm.base_url` |
| `OPENAIDE_DATA_DIR` | `data_dir`（默认 `~/.openaide`） |
| `OPENAIDE_PLUGINS_DIR` | `plugins_dir`（默认 `<data_dir>/plugins`） |
| `OPENAIDE_PORT` | `serve` 命令端口（默认 8080） |

### 1.2.0 内核运行时行为

- **上下文预算**：`kernel.max_tokens` 是上下文 token 预算（默认 200000），用于历史裁剪与压缩阈值——不是单次回复的输出上限。历史按条数（20）与 token 预算双重裁剪，防止长会话上下文膨胀。
- **前缀缓存友好**：system 层（L0 人格 + 项目规则）跨查询字节级稳定；技能提示作为独立消息注入；system 消息携带 `cache_control`。使用 DeepSeek 等 provider 时，多轮对话与跨查询都能命中前缀缓存，prompt 成本大幅降低。
- **空回复即失败**：LLM 返回空内容（限流/静默失败）视为错误——不会显示"完成"却无回复，也不会把空消息写入会话。

### 1.2.2 会话存储与跨设备续聊（多人协作版）

会话**一律随项目走**：在任意目录运行 openaide，工作区自动解析/创建于 `<项目根>/.openaide/`，子目录启动自动归位。无需 init、无需配置。

```
.openaide/
├── sessions/<开发者>/*.json   按人隔离的会话快照（可读、可 diff、按内容 updatedAt 排序）
├── memory/<开发者>/*.jsonl    按人隔离的记忆流水
└── knowledge/*.md             团队共享知识（进 git，自动注入 L1 上下文）
```

**身份**：取 `git config user.name`（降级链：OPENAIDE_USER → email 前缀 → 系统用户名 → 随机 id）。
中途配好名字后目录自动跟随改名；多人各写各的子目录，git 路径不相交，结构上无合并冲突。

**跨机续聊三步**：

```bash
# 电脑 A：正常聊天即可——默认 session_sync: commit 会自动提交 .openaide/
git push
# 电脑 B：
git pull && openaide -c     # 接着聊
```

**同步策略**（config.yaml `session_sync`）：`commit`（默认）/ `push` / `off`。
未配置 `user.name` 也能用——自动提交时注入兜底身份，不动你的全局 git 配置。

**状态一览**：`openaide workspace` 显示工作区路径、身份来源、会话数、knowledge 数与
tracked/private 同步状态（`init` 保留为别名）。

- **隐私**：对话会进版本历史。涉密项目在 `.gitignore` 加 `.openaide/`，
  或 `workspace: off` 退回全局 SQLite；脚本/CI 场景推荐后者。
- **同一会话不要两台机器并行聊天**：per-session 文件已把冲突面缩到最小，但并发写仍属反模式。

> **为什么是文件（JSON/JSONL）而不是数据库？** 约束是 git 同步：文本可 diff 可审查、
> 可三方合并；updatedAt 写在内容里，克隆到新机器排序依然正确；零解析依赖。
> SQLite 实现保留给 `workspace: off` 的全局场景（`SessionStore` 接口随时可换）。

### 1.3 交互式使用

```bash
openaide    # TTY 下为 Ink TUI；非 TTY（管道/脚本）自动降级为 readline REPL
```

TUI 布局（Claude Code / Gemini CLI 风格）：

```
◆ OpenAIDE                        ← Banner（品牌/模型/提示，只渲染一次）
❯ you                             ← 用户消息（加粗）
  你好，有什么可以帮你？
● agent                           ← 助手回答（markdown 渲染：标题/列表/**粗体**/`行内码`/代码块）
  ⚡ builtin__read_file({"path":…}) ← 工具调用（青色一行）
╭─ ❯ Send a message or type / … ─╮ ← 圆角输入框
 model · session a1b2c3d4… · plugins: 3 · tools: 6   ← 状态栏（常驻）
```

- 输入 `/` 即弹出**命令补全菜单**（继续输入过滤；`↑/↓` 选择、`Tab` 补全）
- 处理中显示动态 spinner 状态：Thinking → Reasoning → Running <tool> → Responding
- 助手的思考流（reasoning）以暗色斜体实时展示，与正文分离

- **键盘**：`↑/↓` 翻输入历史（弹层打开时切换补全项），`Tab` 补全命令，`Ctrl+C` 退出
- **Esc 两段语义**：输入框有内容时清空输入；输入框已空时进入撤回模式——再按一次 Esc 撤销最后一轮对话（删除该轮 user 提问、中间工具消息与 assistant 回复，会话与记忆两侧同步裁剪），900ms 内连按才生效以防误触
- **斜杠命令**（TUI 与 REPL 通用）：

| 命令 | 作用 |
|---|---|
| `/help` | 显示帮助 |
| `/new` | 开新会话（清上下文） |
| `/sessions` | 列出历史会话 |
| `/use <id>` | 恢复历史会话继续聊 |
| `/model <id>` | 运行时切换模型（无参显示当前） |
| `/plugins` | 列出全部插件（active / disabled / failed 三态）与工具 |
| `/plugins enable\|disable <name>` | 启停插件（写入 `plugin-state.json`，重启仍生效；disable 立即卸载） |
| `/plugins reload <name>` | 热重载插件（破坏模块缓存重新 import） |
| `/undo` | 撤回最后一轮对话（TUI 中空输入双击 Esc 同效） |
| `/persona` | 列出可用人格 + 当前激活 |
| `/persona <name>` | **运行时切换人格**（下一条消息即以新身份响应；`default` 回到内置） |
| `/exit` `/quit` | 退出 |

**浏览器操作**：安装 Playwright 后自动启用 `browser_*` 工具集
（`npm i -D playwright && npx playwright install chromium`）：
`browser_navigate / snapshot / click / type / search / close`——
快照把页面元素编号为 r1..rN，点击/输入按编号引用；
`browser_search` 直连必应返回结构化结果。默认无头，
`OPENAIDE_BROWSER_HEADLESS=0` 显示窗口。未安装 playwright 时工具会返回安装指引。

**联网搜索**：配置任一后端即自动启用 `web_search` 工具（模型会按需自行调用）：

```bash
export TAVILY_API_KEY=tvly-xxx     # 推荐，tavily.com 免费注册
export BRAVE_API_KEY=SBC-xxx       # 或 brave.com/search/api 免费 2000 次/月
export SEARXNG_URL=http://localhost:8888   # 或自托管 SearXNG
```

> 不用手选工具——内核 ReAct 循环自动判断何时调用哪个工具（`builtin__*` 或插件注册的 `<插件>__<工具>`）。会话默认持久化到 `~/.openaide/sessions.db`，重启不丢。

### 任务变身：一个 SYSTEM.md 变成任意领域 agent

把 OpenAIDE 从编程助手变成任何领域的 agent,只需两个文件（零代码）：

```bash
mkdir -p ~/.openaide/plugins/travel-planner
cat > ~/.openaide/plugins/travel-planner/SYSTEM.md << 'EOF'
You are Voyager, an expert travel planning agent. You are NOT a coding assistant.
Design detailed itineraries, estimate budgets, advise on visas and seasons.
EOF
cat > ~/.openaide/plugins/travel-planner/openaide.yaml << 'EOF'
name: travel-planner
version: 1.0.0
description: 旅行规划师
persona: voyager
toolAllowlist:          # 可选:变身后暴露的工具白名单
  - builtin__read_file
  - builtin__list_directory
EOF

openaide repl
# /persona voyager   ← 热切换,下一条消息即以旅行规划师身份响应
```

- `SYSTEM.md` **整体替换**系统提示词(L0)——身份、规则、能力边界全由它定义
- `toolAllowlist` 联动:激活后内核只暴露白名单内的工具(不写则暴露全部)
- `/persona default` 随时切回内置编码助手

### 子代理编排（Agent-as-Tool）

在 config.yaml 声明后，每个人格即成为一个可委派的工具（`subagent__<名字>`）：

```yaml
kernel:
  subagents:
    - name: travel
      persona: voyager          # 任意插件提供的人格（含声明式 SKILL.md/SYSTEM.md）
      description: 行程规划专家
      max_rounds: 6             # 可选，默认继承 max_rounds
```

父 agent 调用 `subagent__travel({task})` 时，内核以 voyager 人格 + 其工具白名单
跑一轮完全隔离的 ReAct 循环（独立临时会话、不污染父上下文），把最终答复作为
工具结果回传。父级中断会级联中止子循环。

**SKILL.md 生态兼容**：目录里放一个 `SKILL.md`（dsh / agent-skills 可移植技能格式，
支持 YAML frontmatter 的 name/description）同样会被识别为声明式人格插件——
零配置投喂：把任何 SKILL.md 文件夹丢进 pluginsDir 即生效。SYSTEM.md 与之并存时优先。

**MCP 桥接**：Claude 生态的 MCP server 直接变成原生工具。config.yaml 加：

```yaml
mcp:
  servers:
    filesystem:
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/some/dir"]
      env: { HOME: "/home/user" }   # 支持 ${ENV_VAR} 引用
```

- 工具注册为 `<server>__<tool>`，inputSchema 直通 MCP 的 JSON Schema
- 配置格式与 Claude Desktop / Claude Code 的 `mcpServers` 一致——可直接复制过来
- 官方 MCP SDK + stdio 传输；工具调用超时默认 60s（`timeoutMs` 可调）

### 1.4 一次性问答

```bash
openaide "把项目里所有 README 标题改成大写"
openaide "看看当前目录有什么文件"     # 会自动调用 builtin 读文件/列目录/执行命令
```

高级参数（可任意组合）：

```bash
# 指定模型
openaide --model deepseek-v4 "解释这个 bug"

# 文件作为上下文注入 prompt
openaide src/main.ts "找这个文件里的 bug"

# JSON 输出（脚本/管道友好）
openaide --output json "1+1=?"
# → {"content":"...","sessionId":"..."}
```

### 1.5 API 服务与 WebUI

```bash
openaide serve                      # 默认 http://127.0.0.1:8080
OPENAIDE_PORT=9000 openaide serve
```

`serve` 会自动托管 `frontend/dist`（存在时）——浏览器打开 `http://127.0.0.1:8080` 即得内置 WebUI（Vite + React SPA）：

- 会话列表：新建 / 恢复历史 / 删除
- 聊天流：SSE 实时渲染 markdown，思考过程折叠块，工具调用卡片（running/ok/failed）
- 顶栏连接状态 + 当前模型；生成中可随时 Stop 中断

前端开发模式：

```bash
npm install                 # frontend 已纳入 workspaces
npm run dev --workspace frontend   # Vite dev server :5173，代理 API 到 :8080
npm run build --workspace frontend # 构建到 frontend/dist（serve 自动托管）
```

端点：

| 端点 | 说明 |
|---|---|
| `GET /health` | 健康检查与服务信息 |
| `GET/POST /sessions` | 会话列表 / 创建 |
| `GET/DELETE /sessions/:id` | 会话详情（含消息）/ 删除 |
| `POST /v1/chat` | 非流式问答 `{content, session_id?, project_id?}` |
| `POST /v1/chat/stream` | 流式问答（SSE: ready/chunk/error/done 帧） |
| `WS /ws` | 双向流式对话 |

```bash
curl -X POST http://127.0.0.1:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"你好","session_id":"s1"}'
```

### 1.5.1 自定义界面（TUI 也是插件）

内置 ink TUI / readline REPL 本身就是两个界面插件。你可以完全替换它们：

\`\`\`ts
// ~/.openaide/plugins/my-ui/index.ts
import type { OpenAIDePlugin } from '@openaide/plugins';

const plugin: OpenAIDePlugin = {
  name: 'my-ui',
  uis: [{
    name: 'fancy',
    start: async (host) => {
      // host.kernel.processStream(...) —— 界面长什么样，完全由你决定
      // host.registry / host.bus / host.setApprovalHandler?.(...) 同样可用
    },
  }],
};
export default plugin;
\`\`\`

启用：`OPENAIDE_UI=fancy openaide` 或 config.yaml `ui: fancy`。
选择优先级：显式指定 > TTY 默认(ink)/非 TTY(readline) > 回退链；示例见
`examples/plugins/example-mini-ui`（40 行极简问答界面）。

### 1.6 插件的使用

默认从 `~/.openaide/plugins/` 自动发现加载，放一个含 `index.ts` 的目录即生效：

```bash
# macOS / Linux（bash）
OPENAIDE_PLUGINS_DIR=examples/plugins openaide
# 输入 "upper hello" → 调用 example__upper 工具

# Windows（PowerShell）
$env:OPENAIDE_PLUGINS_DIR="examples/plugins"; openaide
```

或复制到默认目录：

```bash
# macOS / Linux
cp -r examples/plugins/example-plugin ~/.openaide/plugins/

# Windows（PowerShell）
Copy-Item -Recurse examples/plugins/example-plugin "$HOME\.openaide\plugins\"
openaide
```

启停管理（CLI 与 REPL/TUI 斜杠命令等价）：

```bash
openaide plugins                  # 列出全部插件（active / disabled / failed 三态）与工具
openaide plugins disable travel   # 立即卸载 + 写入 <data_dir>/plugin-state.json，重启后不再加载
openaide plugins enable travel    # 从名单移除并立即重新加载（目录仍存在时）
openaide plugins reload travel    # 热重载单个插件
```

- 禁用按**插件名**记录；内置插件（`builtin`、`file-tools`）同样可禁用，启动时跳过注册
- 名单文件 `<data_dir>/plugin-state.json` 可手改，损坏时自动回退为空状态

插件市场 = **GitHub 实时生态 ∪ 静态种子索引**：

```bash
openaide plugins search            # GitHub(topic:openaide-plugin) + 种子索引 全域搜索
openaide plugins search travel     # 关键词追加进 GitHub 查询（匹配名称/描述/README）
openaide plugins install <name>    # git clone 浅克隆 → 拷入 pluginsDir → 立即加载
openaide plugins uninstall <name>  # 卸载并删除目录（含清理禁用名单残留）
```

- 搜索结果带来源徽标：`[gh★42]`（GitHub 实时，按星排序）/ `[registry]`（静态索引）；两来源自动去重合并，任一失败另一个兜底
- **发布零门槛**：GitHub 建仓库 → 打 topic `openaide-plugin` → 根目录放 `openaide.yaml`（推荐，含 name/version；纯人格插件放 SYSTEM.md 即可）→ 即可被全生态搜到。设置 `GITHUB_TOKEN` 环境变量可提升搜索配额（匿名 10 次/分钟）
- 静态种子索引：本仓库 `registry/plugins.json`（`registry_url` / `OPENAIDE_REGISTRY_URL` 覆盖，支持 `file://` 离线索引），作为 GitHub 的离线兜底与精选位
- REPL/TUI 内等价命令：`/plugins search <kw>`、`/plugins install|uninstall <name>`

---

## 二、开发者（Developer）

### 2.1 在仓库内开发

```bash
git clone git@github.com:lzy1102/openaide.git
cd openaide
npm install            # 或 node scripts/install.mjs
npm run dev            # tsx 免编译运行，改代码即时生效
```

开发命令汇总：

| 命令 | 用途 |
|---|---|
| `make build` / `make test` / `make serve` … | Makefile 统一入口（npm 脚本的薄封装；Windows 无 make 时用下述 npm 等价命令） |
| `make binary` | 单文件可执行（需 bun ≥1.1）：内嵌运行时 + bun:sqlite，零依赖分发；`make binary-all` 交叉编译 linux/darwin |
| `npm run dev` / `npm run dev:repl` / `npm run dev:serve` | 开发运行（免编译；serve = API+WebUI） |
| `npm run build` | 编译所有包到 `dist/`（frontend 走 vite build） |
| `npm run typecheck` | 全部包类型检查（含 frontend） |
| `npm test` | 全部测试：node --test（packages）+ vitest（frontend） |
| `node scripts/test.mjs <包名>` | 只跑某个包的测试 |
| `npm run dev -- plugins` | 查看插件/工具清单 |

> 注意：`npm run dev -- serve` 中 `--` 与子命令之间必须有空格，否则参数被 npm 吞掉。

### 2.2 写一个插件

```bash
# 在 examples/plugins/my-tool/index.ts
```

```ts
import type { OpenAIDePlugin } from '@openaide/plugins';

const plugin: OpenAIDePlugin = {
  name: 'my-tool',
  category: 'capability',
  tools: [
    {
      name: 'ping',
      description: 'ping 测试',
      parameters: { type: 'object', properties: {} },
      handler: async () => ({ content: 'pong' }),
    },
  ],

  // 拦截器（策略）：可否决/改写工具调用与 LLM 请求
  interceptors: [
    {
      name: 'my-guard',
      beforeToolCall(info) {
        if (/rm\s+-rf/i.test(info.argsJson)) return { action: 'deny', reason: 'destructive' };
        if (info.tool === 'builtin__write_file') {
          // modify：改写参数（如统一编码风格）
          return { action: 'modify', payload: info.argsJson.replace(/\r\n/g, '\n') };
        }
        return { action: 'allow' };
      },
    },
  ],

  // LLM Provider 注册：config.llm.provider: 'acme' 即切换大脑
  providers: [
    {
      name: 'acme',
      create: (cfg) => new MyAcmeProvider(cfg),
    },
  ],
};
export default plugin;
```

拦截器判定语义：`allow` 放行 → `deny`（带 reason，回给模型自行调整）→ `modify` 替换载荷后继续向后传递。三个挂点：`beforeLLM`（脱敏/预算熔断）、`beforeToolCall`（审批门/参数改写）、`afterToolCall`（结果过滤/脱敏）。

工具审批：配置 `kernel.approval: dangerous`（危险工具需确认）或 `always`；TUI 弹出黄色确认卡按 y/n，REPL 行内输入 y/N，API 服务未接 UI 时 fail-closed 默认拒绝。

调试：

```bash
OPENAIDE_PLUGINS_DIR=examples/plugins openaide
```

插件协议详见 [ARCHITECTURE.md](ARCHITECTURE.md#5-插件接口与生命周期) 与 [examples/README.md](../examples/README.md)。

### 2.3 目录速览

| 目录 | 职责 |
|---|---|
| `packages/core/` | 内核（ReAct、事件、接口）—— 一般不动 |
| `packages/plugins/` | 插件机制（加载器、管理器、类型） |
| `packages/tools/` | 内置工具（文件/命令），以插件形态 |
| `packages/llm/` | OpenAI 兼容网关 |
| `packages/config/` | yaml 配置 |
| `packages/memory/` | SQLite 会话 |
| `packages/cli/` | REPL/TUI/命令入口 + bin 全局入口 |
| `packages/api/` | HTTP/WS 服务 |
| `docs/ARCHITECTURE.md` | 内核-插件契约（开发必读） |

### 2.4 发布与分发

发布形态：**源码包 + tsx 运行时**。各包 `main`/`exports` 指向 `src/*.ts`，cli 的 `bin`（`packages/cli/bin/openaide.js`）经 `import('tsx')` 运行时加载，**无需预编译**，本地全局 link 与 npm 全局安装行为一致（已用 tarball 隔离安装验证）。

一键发布全部 `@openaide/*` 包（按依赖顺序）+ 打 tag：

```bash
node scripts/publish.mjs               # 发布 + git tag v<version> + push tag
node scripts/publish.mjs --no-tag      # 仅发布（CI 用）
node scripts/publish.mjs --tag=v0.3.0  # 发布 + 指定 tag
```

发布顺序：`core → config/llm/plugins/memory → tools/api → cli`。打 tag 后 GitHub Actions 的 `release` job 会自动再次发布并创建 GitHub Release。

**别人怎么安装（最终用户）**

```bash
# 方式 A：全局安装（推荐，安装后命令行直接可用）
npm install -g @openaide/cli
openaide            # 交互式
openaide "问题"      # 一次性问答
openaide serve      # API 服务

# 方式 B：源码/自托管（无需 npm registry）
git clone https://github.com/lzy1102/openaide.git
cd openaide
node scripts/install.mjs    # npm install + 全局 link openaide
```
## 基准测试

内置两套 agent 能力基准:

```bash
# 通用能力 8 题(工具调用/代码理解/多步任务)
npx tsx scripts/eval.ts            # 全量
npx tsx scripts/eval.ts --quick    # 仅 easy

# Aider polyglot 编程题 pass@1(10 道 Exercism 练习)
npx tsx scripts/polyglot-eval.ts           # 全量
npx tsx scripts/polyglot-eval.ts proverb   # 指定题
```

polyglot harness 流程:每题在独立工作目录放置 stub + 官方 unittest,agent 自主实现并可用 pytest 迭代,最终以官方测试判定 pass/fail。当前成绩 (DeepSeek):**pass@1 = 8/10 (80%)**。
