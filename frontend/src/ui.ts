/** UI 层消息模型（由后端 Message/StreamChunk 投影而来） */
import type { MessageDto, SessionDto } from './types';

export interface UiUser {
  kind: 'user';
  id: string;
  content: string;
}
export interface UiAssistant {
  kind: 'assistant';
  id: string;
  content: string;
  reasoning?: string;
}
export interface UiTool {
  kind: 'tool';
  id: string;
  name: string;
  args?: string;
  result?: string;
  error?: string;
  status: 'running' | 'done' | 'failed';
}
export type UiMsg = UiUser | UiAssistant | UiTool;

let seq = 0;
export const nextUiId = (): string => `u${++seq}`;

/** 追加/续写尾部助手消息（流式 content/thinking 用） */
export function appendTailAssistant(
  msgs: UiMsg[],
  patch: (m: UiAssistant) => UiAssistant,
): UiMsg[] {
  const last = msgs[msgs.length - 1];
  if (last && last.kind === 'assistant') {
    const next = [...msgs];
    next[next.length - 1] = patch(last);
    return next;
  }
  const fresh: UiAssistant = { kind: 'assistant', id: nextUiId(), content: '' };
  return [...msgs, patch(fresh)];
}

/** 结算最后一条匹配名的 running 工具卡 */
export function settleRunningTool(
  msgs: UiMsg[],
  name: string | undefined,
  result: string | undefined,
  error: string | undefined,
): UiMsg[] {
  for (let i = msgs.length - 1; i >= 0; i--) {
    const m = msgs[i]!;
    if (m.kind === 'tool' && m.status === 'running' && (!name || m.name === name)) {
      const next = [...msgs];
      next[i] = { ...m, status: error ? 'failed' : 'done', result, error };
      return next;
    }
  }
  return msgs;
}

/** 相对时间显示 */
export function relTime(ts: number): string {
  const diff = Date.now() - ts;
  if (diff < 60_000) return 'now';
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h`;
  return `${Math.floor(diff / 86_400_000)}d`;
}

/** 历史会话 → UI 消息（还原工具卡与结果：toolCalls 建卡，role=tool 按 toolCallId 回填） */
export function mapHistory(session: SessionDto): UiMsg[] {
  const out: UiMsg[] = [];
  const toolById = new Map<string, UiTool>();
  for (const m of session.messages) {
    if (m.role === 'user') {
      out.push({ kind: 'user', id: nextUiId(), content: m.content });
    } else if (m.role === 'assistant') {
      for (const tc of m.toolCalls ?? []) {
        const card: UiTool = {
          kind: 'tool',
          id: nextUiId(),
          name: tc.function.name,
          args: tc.function.arguments,
          status: 'running',
        };
        toolById.set(tc.id, card);
        out.push(card);
      }
      out.push({
        kind: 'assistant',
        id: nextUiId(),
        content: m.content,
        reasoning: m.reasoningContent,
      });
    } else if (m.role === 'tool' && m.toolCallId && toolById.has(m.toolCallId)) {
      const card = toolById.get(m.toolCallId)!;
      card.result = m.content;
      card.status = 'done';
    }
  }
  return out;
}

/** 兼容旧导入路径 */
export type { MessageDto };
