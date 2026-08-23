/**
 * 拦截链验收测试 —— beforeLLM/beforeToolCall/afterToolCall 的 allow/deny/modify 全路径。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { ToolCall, ToolDefinition, ToolExecutor, ToolHandler, ToolResult, LLMProvider, Message, Interceptor } from '@openaide/core';
import { collectReact } from '../src/react.js';

/** 有状态 provider：首轮返回 toolCalls，其后返回最终文本（模拟模型按观察调整） */
function scriptedProvider(script: Array<{ toolCalls?: ToolCall[]; content?: string }>): { provider: LLMProvider; chats: () => number } {
  let n = 0;
  const provider: LLMProvider = {
    async chat() {
      const step = script[Math.min(n, script.length - 1)]!;
      n++;
      return { content: step.content ?? '', toolCalls: step.toolCalls } as never;
    },
    async *chatStream() {
      throw new Error('not used');
    },
    getModelId: () => 'fake',
  };
  return { provider, chats: () => n };
}

class FakeExecutor implements ToolExecutor {
  calls: string[] = [];
  private handlers = new Map<string, ToolHandler>();
  register(def: ToolDefinition, handler: ToolHandler): void {
    this.handlers.set(def.function.name, handler);
  }
  unregister(): void {}
  definitions(): ToolDefinition[] {
    return [...this.handlers.keys()].map((name) => ({
      type: 'function' as const,
      function: { name, description: '' },
    }));
  }
  async execute(call: ToolCall): Promise<ToolResult> {
    this.calls.push(`${call.function.name}:${call.function.arguments}`);
    return { content: `ran ${call.function.name}` };
  }
}

const baseMessages: Message[] = [{ role: 'user', content: 'hi' }];
const echoTool: ToolCall = {
  id: 't1',
  type: 'function',
  function: { name: 'echo', arguments: '{"x":1}' },
};

test('beforeToolCall：deny 阻止执行并把原因回给模型', async () => {
  const executor = new FakeExecutor();
  const ix: Interceptor = { name: 'guard', beforeToolCall: () => ({ action: 'deny', reason: 'nope' }) };
  const { provider, chats } = scriptedProvider([
    { toolCalls: [echoTool] },
    { content: 'adjusted after denial' },
  ]);
  const r = await collectReact(
    {
      provider,
      executor,
      messages: baseMessages,
      sessionId: 's1',
      publish: () => {},
      interceptors: [ix],
    },
    { maxRounds: 5 },
  );
  assert.equal(executor.calls.length, 0, '工具不应被执行');
  assert.equal(chats(), 2, '模型应在收到拒绝观察后重新决策');
  assert.match(r.content, /adjusted/);
});

test('beforeToolCall：modify 改写参数后执行', async () => {
  const executor = new FakeExecutor();
  const ix: Interceptor = {
    name: 'rewriter',
    beforeToolCall: () => ({ action: 'modify', payload: '{"x":42}' }),
  };
  await collectReact(
    {
      provider: scriptedProvider([{ toolCalls: [echoTool] }, { content: 'done' }]).provider,
      executor,
      messages: baseMessages,
      sessionId: 's1',
      publish: () => {},
      interceptors: [ix],
    },
    { maxRounds: 5 },
  );
  assert.deepEqual(executor.calls, ['echo:{"x":42}'], '应以改写后的参数执行');
});

test('afterToolCall：deny 把结果替换为错误；modify 可替换结果', async () => {
  const seenOriginal: (string | undefined)[] = [];
  const ix: Interceptor = {
    name: 'filter',
    afterToolCall: (info) => {
      seenOriginal.push(info.result.content);
      if (info.result.content?.includes('secret')) return { action: 'deny', reason: 'leak' };
      return { action: 'modify', payload: { ...info.result, content: '[wrapped]' } };
    },
  };
  // 执行器把参数回显进结果，便于区分秘密/普通两条路径
  const echoExecutor = (calls: string[]): ToolExecutor => ({
    calls,
    register() {},
    unregister() {},
    definitions() {
      return [];
    },
    async execute(call: ToolCall) {
      calls.push(call.function.arguments);
      return { content: `ran with ${call.function.arguments}` };
    },
  }) as unknown as ToolExecutor;

  // 秘密参数 → deny；普通参数 → modify
  const secretCall: ToolCall = { id: 't1', type: 'function', function: { name: 'echo', arguments: '{"a":"my-secret"}' } };
  const plainCall: ToolCall = { id: 't2', type: 'function', function: { name: 'echo', arguments: '{"a":"plain"}' } };

  const denyCalls: string[] = [];
  const rDeny = await collectReact(
    {
      provider: scriptedProvider([{ toolCalls: [secretCall] }, { content: 'adjusted' }]).provider,
      executor: echoExecutor(denyCalls),
      messages: baseMessages,
      sessionId: 's1',
      publish: () => {},
      interceptors: [ix],
    },
    { maxRounds: 5 },
  );
  assert.ok(seenOriginal[0]?.includes('my-secret'), '拦截器应看到原始结果');
  assert.equal(denyCalls.length, 1);
  assert.match(rDeny.content, /adjusted/, 'deny 后模型应收到错误观察并继续');

  seenOriginal.length = 0;
  const modifyCalls: string[] = [];
  const chunks: string[] = [];
  for await (const c of await import('../src/react.js').then((m) =>
    m.reactLoop(
      {
        provider: scriptedProvider([{ toolCalls: [plainCall] }, { content: 'ok' }]).provider,
        executor: echoExecutor(modifyCalls),
        messages: baseMessages,
        sessionId: 's1',
        publish: () => {},
        interceptors: [ix],
      },
      { maxRounds: 5 },
    ),
  )) {
    if (c.type === 'tool_done') chunks.push(c.toolResult?.content ?? '');
  }
  assert.ok(chunks.includes('[wrapped]'), 'modify 应替换流式工具结果');
});

test('beforeLLM：deny 终止查询并产出 Error chunk', async () => {
  const denyIx: Interceptor = { name: 'budget', beforeLLM: () => ({ action: 'deny', reason: 'over budget' }) };
  const errors: string[] = [];
  const { provider } = scriptedProvider([{ content: 'never reached' }]);
  const { reactLoop } = await import('../src/react.js');
  try {
    for await (const chunk of reactLoop(
      {
        provider,
        executor: new FakeExecutor(),
        messages: baseMessages,
        sessionId: 's1',
        publish: () => {},
        interceptors: [denyIx],
      },
      { maxRounds: 3 },
    )) {
      if (chunk.type === 'error' && chunk.error) errors.push(String(chunk.error.message));
    }
  } catch {
    /* deny 抛错终止属预期 */
  }
  assert.ok(errors.some((e) => e.includes('over budget')), `应产出策略否决错误，实际: ${errors}`);
});

test('beforeLLM：modify 对 provider 生效（PII 脱敏）', async () => {
  let received: Message[] = [];
  const probe: LLMProvider = {
    async chat(messages: Message[]) {
      received = messages;
      return { content: 'ok' } as never;
    },
    async *chatStream() {
      throw new Error('not used');
    },
    getModelId: () => 'fake',
  };
  const modIx: Interceptor = {
    name: 'redact',
    beforeLLM: (info) => ({
      action: 'modify',
      payload: info.messages.map((m) =>
        m.role === 'user' && m.content.includes('sk-secret') ? { ...m, content: m.content.replace('sk-secret', '[REDACTED]') } : m,
      ),
    }),
  };
  await collectReact(
    {
      provider: probe,
      executor: new FakeExecutor(),
      messages: [{ role: 'user', content: 'key is sk-secret' }],
      sessionId: 's1',
      publish: () => {},
      interceptors: [modIx],
    },
    { maxRounds: 3 },
  );
  assert.equal(received[received.length - 1]?.content, 'key is [REDACTED]');
});
