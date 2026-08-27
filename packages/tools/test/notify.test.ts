import { test } from 'node:test';
import assert from 'node:assert/strict';
import { notifyPlugin } from '../src/notify.js';

test('notify requires backends', async () => {
  delete process.env.NTFY_URL;
  delete process.env.NTFY_TOPIC;
  delete process.env.BARK_URL;
  delete process.env.WEBHOOK_URL;
  const handler = notifyPlugin.tools.find(t => t.name === 'notify').handler;
  const r = await handler({ message: 'hello' }, 's1');
  assert.match(r.error, /no notify backend/);
});

test('notify sends to ntfy', async () => {
  const calls: any[] = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return new Response('ok', { status: 200 }) as Response;
  };
  process.env.NTFY_TOPIC = 'test-topic-123';
  const handler = notifyPlugin.tools.find(t => t.name === 'notify').handler;
  const r = await handler({ message: 'done', title: 'Test' }, 's1');
  assert.match(r.content, /ntfy: ok/);
  delete process.env.NTFY_TOPIC;
  assert.equal(calls.length, 1);
  assert.match(calls[0].url, /ntfy\.sh\/test-topic-123/);
});

test('notify sends to webhook', async () => {
  let body: any = null;
  globalThis.fetch = async (url, init) => {
    body = JSON.parse(init.body);
    return new Response('ok', { status: 200 }) as Response;
  };
  process.env.WEBHOOK_URL = 'https://example.com/hook';
  const handler = notifyPlugin.tools.find(t => t.name === 'notify').handler;
  const r = await handler({ message: 'hi', level: 'error' }, 's1');
  assert.match(r.content, /webhook: ok/);
  assert.equal(body.level, 'error');
  delete process.env.WEBHOOK_URL;
});
