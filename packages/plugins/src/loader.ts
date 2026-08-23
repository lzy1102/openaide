/**
 * 插件加载器 —— 同进程动态加载。
 * 基于 Node 原生 import()（tsx 环境下可直接加载 .ts/.js/.mjs/.cjs）。
 * 不启动任何子进程。
 */
import { readFileSync, readdirSync, existsSync, statSync } from 'node:fs';
import { join, isAbsolute, resolve, dirname, basename } from 'node:path';
import { pathToFileURL } from 'node:url';
import YAML from 'yaml';
import type { OpenAIDePlugin, PluginManifest, LoadedPlugin } from './types.js';

export interface PluginLoaderOptions {
  /** 入口文件名候选（按优先级） */
  entryNames?: string[];
}

/** 可能的插件入口文件 */
const DEFAULT_ENTRIES = ['index.ts', 'index.tsx', 'index.js', 'index.mjs', 'index.cjs', 'main.ts', 'main.js'];

/** 插件入口探测 */
export function findEntry(dir: string, entries: string[] = DEFAULT_ENTRIES): string | undefined {
  for (const name of entries) {
    const p = join(dir, name);
    if (existsSync(p) && statSync(p).isFile()) return p;
  }
  // 兼容 package.json 的 main 字段
  const pkg = join(dir, 'package.json');
  if (existsSync(pkg)) {
    try {
      const main = (JSON.parse(readFileSync(pkg, 'utf8')) as { main?: string }).main;
      if (main) {
        const p = resolve(dir, main);
        if (existsSync(p)) return p;
      }
    } catch {
      /* 忽略损坏的 package.json */
    }
  }
  return undefined;
}

/**
 * 扫描目录，发现所有插件子目录。
 * 判定：子目录含入口文件，或含 openaide.yaml。
 */
export function discover(dir: string, options: PluginLoaderOptions = {}): string[] {
  if (!existsSync(dir)) return [];
  const entries = options.entryNames ?? DEFAULT_ENTRIES;
  const found: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (!statSync(full).isDirectory()) continue;
    if (name.startsWith('.')) continue;
    if (existsSync(join(full, 'openaide.yaml')) || findEntry(full, entries)) {
      found.push(full);
    }
  }
  return found;
}

/** 解析插件目录下的 openaide.yaml（可选） */
export function loadManifest(dir: string): PluginManifest | undefined {
  const p = join(dir, 'openaide.yaml');
  if (!existsSync(p)) return undefined;
  try {
    const raw = readFileSync(p, 'utf8');
    return YAML.parse(raw) as PluginManifest;
  } catch (err) {
    console.warn(`[plugins] failed to parse ${p}:`, err);
    return undefined;
  }
}

/** 读取插件目录下的人格外置文件 SYSTEM.md（可选，与 persona 机制一致） */
export function readPersonaFile(
  dir: string,
): { name: string; description?: string; systemPrompt: string } | undefined {
  const systemMd = join(dir, 'SYSTEM.md');
  if (!existsSync(systemMd)) return undefined;
  const content = readFileSync(systemMd, 'utf8');
  const manifest = loadManifest(dir);
  return {
    name: manifest?.persona ?? basename(dir),
    description: manifest?.description,
    systemPrompt: content.trim(),
  };
}

/**
 * 动态加载一个插件模块（同进程）。
 * - entry 可以是目录（自动探测入口）或直接是文件路径
 * - fresh=true 时破坏模块缓存实现热重载（追加时间戳 query）
 */
export async function loadPlugin(entry: string, fresh = false): Promise<LoadedPlugin> {
  let file: string;
  let dir: string;

  if (existsSync(entry) && statSync(entry).isDirectory()) {
    const found = findEntry(entry);
    if (!found) {
      // 纯声明式 persona 插件:openaide.yaml + SYSTEM.md,无代码入口。
      // 生成虚拟插件 —— 人格即全部内容(支持"任务变身":提示词整体替换)。
      const manifest = loadManifest(entry);
      const personaFile = readPersonaFile(entry);
      if (manifest && personaFile) {
        const virtual: OpenAIDePlugin = {
          name: manifest.name ?? basename(entry),
          version: manifest.version ?? '1.0.0',
          description: manifest.description ?? personaFile.description ?? 'declarative persona plugin',
          persona: {
            name: manifest.persona ?? personaFile.name,
            description: personaFile.description ?? manifest.description ?? '',
            systemPrompt: personaFile.systemPrompt,
            toolAllowlist: manifest.toolAllowlist,
          },
        };
        return { plugin: virtual, dir: entry, loadedAt: Date.now() };
      }
      throw new Error(`plugin entry not found in ${entry}`);
    }
    file = found;
    dir = entry;
  } else {
    file = entry;
    dir = dirname(entry);
  }
  if (!isAbsolute(file)) file = resolve(file);

  const url = pathToFileURL(file).href + (fresh ? `?t=${Date.now()}` : '');
  const mod = (await import(url)) as { default?: OpenAIDePlugin; plugin?: OpenAIDePlugin };
  const plugin = mod.default ?? mod.plugin;
  if (!plugin || typeof plugin.name !== 'string' || plugin.name.length === 0) {
    throw new Error(`plugin ${file} does not export a valid OpenAIDePlugin`);
  }
  return { plugin, dir, loadedAt: Date.now() };
}
