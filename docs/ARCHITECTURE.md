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

7. **内核稳定（Kernel Stability）**
   内核是**近乎不变**的扩展面：一般情况下不随需求演进而改动。任何新行为都优先以插件吸收，内核只做收敛式演进。

   内核允许的变更（白名单）：
   - 修复 bug、优化 ReAct 循环内部实现（不改变外部行为）
   - 新增接口（向后兼容，不修改既有签名）
   - 新增事件类型（订阅方不感知新增）
   - 调整装配/配置默认值

   内核不允许的变更（红名单）：
   - 修改既有接口签名（破坏性变更，需大版本并同步迁移）
   - 在内核中硬编码业务能力（工具/人格/钩子一律插件化）
   - 引入对具体插件的反向依赖

   含义：插件作者面对的是**只增不减的契约**——今天实现一个接口，未来依然成立；新需求靠"新接口 + 新事件"扩展，不靠改内核。

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

人格可经四种方式提供，统一归一化为 `Persona`：
1. 插件静态字段 `persona`
2. `persona` 为函数（异步返回）
3. `SYSTEM.md` 外置文件（`readPersonaFile`）
4. **声明式 persona 插件**——目录只有 `openaide.yaml` + `SYSTEM.md`，无代码入口；loader 生成虚拟插件（人格即全部内容，支持"任务变身"）

```ts
interface Persona {
  name: string;
  description: string;
  systemPrompt: string;      // L0 系统提示词
  toolAllowlist?: string[];  // 工具白名单
}
```

**任务变身**：激活 persona 后，内核把 `toolAllowlist` 应用到 ReactContext——工具集跟随人格变化。
例如旅行规划师 persona 声明 `toolAllowlist: [builtin__read_file, builtin__list_directory, builtin__write_file]`,
激活后 agent 只见这三个工具，完全脱离编程助手身份。切换通过 `/persona <name>` 运行时热切换。

---

## 5. 插件接口与生命周期

### 5.1 插件统一接口

```ts
interface OpenAIDePlugin {
  name: string;
  version?: string;
  description?: string;
  category?: string;                                 // 分类（轻量元数据，供展示/检索）
  activate?(ctx: PluginContext): void | Promise<void>;   // 激活：注册工具/钩子/人格
  deactivate?(): void | Promise<void>;                   // 卸载
  tools?: PluginTool[];                                  // 工具集
  hooks?: PluginHook[];                                  // 事件钩子
  persona?: Persona | (() => Persona | undefined | Promise<Persona | undefined>);
}
```

### 5.2 插件分类

分类是**轻量元数据**：给人和工具看的标签，**不是内核的运行时开关**——加载、执行、生命周期完全不受分类影响。

- 来源优先级：代码声明 `category` > `openaide.yaml` 的 `category` > 缺省 `uncategorized`
- 解析实现：`resolveCategory()`（`packages/plugins/src/manager.ts`）
- 消费方：`PluginManager.list()` → `openaide plugins` 命令按分类分组展示
- 建议维度：按 **seam 类型** 而非功能域（与架构 7 类 seam 一一对应）

| 建议值 | 对接层 | 示例 |
|---|---|---|
| `kernel` | 内核/装配 | 扩展内核策略、新接口实现 |
| `capability` | 能力（工具组） | 文件、web、浏览器、MCP 工具集 |
| `infrastructure` | 基础设施 | LLM、记忆、鉴权、压缩器 |
| `ui/entry` | 入口/体验 | 人格、工作流预设、前端入口 |

```yaml
# ~/.openaide/plugins/<插件名>/openaide.yaml
name: mytool
category: capability      # 可选；缺省 'uncategorized'
```

> 注意：目录保持**扁平**（插件直接放在 `pluginsDir` 一级子目录）。分类只作为元数据声明，不改变目录结构——`discover` 只扫描一级子目录，放入嵌套目录将不会被发现。

### 5.3 生命周期（`PluginManager`）

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

启停管理（`state.ts` + `manager.disable/enable/isDisabled`）：
- 禁用名单持久化在 `<data_dir>/plugin-state.json`（按插件名）；构造时读入，`loadAll` 跳过名单内目录
- `disable(name)` = 立即卸载 + 写盘；`enable(name)` = 移出名单 + 目录仍存在则立即重载
- `list()` 返回三态快照：`active`（已激活）/ `disabled`（被禁用）/ `failed`（加载失败，含错误信息）
- 内置插件同样可禁用：装配层注册前检查 `isDisabled(name)`

### 5.4 插件目录约定

```
~/.openaide/plugins/            # 默认插件目录（启动自动创建）
└── <插件名>/                   # 每个插件一个一级子目录（扁平，不按类别嵌套）
    ├── index.ts               # 必需：导出 OpenAIDePlugin
    ├── openaide.yaml          # 可选：name/version/description/category/persona
    ├── SYSTEM.md              # 可选：人格系统提示词
    └── package.json           # 可选：插件自身依赖
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
创建 PluginManager（pluginsDir + executor + eventBus + 拦截器活数组）
  ├── add(builtinToolsPlugin)    内置工具（插件形态，受 plugin-state 禁用约束）
  └── loadAll()                  动态加载用户插件（跳过禁用名单）
解析 LLM Provider（config.llm.provider → 插件注册表；缺省内置网关）
存储选路（见下）
创建 PluginPersonaProvider（active 人格，可运行时切换）
创建 AgentKernel（llm + tools + sessions + memory + persona + interceptors + eventBus）
```

### 6.1 存储选路与格式取舍

```
findProjectWorkspace(): cwd 向上探测 .openaide/
  ├── 命中 → FileSessionStore + FileMemory（会话随仓库走，git 同步即跨机器）
  └── 未命中 → SQLiteSessionStore + SqliteMemory（全局 ~/.openaide/，大规模场景）
显式 data_dir / OPENAIDE_DATA_DIR 配置仍优先于自动探测。
```

项目工作区选**文件而非数据库**的理由（约束 = git 同步）：
- 可 diff 可审查：`git log -p` 直接看每轮对话增量
- 一个会话一个文件：merge 冲突面缩到单会话；坏文件只丢一个会话不殃及全库
- updatedAt 写进内容而非 mtime：克隆到新机器排序依然正确
- sessions 用 JSON（整体快照、全量重写），memory 用 JSONL（append-only 匹配增量写入）——两种访问模式各取所长

SQLite 驱动经 `memory/sqlite-driver.ts` 适配双形态：Node 走 better-sqlite3，
Bun 单文件二进制走内置 bun:sqlite（better-sqlite3 的 bindings 运行时探测在
编译产物虚拟 FS 中必然失败）。

---

## 7. 扩展点（无限可能）

内核的扩展面固定为两类：**接口 seam** 与**事件类型**。插件数量不限。

| 已有 seam | 扩展方向 |
|---|---|
| `ToolExecutor` | 任意工具（文件/Git/Web/浏览器/MCP…）；MCP 可封装为插件包 |
| `EventBus` + 事件类型 | 插件响应任意生命周期：审批、通知、遥测、自动重试 |
| `Interceptor`（拦截链） | **策略插件**：可否决/改写 LLM 请求与工具调用——审批门（`kernel.approval`）、PII 脱敏、预算熔断、结果过滤；链式按序执行，deny 短路，modify 载荷向后传递 |
| `PluginProvider`（Provider 注册表） | 插件注册任意 LLM 后端（Anthropic 原生/Ollama/vLLM…），`config.llm.provider` 一行切换 |
| `PersonaProvider` | 人格 / 角色 / 工作流预设全部可插拔 |
| `ContextCompressor` | 上下文压缩策略插件化 |
| `PermissionChecker` | 布尔鉴权（简单场景）；需要改写能力时用 Interceptor |
| `LLMProvider` / `ModelSwitcher` | 多模型提供方（本地 Ollama、路由） |
| 非 TS 插件 | `PluginManifest.tools[].module` 已为任意语言插件预留（未来子进程桥接） |

> 拦截链执行顺序：数组序 = 插件加载顺序 + 内置插件在前；装配层会把审批拦截器 unshift 到链首。

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
