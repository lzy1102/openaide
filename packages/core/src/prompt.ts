/**
 * 提示词组装 —— 分层 L0..L5，persona 可插拔。
 * 对齐 Go 版 core/kernel_prompt.go 的分层思想：
 *  - L0 安全/角色（来源：激活 persona，或内置默认）
 *  - L1 项目上下文
 *  - L2 系统注入（自定义 system_prompt）
 *  - L3 Task Adapter（按任务类型）
 *  - L4 跨会话知识（ProjectKnowledge）
 *  - L5 上轮反思
 */
import { Message, Query, StreamChunk, StreamChunkType } from './types.js';
import type { PersonaProvider, Persona } from './interfaces.js';

/** 内置默认 L0 —— 无激活 persona 时的回退，保证行为一致 */
export const DEFAULT_L0 = `You are OpenAIDE, an autonomous AI coding agent inside a terminal.
You are a coding and automation expert. You help the user solve problems end-to-end.
Follow these operating principles:
1. Act autonomously — inspect files, run commands, read code before answering.
2. Prefer concrete actions over explanations.
3. When a task is ambiguous, state your understanding and ask before acting.
4. Keep responses concise and grounded in what you actually observed.
5. Never fabricate file contents, command output, or API responses.`;

/** 内置角色 L0（与 persona 插件同构，体现“一切皆插件”） */
export const BUILTIN_PERSONAS: Record<string, Persona> = {
  coder: {
    name: 'coder',
    description: '通用编码与自动化助手',
    systemPrompt: DEFAULT_L0,
  },
  architect: {
    name: 'architect',
    description: '架构设计与代码评审',
    systemPrompt:
      'You are an expert software architect. Analyze structure, propose designs, ' +
      'and review code for maintainability, extensibility, and correctness. ' +
      'Prefer precise, structured analysis with explicit trade-offs.',
  },
};

/** 提示词组装上下文 */
export interface PromptContext {
  persona?: Persona;
  customSystemPrompt?: string;
  projectContext?: string;
  taskType?: string;
  reflection?: string;
  skillMessage?: Message;
}

/** 组装系统层（L0 + L1 + L2） */
export function buildSystemLayer(ctx: PromptContext): string {
  const parts: string[] = [];

  // L0：激活 persona 优先，否则内置默认
  const l0 = ctx.persona?.systemPrompt ?? DEFAULT_L0;
  parts.push(l0);

  // L2：自定义 system_prompt 追加，绝不覆盖 L0
  if (ctx.customSystemPrompt) {
    parts.push(ctx.customSystemPrompt);
  }

  return parts.join('\n\n');
}

/** 组装完整的 messages 列表（稳定前缀 + 历史 + 动态尾部） */
export function buildMessages(
  systemLayer: string,
  history: Message[],
  query: Query,
  ctx: PromptContext,
  options: { maxHistory?: number } = {},
): Message[] {
  const messages: Message[] = [];
  if (systemLayer) messages.push({ role: 'system', content: systemLayer });

  // 历史（紧邻 system，稳定前缀）
  const limit = options.maxHistory ?? 20;
  const historySlice = history.slice(-limit);
  for (const m of historySlice) {
    messages.push({ ...m });
  }

  // 技能消息独立注入（保持 L0+L1 前缀字节级稳定，利于前缀缓存）
  if (ctx.skillMessage && ctx.skillMessage.content) {
    messages.push({ ...ctx.skillMessage });
  }

  // 用户查询
  messages.push({ role: 'user', content: query.content });

  // 意图/任务类型适配（L3）
  if (ctx.taskType) {
    messages.push({ role: 'system', content: taskAdapterPrompt(ctx.taskType) });
  }

  // L5 反思（上一轮）
  if (ctx.reflection) {
    messages.push({ role: 'system', content: reflectionPrompt(ctx.reflection) });
  }

  // L4 项目知识
  if (ctx.projectContext) {
    messages.push({ role: 'system', content: `[ProjectKnowledge]\n${ctx.projectContext}` });
  }

  return messages;
}

/** 按任务类型生成适配提示（L3） */
export function taskAdapterPrompt(taskType: string): string {
  const adapters: Record<string, string> = {
    coding:
      '[Task: coding]\nInspect the relevant code first (read_file / search_files), then make minimal, focused edits. Verify with tests/build when appropriate.',
    review:
      '[Task: review]\nReview for correctness, security, and maintainability. Cite concrete file/line evidence. Prioritize findings by severity.',
    debugging:
      '[Task: debugging]\nReproduce the issue, inspect logs and stack traces, identify root cause, then propose or apply the fix. Verify the fix.',
    think:
      '[Task: think]\nAnalyze the question deeply. Consider multiple perspectives and trade-offs before concluding.',
  };
  return adapters[taskType] ?? '[Task: general]\nHandle the request directly and concretely.';
}

/** 反思提示（L5） */
export function reflectionPrompt(reflection: string): string {
  return `[Previous-round reflection]\n${reflection}\n\nIncorporate the lessons above into your next step.`;
}

/** 空工具参数兜底 */
export function ensureSchema(parameters?: Record<string, unknown>): Record<string, unknown> {
  if (parameters && parameters.type) return parameters;
  return { type: 'object', properties: {} };
}

/** 工具结果截断：单条超过上限时头尾保留 */
export const MAX_TOOL_RESULT_CHARS = 20_000;
export function truncateToolResult(content: string): string {
  if (content.length <= MAX_TOOL_RESULT_CHARS) return content;
  const headLen = Math.floor((MAX_TOOL_RESULT_CHARS * 2) / 5);
  const tailLen = Math.floor((MAX_TOOL_RESULT_CHARS * 2) / 5);
  const head = content.slice(0, headLen);
  const tail = content.slice(content.length - tailLen);
  const snipped = content.length - head.length - tail.length;
  return `${head}\n\n... [${snipped} chars truncated] ...\n\n${tail}`;
}

/** 由任意可迭代产生类型化 chunk 的工具函数 */
export function chunk(type: StreamChunkType, partial: Partial<StreamChunk>): StreamChunk {
  return { type, ...partial } as StreamChunk;
}
