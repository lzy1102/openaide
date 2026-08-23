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

### 1.2.1 内核运行时行为

- **上下文预算**：`kernel.max_tokens` 是上下文 token 预算（默认 200000），用于历史裁剪与压缩阈值——不是单次回复的输出上限。历史按条数（20）与 token 预算双重裁剪，防止长会话上下文膨胀。
- **前缀缓存友好**：system 层（L0 人格 + 项目规则）跨查询字节级稳定；技能提示作为独立消息注入；system 消息携带 `cache_control`。使用 DeepSeek 等 provider 时，多轮对话与跨查询都能命中前缀缓存，prompt 成本大幅降低。
- **空回复即失败**：LLM 返回空内容（限流/静默失败）视为错误——不会显示"完成"却无回复，也不会把空消息写入会话。

### 1.2.1 跨设备续聊：会话随仓库走

默认会话存在全局 `~/.openaide/`（SQLite）。想让对话跟着项目走、换电脑不丢：

```bash
cd your-project
openaide init        # 创建 .openaide/ 工作区（含说明 README）
git add .openaide && git commit -m "chore: openaide workspace"
```

之后该目录（含子目录）下运行 openaide，会话与跨轮记忆自动写入 `.openaide/`：

```
.openaide/
├── README.md              # 说明 + 敏感性提示
├── sessions/<id>.json     # 会话快照（人类可读、可 diff，按内容 updatedAt 排序）
└── memory/<id>.jsonl      # 逐条消息流水（append-only）
```

换机器：`git pull` → `openaide -c` 即恢复最近会话继续聊。选择文件而非 SQLite 正是为了 git 友好；按会话分文件把合并冲突面缩到单个会话。**注意：对话内容会进版本历史，涉密项目请把 `.openaide/` 加入 .gitignore。**

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

- **键盘**：`↑/↓` 翻输入历史（弹层打开时切换补全项），`Tab` 补全命令，`Esc` 清空输入框，`Ctrl+C` 退出
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
| `/persona` | 列出可用人格 + 当前激活 |
| `/persona <name>` | **运行时切换人格**（下一条消息即以新身份响应；`default` 回到内置） |
| `/exit` `/quit` | 退出 |

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

插件市场（静态 JSON 索引 + git 安装，无服务器）：

```bash
openaide plugins search            # 列出市场全部插件；加关键词过滤（匹配名称/描述/分类/关键词）
openaide plugins install example-plugin   # git clone 浅克隆 → 拷入 pluginsDir → 立即加载
openaide plugins uninstall example-plugin # 卸载并删除目录（含清理禁用名单残留）
```

- 默认索引：本仓库 `registry/plugins.json` 的 raw 地址；`config.yaml` 的 `registry_url` 或环境变量 `OPENAIDE_REGISTRY_URL` 可覆盖（支持 `http(s)://` 与 `file://` 自建/离线索引）
- 自建市场：fork 本仓库 → 编辑 `registry/plugins.json`（条目含 name/version/description/keywords/source.git url+subdir）→ 推送即生效
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
