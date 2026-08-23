/**
 * 插件市场 —— 静态 JSON 索引 + git 拉取安装。
 * - 索引：一个 plugins.json（默认托管在本仓库 registry/ 目录，raw URL 可经 config/env 覆盖）
 * - 安装：git clone --depth 1 到临时目录 → 拷贝（可选子目录）到 pluginsDir/<name>
 * 无服务器、无账号体系；任何人 fork 仓库即可自建市场。
 */
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, join } from 'node:path';
import { spawnSync } from 'node:child_process';

/** 官方默认索引（本仓库 registry/plugins.json 的 raw 地址） */
export const DEFAULT_REGISTRY_URL =
  'https://raw.githubusercontent.com/lzy1102/openaide/master/registry/plugins.json';

/** 安装来源 */
export interface RegistrySource {
  type: 'git';
  url: string;
  /** 分支/tag（缺省默认分支） */
  ref?: string;
  /** 插件在仓库内的子目录（monorepo 场景） */
  subdir?: string;
}

/** 市场索引条目 */
export interface RegistryEntry {
  name: string;
  version?: string;
  description?: string;
  category?: string;
  author?: string;
  keywords?: string[];
  source: RegistrySource;
}

export interface Registry {
  version: number;
  updatedAt?: string;
  plugins: RegistryEntry[];
}

/** 校验并归一化索引 JSON；不合法时抛错（错误信息含原因） */
export function parseRegistry(data: unknown): Registry {
  if (!data || typeof data !== 'object') throw new Error('registry: root must be an object');
  const obj = data as Record<string, unknown>;
  if (!Array.isArray(obj.plugins)) throw new Error('registry: "plugins" array missing');
  const plugins: RegistryEntry[] = [];
  for (const [i, raw] of obj.plugins.entries()) {
    const e = raw as Partial<RegistryEntry>;
    if (!e || typeof e.name !== 'string' || e.name.length === 0) {
      throw new Error(`registry: plugins[${i}].name missing`);
    }
    const src = e.source as Partial<RegistrySource> | undefined;
    if (!src || src.type !== 'git' || typeof src.url !== 'string' || src.url.length === 0) {
      throw new Error(`registry: plugins[${i}] ("${e.name}") needs source.type=git + source.url`);
    }
    plugins.push({
      name: e.name,
      version: typeof e.version === 'string' ? e.version : undefined,
      description: typeof e.description === 'string' ? e.description : undefined,
      category: typeof e.category === 'string' ? e.category : undefined,
      author: typeof e.author === 'string' ? e.author : undefined,
      keywords: Array.isArray(e.keywords) ? e.keywords.filter((k): k is string => typeof k === 'string') : [],
      source: { type: 'git', url: src.url, ref: src.ref, subdir: src.subdir },
    });
  }
  return { version: typeof obj.version === 'number' ? obj.version : 1, updatedAt: obj.updatedAt as string | undefined, plugins };
}

/**
 * 拉取市场索引。支持 http(s):// 与 file://（离线/自建场景）。
 */
export async function fetchRegistry(url: string = DEFAULT_REGISTRY_URL, timeoutMs = 10_000): Promise<Registry> {
  let text: string;
  if (url.startsWith('file://')) {
    const p = url.slice('file://'.length);
    // Windows file URL: file:///D:/x → /D:/x，去掉开头斜杠
    const path = process.platform === 'win32' ? p.replace(/^\/(?=[A-Za-z]:)/, '') : p;
    text = readFileSync(path, 'utf8');
  } else {
    const res = await fetch(url, { signal: AbortSignal.timeout(timeoutMs) });
    if (!res.ok) throw new Error(`registry fetch failed: ${res.status} ${res.statusText} (${url})`);
    text = await res.text();
  }
  try {
    return parseRegistry(JSON.parse(text));
  } catch (err) {
    throw new Error(`registry invalid at ${url}: ${(err as Error).message}`);
  }
}

/** 关键词搜索：空关键词返回全部；匹配 name/description/category/author/keywords（大小写不敏感） */
export function searchRegistry(registry: Registry, keyword = ''): RegistryEntry[] {
  const kw = keyword.trim().toLowerCase();
  if (!kw) return registry.plugins;
  return registry.plugins.filter((e) =>
    [e.name, e.description ?? '', e.category ?? '', e.author ?? '', ...(e.keywords ?? [])]
      .join(' ')
      .toLowerCase()
      .includes(kw),
  );
}

export interface InstallOptions {
  /** 目标插件目录（pluginsDir） */
  pluginsDir: string;
  /** 覆盖安装名（缺省用条目名） */
  name?: string;
  /** 已存在同名目录时是否覆盖重装 */
  force?: boolean;
}

export interface InstallResult {
  /** 安装到的目录（可直接交给 PluginManager.load） */
  dir: string;
  name: string;
}

/**
 * 从索引条目安装插件到 pluginsDir：
 * git clone --depth 1 → 拷贝（可选 subdir）→ 清理临时目录。
 * 目标已存在且未 force 时抛错。
 */
export async function installEntry(entry: RegistryEntry, opts: InstallOptions): Promise<InstallResult> {
  const name = opts.name?.trim() || entry.name;
  const target = join(opts.pluginsDir, name);
  if (existsSync(target)) {
    if (!opts.force) throw new Error(`plugin already installed: ${name} (uninstall first or use force)`);
    rmSync(target, { recursive: true, force: true });
  }

  // 浅克隆到临时目录
  const tmp = mkdtempSync(join(tmpdir(), 'openaide-install-'));
  const repoDir = join(tmp, 'repo');
  try {
    const args = ['clone', '--depth', '1'];
    if (entry.source.ref) args.push('--branch', entry.source.ref);
    args.push(entry.source.url, repoDir);
    const git = spawnSync('git', args, { encoding: 'utf8' });
    if (git.status !== 0) {
      throw new Error(`git clone failed: ${(git.stderr || git.stdout || '').trim()}`);
    }
    const src = entry.source.subdir ? join(repoDir, entry.source.subdir) : repoDir;
    if (!existsSync(src) || !statSync(src).isDirectory()) {
      throw new Error(`source subdir not found in clone: ${entry.source.subdir ?? '(root)'}`);
    }
    mkdirSync(opts.pluginsDir, { recursive: true });
    cpSync(src, target, {
      recursive: true,
      filter: (s) => basename(s) !== '.git',
    });
    return { dir: target, name };
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}
