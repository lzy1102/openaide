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
