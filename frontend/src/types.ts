/** 与 packages/api 对齐的类型（后端 DTO 的镜像，避免跨包依赖） */

export interface ToolCallDto {
  id: string;
  type: string;
  function: { name: string; arguments: string };
}

export interface MessageDto {
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  reasoningContent?: string;
  toolCalls?: ToolCallDto[];
  toolCallId?: string;
}

export interface SessionDto {
  id: string;
  projectId: string;
  userId: string;
  messages: MessageDto[];
  createdAt: number;
  updatedAt: number;
}

export interface HealthInfo {
  ok: boolean;
  name: string;
  version: string;
  model: string;
  state: string;
  uptime: number;
}

/** SSE chunk（core StreamChunk 的运行时子集） */
export type StreamChunkDto =
  | { type: 'content'; content?: string }
  | { type: 'thinking'; reasoningContent?: string }
  | { type: 'tool_call'; toolName?: string; toolArgs?: string; toolCalls?: ToolCallDto[] }
  | { type: 'tool_done'; toolName?: string; toolResult?: { content?: string; error?: string } }
  | { type: 'progress'; round?: number; totalRounds?: number }
  | { type: 'done'; done?: boolean; round?: number; totalRounds?: number }
  | { type: 'error'; error?: { message?: string } };
