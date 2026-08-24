/**
 * 插件市场验收测试 —— 索引解析/搜索 + git 安装/卸载全流程。
 * 测试用本地临时 git 仓库充当远端，全程无网络依赖。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { fetchRegistry, installEntry, parseRegistry, searchRegistry } from '../src/market.js';
import type { RegistryEntry } from '../src/market.js';

const GIT_ID = ['-c', 'user.email=test@test', '-c', 'user.name=test'];

/** 建一个含插件文件的本地 git 仓库，返回仓库路径 */
function makeSourceRepo(): string {
  const repo = mkdtempSync(join(tmpdir(), 'openaide-mkt-src-'));
  const plug = join(repo, 'examples', 'plugins', 'demo');
  mkdirSync(plug, { recursive: true });
  writeFileSync(join(plug, 'openaide.yaml'), ['name: demo', 'version: 1.0.0', 'description: demo plugin'].join('\n'));
  writeFileSync(join(plug, 'SYSTEM.md'), 'You are Demo.');
  const g = (...args: string[]) => execFileSync('git', ['-C', repo, ...GIT_ID, ...args], { stdio: 'pipe' });
  g('init');
  g('add', '.');
  g('commit', '-m', 'init');
  return repo;
}

const entry: RegistryEntry = {
  name: 'demo',
  version: '1.0.0',
  description: 'demo plugin',
  category: 'capability',
  keywords: ['demo', 'test'],
  source: { type: 'git', url: 'https://example.com/demo.git', subdir: 'examples/plugins/demo' },
};

test('parseRegistry：合法索引通过；缺 plugins / 缺 source 抛错', () => {
  const ok = parseRegistry({ version: 1, plugins: [entry] });
  assert.equal(ok.plugins.length, 1);
  assert.throws(() => parseRegistry({ version: 1 }), /plugins.*missing/);
  assert.throws(
    () => parseRegistry({ plugins: [{ name: 'x' }] }),
    /source\.type=git/,
  );
  assert.throws(() => parseRegistry({ plugins: [{ source: { type: 'git', url: 'u' } }] }), /\.name missing/);
});

test('searchRegistry：空关键词返回全部；多字段大小写不敏感匹配', () => {
  const reg = parseRegistry({
    plugins: [
      entry,
      { name: 'contract-analyst', description: '合同分析师 persona', keywords: ['legal'], source: { type: 'git', url: 'u' } },
    ],
  });
  assert.equal(searchRegistry(reg).length, 2);
  assert.deepEqual(searchRegistry(reg, 'DEMO').map((e) => e.name), ['demo']);
  assert.deepEqual(searchRegistry(reg, 'persona').map((e) => e.name), ['contract-analyst']);
  assert.equal(searchRegistry(reg, 'nope').length, 0);
});

test('installEntry：从本地 git 仓库克隆子目录安装；重复安装拒绝；file:// 索引可拉取', async () => {
  const repo = makeSourceRepo();
  const workspace = mkdtempSync(join(tmpdir(), 'openaide-mkt-dst-'));
  try {
    const pluginsDir = join(workspace, 'plugins');

    // file:// 索引（离线场景）
    const regPath = join(workspace, 'registry.json');
    const localEntry: RegistryEntry = { ...entry, source: { ...entry.source, url: repo } };
    writeFileSync(regPath, JSON.stringify({ version: 1, plugins: [localEntry] }));
    const reg = await fetchRegistry(pathToFileURL(regPath).href);
    assert.equal(reg.plugins[0]?.name, 'demo');

    // 安装
    const res = await installEntry(localEntry, { pluginsDir });
    assert.ok(existsSync(join(res.dir, 'openaide.yaml')));
    assert.ok(existsSync(join(res.dir, 'SYSTEM.md')));
    assert.match(readFileSync(join(res.dir, 'SYSTEM.md'), 'utf8'), /Demo/);
    // 不应带入 .git
    assert.ok(!existsSync(join(res.dir, '.git')));

    // 重复安装拒绝
    await assert.rejects(
      () => installEntry(localEntry, { pluginsDir }),
      /already installed/,
    );

    // force 覆盖重装
    const res2 = await installEntry(localEntry, { pluginsDir, force: true });
    assert.equal(res2.dir, res.dir);

    // subdir 不存在时明确报错
    await assert.rejects(
      () =>
        installEntry(
          { ...localEntry, source: { ...localEntry.source, subdir: 'no/such/dir' } },
          { pluginsDir, force: true },
        ),
      /subdir not found/,
    );
  } finally {
    rmSync(repo, { recursive: true, force: true });
    rmSync(workspace, { recursive: true, force: true });
  }
});

test('installEntry：无 subdir 时整个仓库即插件；clone 失败报错清晰', async () => {
  const repo = makeSourceRepo();
  const workspace = mkdtempSync(join(tmpdir(), 'openaide-mkt-root-'));
  try {
    const pluginsDir = join(workspace, 'plugins');
    const res = await installEntry({ name: 'whole', source: { type: 'git', url: repo } }, { pluginsDir });
    assert.ok(existsSync(join(res.dir, 'examples')), '整仓内容被拷入');
    assert.ok(!existsSync(join(res.dir, '.git')), '.git 被过滤');

    await assert.rejects(
      () =>
        installEntry({ name: 'bad', source: { type: 'git', url: join(workspace, 'not-a-repo') } }, { pluginsDir }),
      /git clone failed/,
    );
  } finally {
    rmSync(repo, { recursive: true, force: true });
    rmSync(workspace, { recursive: true, force: true });
  }
});

/* ────────────── GitHub 生态搜索 ────────────── */
import { afterEach } from 'node:test';
import { searchGithubPlugins, GITHUB_PLUGIN_TOPIC } from '../src/market.js';

const ghResponse = {
  items: [
    {
      full_name: 'alice/openaide-travel',
      name: 'openaide-travel',
      description: '旅行规划师人格',
      clone_url: 'https://github.com/alice/openaide-travel.git',
      default_branch: 'main',
      stargazers_count: 42,
      topics: ['openaide-plugin', 'persona'],
      owner: { login: 'alice' },
    },
  ],
};

afterEach(() => { delete process.env.GITHUB_TOKEN; });

test('searchGithubPlugins：topic 查询 + 关键词、字段映射、token 鉴权', async () => {
  let captured: { url: string; headers: Record<string, string> } | null = null;
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async (url: string | URL, init?: { headers?: Record<string, string> }) => {
    captured = { url: String(url), headers: init?.headers ?? {} };
    return new Response(JSON.stringify(ghResponse), { status: 200 });
  }) as typeof fetch;

  try {
    const entries = await searchGithubPlugins('travel persona', { token: 'gh-test-token' });
    assert.match(captured!.url, /topic%3Aopenaide-plugin/);
    assert.match(captured!.url, /travel(\+|%2B| )persona/i);
    assert.equal(captured!.headers.authorization, 'Bearer gh-test-token');

    assert.equal(entries.length, 1);
    const e = entries[0]!;
    assert.equal(e.name, 'openaide-travel');
    assert.equal(e.origin, 'github');
    assert.equal(e.stars, 42);
    assert.deepEqual(e.keywords, ['persona'], '生态约定 topic 本身应从关键词中剔除');
    assert.equal(e.source.url, 'https://github.com/alice/openaide-travel.git');
    assert.equal(e.source.ref, 'main');
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('searchGithubPlugins：403 时给出限流提示', async () => {
  delete process.env.GITHUB_TOKEN;
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () => new Response('rate limited', { status: 403 })) as typeof fetch;
  try {
    await assert.rejects(
      () => searchGithubPlugins(),
      /rate limited — set GITHUB_TOKEN/,
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('GITHUB_PLUGIN_TOPIC 常量符合生态约定', () => {
  assert.equal(GITHUB_PLUGIN_TOPIC, 'openaide-plugin');
});
