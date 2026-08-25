/**
 * 文件型存储验收 —— 会话随项目走的持久化实现：
 * 往返/列表排序（按内容而非 mtime）/删除联动/损坏容错/路径注入防护。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { FileMemory, FileSessionStore } from '../src/file-store.js';
import type { Session } from '@openaide/core';

const newRoot = () => mkdtempSync(join(tmpdir(), 'openaide-filestore-'));

test('create/get/update 往返；update 写在内容里的 updatedAt 递增', async () => {
  const root = newRoot();
  try {
    const store = new FileSessionStore(join(root, 'sessions'));
    const s = await store.create('proj-a', 'u1');
    assert.ok(existsSync(join(root, 'sessions', `${s.id}.json`)));

    s.messages.push({ role: 'user', content: 'hi' });
    await store.update(s);
    const loaded = await store.get(s.id);
    assert.equal(loaded?.messages[0]?.content, 'hi');
    assert.ok((loaded?.updatedAt ?? 0) >= (loaded?.createdAt ?? 0));

    const raw = JSON.parse(readFileSync(join(root, 'sessions', `${s.id}.json`), 'utf8')) as Session;
    assert.equal(raw.updatedAt, loaded?.updatedAt, '排序依据写在内容里，克隆后仍正确');
    void raw;
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('list 按 updatedAt 降序 + projectId 过滤（与 mtime 无关）', async () => {
  const root = newRoot();
  try {
    const store = new FileSessionStore(join(root, 'sessions'));
    const a = await store.create('p1', 'u');
    const b = await store.create('p2', 'u');
    // 人为把 b 的内容时间戳改得更早，同时让文件 mtime 更晚
    b.updatedAt = 1_000;
    await store.update(b);
    await store.update(a); // a 最后写盘 → mtime 最大

    const all = await store.list();
    assert.deepEqual(
      all.map((s) => s.id),
      [a.id, b.id],
      '按内容 updatedAt 排序，b 虽然 mtime 新但时间戳旧',
    );
    assert.equal((await store.list('p2')).length, 1);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('FileMemory append/load：跨多次 save 累积、limit 取最近 N 条', async () => {
  const root = newRoot();
  try {
    const mem = new FileMemory(join(root, 'memory'));
    await mem.save('s1', [{ role: 'user', content: 'q1' }]);
    await mem.save('s1', [
      { role: 'assistant', content: 'a1' },
      { role: 'user', content: 'q2' },
    ]);
    await mem.save('s1', [{ role: 'assistant', content: '' }]); // 空中间态被跳过

    const all = await mem.load('s1', -1);
    assert.deepEqual(all.map((m) => m.content), ['q1', 'a1', 'q2']);

    const recent = await mem.load('s1', 2);
    assert.deepEqual(recent.map((m) => m.content), ['a1', 'q2']);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('delete 联动清理 memory 流水；损坏会话文件被容忍', async () => {
  const root = newRoot();
  try {
    const store = new FileSessionStore(join(root, 'sessions'));
    const mem = new FileMemory(join(root, 'memory'));
    const s = await store.create('p', 'u');
    await mem.save(s.id, [{ role: 'user', content: 'x' }]);
    store.onDelete = (id) => rmSync(join(root, 'memory', `${id}.jsonl`), { force: true });
    await store.delete(s.id);
    assert.ok(!existsSync(join(root, 'sessions', `${s.id}.json`)));
    assert.equal(await mem.load(s.id, -1).then((r) => r.length), 0, '流水应一并删除');

    writeFileSync(join(root, 'sessions', 'broken.json'), '{oops');
    await store.create('p', 'u');
    assert.equal(await store.list().then((r) => r.length), 1, '坏文件跳过不阻塞');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('路径注入防护：非法 id 拒绝落盘', async () => {
  const root = newRoot();
  try {
    const store = new FileSessionStore(join(root, 'sessions'));
    await assert.rejects(() => store.get('../evil'));
    await assert.rejects(() => store.delete('..\\evil'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
