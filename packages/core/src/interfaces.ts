/**
 * 内核接口 —— 一切能力通过接口注入。
 * 对齐 Go 版 core/interfaces.go 的语义。
 */
import {
  KernelEvent,
  LLMResponse,
  Message,
  Query,
  Response,
  StreamChunk,
  ToolCall,
  ToolDefinition,
  ToolResult,
  KernelState,
} from './types.js';
import type { EventHandler } from './events.js';

/** LLM 提供者接口（内核层抽象） */
export interface LLMProvider {
  /** 发送聊天请求 */
  chat(messages: Message[], tools: ToolDefinition[], options: Record<string, unknown>): Promise<LLMResponse>;
  /** 发送流式聊天请求 */
  chatStream(
    messages: Message[],
    tools: ToolDefinition[],
    options: Record<string, unknown>,
  ): AsyncIterable<StreamChunk>;
  /** 获取当前模型 ID */
  getModelId(): string;
}

/** 可选：运行时切换模型的提供者 */
export interface ModelSwitcher {
  setModelId(modelId: string): void;
}

/** 工具执行器 —— 注册表 + 分发 */
export interface ToolExecutor {
  /** 注册一个工具（name 已含命名空间前缀，如 plugin__tool） */
  register(def: ToolDefinition, handler: ToolHandler): void;
  /** 执行一个工具调用 */
  execute(toolCall: ToolCall, sessionId: string, signal?: AbortSignal): Promise<ToolResult>;
  /** 列出所有已注册工具 */
  definitions(): ToolDefinition[];
  /** 移除一个工具（动态卸载插件用） */
  unregister?(name: string): void;
}

/** 工具处理器：入参为 JSON 字符串，返回 ToolResult */
export type ToolHandler = (args: string, sessionId: string, signal?: AbortSignal) => Promise<ToolResult> | ToolResult;

/** 记忆接口（内核经此存取会话历史） */
export interface Memory {
  save(sessionId: string, messages: Message[]): Promise<void>;
  load(sessionId: string, limit: number): Promise<Message[]>;
  /**
   * 可选:撤回支持 —— 删除指定会话末尾 n 条记忆流水。
   * 与 SessionStore 的消息截断配合使用;未实现时内核跳过记忆侧裁剪。
   */
  truncateLast?(sessionId: string, n: number): Promise<void>;
}

/** 会话存储 */
export interface SessionStore {
  create(projectId: string, userId: string): Promise<Session>;
  get(sessionId: string): Promise<Session | undefined>;
  update(session: Session): Promise<void>;
  list(projectId?: string): Promise<Session[]>;
  delete(sessionId: string): Promise<void>;
}

import type { Session } from './types.js';

/** 上下文压缩器 */
export interface ContextCompressor {
  compress(messages: Message[], maxTokens: number): Promise<{ messages: Message[]; saved: number }>;
  estimateTokens(messages: Message[]): number;
}

/** 权限检查器（工具执行前鉴权） */
export interface PermissionChecker {
  check(
    resource: string,
    action: string,
    name: string,
    ctx: Record<string, unknown>,
  ): { allowed: boolean; reason: string };
}

/** 拦截判定：放行 / 否决 / 替换载荷 */
export interface Verdict<T> {
  action: 'allow' | 'deny' | 'modify';
  /** deny 时的原因（回传给模型与用户） */
  reason?: string;
  /** modify 时的替换载荷 */
  payload?: T;
}

/** 工具调用拦截信息（argsJson 为前一拦截器改写后的最新值） */
export interface ToolCallInfo {
  sessionId: string;
  tool: string;
  argsJson: string;
}

/**
 * 拦截器 —— 策略插件接缝：可否决/改写工具调用与 LLM 请求。
 * 与 PermissionChecker 的区别：PermissionChecker 只能 allow/deny；
 * Interceptor 链式执行、按序传递可修改的载荷，支持审批/脱敏/限流/审计等一切策略。
 */
export interface Interceptor {
  /** 拦截器名（事件与日志标识） */
  name: string;
  /** LLM 请求前：可否决整轮或替换消息数组（如 PII 脱敏） */
  beforeLLM?(info: { sessionId: string; messages: Message[] }): Verdict<Message[]> | Promise<Verdict<Message[]>>;
  /** 工具执行前：可否决或替换调用参数 */
  beforeToolCall?(info: ToolCallInfo): Verdict<string> | Promise<Verdict<string>>;
  /** 工具执行后：可否决（结果改为错误）或替换结果 */
  afterToolCall?(
    info: ToolCallInfo & { result: ToolResult },
  ): Verdict<ToolResult> | Promise<Verdict<ToolResult>>;
}

/** 人格/能力集提供者 —— L0 系统提示词来源 */
export interface PersonaProvider {
  /** 当前激活的人格；未设置时返回 undefined 走内置 L0 */
  active(): Promise<Persona | undefined>;
}

/** 人格定义 */
export interface Persona {
  name: string;
  description: string;
  systemPrompt: string;
  toolAllowlist?: string[];
}

/**
 * Agent 内核 —— 智能核心。
 * 收敛：消息/会话 + ReAct 循环 + 提示词组装 + 事件总线。
 */
export interface Kernel {
  process(query: Query, signal?: AbortSignal): Promise<Response>;
  processStream(query: Query, signal?: AbortSignal): AsyncIterable<StreamChunk>;
  getState(): KernelState;
  subscribe(handler: EventHandler): number;
  unsubscribe(id: number): void;
  getSlashCommands(): Map<string, string>;
}

export type { KernelEvent } from './types.js';
export type { EventHandler } from './events.js';
