/**
 * web_fetch —— 抓取任意 URL 并返回可读文本。
 * 与 web_search 组成"搜到 → 打开读全文"闭环。
 *
 * 实现：零依赖直抓 + HTML 正文提取（优先 article/main 区域，剥离
 * script/style/nav 等噪音，实体解码，空白折叠）。
 * 边界说明：纯静态抓取——JS 渲染的 SPA 页面请改用 browser_* 工具。
 */
import type { OpenAIDePlugin } from '@openaide/plugins';

const TIMEOUT_MS = 25_000;
const DEFAULT_MAX = 8_000;

/** 组合外部信号与超时（Node18 无 AbortSignal.any） */
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

const ENTITIES: Record<string, string> = {
  amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ', '#39': "'",
};

function decodeEntities(s: string): string {
  return s.replace(/&(#x?[0-9a-fA-F]+|[a-zA-Z]+);/g, (m, code: string) => {
    if (code[0] === '#') {
      const n = code[1] === 'x' || code[1] === 'X'
        ? parseInt(code.slice(2), 16)
        : parseInt(code.slice(1), 10);
      return Number.isFinite(n) ? String.fromCodePoint(n) : m;
    }
    return ENTITIES[code] ?? m;
  });
}

/** HTML → 可读文本：优先 main/article 正文区，剥离噪音标签与实体解码 */
export function htmlToText(html: string): { title: string; text: string } {
  const title = decodeEntities(/<title[^>]*>([\s\S]*?)<\/title>/i.exec(html)?.[1] ?? '').trim();

  // 正文区域优先级：article > main > body
  const region =
    /<article[^>]*>([\s\S]*?)<\/article>/i.exec(html)?.[1] ??
    /<main[^>]*>([\s\S]*?)<\/main>/i.exec(html)?.[1] ??
    /<body[^>]*>([\s\S]*?)<\/body>/i.exec(html)?.[1] ??
    html;

  let s = region
    .replace(/<script[\s\S]*?<\/script>/gi, ' ')
    .replace(/<style[\s\S]*?<\/style>/gi, ' ')
    .replace(/<(nav|header|footer|aside|noscript|form|svg|iframe)[\s\S]*?<\/\1>/gi, ' ')
    .replace(/<!--[\s\S]*?-->/g, ' ')
    .replace(/<\/(p|div|section|li|tr|h[1-6]|pre|blockquote)>/gi, '\n')
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<li[^>]*>/gi, '• ')
    .replace(/<h([1-6])[^>]*>/gi, (_m, lvl: string) => `\n${'#'.repeat(Number(lvl))} `)
    .replace(/<[^>]+>/g, ' ');

  s = decodeEntities(s)
    .replace(/[ \t]+/g, ' ')
    .replace(/\n[ \t]+/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();

  return { title, text: s };
}

export const webFetchPlugin: OpenAIDePlugin = {
  name: 'webfetch',
  version: '0.3.0',
  description: '抓取 URL 返回可读文本（与 web_search 配套）',
  category: 'capability',

  tools: [
    {
      name: 'web_fetch',
      description:
        '抓取 URL 并返回正文文本（自动剥离 HTML 噪音，保留标题/列表/标题层级）。' +
        '适用于：阅读 web_search 命中的网页、拉取 API JSON、读取在线文档。' +
        '注意：纯静态抓取，JS 渲染的动态页面请改用 browser_navigate + browser_snapshot。',
      parameters: {
        type: 'object',
        properties: {
          url: { type: 'string', description: '完整 URL（含 http:// 或 https://）' },
          max_length: { type: 'number', description: '返回文本上限字符数（默认 8000）' },
        },
        required: ['url'],
      },
      handler: async (args, _sessionId, signal) => {
        const url = String(args.url ?? '').trim();
        if (!/^https?:\/\//i.test(url)) {
          return { content: '', error: 'url must start with http:// or https://', errorCode: 'INVALID_ARGS' };
        }
        const maxLength = Math.min(Math.max(Number(args.max_length ?? DEFAULT_MAX), 500), 50_000);

        const t = withTimeout(signal, TIMEOUT_MS);
        try {
          const res = await fetch(url, {
            headers: {
              'user-agent': 'openaide-cli',
              accept: 'text/html,application/json,text/plain;q=0.9,*/*;q=0.5',
              'accept-language': 'zh-CN,zh;q=0.9,en;q=0.8',
            },
            redirect: 'follow',
            signal: t.signal,
          });
          t.cancel();

          if (!res.ok) {
            return {
              content: '',
              error: `HTTP ${res.status} ${res.statusText} for ${url}`,
              errorCode: res.status === 404 ? 'NOT_FOUND' : 'EXEC_FAILED',
            };
          }

          const ctype = res.headers.get('content-type') ?? '';
          const raw = await res.text();

          let body: string;
          let title = '';
          if (ctype.includes('json')) {
            try {
              body = JSON.stringify(JSON.parse(raw), null, 2);
            } catch {
              body = raw;
            }
          } else if (ctype.includes('html')) {
            const parsed = htmlToText(raw);
            title = parsed.title;
            body = parsed.text;
          } else {
            body = raw;
          }

          if (body.length > maxLength) {
            body = body.slice(0, maxLength) + `\n\n[... truncated, ${body.length - maxLength} more chars — raise max_length if needed]`;
          }
          const header = title ? `# ${title}\n${url}\n\n` : `${url}\n\n`;
          return { content: header + body };
        } catch (err) {
          t.cancel();
          const msg = (err as Error).message ?? String(err);
          return { content: '', error: `web_fetch failed: ${msg}`, errorCode: 'EXEC_FAILED' };
        }
      },
    },
  ],
};
