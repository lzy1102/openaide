import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { AddressInfo } from 'node:net';
import { WebSocket } from 'ws';
import { StreamChunk, StreamChunkType } from '@openaide/core';
import { SQLiteSessionStore } from '@openaide/memory';
import { createApiServer, ApiContext } from '../src/index.js';

/** mock 内核：process 返回固定内容，processStream 产出固定 chunk */
const fakeKernel = {
  async process(query: { content: string }) {
    return { content: `echo: ${query.content}`, sessionId: 's1', round: 1, totalRounds: 1 };
  },
  async *processStream(query: { content: string }): AsyncGenerator<StreamChunk> {
    yield { type: StreamChunkType.Content, content: `echo: ${query.content}` };
    yield { type: StreamChunkType.Done, done: true, round: 1, totalRounds: 1 };
  },
  getState() {
    return 'idle';
  },
};

function makeCtx(dbPath: string): ApiContext {
  return {
    kernel: fakeKernel,
    sessions: new SQLiteSessionStore(dbPath),
    service: { name: 'openaide', version: 'test', model: 'mock' },
  };
}

async function req(port: number, path: string, init: { method?: string; body?: unknown } = {}) {
  const res = await fetch(`http://127.0.0.1:${port}${path}`, {
    method: init.method ?? 'GET',
    headers: init.body !== undefined ? { 'content-type': 'application/json' } : undefined,
    body: init.body !== undefined ? JSON.stringify(init.body) : undefined,
  });
  return { status: res.status, json: (await res.json()) as Record<string, unknown> };
}

let dbPath = '';
let server: ReturnType<typeof createApiServer>;
let port = 0;
let store: SQLiteSessionStore | undefined;

before(async () => {
  dbPath = join(mkdtempSync(join(tmpdir(), 'openaide-api-')), 'test.db');
  store = new SQLiteSessionStore(dbPath);
  server = createApiServer({ ...makeCtx(dbPath), sessions: store });
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  port = (server.address() as AddressInfo).port;
});

after(async () => {
  await new Promise<void>((resolve, reject) => server.close((e) => (e ? reject(e) : resolve())));
  store?.close();
  rmSync(dbPath, { force: true });
});

test('GET /health 返回服务信息', async () => {
  const { status, json } = await req(port, '/health');
  assert.equal(status, 200);
  assert.equal(json.ok, true);
  assert.equal(json.version, 'test');
  assert.equal(json.model, 'mock');
});

test('POST /v1/chat 非流式问答', async () => {
  const { status, json } = await req(port, '/v1/chat', { method: 'POST', body: { content: 'hi' } });
  assert.equal(status, 200);
  assert.equal(json.content, 'echo: hi');
  assert.ok(json.sessionId);
});

test('POST /v1/chat/stream SSE 流式问答', async () => {
  const res = await fetch(`http://127.0.0.1:${port}/v1/chat/stream`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ content: 'stream me' }),
  });
  assert.equal(res.status, 200);
  const text = await res.text();
  assert.match(text, /event: ready/);
  assert.match(text, /event: chunk/);
  assert.match(text, /echo: stream me/);
  assert.match(text, /event: done/);
});

test('会话管理：创建/列表/删除', async () => {
  const created = await req(port, '/sessions', { method: 'POST', body: { project_id: 'p1' } });
  assert.equal(created.status, 201);
  const id = (created.json.session as { id: string }).id;

  const list = await req(port, '/sessions');
  assert.equal(list.status, 200);
  const sessions = list.json.sessions as Array<{ id: string }>;
  assert.ok(sessions.some((s) => s.id === id));

  const del = await req(port, `/sessions/${id}`, { method: 'DELETE' });
  assert.equal(del.status, 200);
  assert.equal(del.json.ok, true);

  const getGone = await req(port, `/sessions/${id}`);
  assert.equal(getGone.status, 404);
});

test('WebSocket /ws 流式对话', async () => {
  const ws = new WebSocket(`ws://127.0.0.1:${port}/ws`);
  const events: Array<Record<string, unknown>> = [];
  await new Promise<void>((resolve, reject) => {
    ws.on('message', (raw: Buffer) => {
      const msg = JSON.parse(raw.toString()) as Record<string, unknown>;
      events.push(msg);
      if (msg.type === 'done') resolve();
    });
    ws.on('error', reject);
    ws.on('open', () => {
      ws.send(JSON.stringify({ type: 'chat', content: 'ws hello' }));
    });
  });
  ws.close();

  assert.ok(events.some((e) => e.type === 'ready'));
  assert.ok(events.some((e) => e.type === 'chunk'));
  assert.ok(events.some((e) => e.type === 'done' && typeof e.sessionId === 'string'));
});
