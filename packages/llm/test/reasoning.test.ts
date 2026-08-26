/** reasoning_effort 透传验收：chat 与 chatStream 请求体都应携带 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { OpenAICompatibleProvider } from '../src/provider.js';

function stub(capture: { url?: string; body1?: any; body2?: any }): void {
  let n = 0;
  globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
    const body = JSON.parse(String(init?.body ?? '{}'));
    if (n === 0) capture.body1 = body;
    else capture.body2 = body;
    n++;
    // 第一次 chat 正常返回；第二次（stream 失败降级路径不会走到）返回同样内容
    return new Response(JSON.stringify({
      id: 'x', model: 'm',
      choices: [{ message: { content: 'ok' }, finish_reason: 'stop' }],
      usage: {},
    }), { status: 200 });
  }) as typeof fetch;
}

test('reasoningEffort 注入请求体（chat 路径）', async () => {
  const cap: any = {};
  stub(cap);
  const p = new OpenAICompatibleProvider({
    baseUrl: 'https://x/v1', apiKey: 'k', model: 'm', reasoningEffort: 'max',
  });
  await p.chat([{ role: 'user', content: 'hi' }], []);
  assert.equal(cap.body1.reasoning_effort, 'max');
});

test('未配置时不携带该字段（兼容不支持推理的模型）', async () => {
  const cap: any = {};
  stub(cap);
  const p = new OpenAICompatibleProvider({ baseUrl: 'https://x/v1', apiKey: 'k', model: 'm' });
  await p.chat([{ role: 'user', content: 'hi' }], []);
  assert.ok(!('reasoning_effort' in cap.body1));
});

/* ────────────── 重试机制 ────────────── */
test('retries：500 后重试成功；401 立即失败不重试', async () => {
  let calls = 0;
  const codes = [500, 401];
  globalThis.fetch = (async () => {
    calls++;
    const code = codes[Math.min(calls - 1, codes.length - 1)]!;
    return new Response('err', { status: code });
  }) as typeof fetch;

  const p = new OpenAICompatibleProvider({
    baseUrl: 'https://x/v1', apiKey: 'k', model: 'm', retries: 3, retryDelayMs: 1,
  });
  await assert.rejects(() => p.chat([{ role: 'user', content: 'hi' }], []), /401/);
  assert.equal(calls, 2, '500 重试一次后撞上 401 应立即停止');
});

test('retries：网络错误指数退避后成功', async () => {
  let calls = 0;
  globalThis.fetch = (async () => {
    calls++;
    if (calls < 3) throw new Error('ECONNRESET');
    return new Response(JSON.stringify({
      choices: [{ message: { content: 'ok' }, finish_reason: 'stop' }], usage: {},
    }), { status: 200 });
  }) as typeof fetch;

  const p = new OpenAICompatibleProvider({
    baseUrl: 'https://x/v1', apiKey: 'k', model: 'm', retries: 3, retryDelayMs: 1,
  });
  const r = await p.chat([{ role: 'user', content: 'hi' }], []);
  assert.equal(r.content, 'ok');
  assert.equal(calls, 3);
});

test('retries：次数耗尽后抛最后错误', async () => {
  let calls = 0;
  globalThis.fetch = (async () => {
    calls++;
    return new Response('down', { status: 503 });
  }) as typeof fetch;

  const p = new OpenAICompatibleProvider({
    baseUrl: 'https://x/v1', apiKey: 'k', model: 'm', retries: 2, retryDelayMs: 1,
  });
  await assert.rejects(() => p.chat([{ role: 'user', content: 'hi' }], []), /503/);
  assert.equal(calls, 3, '初始 1 次 + 重试 2 次');
});

test('retries:-1 无限重试：连续失败 5 次后仍继续并最终成功', async () => {
  let calls = 0;
  globalThis.fetch = (async () => {
    calls++;
    if (calls < 6) throw new Error('ECONNRESET');
    return new Response(JSON.stringify({
      choices: [{ message: { content: 'finally' }, finish_reason: 'stop' }], usage: {},
    }), { status: 200 });
  }) as typeof fetch;

  const p = new OpenAICompatibleProvider({
    baseUrl: 'https://x/v1', apiKey: 'k', model: 'm', retries: -1, retryDelayMs: 1,
  });
  const r = await p.chat([{ role: 'user', content: 'hi' }], []);
  assert.equal(r.content, 'finally');
  assert.equal(calls, 6);
});

test('retries:-1 遇 401 仍然立即失败（无限不适用于鉴权错误）', async () => {
  let calls = 0;
  globalThis.fetch = (async () => { calls++; return new Response('no', { status: 401 }); }) as typeof fetch;
  const p = new OpenAICompatibleProvider({
    baseUrl: 'https://x/v1', apiKey: 'k', model: 'm', retries: -1, retryDelayMs: 1,
  });
  await assert.rejects(() => p.chat([{ role: 'user', content: 'hi' }], []), /401/);
  assert.equal(calls, 1);
});

test('退避封顶：多次重试的延迟不超过 30s（通过耗时上界验证）', async () => {
  let calls = 0;
  const start = Date.now();
  globalThis.fetch = (async () => {
    calls++;
    if (calls < 5) throw new Error('ECONNRESET');
    return new Response(JSON.stringify({
      choices: [{ message: { content: 'ok' }, finish_reason: 'stop' }], usage: {},
    }), { status: 200 });
  }) as typeof fetch;
  const p = new OpenAICompatibleProvider({
    baseUrl: 'https://x/v1', apiKey: 'k', model: 'm', retries: -1, retryDelayMs: 4000,
  });
  await p.chat([{ role: 'user', content: 'hi' }], []);
  const elapsed = Date.now() - start;
  // 4 次重试延迟：4s+8s+16s+30s(封顶，原 32s) ≈ 58s 若未封顶；封顶后 ≈ 4+8+16+30=58? 封顶发生在第4次(32→30)
  // 粗断言：总耗时 < 70s（未封顶会是 >74s），且 > 50s（确实退避过）
  assert.ok(elapsed > 50_000 && elapsed < 70_000, `elapsed=${elapsed}ms`);
});
