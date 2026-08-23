/**
 * 插件状态持久化 + 启停管理验收测试：
 * 禁用跳过加载 / 卸载并持久化 / 重新启用 / 失败记录三态展示。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync, existsSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { PluginManager } from '../src/manager.js';
import { emptyPluginState, readPluginState, writePluginState, pluginStatePath } from '../src/state.js';

/** 构造隔离的插件目录 + 数据目录，并写入一个声明式 persona 插件 */
function makeWorkspace(): { root: string; pluginsDir: string; dataDir: string; pluginDir: string } {
  const root = mkdtempSync(join(tmpdir(), 'openaide-plugins-'));
  const pluginsDir = join(root, 'plugins');
  const dataDir = join(root, 'data');
  const pluginDir = join(pluginsDir, 'travel');
  mkdirSync(pluginDir, { recursive: true });
  writeFileSync(
    join(pluginDir, 'openaide.yaml'),
    ['name: travel', 'version: 1.0.0', 'description: 旅行规划师'].join('\n'),
  );
  writeFileSync(join(pluginDir, 'SYSTEM.md'), 'You are Voyager.');
  return { root, pluginsDir, dataDir, pluginDir };
}

const newManager = (w: { pluginsDir: string; dataDir: string }) =>
  new PluginManager({ pluginsDir: w.pluginsDir, dataDir: w.dataDir, autoActivate: false });

test('state 读写往返；损坏文件回退为空状态', () => {
  const w = makeWorkspace();
  try {
    assert.deepEqual(readPluginState(w.dataDir), emptyPluginState());
    writePluginState(w.dataDir, { version: 1, disabled: ['a', 'b'] });
    assert.deepEqual(readPluginState(w.dataDir).disabled, ['a', 'b']);
    writeFileSync(pluginStatePath(w.dataDir), '{broken json');
    assert.deepEqual(readPluginState(w.dataDir), emptyPluginState());
  } finally {
    rmSync(w.root, { recursive: true, force: true });
  }
});

test('disable：立即卸载 + 持久化；重启后 loadAll 跳过；enable 恢复加载', async () => {
  const w = makeWorkspace();
  try {
    // 首次：正常加载
    const m1 = newManager(w);
    assert.deepEqual(await m1.loadAll(), ['travel']);
    assert.equal(m1.names().length, 1);

    // 运行时禁用：卸载 + 写盘
    await m1.disable('travel');
    assert.equal(m1.names().length, 0);
    assert.ok(m1.isDisabled('travel'));
    assert.deepEqual(readPluginState(w.dataDir).disabled, ['travel']);

    // 三态展示：disabled
    const info = m1.list().find((p) => p.name === 'travel');
    assert.equal(info?.status, 'disabled');

    // "重启"：新 manager 读同一份状态 → 跳过加载
    const m2 = newManager(w);
    assert.deepEqual(await m2.loadAll(), []);
    assert.ok(!m2.names().includes('travel'));
    assert.ok(m2.isDisabled('travel'), '构造时即读入禁用名单（供内置插件检查）');

    // 重新启用：名单清除 + 立即重新加载
    assert.ok(await m2.enable('travel'));
    assert.deepEqual(m2.names(), ['travel']);
    assert.deepEqual(readPluginState(w.dataDir).disabled, []);
    assert.equal(m2.list().find((p) => p.name === 'travel')?.status, 'active');
    assert.equal(m2.getPersonas()[0]?.name, 'travel');
  } finally {
    rmSync(w.root, { recursive: true, force: true });
  }
});

test('enable 对目录已删除的残留记录返回 false 但仍清名单', async () => {
  const w = makeWorkspace();
  try {
    writePluginState(w.dataDir, { version: 1, disabled: ['gone'] });
    const m = newManager(w);
    assert.equal(await m.enable('gone'), false);
    assert.ok(!m.isDisabled('gone'));
    assert.deepEqual(readPluginState(w.dataDir).disabled, []);
  } finally {
    rmSync(w.root, { recursive: true, force: true });
  }
});

test('loadAll 失败的插件进入 failed 态（含错误信息），不阻塞其余', async () => {
  const w = makeWorkspace();
  try {
    // 只有 manifest、无代码入口也无 SYSTEM.md → 必然加载失败
    const badDir = join(w.pluginsDir, 'broken');
    mkdirSync(badDir, { recursive: true });
    writeFileSync(join(badDir, 'openaide.yaml'), 'name: broken\n');

    const m = newManager(w);
    const loaded = await m.loadAll();
    assert.deepEqual(loaded, ['travel']);
    const failed = m.list().find((p) => p.name === 'broken');
    assert.equal(failed?.status, 'failed');
    assert.match(failed?.error ?? '', /entry not found/);
  } finally {
    rmSync(w.root, { recursive: true, force: true });
  }
});

test('禁用名单按目录名也能命中（manifest 缺失时回退 basename）', async () => {
  const w = makeWorkspace();
  try {
    // 无 manifest 的代码插件目录：候选名回退为目录名
    const plainDir = join(w.pluginsDir, 'plain');
    mkdirSync(plainDir, { recursive: true });
    writeFileSync(
      join(plainDir, 'index.ts'),
      'export default { name: "plain-code", tools: [] };',
    );
    writePluginState(w.dataDir, { version: 1, disabled: ['plain'] });
    const m = newManager(w);
    assert.deepEqual(await m.loadAll(), ['travel']);
    assert.ok(m.isDisabled('plain'));
  } finally {
    rmSync(w.root, { recursive: true, force: true });
  }
});
