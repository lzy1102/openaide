/**
 * browser —— 原生浏览器操作工具集（基于 Playwright，懒加载）。
 *
 * 交互模型：snapshot 把页面可交互元素提取成带编号的清单（r1、r2…），
 * 后续 click/type 用编号引用——与模型友好，也避免脆弱的 CSS 选择器。
 *
 * 依赖：playwright 为可选依赖。首次调用任一工具时若未安装，
 * 返回带安装指引的错误（npm i -D playwright && npx playwright install chromium）。
 * 默认无头；OPENAIDE_BROWSER_HEADLESS=0 可显示窗口。
 */
import { resolveProvider } from './web-search.js';
import type { OpenAIDePlugin, PluginTool } from '@openaide/plugins';

/* ── 最小结构类型（避免对 playwright 的编译期依赖）── */
/* eslint-disable @typescript-eslint/no-explicit-any */
type AnyPage = any;
type AnyBrowser = any;

let browser: AnyBrowser | null = null;
let page: AnyPage | null = null;
/** 最近一次 snapshot 的 ref → 定位信息 */
let refs: Array<{ selector: string; label: string }> = [];

async function ensurePage(): Promise<AnyPage> {
  if (page) return page;
  let pw: any;
  const binaryHint = process.env.OPENAIDE_BINARY
    ? 'browser tools need the Node.js edition of OpenAIDE — install it via npm (node scripts/install.mjs), then: npm i -D playwright && npx playwright install chromium'
    : 'Run: npm i -D playwright && npx playwright install chromium';
  try {
    // 变量间接引用：避免 TS 在未安装 playwright 的环境下解析模块类型
    const specifier = 'play' + 'wright';
    pw = await import(/* @vite-ignore */ specifier);
  } catch (err) {
    // 模块本身缺失
    throw new Error(`playwright is not installed. ${binaryHint}`);
  }
  // 模块在但浏览器内核未下载（常见于 CI：npm 包随 devDeps 安装、未跑 install 命令）
  try {
    void pw.chromium;
  } catch (err) {
    throw new Error(`playwright browsers not downloaded. ${binaryHint}`);
  }
  const headless = process.env.OPENAIDE_BROWSER_HEADLESS !== '0';
  // 标准代理环境变量透传（Chromium 无头默认不读系统代理——DDG 等站点需要它）
  const proxyUrl =
    process.env.HTTPS_PROXY || process.env.https_proxy || process.env.HTTP_PROXY || process.env.http_proxy || process.env.ALL_PROXY || process.env.all_proxy;
  const launchOpts: Record<string, unknown> = { headless };
  if (proxyUrl) launchOpts.proxy = { server: proxyUrl };
  try {
    browser = await pw.chromium.launch(launchOpts);
  } catch (err) {
    // npm 包在而浏览器内核未下载（CI 常见）：给出可操作的修复指引
    if (/Executable doesn|playwright.*install/i.test(String((err as Error).message))) {
      throw new Error(`playwright browsers not downloaded. ${binaryHint}`);
    }
    throw err;
  }
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  page = await ctx.newPage();
  page.setDefaultTimeout(15_000);
  return page;
}

export async function closeBrowser(): Promise<void> {
  try {
    await browser?.close();
  } catch {
    /* 忽略关闭失败 */
  }
  browser = null;
  page = null;
  refs = [];
}

/** 提取页面可交互元素并分配编号 */
async function takeSnapshot(p: AnyPage): Promise<string> {
  const items: Array<{ kind: string; label: string; selector: string }> = await p.evaluate(() => {
    /* eslint-disable @typescript-eslint/no-explicit-any */
    // 此回调被序列化到浏览器执行：禁止具名 const 函数（esbuild keepNames 注入的
    // __name helper 只存在于 Node 侧，浏览器里会抛 ReferenceError）
    const doc: any = (globalThis as unknown as { document: any }).document;
    const out: Array<{ kind: string; label: string; selector: string }> = [];
    const els: any[] = [
      ...(doc.querySelectorAll('a[href]') as Iterable<any>),
      ...(doc.querySelectorAll('button,[role=button]') as Iterable<any>),
      ...(doc.querySelectorAll('input:not([type=hidden]),textarea,select') as Iterable<any>),
    ];
    for (let k = 0; k < els.length && out.length < 80; k++) {
      const tag = String(els[k].tagName).toUpperCase();
      const kind = tag === 'A' ? 'link' : tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' ? 'input' : 'button';
      const rect = els[k].getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) continue;
      const label =
        String(els[k].innerText ?? '').trim().slice(0, 80) ||
        els[k].getAttribute('aria-label') ||
        els[k].getAttribute('placeholder') ||
        els[k].getAttribute('href') ||
        '';
      if (!label && kind !== 'input') continue;
      out.push({ kind, label: label.replace(/\s+/g, ' ').slice(0, 80), selector: '' });
    }
    return out;
  });

  // 编号 r1..rN 与提取顺序一一对应；click/type 按 kind+nth 定位
  refs = items.map((it) => ({ selector: `${it.kind} >> nth=-1`, label: `${it.kind}: ${it.label}` }));
  const perKindCount = new Map<string, number>();
  items.forEach((it, i) => {
    const n = perKindCount.get(it.kind) ?? 0;
    perKindCount.set(it.kind, n + 1);
    refs[i] = { selector: `${it.kind} >> nth=${n}`, label: `${it.kind}: ${it.label}` };
  });

  const lines = items.map((it, i) => `r${i + 1}. ${refs[i]!.selector} :: ${it.kind}: ${it.label}`);
  const title = await p.title();
  const textExcerpt = (await p.evaluate(
    () => String((globalThis as unknown as { document: any }).document.body.innerText),
  )).replace(/[ \t]+\n/g, '\n').slice(0, 1200);

  return [
    `page: ${await p.url()}  |  ${title}`,
    '',
    'interactive elements:',
    ...lines,
    '',
    '--- text excerpt ---',
    textExcerpt,
  ].join('\n');
}

function refSelector(ref: string): string {
  const m = /^r(\d+)$/.exec(ref.trim());
  if (!m) throw new Error(`invalid ref "${ref}" — run browser_snapshot first and use ids like r3`);
  const idx = Number(m[1]) - 1;
  const item = refs[idx];
  if (!item) throw new Error(`ref ${ref} not found in last snapshot (${refs.length} items)`);
  return item.selector;
}

/** DuckDuckGo HTML 版兜底解析（/l/?uddg=<encoded> 还原真实 URL） */
async function extractDdgResults(p: AnyPage, max: number): Promise<Array<{ title: string; url: string; snippet: string }>> {
  return p.evaluate((max: number) => {
    /* eslint-disable @typescript-eslint/no-explicit-any */
    const doc: any = (globalThis as unknown as { document: any }).document;
    const out: Array<{ title: string; url: string; snippet: string }> = [];
    const nodes: any[] = [...(doc.querySelectorAll('#links .result') as Iterable<any>)];
    for (let i = 0; i < nodes.length && out.length < max; i++) {
      const a = nodes[i].querySelector('h2 a, .result__a');
      if (!a) continue;
      let href = String(a.href ?? '');
      const m = /[?&]uddg=([^&]+)/.exec(href);
      if (m && m[1]) href = decodeURIComponent(m[1]);
      out.push({
        title: String(a.textContent ?? '').trim(),
        url: href,
        snippet: nodes[i].querySelector('.result__snippet')?.textContent?.trim() ?? '',
      });
    }
    return out;
  }, max);
}

/** 从 Bing 结果页提取自然搜索结果（跳过 bing.com/ck/ 广告跟踪位） */
async function extractBingResults(p: AnyPage, max: number): Promise<Array<{ title: string; url: string; snippet: string }>> {
  return p.evaluate((max: number) => {
    /* eslint-disable @typescript-eslint/no-explicit-any */
    const doc: any = (globalThis as unknown as { document: any }).document;
    const out: Array<{ title: string; url: string; snippet: string }> = [];
    const nodes: any[] = [...(doc.querySelectorAll('#b_results > li.b_algo') as Iterable<any>)];
    for (let i = 0; i < nodes.length && out.length < max * 3; i++) {
      const a = nodes[i].querySelector('h2 a');
      if (!a) continue;
      // 广告特征：跟踪跳转链接；真实结果为直达 URL
      if (String(a.href ?? '').includes('bing.com/ck/')) continue;
      out.push({
        title: String(a.textContent ?? '').trim(),
        url: String(a.href ?? ''),
        snippet: nodes[i].querySelector('.b_caption p, .b_lineclamp2')?.textContent?.trim() ?? '',
      });
    }
    return out;
  }, max);
}

/** 统一错误包装：任何失败都以错误结果返回（模型可读），不向内核抛异常 */
function safe(
  fn: (args: Record<string, unknown>, sessionId: string, signal?: AbortSignal) => Promise<{ content: string }>,
): PluginTool['handler'] {
  return async (args, sessionId, signal) => {
    try {
      return await fn(args, sessionId, signal);
    } catch (err) {
      return {
        content: '',
        error: `${(err as Error).message}`,
        errorCode:
          String((err as Error).message).includes('not installed') ? 'NOT_CONFIGURED' : 'EXEC_FAILED',
      };
    }
  };
}

const TOOLS: PluginTool[] = [
  {
    name: 'browser_navigate',
    description: '打开一个网页，返回页面标题、URL、可交互元素清单和文本摘录',
    parameters: {
      type: 'object',
      properties: { url: { type: 'string', description: '完整 URL（含 https://）' } },
      required: ['url'],
    },
    handler: safe(async (args) => {
      const p = await ensurePage();
      await p.goto(String(args.url), { waitUntil: 'domcontentloaded', timeout: 25_000 });
      return { content: await takeSnapshot(p) };
    }),
  },
  {
    name: 'browser_snapshot',
    description: '读取当前页面：URL、标题、可交互元素编号清单、文本摘录',
    parameters: { type: 'object', properties: {} },
    handler: async () => ({ content: await takeSnapshot(await ensurePage()) }),
  },
  {
    name: 'browser_click',
    description: '点击快照中的某个元素（如 r3），返回点击后的新页面状态',
    parameters: {
      type: 'object',
      properties: { ref: { type: 'string', description: '快照里的编号，如 r3' } },
      required: ['ref'],
    },
    handler: safe(async (args) => {
      const p = await ensurePage();
      await p.locator(refSelector(String(args.ref))).first().click({ timeout: 10_000 });
      await p.waitForLoadState('domcontentloaded', { timeout: 10_000 }).catch(() => {});
      return { content: await takeSnapshot(p) };
    }),
  },
  {
    name: 'browser_type',
    description:
      '向输入框输入文本。ref 缺省用页面最后一个输入框；submit=true 时回车提交（用于站内搜索）',
    parameters: {
      type: 'object',
      properties: {
        text: { type: 'string' },
        ref: { type: 'string', description: '可选：目标输入框编号' },
        submit: { type: 'boolean', description: '输入后是否回车' },
      },
      required: ['text'],
    },
    handler: safe(async (args) => {
      const p = await ensurePage();
      const loc = args.ref
        ? p.locator(refSelector(String(args.ref))).first()
        : p.locator('input:visible').last();
      await loc.fill(String(args.text ?? ''), { timeout: 10_000 });
      if (args.submit) {
        await loc.press('Enter');
        await p.waitForLoadState('domcontentloaded', { timeout: 10_000 }).catch(() => {});
      }
      return { content: await takeSnapshot(p) };
    }),
  },
  {
    name: 'browser_search',
    description:
      '直接在必应搜索并返回结构化结果列表（标题/URL/摘要）。比 navigate+snapshot 更快捷。',
    parameters: {
      type: 'object',
      properties: {
        query: { type: 'string', description: '搜索关键词（任意语言）' },
        max_results: { type: 'number', description: '条数上限（默认 8）' },
      },
      required: ['query'],
    },
    handler: safe(async (args) => {
      const query = String(args.query ?? '').trim();
      const max = Math.min(Math.max(Number(args.max_results ?? 8), 1), 15);

      // 第一优先：搜索 API（Tavily/Brave/SearXNG 已配置时）——结果最稳，
      // 国内主流引擎对无头浏览器均有反爬（Bing 推荐流 / 百度验证码，实测）
      const apiResolved = resolveProvider();
      if (!('error' in apiResolved)) {
        try {
          const results = await apiResolved.provider.run(query, max);
          if (results.length > 0) {
            const body = results
              .map((r, i) => `${i + 1}. ${r.title}\n   ${r.url}\n   ${r.snippet}`)
              .join('\n');
            return { content: `[browser_search · ${apiResolved.name}] "${query}"\n\n${body}` };
          }
        } catch {
          /* API 失败则降级浏览器 */
        }
      }

      const p = await ensurePage();

      // 首选：DuckDuckGo HTML 版（结果最相关、无推荐流污染）。
      // 配置 HTTPS_PROXY 时体验最佳；直连不通时 8s 内快速跳过。
      await p.goto(
        `https://html.duckduckgo.com/html/?q=${encodeURIComponent(query)}`,
        { waitUntil: 'domcontentloaded', timeout: 8_000 },
      ).catch(() => {});
      await p.waitForSelector('#links .result', { timeout: 5_000 }).catch(() => {});
      let results = await extractDdgResults(p, max);
      if (results.length === 0) {
        // 兜底：必应 ensearch=1（强制国际结果）
        await p.goto(
          `https://www.bing.com/search?q=${encodeURIComponent(query)}&ensearch=1`,
          { waitUntil: 'domcontentloaded', timeout: 25_000 },
        );
        await p.waitForSelector('#b_results > li.b_algo', { timeout: 8_000 }).catch(() => {});
        results = await extractBingResults(p, max);
      }
      if (results.length === 0) return { content: `(no results extracted for "${query}")` };
      const body = results
        .map((r, i) => `${i + 1}. ${r.title}\n   ${r.url}\n   ${r.snippet}`)
        .join('\n');
      return { content: `[browser_search · bing] "${query}"\n\n${body}` };
    }),
  },
  {
    name: 'browser_close',
    description: '关闭浏览器释放资源',
    parameters: { type: 'object', properties: {} },
    handler: async () => {
      await closeBrowser();
      return { content: 'browser closed' };
    },
  },
];

export const browserPlugin: OpenAIDePlugin = {
  name: 'browser',
  version: '0.3.0',
  description: '浏览器操作：navigate/snapshot/click/type/search（需 npm i -D playwright）',
  category: 'capability',

  tools: TOOLS,

  deactivate: async () => {
    await closeBrowser();
  },
};
