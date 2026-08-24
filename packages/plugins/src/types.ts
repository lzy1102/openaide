/**
 * 插件接入格式 —— TS 模块动态加载（同进程，无子进程）。
 *
 * 一个插件 = 一个目录/包，导出标准接口 OpenAIDePlugin。
 * 运行时装进同一进程，可热加载/热卸载（不用多进程）。
 *
 * 可选 openaide.yaml 声明元信息（name/version/persona/入口），
 * 也可纯代码声明 —— 二选一或并存。
 */
import type { Interceptor, KernelEvent, LLMProvider, Persona, ToolDefinition, ToolHandler, ToolResult } from '@openaide/core';

/** 插件工具规范：handler 与内核同进程执行 */
export interface PluginTool {
  /** 相对名，注册时自动加 <插件名>__ 前缀避免冲突 */
  name: string;
  description: string;
  /** JSON schema（OpenAI function parameters） */
  parameters: Record<string, unknown>;
  /** 执行器：入参为解析后的参数对象 */
  handler: (args: Record<string, unknown>, sessionId: string, signal?: AbortSignal) => Promise<ToolResult> | ToolResult;
  /**
   * 危险标记:声明本工具有副作用/不可逆(如删数据、外发请求)。
   * 内核在执行前会发布 tool.permission 提示事件,装配层可据此接审批;
   * 未标记的工具默认可直接执行。
   */
  dangerous?: boolean;
}

/** 长任务进度上报(插件 → 内核事件总线) */
export interface ProgressReporter {
  /** 上报一条进度;note 为人类可读的进展描述 */
  report(note: string, percent?: number): void;
}

/** 受限 LLM 访问 —— 插件可调 LLM 但不触碰网关配置/路由 */
export interface PluginLLM {
  /** 发送一次补全请求 */
  chat(messages: Array<{ role: string; content: string }>, options?: Record<string, unknown>): Promise<string>;
  /** 当前模型 ID(只读) */
  model(): string;
}

/** 只读会话访问 —— 插件可查询历史但不可修改 */
export interface PluginSessions {
  get(sessionId: string): Promise<{ id: string; messages: Array<{ role: string; content: string }> } | undefined>;
  list(projectId?: string): Promise<Array<{ id: string; projectId: string; messageCount: number; updatedAt: number }>>;
}

/** 插件运行时上下文（激活时注入） */
export interface PluginContext {
  /** 插件自身目录（可用于解析资源文件） */
  dir: string;
  /** 可访问的应用数据目录 */
  dataDir: string;
  /** 受限 LLM 访问(摘要/自检类插件用) */
  llm?: PluginLLM;
  /** 只读会话访问 */
  sessions?: PluginSessions;
  /** 长任务进度上报 → 发布 context.progress 事件 */
  reportProgress?: ProgressReporter['report'];
  /** 宿主日志通道（可选；缺省实现打 console） */
  log?(msg: string): void;
  /**
   * 动态注册工具（activate 阶段调用）——用于运行时才知道工具清单的场景
   * （如 MCP 桥按服务器枚举）。回收由 manager 的 Scope 账本统一处理。
   */
  registerTool?(def: ToolDefinition, handler: ToolHandler): void;
}

/** 插件 manifest（可选 openaide.yaml） */
export interface PluginManifest {
  name: string;
  version?: string;
  description?: string;
  author?: string;
  /** 插件分类（轻量元数据，仅供展示/检索；缺省 'uncategorized'） */
  category?: string;
  /** 该插件提供的人格名（与 persona 字段互斥使用场景） */
  persona?: string;
  /** 任务变身的工具白名单:激活该 persona 时内核只暴露名单内的工具(匹配 <插件>__<工具> 或工具名) */
  toolAllowlist?: string[];
  /** manifest 声明的工具入口（实现仍为 TS 模块） */
  tools?: Array<{
    name: string;
    description: string;
    /** 工具实现模块路径（相对插件目录），导出该工具 handler */
    module?: string;
    schema?: Record<string, unknown>;
  }>;
}

/** 插件事件钩子 */
export interface PluginHook {
  /** 监听的事件类型（与内核 EventTypes 对齐） */
  event: string;
  handler: (event: KernelEvent) => void | Promise<void>;
}

/** LLM Provider 工厂 —— 插件可注册新的模型后端（内核按 config.llm.provider 选择） */
export interface PluginProvider {
  /** Provider 名（config.llm.provider 引用此名） */
  name: string;
  /** 由 llm 配置段创建 provider 实例 */
  create(config: {
    apiKey: string;
    model: string;
    baseUrl: string;
    timeoutMs?: number;
    /** 透传其余配置（各 provider 自行解释） */
    [key: string]: unknown;
  }): LLMProvider;
}

/**
 * UI 运行时句柄 —— 装配层传给界面插件的宿主对象（cli 的 App 满足此结构）。
 * 只约束界面的最小必需面，其余字段经结构化访问（宽松扩展位）。
 */
export interface UiRuntime {
  kernel: import('@openaide/core').AgentKernel;
  registry: import('@openaide/core').ToolExecutor;
  bus: import('@openaide/core').EventBus;
  /** 注册审批 UI；未注册时内核 fail-closed 默认拒绝危险工具 */
  setApprovalHandler?(h: (req: unknown) => Promise<boolean>): void;
  /** -c 续聊时由装配入口注入的初始会话 ID */
  initialSessionId?: string;
  [extra: string]: unknown;
}

/** 可插拔界面：TUI/REPL/任何交互形态。start resolve = 用户退出 */
export interface PluginUi {
  /** 界面名（config.ui / OPENAIDE_UI 引用此名，如 'ink'、'readline'） */
  name: string;
  description?: string;
  /** 启动并阻塞至用户退出；host 为装配完成的运行时句柄 */
  start(host: UiRuntime): Promise<void>;
}

/** 统一插件接口 —— 一切能力皆插件 */
export interface OpenAIDePlugin {
  name: string;
  version?: string;
  description?: string;
  /** 插件分类（轻量元数据，供展示/检索；也可由 openaide.yaml 的 category 声明，缺省 'uncategorized'） */
  category?: string;
  /** 激活：注册工具/钩子/人格（可选） */
  activate?(ctx: PluginContext): void | Promise<void>;
  /** 卸载钩子（可选） */
  deactivate?(): void | Promise<void>;
  /** 工具集（也可在 activate 里注册到 ctx） */
  tools?: PluginTool[];
  /** 事件钩子（只读旁听） */
  hooks?: PluginHook[];
  /** 拦截器（可否决/改写工具调用与 LLM 请求 —— 审批/脱敏/限流等策略） */
  interceptors?: Interceptor[];
  /** LLM Provider 注册（内核按 config.llm.provider 选择其一） */
  providers?: PluginProvider[];
  /** 可插拔界面（TUI/REPL/…）；config.ui 或 OPENAIDE_UI 按名选择 */
  uis?: PluginUi[];
  /** 人格：可静态提供，或异步返回 */
  persona?: Persona | (() => Persona | undefined | Promise<Persona | undefined>);
  /** 多人格包（人格包插件用；与 persona 字段并存时合并收集） */
  personas?: Persona[];
}

/** 插件加载结果 */
export interface LoadedPlugin {
  plugin: OpenAIDePlugin;
  /** 插件所在目录 */
  dir: string;
  /** 加载时间戳（热重载缓存破坏用） */
  loadedAt: number;
}

/** 插件运行状态：active=已激活；disabled=被禁用未加载；failed=加载失败 */
export type PluginStatus = 'active' | 'disabled' | 'failed';

/** 插件信息快照（含分类与状态，供展示/检索/管理） */
export interface PluginInfo {
  name: string;
  version?: string;
  description?: string;
  /** 分类：代码声明 > manifest > 'uncategorized' */
  category: string;
  /** 运行状态 */
  status: PluginStatus;
  /** 已注册工具全名（<插件名>__<工具名>）；仅 active 有值 */
  tools: string[];
  /** 已挂载钩子数；仅 active 有值 */
  hooks: number;
  /** 是否提供人格 */
  persona: boolean;
  /** 加载失败原因；仅 failed 有值 */
  error?: string;
}
