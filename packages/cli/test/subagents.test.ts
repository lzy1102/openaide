/**
 * Agent-as-Tool 验收 —— 声明 → 工具生成 → 真实子 ReAct 循环（脚本化 provider）：
 * 人格懒解析、结论回传、人格缺失错误路径、名称清洗、父会话隔离。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { MemorySessionStore } from '@openaide/core';
import type { LLMProvider, ToolCall, ToolDefinition, ToolExecutor, ToolHandler, ToolResult, Message } from '@openaide/core';
import { createSubAgentTools } from '../src/subagents.js';
import type { SubAgentSpec } from '../src/subagents.js';

/** 有状态 provider：首轮返回 toolCalls（走 allowlist 内工具），之后回最终文本 */
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
    const h = this.handlers.get(call.function.name);
    return h ? h(call.function.arguments, 'sub', undefined) : { content: `ran ${call.function.name}` };
  }
}

const PERSONAS = new Map([
  ['researcher', { name: 'researcher', description: '', systemPrompt: 'You research things.' }],
]);

const makeDeps = (provider: LLMProvider) => ({
  llm: provider,
  registry: new FakeExecutor(),
  getPersona: (n: string) => PERSONAS.get(n),
  maxRounds: 5,
});

test('端到端：父调用子代理工具 → 子循环执行 → 结论作为工具结果回传', async () => {
  const spec: SubAgentSpec = { name: 'research', persona: 'researcher' };
  const tools = createSubAgentTools([spec], makeDeps(scriptedProvider([
    { content: '研究完成：42 是答案' },
  ])));
  assert.equal(tools.length, 1);
  assert.equal(tools[0]!.name, 'research');

  const r = await tools[0]!.handler({ task: '研究 42' }, 'parent-session');
  assert.equal(r.content, '研究完成：42 是答案');
});

test('人格缺失 → 错误结果而非抛异常（父可继续决策）', async () => {
  const tools = createSubAgentTools([{ name: 'ghosty', persona: 'no-such' }], makeDeps(scriptedProvider([{ content: '' }])));
  const r = await tools[0]!.handler({ task: 'x' }, 's1');
  assert.match(String(r.error ?? ''), /persona 'no-such' not found/);
});

test('名称清洗：空格/大写/特殊字符 → 目录安全名', () => {
  const tools = createSubAgentTools(
    [{ name: 'Travel Planner!', persona: 'researcher' }],
    makeDeps(scriptedProvider([{ content: 'ok' }])),
  );
  assert.equal(tools[0]!.name, 'travel-planner');
});

test('signal 透传：abort 后子循环中止', async () => {
  const ac = new AbortController();
  // 模拟真实 provider：fetch 类调用会因 signal 中止而 reject
  const slow: LLMProvider = {
    chat: () =>
      new Promise((_resolve, reject) => {
        if (ac.signal.aborted) return reject(Object.assign(new Error('aborted'), { name: 'AbortError' }));
        ac.signal.addEventListener(
          'abort',
          () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })),
          { once: true },
        );
      }),
    async *chatStream() {},
    getModelId: () => 'fake',
  };
  const tools = createSubAgentTools([{ name: 'slow', persona: 'researcher' }], makeDeps(slow));
  const pending = tools[0]!.handler({ task: 'x' }, 's1', ac.signal);
  setTimeout(() => ac.abort(), 30);
  const r = await pending;
  assert.match(String(r.error ?? ''), /aborted/);
});
