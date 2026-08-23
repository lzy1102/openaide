/**
 * 审批拦截器 —— 把「危险操作需人工确认」策略挂进内核拦截链。
 * 模式：
 *   dangerous — 仅描述带 [dangerous 标记的工具需确认
 *   always    — 所有工具调用都需确认
 * UI 无关：由装配方注入 ask 回调（TUI 卡片 / REPL 行内输入 / API 默认拒绝）。
 */
import type { Interceptor } from '@openaide/core';

export interface ApprovalRequest {
  tool: string;
  args: string;
  sessionId: string;
}

export type ApprovalHandler = (req: ApprovalRequest) => Promise<boolean>;

export function createApprovalInterceptor(
  mode: 'dangerous' | 'always',
  opts: { ask: ApprovalHandler; isDangerous: (tool: string) => boolean },
): Interceptor {
  return {
    name: 'approval',
    async beforeToolCall(info) {
      if (mode === 'dangerous' && !opts.isDangerous(info.tool)) return { action: 'allow' };
      let ok = false;
      try {
        ok = await opts.ask({ tool: info.tool, args: info.argsJson, sessionId: info.sessionId });
      } catch {
        ok = false; // UI 异常按拒绝处理
      }
      return ok ? { action: 'allow' } : { action: 'deny', reason: 'denied by user' };
    },
  };
}
