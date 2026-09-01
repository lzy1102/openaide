/**
 * 上下文压缩 —— 长会话防止上下文膨胀。
 *
 * 策略(对齐 Go 版 LLMCompressor):
 *  - system 消息全量保留(缓存前缀稳定)
 *  - 最近 keepRecent 条消息原样保留(当前任务状态)
 *  - 更旧的消息交给 LLM 生成结构化摘要,替换为单条 system 消息
 *  - LLM 失败时回退到确定性截断(fallback),永不阻塞主流程
 *  - compressToBudget 渐进式重压缩,最多 3 次,直到进入预算
 */
import { Message } from './types.js';
import type { ContextCompressor, LLMProvider } from './interfaces.js';
import { estimateTextTokens } from './prompt.js';

/** 压缩后保留的最近消息条数（默认 12，适配 1M 上下文） */
const DEFAULT_KEEP_RECENT = 12;
/** 单条消息进摘要前的截断长度(字符) */
const DEFAULT_MAX_MSG_CHARS_FOR_SUMMARY = 1200;
/** 摘要的最大生成 tokens */
const DEFAULT_SUMMARY_MAX_TOKENS = 600;

export interface CompressorOptions {
  keepRecent?: number;
  maxCharsForSummary?: number;
  summaryMaxTokens?: number;
}

const SUMMARIZE_PROMPT = `Compress the conversation below into a structured summary. The summary replaces the full history in the context window, so preserve everything needed to continue the task.

Priority order:
1. User requests & intents — all explicit requests
2. Decisions & agreements
3. Technical facts — file paths, errors, outputs, code patterns
4. Current task state — what is done, what remains
5. Tool results that changed state

Discard: greetings, boilerplate output, redundant confirmations, already-corrected failed attempts.

Output format (plain text, under 200 words, same language as the user):
[User Intent] ...
[Key Facts] ...
[Current State] ...
[Notes] ...

## Conversation
%s

## Summary`;

/** 估算一组消息的总 token 数(与 prompt.ts 的估算口径一致) */
export function estimateMessagesTokens(messages: Message[]): number {
  let total = 0;
  for (const m of messages) {
    total += estimateTextTokens(m.content ?? '') + 4;
    if (m.toolCalls && m.toolCalls.length > 0) total += 20;
    if (m.reasoningContent) total += estimateTextTokens(m.reasoningContent);
  }
  return total;
}

/**
 * 基于 LLM 的语义压缩器。
 * provider 为必选——摘要质量直接决定压缩后任务的可持续性。
 */
export class LLMCompressor implements ContextCompressor {
  private provider: LLMProvider;
  private keepRecent: number;
  private maxCharsForSummary: number;
  private summaryMaxTokens: number;

  constructor(provider: LLMProvider, opts: CompressorOptions = {}) {
    this.provider = provider;
    this.keepRecent = opts.keepRecent ?? DEFAULT_KEEP_RECENT;
    this.maxCharsForSummary = opts.maxCharsForSummary ?? DEFAULT_MAX_MSG_CHARS_FOR_SUMMARY;
    this.summaryMaxTokens = opts.summaryMaxTokens ?? DEFAULT_SUMMARY_MAX_TOKENS;
  }

  estimateTokens(messages: Message[]): number {
    return estimateMessagesTokens(messages);
  }

  async compress(
    messages: Message[],
    maxTokens: number,
  ): Promise<{ messages: Message[]; saved: number }> {
    const before = estimateMessagesTokens(messages);
    if (messages.length <= this.keepRecent + 1 || before <= maxTokens) {
      return { messages, saved: 0 };
    }

    // 分离 system 与对话历史
    const systemMsgs = messages.filter((m) => m.role === 'system');
    const conversation = messages.filter((m) => m.role !== 'system');

    if (conversation.length <= this.keepRecent) {
      return { messages, saved: 0 };
    }

    const oldMsgs = conversation.slice(0, conversation.length - this.keepRecent);
    const recentMsgs = conversation.slice(conversation.length - this.keepRecent);

    // LLM 生成摘要;失败则回退到确定性截断
    let summary = '';
    try {
      summary = await this.summarize(oldMsgs);
    } catch {
      summary = '';
    }
    if (!summary) {
      summary = truncateFallback(oldMsgs);
    }

    const compressed: Message[] = [
      ...systemMsgs,
      { role: 'system', content: `[Conversation Summary]\n${summary}` },
      ...recentMsgs,
    ];

    const after = estimateMessagesTokens(compressed);
    return { messages: compressed, saved: Math.max(0, before - after) };
  }

  /** 调用 LLM 生成结构化摘要;空摘要视为失败交由回退处理 */
  private async summarize(oldMsgs: Message[]): Promise<string> {
    const parts = oldMsgs.map((m) => {
      let content = m.content ?? '';
      if (content.length > this.maxCharsForSummary) {
        content = content.slice(0, this.maxCharsForSummary) + '...';
      }
      return `${m.role}: ${content}`;
    });

    const resp = await this.provider.chat(
      [{ role: 'user', content: SUMMARIZE_PROMPT.replace('%s', parts.join('\n')) }],
      [],
      { max_tokens: this.summaryMaxTokens, temperature: 0.2 },
    );
    return (resp.content ?? '').trim();
  }
}

/** 确定性回退:保留每条头尾,中间省略(LLM 不可用时仍能进入预算) */
function truncateFallback(oldMsgs: Message[]): string {
  const lines = oldMsgs.map((m) => {
    let c = m.content ?? '';
    if (c.length > 200) c = c.slice(0, 120) + '...' + c.slice(c.length - 60);
    return `${m.role}: ${c}`;
  });
  return `[Truncated history]\n${lines.join('\n')}`;
}

/**
 * 渐进式压缩到预算内:最多 3 次重压缩(超长单条/海量工具输出可能一次压不完)。
 * 压缩成功后由调用方重注入任务上下文,防止摘要丢失目标导致跑偏。
 */
export async function compressToBudget(
  compressor: ContextCompressor,
  messages: Message[],
  maxTokens: number,
): Promise<{ messages: Message[]; compressed: boolean }> {
  const total = estimateMessagesTokens(messages);
  if (total <= maxTokens) {
    return { messages, compressed: false };
  }

  let current = messages;
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const { messages: next, saved } = await compressor.compress(current, maxTokens);
      if (saved <= 0) break;
      current = next;
      if (estimateMessagesTokens(current) <= maxTokens * 0.9) break;
    } catch {
      break;
    }
  }
  return { messages: current, compressed: current !== messages };
}
