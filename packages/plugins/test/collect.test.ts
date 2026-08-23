/**
 * 插件扩展面验收测试 —— providers / interceptors 的注册、收集、卸载反注册。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { LLMProvider, ToolDefinition, ToolExecutor, ToolHandler, ToolCall, ToolResult } from '@openaide/core';
import { PluginManager } from '../src/manager.js';
import type { OpenAIDePlugin } from '../src/types.js';

class FakeExecutor implements ToolExecutor {
  private defs = new Map<string, ToolDefinition>();
  register(def: ToolDefinition): void {
    this.defs.set(def.function.name, def);
  }
  unregister(name: string): void {
    this.defs.delete(name);
  }
  definitions(): ToolDefinition[] {
    return [...this.defs.values()];
  }
  async execute(_call: ToolCall): Promise<ToolResult> {
    return { content: '' };
  }
}

const fakeProvider: LLMProvider = {
  async chat() {
    return { content: 'x' } as never;
  },
  async *chatStream() {
    throw new Error('not used');
  },
  getModelId: () => 'fake-model',
};

const policyPlugin: OpenAIDePlugin = {
  name: 'policy',
  interceptors: [
    { name: 'redact', beforeLLM: () => ({ action: 'allow' }) },
    { name: 'guard', beforeToolCall: () => ({ action: 'allow' }) },
  ],
  providers: [
    { name: 'acme', create: () => fakeProvider },
    { name: 'local-llm', create: () => fakeProvider },
  ],
};

test('add 后可取到 provider 与拦截器；unload 反注册且活数组原地收缩', async () => {
  const m = new PluginManager({
    pluginsDir: '/nonexistent',
    dataDir: '/nonexistent',
    executor: new FakeExecutor(),
    autoActivate: false,
  });

  await m.add(policyPlugin, process.cwd());

  // provider 注册
  assert.ok(m.getProvider('acme'), '应能取到 acme 工厂');
  assert.deepEqual(m.providerNames().sort(), ['acme', 'local-llm']);

  // 拦截器进活数组（同一引用）
  assert.equal(m.interceptors.length, 2);
  const ref = m.interceptors;
  await m.unload('policy');
  assert.equal(ref.length, 0, '卸载后同一数组应为空（内核持引用即时生效）');
  assert.equal(m.getProvider('acme'), undefined);
  assert.equal(m.providerNames().length, 0);

  // 重复 add 同名插件：旧实例的反注册不误删新实例的同名拦截器对象
  await m.add(policyPlugin, process.cwd());
  await m.add({ ...policyPlugin }, process.cwd());
  assert.equal(m.interceptors.length, 2, '同名替换后应恰好保留新插件的两条拦截器');
});
