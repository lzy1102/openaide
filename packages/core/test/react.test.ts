/**
 * ReAct 循环验收 —— 工具并发 / 轮数耗尽 / 事件顺序。
 *
 * 并发用「栅栏」而非计时断言：三个 handler 都启动后栅栏才放行。
 * 串行执行时第一个 handler 等不到栅栏 → 超时返回 'timeout'，
 * 测试必然失败而不是偶发失败。
 */
import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import {
  collectReact,
  reactLoop,
  StreamChunk,
  StreamChunkType,
  ToolCall,
  ToolDefinition,
  ToolResult,
} from '../src/index.js';
import type { LLMProvider, LLMResponse, Message } from '../src/index.js';

function toolCall(id: string, name: string, args: unknown = {}): ToolCall {
  return { id, type: 'function', function: { name, arguments: JSON.stringify(args) } };
}

/** 第一轮返回给定的工具调用，第二轮返回最终回答 */
class ScriptedLLM implements LLMProvider {
  calls = 0;
  constructor(
    private readonly calls1: ToolCall[],
    private readonly firstContent = '',
    private readonly alwaysTools = false,
  ) {}

  async chat(): Promise<LLMResponse> {
    this.calls++;
    if (this.alwaysTools || this.calls === 1) {
      return { id: `r${this.calls}`, model: 'mock', content: this.firstContent, toolCalls: this.calls1 };
    }
    return { id: 'rf', model: 'mock', content: 'final answer' };
  }

  async *chatStream(): AsyncGenerator<StreamChunk> {
    // 空流 → react 降级调用 chat()
  }

  getModelId(): string {
    return 'mock';
  }
}

interface Executor {
  register(def: ToolDefinition, handler: (args: string) => Promise<ToolResult> | ToolResult): void;
  definitions(): ToolDefinition[];
  execute(call: ToolCall): Promise<ToolResult>;
}

function makeExecutor(handlers: Record<string, (args: string) => Promise<ToolResult> | ToolResult>) {
  const defs = new Map<string, ToolDefinition>();
  const executor: Executor = {
    register(def) {
      defs.set(def.function.name, def);
    },
    definitions: () => [...defs.values()],
    async execute(call) {
      const h = handlers[call.function.name];
      if (!h) return { content: '', error: `unknown tool: ${call.function.name}` };
      return h(call.function.arguments);
    },
  };
  for (const name of Object.keys(handlers)) {
    executor.register({ type: 'function', function: { name, description: name, parameters: {} } });
  }
  return executor;
}

const messages: Message[] = [{ role: 'user', content: 'go' }];

async function collect(gen: AsyncGenerator<StreamChunk>): Promise<StreamChunk[]> {
  const out: StreamChunk[] = [];
  for await (const c of gen) out.push(c);
  return out;
}

describe('工具并发执行', () => {
  function barrierExecutors() {
    const started: string[] = [];
    let open!: () => void;
    const gate = new Promise<void>((resolve) => {
      open = resolve;
    });
    const handlers = {
      a__one: async () => {
        started.push('a__one');
        if (started.length === 3) open();
        const timedOut = await Promise.race([
          gate.then(() => false),
          new Promise<boolean>((r) => setTimeout(() => r(true), 400)),
        ]);
        return { content: timedOut ? 'timeout' : 'one' };
      },
      a__two: async () => {
        started.push('a__two');
        if (started.length === 3) open();
        const timedOut = await Promise.race([
          gate.then(() => false),
          new Promise<boolean>((r) => setTimeout(() => r(true), 400)),
        ]);
        return { content: timedOut ? 'timeout' : 'two' };
      },
      a__three: async () => {
        started.push('a__three');
        if (started.length === 3) open();
        const timedOut = await Promise.race([
          gate.then(() => false),
          new Promise<boolean>((r) => setTimeout(() => r(true), 400)),
        ]);
        return { content: timedOut ? 'timeout' : 'three' };
      },
    };
    return { executor: makeExecutor(handlers), started };
  }

  function ctx(executor: Executor, parallelTools?: boolean) {
    return {
      provider: new ScriptedLLM([toolCall('1', 'a__one'), toolCall('2', 'a__two'), toolCall('3', 'a__three')]),
      executor: executor as never,
      messages: [...messages],
      sessionId: 's',
      publish: () => {},
    };
  }

  test('同一轮的工具调用并行跑（串行会超时）', async () => {
    const { executor, started } = barrierExecutors();
    const chunks = await collect(reactLoop(ctx(executor), { maxRounds: 3 }));
    const results = chunks.filter((c) => c.type === StreamChunkType.ToolDone).map((c) => c.toolResult?.content);
    assert.deepEqual(started.sort(), ['a__one', 'a__three', 'a__two']);
    assert.deepEqual(results, ['one', 'two', 'three'], '并行时都不应超时');
  });

  test('ToolDone 顺序与模型给出的调用顺序一致（并行不乱序）', async () => {
    const { executor } = barrierExecutors();
    const chunks = await collect(reactLoop(ctx(executor), { maxRounds: 3 }));
    const ids = chunks.filter((c) => c.type === StreamChunkType.ToolDone).map((c) => c.toolCallId);
    assert.deepEqual(ids, ['1', '2', '3']);
  });

  test('parallelTools=false 时退回串行', async () => {
    const { executor } = barrierExecutors();
    const chunks = await collect(reactLoop(ctx(executor), { maxRounds: 3, parallelTools: false }));
    const results = chunks.filter((c) => c.type === StreamChunkType.ToolDone).map((c) => c.toolResult?.content);
    assert.ok(results.includes('timeout'), '串行时第一个调用等不到栅栏');
  });
});

describe('轮数耗尽不再静默完成', () => {
  const loopCtx = () => ({
    provider: new ScriptedLLM([toolCall('1', 'a__loop')], 'partial thought', true),
    executor: makeExecutor({ a__loop: () => ({ content: 'still working' }) }) as never,
    messages: [...messages],
    sessionId: 's',
    publish: () => {},
  });

  test('processStream 产出错误块 + done，并保留最后一轮文本', async () => {
    const chunks = await collect(reactLoop(loopCtx(), { maxRounds: 2 }));
    const error = chunks.find((c) => c.type === StreamChunkType.Error);
    assert.ok(error, '必须产出 Error chunk');
    assert.match(error!.error?.message ?? '', /reached max rounds \(2\)/);
    const content = chunks.filter((c) => c.type === StreamChunkType.Content).map((c) => c.content).join('');
    assert.equal(content, 'partial thought');
    assert.equal(chunks.at(-1)?.type, StreamChunkType.Done);
  });

  test('collectReact 把该错误透传给调用方', async () => {
    const r = await collectReact(loopCtx(), { maxRounds: 2 });
    assert.ok(r.error, 'error 应回传');
    assert.equal(r.content, 'partial thought');
  });
});

describe('工具错误对模型可见', () => {
  test('失败的工具调用把 error 写进 tool 消息而非空观察', async () => {
    let toolMsg = '';
    let n = 0;
    const provider: LLMProvider = {
      async chat(messages: Message[]) {
        n++;
        if (n === 1) {
          return {
            content: '',
            toolCalls: [toolCall('1', 'a__boom')],
            id: 'r1',
            model: 'm',
          };
        }
        toolMsg = messages.find((m) => m.role === 'tool')?.content ?? '';
        return { id: 'r2', model: 'm', content: 'done' };
      },
      async *chatStream(): AsyncGenerator<StreamChunk> {},
      getModelId: () => 'mock',
    };
    const executor = makeExecutor({ a__boom: () => ({ content: '', error: 'permission denied: nope' }) });
    await collect(
      reactLoop(
        { provider, executor: executor as never, messages: [...messages], sessionId: 's', publish: () => {} },
        { maxRounds: 3 },
      ),
    );
    assert.equal(toolMsg, 'permission denied: nope');
  });

  test('有输出也有错误时两者都保留', async () => {
    let toolMsg = '';
    let n = 0;
    const provider: LLMProvider = {
      async chat(messages: Message[]) {
        n++;
        if (n === 1) return { content: '', toolCalls: [toolCall('1', 'a__half')], id: 'r1', model: 'm' };
        toolMsg = messages.find((m) => m.role === 'tool')?.content ?? '';
        return { id: 'r2', model: 'm', content: 'done' };
      },
      async *chatStream(): AsyncGenerator<StreamChunk> {},
      getModelId: () => 'mock',
    };
    const executor = makeExecutor({
      a__half: () => ({ content: 'partial stdout', error: 'exit code 1' }),
    });
    await collect(
      reactLoop(
        { provider, executor: executor as never, messages: [...messages], sessionId: 's', publish: () => {} },
        { maxRounds: 3 },
      ),
    );
    assert.equal(toolMsg, 'exit code 1\npartial stdout');
  });
});
