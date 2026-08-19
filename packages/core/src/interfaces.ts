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
