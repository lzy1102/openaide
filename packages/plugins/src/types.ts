/**
 * 插件接入格式 —— TS 模块动态加载（同进程，无子进程）。
 *
 * 一个插件 = 一个目录/包，导出标准接口 OpenAIDePlugin。
 * 运行时装进同一进程，可热加载/热卸载（不用多进程）。
 *
 * 可选 openaide.yaml 声明元信息（name/version/persona/入口），
 * 也可纯代码声明 —— 二选一或并存。
 */
import type { KernelEvent, Persona, ToolResult } from '@openaide/core';

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
  /** 事件钩子 */
  hooks?: PluginHook[];
  /** 人格：可静态提供，或异步返回 */
  persona?: Persona | (() => Persona | undefined | Promise<Persona | undefined>);
}

/** 插件加载结果 */
export interface LoadedPlugin {
  plugin: OpenAIDePlugin;
  /** 插件所在目录 */
  dir: string;
  /** 加载时间戳（热重载缓存破坏用） */
  loadedAt: number;
}

/** 插件信息快照（含分类，供展示/检索） */
export interface PluginInfo {
  name: string;
  version?: string;
  description?: string;
  /** 分类：代码声明 > manifest > 'uncategorized' */
  category: string;
  /** 已注册工具全名（<插件名>__<工具名>） */
  tools: string[];
  /** 已挂载钩子数 */
  hooks: number;
  /** 是否提供人格 */
  persona: boolean;
}
