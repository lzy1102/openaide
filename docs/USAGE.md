# OpenAIDE 使用指南

本文档分「使用者」和「开发者」两部分，覆盖安装、配置、交互式/一次性使用、API 服务与插件开发。

> 版本：v0.3.0（TypeScript monorepo）｜插件同进程动态加载，无多进程。

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
kernel:
  max_rounds: 10
  max_tokens: 4000
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

### 1.3 交互式使用

```bash
openaide    # TTY 下为 Ink TUI；非 TTY（管道/脚本）自动降级为 readline REPL
```

TUI 顶部显示会话 ID / 工具数 / 插件；对话流实时显示思考、工具调用、回答。底部直接打字回车即发送。

- **键盘**：`↑/↓` 翻输入历史，`Ctrl+C` 退出
- **斜杠命令**（TUI 与 REPL 通用）：

| 命令 | 作用 |
|---|---|
| `/help` | 显示帮助 |
| `/new` | 开新会话（清上下文） |
| `/sessions` | 列出历史会话 |
| `/use <id>` | 恢复历史会话继续聊 |
| `/plugins` | 查看已加载插件与工具 |
| `/persona` | 查看可用人格 |
| `/exit` `/quit` | 退出 |

> 不用手选工具——内核 ReAct 循环自动判断何时调用哪个工具（`builtin__*` 或插件注册的 `<插件>__<工具>`）。会话默认持久化到 `~/.openaide/sessions.db`，重启不丢。

### 1.4 一次性问答

```bash
openaide "把项目里所有 README 标题改成大写"
openaide "看看当前目录有什么文件"     # 会自动调用 builtin 读文件/列目录/执行命令
```

### 1.5 API 服务（给前端/远程使用）

```bash
openaide serve                      # 默认 http://127.0.0.1:8080
OPENAIDE_PORT=9000 openaide serve
```

端点：

| 端点 | 说明 |
|---|---|
| `GET /health` | 健康检查与服务信息 |
| `POST /v1/chat` | 非流式问答 `{content, session_id?, project_id?}` |
| `POST /v1/chat/stream` | 流式问答（SSE） |
| `WS /ws` | 双向流式对话 |

```bash
curl -X POST http://127.0.0.1:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"你好","session_id":"s1"}'
```

### 1.6 插件的使用

默认从 `~/.openaide/plugins/` 自动发现加载，放一个含 `index.ts` 的目录即生效：

```bash
# 用示例插件演示
OPENAIDE_PLUGINS_DIR=examples/plugins openaide
# 输入 "upper hello" → 调用 example__upper 工具

# 或复制到默认目录
cp -r examples/plugins/example-plugin ~/.openaide/plugins/
openaide
```

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
| `npm run dev` | 开发运行（免编译） |
| `npm run build` | 编译所有包到 `dist/` |
| `npm run typecheck` | 全部包类型检查 |
| `npm test` | 全部测试（Node 18 `node --test`） |
| `npm test --workspace @openaide/<包名>` | 单包测试 |
| `npm run dev -- plugins` | 查看插件/工具清单 |

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
};
export default plugin;
```

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