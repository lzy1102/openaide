/**
 * PluginManager 验收测试 —— 同进程动态加载、工具注册、钩子接线、人格收集、卸载/热重载。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  EventBus,
  ToolCall,
  ToolDefinition,
  ToolExecutor,
  ToolHandler,
  ToolResult,
} from '@openaide/core';
import { PluginManager } from '../src/manager.js';
import { state } from './fixtures/hello/index.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const fixturesDir = join(__dirname, 'fixtures');
const helloDir = join(fixturesDir, 'hello');

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
  has(name: string): boolean {
    return this.defs.has(name);
  }
  async execute(toolCall: ToolCall, sessionId: string, signal?: AbortSignal): Promise<ToolResult> {
    const handler = this.handlers.get(toolCall.function.name);
    if (!handler) return { content: '', error: `unknown tool: ${toolCall.function.name}`, errorCode: 'NOT_FOUND' };
    return handler(toolCall.function.arguments, sessionId, signal);
  }
}

const nextTick = () => new Promise((r) => setImmediate(r));

test('动态加载插件：注册命名空间工具 + 收集人格 + 钩子接线 + 卸载清理', async () => {
  const executor = new FakeExecutor();
  const bus = new EventBus();
  const manager = new PluginManager({
    pluginsDir: fixturesDir,
    dataDir: fixturesDir,
    executor,
    eventBus: bus,
    autoActivate: false,
  });

  // 加载并激活
  const name = await manager.load(helloDir);
  assert.equal(name, 'hello');
  assert.equal(manager.names().length, 1);
  assert.ok(state.activated, 'activate 应被调用');

  // 工具以 <插件名>__<工具名> 注册
  assert.ok(executor.has('hello__greet'), '工具应注册为 hello__greet');

  // 信息列表含分类（代码声明 category）
  const info = manager.list();
  assert.equal(info.length, 1);
  assert.equal(info[0]?.name, 'hello');
  assert.equal(info[0]?.category, 'capability');
  assert.deepEqual(info[0]?.tools, ['hello__greet']);
  assert.equal(info[0]?.persona, true);

  // 工具执行
  const result = await executor.execute(
    { id: 't1', type: 'function', function: { name: 'hello__greet', arguments: JSON.stringify({ name: 'world' }) } },
    's1',
  );
  assert.equal(result.content, 'hello, world');

  // 人格收集
  const personas = manager.getPersonas();
  assert.equal(personas.length, 1);
  assert.equal(personas[0]?.name, 'hello');

  // 钩子接线：发布 tool.call.ended → 插件钩子收到
  bus.publish({ type: 'tool.call.ended', source: 'kernel', data: { tool: 'hello__greet' }, timestamp: Date.now() });
  await nextTick();
  assert.deepEqual(state.hookEvents, ['hello__greet']);

  // 卸载：注销工具 + 取消钩子 + 调 deactivate + 移除人格
  await manager.unload('hello');
  assert.equal(manager.names().length, 0);
  assert.ok(!executor.has('hello__greet'));
  assert.ok(state.deactivated, 'deactivate 应被调用');
  assert.equal(manager.getPersonas().length, 0);

  // 卸载后钩子不再触发
  state.hookEvents.length = 0;
  bus.publish({ type: 'tool.call.ended', source: 'kernel', data: { tool: 'x' }, timestamp: Date.now() });
  await nextTick();
  assert.deepEqual(state.hookEvents, []);
});

test('热重载：破坏模块缓存重新加载', async () => {
  const executor = new FakeExecutor();
  const manager = new PluginManager({
    pluginsDir: fixturesDir,
    dataDir: fixturesDir,
    executor,
    autoActivate: false,
  });

  await manager.load(helloDir);
  assert.ok(executor.has('hello__greet'));

  await manager.reload('hello');
  assert.equal(manager.names().length, 1);
  assert.ok(executor.has('hello__greet'), '重载后工具重新注册');
});
