# OpenAIDE 架构

本文档描述 OpenAIDE 的插件化架构：**内核（thin core）与插件之间的契约（seam）、约束原则、暴露信息、生命周期与扩展点**。

> 版本：v0.3.0（TypeScript monorepo）｜插件同进程动态加载，无多进程。

---

## 1. 概览

OpenAIDE 遵循 **thin core + plugin** 架构：内核只做智能决策的最小闭环（ReAct 循环、提示词组装、会话、事件总线），一切能力（工具、人格、钩子）通过**插件**注入。

```
cli (REPL + TUI + 命令)      api (HTTP/WS + SSE)
        ↓                        ↓
      core ── AgentKernel (ReAct 循环, 事件总线, 会话)
        ↓                        ↓
   plugins (动态加载)      memory (SQLite)   llm (OpenAI 网关)
        ↓
   tools (内置, 以插件形态)  config (yaml + env)
```

| 包 | 职责 |
|---|---|
| `@openaide/core` | 内核：类型、事件总线、ReAct 循环、AgentKernel、接口定义 |
| `@openaide/plugins` | 插件：`OpenAIDePlugin` 接口、动态加载器、生命周期管理 |
| `@openaide/llm` | LLM 网关（OpenAI 兼容，纯 fetch，流式） |
| `@openaide/tools` | 内置工具（文件/命令），以插件形态注册 |
| `@openaide/config` | yaml 配置 + 环境变量覆盖 |
| `@openaide/memory` | SQLite 会话持久化 |
| `@openaide/cli` | REPL / Ink TUI / 命令入口 |
| `@openaide/api` | HTTP REST / SSE / WebSocket 服务 |

---

## 2. 核心原则

插件与内核之间的约束由以下原则决定（详见 `packages/core/src/interfaces.ts`、`packages/plugins/src/types.ts`）：

1. **依赖倒置（Dependency Inversion）**
   内核不依赖任何具体插件，只依赖自己定义的接口；插件实现接口；装配层 `buildApp()` 注入。依赖方向单向：`插件 → 接口契约 ← 内核`。

2. **接口隔离**
   `ToolExecutor` / `PersonaProvider` / `EventBus` / `SessionStore` 等接口相互独立，各司其职。

3. **命名空间隔离**
   工具注册为 `<插件名>__<工具名>`（如 `example__upper`），不同插件工具不可能冲突。

4. **单向订阅**
   内核通过事件总线发布事件，插件订阅感知；插件不能反向控制内核内部状态。

5. **生命周期契约**
   插件只需实现 `OpenAIDePlugin`（`activate` / `deactivate` / `tools` / `hooks` / `persona`），加载、注册、清理全部由 `PluginManager` 接管。

6. **同进程、一切皆插件**
   插件经 `import()` 同进程加载，无子进程通信成本；内置插件与用户插件走同一套注册逻辑。

---

## 3. 契约层（Seam）

内核只依赖这些接口（`packages/core/src/interfaces.ts`）：

| 接口 | 契约 | 插件/实现方 |
|---|---|---|
| `ToolExecutor` | `register` / `execute` / `definitions` / `unregister` | 工具注册表（`@openaide/tools`），插件经此注册工具 |
| `LLMProvider` | `chat` / `chatStream` / `getModelId` | LLM 网关（`@openaide/llm`） |
| `PersonaProvider` | `active()` | 人格（L0 系统提示词）来源 |
| `EventBus` | `subscribe` / `unsubscribe` / `publish` | 内核事件分发，插件钩子订阅目标 |
| `SessionStore` | `create` / `get` / `update` / `list` / `delete` | 会话持久化（`@openaide/memory`） |
| `Memory` | `save` / `load` | 会话历史存取 |
| `ContextCompressor` | `compress` / `estimateTokens` | 上下文压缩策略（可插件化） |
| `PermissionChecker` | `check` | 工具执行前鉴权（可插件化） |
| `ModelSwitcher` | `setModelId` | 运行时切换模型（可插件化） |

---

## 4. 暴露给插件的信息

插件可见的世界（`packages/plugins/src/types.ts`）：

### 4.1 插件上下文 `PluginContext`

```ts
interface PluginContext {
  dir: string;      // 插件自身目录（读取资源文件）
  dataDir: string;  // 应用数据目录
}
```

在 `activate(ctx)` 时注入。

### 4.2 工具 handler 入参

```ts
type ToolHandler = (args: Record<string, unknown>, sessionId: string, signal?: AbortSignal) => Promise<ToolResult>;
```

- `args`：按 JSON Schema 解析后的参数对象
- `sessionId`：当前会话 ID
- `signal`：可取消的 `AbortSignal`

### 4.3 事件 `KernelEvent`

```ts
interface KernelEvent {
  type: string;
  source: string;
  data?: Record<string, unknown>;
  timestamp: number;
}
```

事件类型（`packages/core/src/types.ts` 的 `EventTypes`）：

| 事件 | 含义 |
|---|---|
| `query.start` / `query.end` | 一次查询开始/结束 |
| `round.start` / `round.end` | ReAct 一轮开始/结束 |
| `tool.call.started` / `tool.call.ended` | 工具调用开始/结束 |
| `tool.executed` | 工具执行完成 |
| `message.added` | 消息入会话 |
| `session.created` | 会话创建 |
| `state.changed` | 内核状态切换（idle/thinking/executing…） |
| `error` | 错误 |

内核不暴露内部私有状态——插件只能经「接口调用」与「事件感知」两条通道交互。

### 4.4 人格

人格可经三种方式提供，统一归一化为 `Persona`：
1. 插件静态字段 `persona`
2. `persona` 为函数（异步返回）
3. `SYSTEM.md` 外置文件（`readPersonaFile`）

```ts
interface Persona {
  name: string;
  description: string;
  systemPrompt: string;      // L0 系统提示词
  toolAllowlist?: string[];  // 工具白名单
}
```

---

## 5. 插件接口与生命周期

### 5.1 插件统一接口

```ts
interface OpenAIDePlugin {
  name: string;
  version?: string;
  description?: string;
  activate?(ctx: PluginContext): void | Promise<void>;   // 激活：注册工具/钩子/人格
  deactivate?(): void | Promise<void>;                   // 卸载
  tools?: PluginTool[];                                  // 工具集
  hooks?: PluginHook[];                                  // 事件钩子
  persona?: Persona | (() => Persona | undefined | Promise<Persona | undefined>);
}
```

### 5.2 生命周期（`PluginManager`）

```
load(dir) ──▶ loadPlugin(dir)    动态 import() 插件入口
        └──▶ activate(loaded)    注册工具(<插件>__<工具>) → 挂钩子到 EventBus → 收集人格
reload(name) ──▶ unload + 重新 import（URL 加时间戳破坏模块缓存 = 热重载）
unload(name) ──▶ 注销工具 → 退订钩子 → 调 deactivate
```

关键行为（`packages/plugins/src/manager.ts`）：
- **加载顺序**：扫描 `pluginsDir` 一级子目录，判定含 `openaide.yaml` 或入口文件即为插件（`discover`）
- **容错**：单个插件加载失败仅告警，不阻塞其余
- **热重载**：`reload(name)` 破坏模块缓存重新 import
- **内置插件同路**：`builtinToolsPlugin` 经 `plugins.add()` 注册，与用户插件一致

### 5.3 插件目录约定

```
~/.openaide/plugins/            # 默认插件目录（启动自动创建）
├── <插件名>/                   # 每个插件一个子目录
│   ├── index.ts               # 必需：导出 OpenAIDePlugin
│   ├── openaide.yaml          # 可选：name/version/description/persona
│   ├── SYSTEM.md              # 可选：人格系统提示词
│   └── package.json           # 可选：插件自身依赖
```

覆盖方式（优先级从高到低）：
1. 环境变量 `OPENAIDE_PLUGINS_DIR`
2. 配置文件 `config.yaml` 的 `plugins_dir`
3. 默认 `~/.openaide/plugins`

---

## 6. 装配（buildApp）

`packages/cli/src/app.ts` 的 `buildApp()` 完成依赖注入：

```
创建 ToolRegistry（工具注册表）
创建共享 EventBus（内核发布 → 插件订阅）
创建 PluginManager（pluginsDir + executor + eventBus）
  ├── add(builtinToolsPlugin)    内置工具（插件形态）
  └── loadAll()                  动态加载用户插件
创建 LLMProvider / SQLiteSessionStore
创建 PluginPersonaProvider（active 人格）
创建 AgentKernel（llm + tools + sessions + persona + eventBus）
```

---

## 7. 扩展点（无限可能）

内核的扩展面固定为两类：**接口 seam** 与**事件类型**。插件数量不限。

| 已有 seam | 扩展方向 |
|---|---|
| `ToolExecutor` | 任意工具（文件/Git/Web/浏览器/MCP…）；MCP 可封装为插件包 |
| `EventBus` + 事件类型 | 插件响应任意生命周期：审批、通知、遥测、自动重试 |
| `PersonaProvider` | 人格 / 角色 / 工作流预设全部可插拔 |
| `ContextCompressor` | 上下文压缩策略插件化 |
| `PermissionChecker` | 权限鉴权策略插件化 |
| `LLMProvider` / `ModelSwitcher` | 多模型提供方（本地 Ollama、路由） |
| 非 TS 插件 | `PluginManifest.tools[].module` 已为任意语言插件预留（未来子进程桥接） |

---

## 8. 相关文件索引

| 领域 | 文件 |
|---|---|
| 内核接口 | `packages/core/src/interfaces.ts` |
| 内核类型 / 事件 | `packages/core/src/types.ts` |
| 事件总线 | `packages/core/src/events.ts` |
| 内核实现 | `packages/core/src/kernel.ts` / `react.ts` / `prompt.ts` |
| 插件类型 | `packages/plugins/src/types.ts` |
| 插件加载器 | `packages/plugins/src/loader.ts` |
| 插件管理器 | `packages/plugins/src/manager.ts` |
| 装配 | `packages/cli/src/app.ts` |
| 配置 | `packages/config/src/config.ts` |
| 示例插件 | `examples/plugins/example-plugin/` |
