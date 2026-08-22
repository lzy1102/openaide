/**
 * 内核核心类型 —— 与 LLM 层解耦的通用格式。
 * 对齐 Go 版 core/types.go 的语义。
 */

/** 消息角色 */
export type MessageRole = 'system' | 'user' | 'assistant' | 'tool';

/** 内核消息 */
export interface Message {
  role: MessageRole;
  content: string;
  reasoningContent?: string;
  name?: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
}

/** 工具调用 */
export interface ToolCall {
  id: string;
  type: string; // "function"
  function: FunctionCall;
}

/** 函数调用 */
export interface FunctionCall {
  name: string;
  arguments: string; // JSON string
}

/** 工具执行结果 */
export interface ToolResult {
  content: unknown;
  error?: string;
  errorCode?: string; // NOT_FOUND, PERMISSION_DENIED, TIMEOUT, INVALID_ARGS, EXEC_FAILED
  isRetryable?: boolean;
}

/** 工具定义 */
export interface ToolDefinition {
  type: string; // "function"
  function: FunctionDef;
}

/** 函数定义 */
export interface FunctionDef {
  name: string;
  description: string;
  parameters: Record<string, unknown>; // JSON schema
  strict?: boolean;
}

/** LLM 响应（内核通用格式） */
export interface LLMResponse {
  id: string;
  content: string;
  reasoningContent?: string;
  toolCalls?: ToolCall[];
  usage?: TokenUsage;
  model: string;
}

/** Token 使用统计 */
export interface TokenUsage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  promptCacheHitTokens?: number;
  promptCacheMissTokens?: number;
}

/** 流式块类型 */
export const enum StreamChunkType {
  Content = 'content',
  Thinking = 'thinking',
  ToolCall = 'tool_call',
  ToolDone = 'tool_done',
  Progress = 'progress',
  Done = 'done',
  Error = 'error',
}

/** 流式响应块 */
export interface StreamChunk {
  type: StreamChunkType;
  content?: string;
  reasoningContent?: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
  toolName?: string;
  toolArgs?: string;
  toolResult?: ToolResult;
  round?: number;
  totalRounds?: number;
  done?: boolean;
  usage?: TokenUsage;
  error?: Error;
}

/** 对话会话 */
export interface Session {
  id: string;
  projectId: string;
  userId: string;
  messages: Message[];
  metadata: Record<string, unknown>;
  createdAt: number;
  updatedAt: number;
}

/** 查询选项 */
export interface QueryOptions {
  temperature?: number;
  maxTokens?: number;
  responseFormat?: unknown;
  projectContext?: string;
  route?: string;
}

/** 用户查询 */
export interface Query {
  sessionId: string;
  projectId: string;
  userId: string;
  content: string;
  options: QueryOptions;
}

/** 内核响应 */
export interface Response {
  content: string;
  reasoningContent?: string;
  sessionId: string;
  round: number;
  totalRounds: number;
  usage?: TokenUsage;
}

/** 内核状态 */
export type KernelState = 'idle' | 'processing' | 'thinking' | 'executing' | 'done' | 'error';

export const StateIdle: KernelState = 'idle';
export const StateProcessing: KernelState = 'processing';
export const StateThinking: KernelState = 'thinking';
export const StateExecuting: KernelState = 'executing';
export const StateDone: KernelState = 'done';
export const StateError: KernelState = 'error';

/** 事件（事件总线载荷） —— 命名为 KernelEvent 避免与 DOM 全局 Event 冲突 */
export interface KernelEvent {
  type: string;
  source: string;
  data?: Record<string, unknown>;
  timestamp: number;
}

/** 事件类型常量 */
export const EventTypes = {
  SessionCreated: 'session.created',
  MessageAdded: 'message.added',
  RoundStart: 'round.start',
  RoundEnd: 'round.end',
  ToolCallStarted: 'tool.call.started',
  ToolCallEnded: 'tool.call.ended',
  ToolExecuted: 'tool.executed',
  QueryStart: 'query.start',
  QueryEnd: 'query.end',
  StateChanged: 'state.changed',
  ContextCompressed: 'context.compressed',
  ToolPermission: 'tool.permission',
  ContextProgress: 'context.progress',
  Error: 'error',
} as const;
