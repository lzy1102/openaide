/**
 * AgentKernel + ReAct 循环验收测试 —— 用 mock LLM 验证 思考→工具调用→观察→最终回答。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  AgentKernel,
  EventTypes,
  LLMProvider,
  LLMResponse,
  Message,
  StreamChunk,
  StreamChunkType,
  ToolCall,
  ToolDefinition,
  ToolExecutor,
  ToolHandler,
  ToolResult,
} from '../src/index.js';

/** 最小工具执行器（满足 ToolExecutor 契约） */
class FakeExecutor implements ToolExecutor {
  private defs = new Map<string, ToolDefinition>();
  private handlers = new Map<string, ToolHandler>();

  register(def: ToolDefinition, handler: ToolHandler): void {
    this.defs.set(def.function.name, def);
    this.handlers.set(def.function.name, handler);
  }
  unregister(name: string): void {
    this.defs.delete(name);
    this.handlers.delete(name);
  }
  definitions(): ToolDefinition[] {
    return [...this.defs.values()];
  }
  async execute(toolCall: ToolCall, sessionId: string, signal?: AbortSignal): Promise<ToolResult> {
    const handler = this.handlers.get(toolCall.function.name);
    if (!handler) return { content: '', error: `unknown tool: ${toolCall.function.name}`, errorCode: 'NOT_FOUND' };
    try {
      return await handler(toolCall.function.arguments, sessionId, signal);
    } catch (err) {
      return { content: '', error: String(err), errorCode: 'EXEC_FAILED' };
    }
  }
}

/** mock LLM：第一轮返回工具调用，第二轮返回最终回答 */
class MockLLM implements LLMProvider {
  calls = 0;
  lastTools: string[] = [];

  async chat(messages: Message[], tools: ToolDefinition[]): Promise<LLMResponse> {
    this.calls++;
    this.lastTools = tools.map((t) => t.function.name);
    if (this.calls === 1) {
      return {
        id: 'r1',
        model: 'mock',
        content: '',
        toolCalls: [
          {
            id: 'tc1',
            type: 'function',
            function: { name: 'demo__upper', arguments: JSON.stringify({ text: 'hello' }) },
          },
        ],
      };
    }
    return {
      id: 'r2',
      model: 'mock',
      content: 'final answer: HELLO',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    };
  }

  async *chatStream(): AsyncGenerator<StreamChunk> {
    // process 同步路径不使用流式
  }

  getModelId(): string {
    return 'mock';
  }
}

test('ReAct 循环：工具调用 → 观察 → 最终回答（process）', async () => {
  const llm = new MockLLM();
  const executor = new FakeExecutor();
  executor.register(
    { type: 'function', function: { name: 'demo__upper', description: '转为大写', parameters: {} } },
    async (args) => ({ content: String((JSON.parse(args) as { text: string }).text).toUpperCase() }),
  );

  const events: string[] = [];
  const kernel = new AgentKernel({ llm, tools: executor, config: { maxRounds: 5 } });
  kernel.subscribe((e) => events.push(e.type));

  const resp = await kernel.process({ sessionId: '', projectId: 'p', userId: 'u', content: 'upper hello', options: {} });

  assert.equal(llm.calls, 2, '应调用两次 LLM（思考 + 工具后收尾）');
  assert.ok(llm.lastTools.includes('demo__upper'));
  assert.equal(resp.content, 'final answer: HELLO');
  assert.equal(resp.round, 2);
  assert.equal(resp.usage?.totalTokens, 15);

  // 事件总线：工具生命周期 + 查询生命周期
  for (const t of [EventTypes.QueryStart, EventTypes.ToolCallStarted, EventTypes.ToolCallEnded, EventTypes.QueryEnd]) {
    assert.ok(events.includes(t), `应发布事件 ${t}`);
  }
});

test('未知工具：错误进入 tool 消息并继续循环', async () => {
  const llm = new MockLLM();
  const executor = new FakeExecutor(); // 未注册任何工具
  const kernel = new AgentKernel({ llm, tools: executor, config: { maxRounds: 5 } });
  const resp = await kernel.process({ sessionId: '', projectId: 'p', userId: 'u', content: 'hi', options: {} });
  assert.equal(resp.content, 'final answer: HELLO');
  assert.equal(resp.round, 2);
});

test('流式入口 processStream：产出 content + done chunk', async () => {
  const llm = new MockLLM();
  const executor = new FakeExecutor();
  executor.register(
    { type: 'function', function: { name: 'demo__upper', description: '转为大写', parameters: {} } },
    async (args) => ({ content: String((JSON.parse(args) as { text: string }).text).toUpperCase() }),
  );

  const kernel = new AgentKernel({ llm, tools: executor, config: { maxRounds: 5 } });
  const types: string[] = [];
  let content = '';
  for await (const chunk of kernel.processStream({ sessionId: '', projectId: 'p', userId: 'u', content: 'hi', options: {} })) {
    types.push(chunk.type);
    if (chunk.type === StreamChunkType.Content && chunk.content) content += chunk.content;
  }
  assert.equal(content, 'final answer: HELLO');
  assert.ok(types.includes(StreamChunkType.ToolCall));
  assert.ok(types.includes(StreamChunkType.ToolDone));
  assert.ok(types.includes(StreamChunkType.Done));
});

test('会话存储：自动创建并持久化消息', async () => {
  const llm = new MockLLM();
  const executor = new FakeExecutor();
  executor.register(
    { type: 'function', function: { name: 'demo__upper', description: '转为大写', parameters: {} } },
    async (args) => ({ content: String((JSON.parse(args) as { text: string }).text).toUpperCase() }),
  );
  const kernel = new AgentKernel({ llm, tools: executor, config: { maxRounds: 5 } });

  const resp = await kernel.process({ sessionId: '', projectId: 'p', userId: 'u', content: 'hi', options: {} });
  const session = await kernel.sessions.get(resp.sessionId);
  assert.ok(session, '会话应被创建');
  assert.ok(session!.messages.length >= 2, '应持久化 user + assistant 消息');
});
