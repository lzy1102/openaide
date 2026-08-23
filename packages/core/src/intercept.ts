/**
 * 拦截链运行器 —— 按序执行拦截器，链式传递可修改的载荷。
 * 任一 deny 短路返回；modify 更新当前载荷继续向后传递。
 */
import type { Interceptor, ToolCallInfo, Verdict } from './interfaces.js';
import type { Message, ToolResult } from './types.js';

export type Gate<T> = { ok: true; payload: T } | { ok: false; reason: string };

/** LLM 请求前拦截：返回最终消息数组或否决原因 */
export async function runBeforeLLM(
  interceptors: Interceptor[],
  info: { sessionId: string; messages: Message[] },
): Promise<Gate<Message[]>> {
  let messages = info.messages;
  for (const ix of interceptors) {
    if (!ix.beforeLLM) continue;
    const v: Verdict<Message[]> = await ix.beforeLLM({ sessionId: info.sessionId, messages });
    if (v.action === 'deny') return { ok: false, reason: v.reason ?? `denied by ${ix.name}` };
    if (v.action === 'modify' && Array.isArray(v.payload)) messages = v.payload;
  }
  return { ok: true, payload: messages };
}

/** 工具执行前拦截：返回最终参数 JSON 或否决原因 */
export async function runBeforeToolCall(
  interceptors: Interceptor[],
  info: ToolCallInfo,
): Promise<Gate<string>> {
  let argsJson = info.argsJson;
  for (const ix of interceptors) {
    if (!ix.beforeToolCall) continue;
    const v: Verdict<string> = await ix.beforeToolCall({ ...info, argsJson });
    if (v.action === 'deny') return { ok: false, reason: v.reason ?? `denied by ${ix.name}` };
    if (v.action === 'modify' && typeof v.payload === 'string') argsJson = v.payload;
  }
  return { ok: true, payload: argsJson };
}

/** 工具执行后拦截：返回最终结果（deny → 结果替换为错误） */
export async function runAfterToolCall(
  interceptors: Interceptor[],
  info: ToolCallInfo,
  result: ToolResult,
): Promise<ToolResult> {
  let current = result;
  for (const ix of interceptors) {
    if (!ix.afterToolCall) continue;
    const v: Verdict<ToolResult> = await ix.afterToolCall({ ...info, result: current });
    if (v.action === 'deny') {
      return {
        content: '',
        error: `blocked by ${ix.name}${v.reason ? `: ${v.reason}` : ''}`,
        errorCode: 'INTERCEPTED',
      };
    }
    if (v.action === 'modify' && v.payload) current = v.payload;
  }
  return current;
}
