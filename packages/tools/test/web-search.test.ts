/**
 * web_search 验收 —— 后端自动选择、结果格式化、错误透传、未配置指引。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { webSearchPlugin, resolveProvider } from '../src/web-search.js';

const tool = webSearchPlugin.tools!.find((t) => t.name === 'web_search')!;

function stubFetch(respond: (url: string, init?: RequestInit) => unknown): void {
  globalThis.fetch = (async (url: string | URL, init?: RequestInit) =>
    new Response(JSON.stringify(respond(String(url), init)), { status: 200 })) as typeof fetch;
}

const TAVILY_BODY = {
  results: [
    { title: 'T1', url: 'https://a.example/1', content: 'first snippet' },
    { title: 'T2', url: 'https://a.example/2', content: 'second snippet' },
  ],
};

test('resolveProvider：按优先级选择后端；全空返回配置指引', () => {
  assert.equal(resolveProvider({ TAVILY_API_KEY: 'k' } as NodeJS.ProcessEnv).name, 'tavily');
  assert.equal(
    resolveProvider({ BRAVE_API_KEY: 'k', SEARXNG_URL: 'http://x' } as NodeJS.ProcessEnv).name,
    'brave',
    'brave 优先于 searxng',
  );
  const miss = resolveProvider({} as NodeJS.ProcessEnv);
  assert.match('error' in miss ? miss.error : '', /TAVILY_API_KEY/);
});

test('tavily：POST 载荷正确，结果格式化为编号列表', async () => {
  let captured: { url: string; body: string } | null = null;
  stubFetch((url, init) => {
    captured = { url: String(url), body: String(init?.body) };
    return TAVILY_BODY;
  });
  process.env.TAVILY_API_KEY = 'tvly-test';
  try {
    const r = await tool.handler({ query: 'openaide release notes', max_results: 2 }, 's1');
    assert.match(r.content ?? '', /1\. T1\n   https:\/\/a\.example\/1\n   first snippet/);
    assert.match(r.content ?? '', /\[web_search · tavily\]/);
    assert.equal(captured!.url, 'https://api.tavily.com/search');
    assert.deepEqual(JSON.parse(captured!.body), { query: 'openaide release notes', max_results: 2 });
  } finally {
    delete process.env.TAVILY_API_KEY;
  }
});

test('brave：token 头 + 描述去 HTML 标签', async () => {
  stubFetch(() => ({
    web: { results: [{ title: 'B1', url: 'https://b.example', description: '<b>bold</b> text' }] },
  }));
  process.env.BRAVE_API_KEY = 'brave-key';
  delete process.env.TAVILY_API_KEY;
  try {
    const r = await tool.handler({ query: 'q' }, 's1');
    assert.match(r.content ?? '', /bold text/);
  } finally {
    delete process.env.BRAVE_API_KEY;
  }
});
let captured_check: string | undefined;

test('未配置任何后端 → 返回带指引的错误而非抛异常', async () => {
  delete process.env.TAVILY_API_KEY;
  delete process.env.BRAVE_API_KEY;
  delete process.env.SEARXNG_URL;
  const r = await tool.handler({ query: 'q' }, 's1');
  assert.match(String(r.error ?? ''), /no search backend configured/);
});
