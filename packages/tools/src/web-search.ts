/**
 * web_search —— 联网搜索工具（声明式内置插件）。
 *
 * 后端按环境变量自动选择（优先级从上到下）：
 *   TAVILY_API_KEY  → Tavily Search API（为 agent 设计，免费额度大，推荐）
 *   BRAVE_API_KEY   → Brave Search API（免费 2000 次/月）
 *   SEARXNG_URL     → 自托管 SearXNG 实例（format=json）
 * 都未配置时返回带配置指引的错误结果——工具存在但明确告知如何启用。
 *
 * 返回格式：编号列表（标题/URL/摘要），模型可直接引用并决定是否用
 * execute_command 等进一步抓取。
 */
import type { OpenAIDePlugin } from '@openaide/plugins';

interface WebResult {
  title: string;
  url: string;
  snippet: string;
}

interface Provider {
  name: string;
  run(query: string, maxResults: number, signal?: AbortSignal): Promise<WebResult[]>;
}

/* ────────────────────────── 各后端实现 ────────────────────────── */

async function postJson(url: string, body: unknown, headers: Record<string, string>, signal?: AbortSignal) {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'content-type': 'application/json', ...headers },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${(await res.text()).slice(0, 200)}`);
  return res.json();
}

async function getJson(url: string, headers: Record<string, string>, signal?: AbortSignal) {
  const res = await fetch(url, { headers, signal });
  if (!res.ok) throw new Error(`HTTP ${res.status}: ${(await res.text()).slice(0, 200)}`);
  return res.json();
}

type RunFn = (query: string, maxResults: number, signal?: AbortSignal) => Promise<WebResult[]>;
const PROVIDERS: Record<string, (key?: string) => { name: string; run: RunFn }> = {
  tavily: (key) => ({
    name: 'tavily',
    run: async (query: string, maxResults: number, signal?: AbortSignal) => {
      const data = (await postJson(
        'https://api.tavily.com/search',
        { query, max_results: maxResults },
        { 'content-type': 'application/json' },
        signal,
      )) as { results?: Array<{ title?: string; url?: string; content?: string }> };
      void key;
      return (data.results ?? []).map((r) => ({
        title: r.title ?? '(untitled)',
        url: r.url ?? '',
        snippet: (r.content ?? '').slice(0, 300),
      }));
    },
  }),

  brave: (key) => ({
    name: 'brave',
    run: async (query: string, maxResults: number, signal?: AbortSignal) => {
      const data = (await getJson(
        `https://api.search.brave.com/res/v1/web/search?q=${encodeURIComponent(query)}&count=${maxResults}`,
        { 'x-subscription-token': key ?? '', accept: 'application/json' },
        signal,
      )) as { web?: { results?: Array<{ title?: string; url?: string; description?: string }> } };
      return (data.web?.results ?? []).map((r) => ({
        title: r.title ?? '(untitled)',
        url: r.url ?? '',
        snippet: (r.description ?? '').replace(/<[^>]+>/g, '').slice(0, 300),
      }));
    },
  }),

  searxng: (base) => ({
    name: 'searxng',
    run: async (query: string, maxResults: number, signal?: AbortSignal) => {
      const base2 = String(base ?? '').replace(/\/+$/, '');
      const data = (await getJson(
        `${base2}/search?q=${encodeURIComponent(query)}&format=json`,
        { accept: 'application/json' },
        signal,
      )) as { results?: Array<{ title?: string; url?: string; content?: string }> };
      return (data.results ?? []).slice(0, maxResults).map((r) => ({
        title: r.title ?? '(untitled)',
        url: r.url ?? '',
        snippet: (r.content ?? '').slice(0, 300),
      }));
    },
  }),
};

/** 按环境变量解析可用后端 */
export function resolveProvider(env: NodeJS.ProcessEnv = process.env): { name: string; provider: Provider } | { error: string } {
  if (env.TAVILY_API_KEY) return { name: 'tavily', provider: PROVIDERS.tavily!(env.TAVILY_API_KEY) };
  if (env.BRAVE_API_KEY) return { name: 'brave', provider: PROVIDERS.brave!(env.BRAVE_API_KEY) };
  if (env.SEARXNG_URL) return { name: 'searxng', provider: PROVIDERS.searxng!(env.SEARXNG_URL) };
  return {
    error:
      'no search backend configured. Set one of these env vars: TAVILY_API_KEY (recommended, free tier at tavily.com) | BRAVE_API_KEY | SEARXNG_URL',
  };
}

const TIMEOUT_MS = 20_000;

/** 组合外部信号与超时：任一触发即中止（Node18 无 AbortSignal.any 的兼容实现） */
function withTimeout(
  signal: AbortSignal | undefined,
  ms: number,
): { signal: AbortSignal; cancel: () => void } {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(new Error(`timeout after ${ms}ms`)), ms);
  const onAbort = () => ctrl.abort(signal?.reason);
  if (signal) {
    if (signal.aborted) onAbort();
    else signal.addEventListener('abort', onAbort, { once: true });
  }
  return {
    signal: ctrl.signal,
    cancel: () => {
      clearTimeout(timer);
      signal?.removeEventListener('abort', onAbort);
    },
  };
}

export const webSearchPlugin: OpenAIDePlugin = {
  name: 'websearch',
  version: '0.3.0',
  description: '联网搜索：Tavily / Brave / SearXNG 后端（环境变量自动选择）',
  category: 'capability',

  tools: [
    {
      name: 'web_search',
      description:
        '搜索互联网并返回编号结果列表（标题/URL/摘要）。用于查询最新信息、文档、事实核查。',
      parameters: {
        type: 'object',
        properties: {
          query: { type: 'string', description: '搜索关键词（支持任意语言）' },
          max_results: { type: 'number', description: '返回条数上限（默认 5，最大 10）' },
        },
        required: ['query'],
      },
      handler: async (args, _sessionId, signal) => {
        const query = String(args.query ?? '').trim();
        if (!query) return { content: '', error: 'query required', errorCode: 'INVALID_ARGS' };
        const maxResults = Math.min(Math.max(Number(args.max_results ?? 5), 1), 10);

        const resolved = resolveProvider();
        if ('error' in resolved) return { content: '', error: resolved.error, errorCode: 'NOT_CONFIGURED' };

        try {
          const t = withTimeout(signal, TIMEOUT_MS);
          const results = await resolved.provider.run(query, maxResults, t.signal);
          t.cancel();
          if (results.length === 0) return { content: `(no results for "${query}")` };
          const body = results
            .map((r, i) => `${i + 1}. ${r.title}\n   ${r.url}\n   ${r.snippet}`)
            .join('\n');
          return { content: `[web_search · ${resolved.name}] "${query}"\n\n${body}` };
        } catch (err) {
          return { content: '', error: `web_search failed (${resolved.name}): ${(err as Error).message}`, errorCode: 'EXEC_FAILED' };
        }
      },
    },
  ],
};
