import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Message, Session } from '@openaide/core';
import { SQLiteSessionStore } from '../src/index.js';

function tmpDb(): string {
  return join(mkdtempSync(join(tmpdir(), 'openaide-memory-')), 'test.db');
}

test('SQLite 会话存储：创建/读取/更新/列表/删除 全流程', async () => {
  const db = tmpDb();
  const store = new SQLiteSessionStore(db);
  try {
    const s = await store.create('proj-a', 'user-1');
    assert.ok(s.id, '应生成会话 ID');
    assert.deepEqual(s.messages, []);

    // 读取
    const got = await store.get(s.id);
    assert.ok(got);
    assert.equal(got!.projectId, 'proj-a');
    assert.equal(got!.userId, 'user-1');

    // 追加消息并更新
    got!.messages.push({ role: 'user', content: 'hello' });
    await store.update(got!);
    const updated = await store.get(s.id)!;
    assert.equal(updated!.messages[0]!.content, 'hello');
    assert.ok(updated!.updatedAt >= got!.createdAt, 'updatedAt 应更新');

    // 列表
    const all = await store.list();
    assert.equal(all.length, 1);
    const byProject = await store.list('proj-a');
    assert.equal(byProject.length, 1);
    const byOther = await store.list('proj-b');
    assert.equal(byOther.length, 0);

    // 删除
    await store.delete(s.id);
    assert.equal(await store.get(s.id), undefined);
  } finally {
    store.close();
    rmSync(db, { force: true });
  }
});

test('SQLite 会话存储：消息含 toolCalls 序列化往返', async () => {
  const db = tmpDb();
  const store = new SQLiteSessionStore(db);
  try {
    const s = await store.create('p', 'u');
    const messages: Message[] = [
      { role: 'user', content: 'call a tool' },
      {
        role: 'assistant',
        content: '',
        toolCalls: [{ id: 'c1', type: 'function', function: { name: 'demo__upper', arguments: '{"text":"x"}' } }],
      },
      { role: 'tool', content: 'X', toolCallId: 'c1' },
    ];
    s.messages.push(...messages);
    await store.update(s);

    const reloaded = await store.get(s.id);
    assert.equal(reloaded!.messages.length, 3);
    assert.equal(reloaded!.messages[1]!.toolCalls![0]!.function.name, 'demo__upper');
    assert.equal(reloaded!.messages[2]!.toolCallId, 'c1');
  } finally {
    store.close();
    rmSync(db, { force: true });
  }
});

test('SQLite 会话存储：关闭后重新打开数据仍在（持久化）', async () => {
  const db = tmpDb();
  const store = new SQLiteSessionStore(db);
  const s = await store.create('proj-persist', 'user-9');
  s.messages.push({ role: 'user', content: 'persist me' });
  await store.update(s);
  store.close();

  // 重新打开同一文件
  const reopened = new SQLiteSessionStore(db);
  try {
    const got = await reopened.get(s.id);
    assert.ok(got, '重新打开后应能读到会话');
    assert.equal(got!.messages[0]!.content, 'persist me');
    assert.ok(existsSync(db), '数据库文件应存在于磁盘');
  } finally {
    reopened.close();
    rmSync(db, { force: true });
  }
});
