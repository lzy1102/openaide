# OpenAIDE — AI Agent 内核

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[English](README.md) | [中文](#)

**TypeScript 实现的 AI Agent 内核——一切皆插件。** ReAct 循环 · 插件动态加载 · SQLite 持久化 · OpenAI 兼容网关

> 同进程动态加载插件（无子进程）——插件通过统一接口注册工具、钩子与人格。

---

## 快速开始

需要 Node.js ≥ 18。

```bash
# 1. 安装（装依赖 + 全局 openaide 命令）
node scripts/install.mjs

# 2. 配置（交互式向导，或复制 config.example.yaml）
openaide setup

# 3. 运行
openaide "你好"          # 一次性问答
openaide                  # 交互式 TUI / REPL
openaide serve            # HTTP/WS API 服务
```

## 安装

```bash
# 安装依赖 + 全局 link openaide 命令（推荐）
node scripts/install.mjs

# 或仅开发模式：只装依赖，不生成全局命令
npm install
```

卸载全局命令：

```bash
npm unlink -g openaide
```

## 用法

```bash
openaide "问题"             # 一次性问答
openaide file.ts "问题"     # 文件作为上下文注入，再问答
openaide --model <id> ...   # 任意命令指定模型
openaide --output json "q"  # 一次性问答输出 JSON
openaide -c                 # 恢复最近一次会话续聊
openaide                    # 交互式 TUI（Ink）/ readline REPL
openaide serve              # HTTP/WS + SSE 服务（默认 :8080）— 自动托管 frontend/dist 内置 WebUI
openaide plugins            # 查看全部插件（active / disabled / failed 三态）与工具
openaide plugins search <kw>      # 搜索插件市场
openaide plugins install <name>   # 从市场安装（git 拉取）
openaide plugins disable <name>   # 立即卸载 + 重启后不再加载（持久化）
openaide plugins enable <name>    # 移出禁用名单并立即重新加载
openaide sessions           # 查看历史会话
openaide setup              # 配置向导
openaide --version | --help
```

完整使用指南 → [docs/USAGE.md](docs/USAGE.md)。

## 配置

配置文件位于 `~/.openaide/config.yaml`（自动创建；模板见 [config.example.yaml](config.example.yaml)，也可运行 `openaide setup` 向导）。所有字段均可被环境变量覆盖。

```yaml
llm:
  api_key: sk-xxx
  model: deepseek-v4-pro
  base_url: https://api.deepseek.com/v1
kernel:
  max_rounds: 10
  max_tokens: 200000   # 上下文 token 预算（压缩/历史裁剪阈值），非单次输出上限
```

环境变量覆盖：

| 变量 | 覆盖项 |
|---|---|
| `OPENAIDE_API_KEY` | `llm.api_key` |
| `OPENAIDE_MODEL` | `llm.model` |
| `OPENAIDE_BASE_URL` | `llm.base_url` |
| `OPENAIDE_DATA_DIR` | `data_dir`（默认 `~/.openaide`） |
| `OPENAIDE_PLUGINS_DIR` | `plugins_dir`（默认 `<data_dir>/plugins`） |
| `OPENAIDE_PORT` | `serve` 命令端口（默认 8080） |

## 架构

> 文档：[USAGE.md](docs/USAGE.md)（使用者 + 开发者指南）· [ARCHITECTURE.md](docs/ARCHITECTURE.md)（内核-插件契约）· [examples/README.md](examples/README.md)（示例插件）

Monorepo（npm workspaces）：

```
cli (TUI + REPL + 命令)      api (HTTP/WS + SSE)
        ↓                        ↓
      core ── AgentKernel (ReAct 循环, 事件总线, 会话)
        ↓                        ↓
   plugins (动态加载)      memory (SQLite)   llm (OpenAI 网关)
        ↓
   tools (内置, 以插件形态)  config (yaml + env)
```

> 内核与插件的完整契约（原则、seam、暴露信息、生命周期、扩展点）→ [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

## 核心概念

- **一切皆插件**：内核的工具、人格、钩子全部经插件体系注入——内置与用户插件走同一套注册逻辑
- **策略即插件**：拦截链可否决/改写 LLM 请求与工具调用——审批门、PII 脱敏、预算熔断统统只是插件
- **大脑可插拔**：插件可注册 LLM Provider（Anthropic/Ollama/…），`config.llm.provider` 一行切换
- **一键变身任意 agent**：一个 `SYSTEM.md` 文件即可把 OpenAIDE 变成其他领域 agent（旅行规划师、合同分析师……）——声明式人格插件 + 可选工具白名单，`/persona <名字>` 热切换
- **同进程动态加载**：插件经 `import()` 加载，无子进程；破坏模块缓存即可热重载
- **ReAct 循环**：思考 → 工具调用 → 观察 → 循环，支持流式响应
- **SQLite 持久化**：会话重启不丢，REPL 可恢复历史会话
- **OpenAI 兼容网关**：纯 `fetch` 实现，支持流式，无重依赖
- **Token 效率设计**：稳定的 system 前缀 + `cache_control` 命中 provider 缓存；历史按 token 预算裁剪；LLM 空回复视为失败
- **插件接口**：`OpenAIDePlugin` —— `tools`（注册为 `<插件>__<工具>`）、`hooks`（订阅内核事件）、`persona`（L0 系统提示词）

## 插件示例

见 [examples/plugins/example-plugin](examples/plugins/example-plugin) —— 一个导出工具、钩子、人格的 TypeScript 插件。

```ts
import type { OpenAIDePlugin } from '@openaide/plugins';

const plugin: OpenAIDePlugin = {
  name: 'example',
  version: '0.1.0',
  tools: [
    {
      name: 'upper',
      description: '把 text 转为大写',
      handler: async (args) => ({ content: String(args.text).toUpperCase() }),
    },
  ],
  hooks: [{ event: 'tool.call.ended', handler: async (e) => console.log('工具执行完毕') }],
  persona: { name: 'example', description: '示例人格', systemPrompt: '...' },
};

export default plugin;
```

## 开发

```bash
npm run dev            # tsx 直接运行（免编译，默认）
npm run build          # 编译所有包到 dist/
npm run typecheck      # 全部包类型检查
npm test               # 运行全部测试
```

## GitHub 搜索与 Topics

GitHub 会索引 README 文本，并依据 **topics** 匹配仓库——关键词丰富的标题加上若干 topics，可让项目在 [仓库搜索](https://github.com/search?q=topic%3Aai-agent+language%3ATypeScript&type=repositories) 中更容易被发现。

推荐 topics——在仓库 **Settings → Topics** 一次性添加，或运行（需 [GitHub CLI](https://cli.github.com/)）：

```bash
gh repo edit lzy1102/openaide --add-topic ai-agent --add-topic agent --add-topic llm --add-topic typescript --add-topic plugin --add-topic plugin-system --add-topic openai --add-topic nodejs --add-topic cli --add-topic monorepo
```

同时设置简洁的仓库描述（Settings → About）：

> TypeScript AI agent kernel — everything is a plugin. ReAct loop, in-process plugin loading, SQLite persistence, OpenAI-compatible gateway.

## License

MIT License — 见 [LICENSE](LICENSE)。
