/**
 * PluginManager 验收测试 —— 同进程动态加载、工具注册、钩子接线、人格收集、卸载/热重载、uninstall 路径安全。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
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
import { readPluginState, writePluginState } from '../src/state.js';
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

/** 构造 pluginsDir + dataDir + 一个已安装插件目录的隔离工作区 */
function makeUninstallWorkspace(): {
  root: string;
  pluginsDir: string;
  dataDir: string;
  pluginDir: string;
} {
  const root = mkdtempSync(join(tmpdir(), 'openaide-uninstall-'));
  const pluginsDir = join(root, 'plugins');
  const dataDir = join(root, 'data');
  const pluginDir = join(pluginsDir, 'demo');
  mkdirSync(pluginDir, { recursive: true });
  mkdirSync(dataDir, { recursive: true });
  writeFileSync(
    join(pluginDir, 'openaide.yaml'),
    ['name: demo', 'version: 1.0.0', 'description: uninstall 验收'].join('\n'),
  );
  writeFileSync(join(pluginDir, 'SYSTEM.md'), 'You are Demo.');
  return { root, pluginsDir, dataDir, pluginDir };
}

test('uninstall：pluginsDir 内的插件删盘 + 卸载 + 清禁用名单', async () => {
  const w = makeUninstallWorkspace();
  try {
    writePluginState(w.dataDir, { version: 1, disabled: ['demo'] });
    const m = new PluginManager({
      pluginsDir: w.pluginsDir,
      dataDir: w.dataDir,
      autoActivate: false,
    });
    // 禁用名单内不会被 loadAll 加载，但 knownDirs 仍登记
    await m.loadAll();
    assert.ok(!m.names().includes('demo'));

    // 直接 load 绕过禁用（模拟运行中状态），再 uninstall
    await m.load(w.pluginDir);
    assert.ok(m.names().includes('demo'));

    const removed = await m.uninstall('demo');
    assert.equal(removed, w.pluginDir, '应返回被删除的目录');
    assert.ok(!existsSync(w.pluginDir), '目录应已删除');
    assert.ok(!m.names().includes('demo'), '应已卸载');
    assert.deepEqual(readPluginState(w.dataDir).disabled, [], '禁用名单应清空');
    assert.equal(m.dirOf('demo'), undefined, 'knownDirs 应清除');
  } finally {
    rmSync(w.root, { recursive: true, force: true });
  }
});

test('uninstall：pluginsDir 外部目录只卸载不删盘（防误删 cwd）', async () => {
  const root = mkdtempSync(join(tmpdir(), 'openaide-uninstall-ext-'));
  try {
    const pluginsDir = join(root, 'plugins');
    const dataDir = join(root, 'data');
    // 外部插件目录（模拟内置插件注册到项目 cwd）
    const externalDir = join(root, 'project');
    mkdirSync(pluginsDir, { recursive: true });
    mkdirSync(dataDir, { recursive: true });
    mkdirSync(externalDir, { recursive: true });
    writeFileSync(
      join(externalDir, 'openaide.yaml'),
      ['name: external', 'version: 1.0.0'].join('\n'),
    );
    writeFileSync(join(externalDir, 'SYSTEM.md'), 'You are External.');

    const m = new PluginManager({ pluginsDir, dataDir, autoActivate: false });
    await m.load(externalDir);
    assert.ok(m.names().includes('external'));

    const removed = await m.uninstall('external');
    assert.equal(removed, null, '外部路径不返回删除结果');
    assert.ok(existsSync(externalDir), '外部目录必须保留');
    assert.ok(!m.names().includes('external'), '仍应卸载（内存态清理）');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('uninstall：目标为 pluginsDir 自身或未知插件 → 不删盘', async () => {
  const w = makeUninstallWorkspace();
  try {
    const m = new PluginManager({
      pluginsDir: w.pluginsDir,
      dataDir: w.dataDir,
      autoActivate: false,
    });
    // knownDirs 手工塞入 pluginsDir 自身（防御 rel === '' 分支）
    await m.load(w.pluginDir);
    // 模拟恶意/异常：把 pluginsDir 自己当插件目录
    (m as unknown as { knownDirs: Map<string, string> }).knownDirs.set('evil', w.pluginsDir);

    const removedSelf = await m.uninstall('evil');
    assert.equal(removedSelf, null, 'pluginsDir 自身不可删除');
    assert.ok(existsSync(w.pluginsDir), 'pluginsDir 应保留');

    // 未知插件：无目录记录 → null，不抛
    const removedUnknown = await m.uninstall('no-such-plugin');
    assert.equal(removedUnknown, null);
  } finally {
    rmSync(w.root, { recursive: true, force: true });
  }
});
