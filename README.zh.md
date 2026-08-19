# OpenAIDE — AI Agent 内核

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[English](README.md) | [中文](#)

**TypeScript 实现的 AI Agent 内核——一切皆插件。** ReAct 循环 · 插件动态加载 · SQLite 持久化 · OpenAI 兼容网关

> 同进程动态加载插件（无子进程）——插件通过统一接口注册工具、钩子与人格。

---

## 安装

需要 Node.js ≥ 18。

```bash
# 安装依赖 + 全局 link openaide 命令（推荐）
node scripts/install.mjs

# 或仅开发模式：只装依赖，不生成全局命令
npm install
```

全局安装后可用 `openaide` 命令。卸载全局命令：

```bash
npm unlink -g openaide
```

## 编译与运行

```bash
npm run dev            # tsx 直接运行（免编译，默认）
npm run build          # 编译所有包到 dist/
npm run typecheck      # 全部包类型检查
npm test               # 运行全部测试

openaide               # 交互式 REPL
openaide "修复这个 bug" # 一次性问答
openaide --version
```

## 配置

配置文件位于 `~/.openaide/config.yaml`（自动创建；模板见 [config.example.yaml](config.example.yaml)，也可运行 `openaide setup` 向导）。所有字段均可被环境变量覆盖。

```bash
openaide setup         # 交互式配置向导
```

```yaml
llm:
  api_key: sk-xxx
  model: deepseek-v4-pro
  base_url: https://api.deepseek.com/v1
kernel:
  max_rounds: 10
  max_tokens: 4000
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

Monorepo（npm workspaces）：

```
cli (REPL + 命令)          api (HTTP/WS + SSE)
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
- **同进程动态加载**：插件经 `import()` 加载，无子进程；破坏模块缓存即可热重载
- **ReAct 循环**：思考 → 工具调用 → 观察 → 循环，支持流式响应
- **SQLite 持久化**：会话重启不丢，REPL 可恢复历史会话
- **OpenAI 兼容网关**：纯 `fetch` 实现，支持流式，无重依赖
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
npm run typecheck  # 全部包类型检查
npm test           # 运行全部测试
```

## License

MIT License — 见 [LICENSE](LICENSE)。
