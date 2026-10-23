/**
 * 前缀缓存断点验收 —— cache_control 只应出现在最后一条 system 消息上。
 *
 * 多轮装配后 system 不止一条（L0+L2+工具清单+环境 / [Platform] /
 * [ProjectKnowledge] / [Task]）。旧实现给每条都打 ephemeral，
 * 单次请求塞进多个断点；多数网关只认最后一个，多余的甚至可能被判非法。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { OpenAICompatibleProvider } from '../src/provider.js';

function capture(body: { value?: any }): void {
  globalThis.fetch = (async (_url: string | URL, init?: RequestInit) => {
    body.value = JSON.parse(String(init?.body ?? '{}'));
    return new Response(
      JSON.stringify({
        id: 'x',
        model: 'm',
        choices: [{ message: { content: 'ok' }, finish_reason: 'stop' }],
        usage: {},
      }),
      { status: 200 },
    );
  }) as typeof fetch;
}

test('只有最后一条 system 消息携带 cache_control', async () => {
  const cap: { value?: any } = {};
  capture(cap);
  const p = new OpenAICompatibleProvider({ baseUrl: 'https://x/v1', apiKey: 'k', model: 'm' });

  await p.chat(
    [
      { role: 'system', content: 'L0 + tools + env' },
      { role: 'system', content: '[Platform]' },
      { role: 'system', content: '[ProjectKnowledge]' },
      { role: 'system', content: '[Task: coding]' },
      { role: 'user', content: 'hi' },
    ],
    [],
  );

  const wire = cap.value.messages as Array<Record<string, unknown>>;
  const marked = wire.map((m, i) => (m['cache_control'] ? i : -1)).filter((i) => i >= 0);
  assert.deepEqual(marked, [3], '断点只应落在最后一条 system 上');
  assert.equal(wire[4]?.['cache_control'], undefined, 'user 消息不应带断点');
});

test('没有 system 消息时不打任何断点', async () => {
  const cap: { value?: any } = {};
  capture(cap);
  const p = new OpenAICompatibleProvider({ baseUrl: 'https://x/v1', apiKey: 'k', model: 'm' });
  await p.chat([{ role: 'user', content: 'hi' }], []);
  const wire = cap.value.messages as Array<Record<string, unknown>>;
  assert.equal(wire[0]?.['cache_control'], undefined);
});
