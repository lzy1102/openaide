/**
 * 审批拦截器验收 —— dangerous/always 模式 × 允许/拒绝 全路径，
 * 并经真实 reactLoop 验证拒绝后模型收到观察并重新决策。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { collectReact } from '@openaide/core';
import type { Interceptor, LLMProvider, ToolCall, ToolExecutor, ToolDefinition, ToolHandler, ToolResult, Message } from '@openaide/core';
import { createApprovalInterceptor } from '../src/approval.js';
import type { ApprovalRequest } from '../src/approval.js';

/** 有状态 provider：首轮发工具调用，之后回文本 */
function scriptedProvider(steps: Array<{ toolCalls?: ToolCall[]; content?: string }>): LLMProvider {
  let n = 0;
  return {
    async chat() {
      const s = steps[Math.min(n, steps.length - 1)]!;
      n++;
      return { content: s.content ?? '', toolCalls: s.toolCalls } as never;
    },
    async *chatStream() {
      throw new Error('not used');
    },
    getModelId: () => 'fake',
  };
}

class FakeExecutor implements ToolExecutor {
  calls: string[] = [];
  private handlers = new Map<string, ToolHandler>();
  register(def: ToolDefinition, handler: ToolHandler): void {
    this.handlers.set(def.function.name, handler);
  }
  unregister(): void {}
  definitions(): ToolDefinition[] {
    return [...this.handlers.keys()].map((name) => ({ type: 'function' as const, function: { name, description: '' } }));
  }
  async execute(call: ToolCall): Promise<ToolResult> {
    this.calls.push(call.function.name);
    return { content: 'ran' };
  }
}

const dangerCall: ToolCall = { id: 't1', type: 'function', function: { name: 'builtin__execute_command', arguments: '{"cmd":"rm x"}' } };
const safeCall: ToolCall = { id: 't2', type: 'function', function: { name: 'builtin__read_file', arguments: '{"path":"a"}' } };

/** 与 app.ts 一致的危险工具判定（描述前缀标记） */
const isDangerous = (tool: string): boolean => tool.includes('execute_command');

test('dangerous 模式：安全工具直接放行，不触发询问', async () => {
  const asks: ApprovalRequest[] = [];
  const ix = createApprovalInterceptor('dangerous', {
    ask: async (q) => {
      asks.push(q);
      return true;
    },
    isDangerous,
  });
  const ex = new FakeExecutor();
  await collectReact(
    {
      provider: scriptedProvider([{ toolCalls: [safeCall] }, { content: 'ok' }]),
      executor: ex,
      messages: [{ role: 'user', content: 'q' }] as Message[],
      sessionId: 's1',
      publish: () => {},
      interceptors: [ix],
    },
    { maxRounds: 5 },
  );
  assert.equal(asks.length, 0, '安全工具不应询问');
  assert.deepEqual(ex.calls, ['builtin__read_file']);
});

test('dangerous 模式：危险工具拒绝 → 不执行 + 模型收到观察后改道', async () => {
  const asks: ApprovalRequest[] = [];
  const ix = createApprovalInterceptor('dangerous', {
    ask: async (q) => {
      asks.push(q);
      return false; // 用户点 n
    },
    isDangerous,
  });
  const ex = new FakeExecutor();
  const r = await collectReact(
    {
      provider: scriptedProvider([{ toolCalls: [dangerCall] }, { content: '改用安全方案' }]),
      executor: ex,
      messages: [{ role: 'user', content: 'q' }] as Message[],
      sessionId: 's1',
      publish: () => {},
      interceptors: [ix],
    },
    { maxRounds: 5 },
  );
  assert.equal(asks.length, 1);
  assert.equal(ex.calls.length, 0, '被拒绝的工具不得执行');
  assert.match(r.content, /改用安全方案/, '模型应基于拒绝观察继续完成任务');
});

test('dangerous 模式：批准 → 正常执行', async () => {
  const ix = createApprovalInterceptor('dangerous', { ask: async () => true, isDangerous });
  const ex = new FakeExecutor();
  await collectReact(
    {
      provider: scriptedProvider([{ toolCalls: [dangerCall] }, { content: 'done' }]),
      executor: ex,
      messages: [{ role: 'user', content: 'q' }] as Message[],
      sessionId: 's1',
      publish: () => {},
      interceptors: [ix],
    },
    { maxRounds: 5 },
  );
  assert.deepEqual(ex.calls, ['builtin__execute_command']);
});

test('always 模式：所有工具都询问；ask 抛异常按拒绝处理', async () => {
  let n = 0;
  const asks: string[] = [];
  const ix: Interceptor = createApprovalInterceptor('always', {
    ask: async (q) => {
      asks.push(q.tool);
      n++;
      if (n === 2) throw new Error('ui crashed'); // 模拟 UI 异常
      return true;
    },
    isDangerous,
  });
  const ex = new FakeExecutor();
  await collectReact(
    {
      provider: scriptedProvider([
        { toolCalls: [safeCall] },
        { toolCalls: [dangerCall] },
        { content: 'end' },
      ]),
      executor: ex,
      messages: [{ role: 'user', content: 'q' }] as Message[],
      sessionId: 's1',
      publish: () => {},
      interceptors: [ix],
    },
    { maxRounds: 5 },
  );
  assert.equal(asks.length, 2, 'always 模式两个工具都应询问');
  // 第二次 ask 抛异常 → 该调用被拒
  assert.deepEqual(ex.calls, ['builtin__read_file']);
});
