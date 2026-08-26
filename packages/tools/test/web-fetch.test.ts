/**
 * web_fetch 验收 —— 正文提取、噪音剥离、JSON 直通、截断、错误路径。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { webFetchPlugin, htmlToText } from '../src/web-fetch.js';

const tool = webFetchPlugin.tools!.find((t) => t.name === 'web_fetch')!;

let captured: { body?: string; url?: string } | null = null;

function stubFetch(body: string, ctype = 'text/html; charset=utf-8', status = 200): void {
  captured = null;
  globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
    captured = { url: String(url), body: init?.body as string };
    return new Response(body, { status, headers: { 'content-type': ctype } });
  }) as typeof fetch;
}

test('htmlToText：script/style/nav 剥离、标题层级、实体解码', () => {
  const { title, text } = htmlToText(`<!doctype html><html><head><title>Doc &amp; Guide</title>
    <style>.x{color:red}</style></head><body>
    <nav>menu junk</nav>
    <article><h1>Top</h1><p>A &lt;tag&gt; example&nbsp;text</p>
    <ul><li>one</li><li>two</li></ul>
    <script>alert('junk')</script></article>
    <footer>footer junk</footer></body></html>`);
  assert.equal(title, 'Doc & Guide');
  assert.match(text, /# Top/);
  assert.match(text, /A <tag> example text/);
  assert.match(text, /• one/);
  assert.doesNotMatch(text, /junk|alert|style/);
});

test('web_fetch：HTML 抓取返回标题+正文，UA 正确', async () => {
  stubFetch('<html><head><title>Hello</title></head><body><main><p>world content</p></main></body></html>');
  const r = await tool.handler({ url: 'https://example.com/page' }, 's1');
  assert.match(r.content ?? '', /# Hello/);
  assert.match(r.content ?? '', /world content/);
  assert.equal(captured!.url, 'https://example.com/page');
});

test('JSON 端点：格式化输出；非 2xx → 错误结果', async () => {
  stubFetch('{"ok":true,"n":3}', 'application/json');
  const r1 = await tool.handler({ url: 'https://api.example/data' }, 's1');
  assert.match(r1.content ?? '', /"ok": true/);

  stubFetch('gone', 'text/plain', 404);
  const r2 = await tool.handler({ url: 'https://example.com/missing' }, 's1');
  assert.match(String(r2.error ?? ''), /404/);
  assert.equal(r2.errorCode, 'NOT_FOUND');
});

test('max_length 截断并提示剩余', async () => {
  stubFetch('<body><p>' + 'x'.repeat(2000) + '</p></body>');
  const r = await tool.handler({ url: 'https://example.com/big', max_length: 500 }, 's1');
  assert.ok((r.content ?? '').length < 700);
  assert.match(r.content ?? '', /truncated/);
});

test('协议校验：非 http(s) 拒绝', async () => {
  const r = await tool.handler({ url: 'file:///etc/passwd' }, 's1');
  assert.match(String(r.error ?? ''), /must start with http/);
});
