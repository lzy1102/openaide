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
    // 声明式插件三形态：openaide.yaml / 代码入口 / 纯 Markdown（SYSTEM.md、SKILL.md）
    if (
      existsSync(join(full, 'openaide.yaml')) ||
      existsSync(join(full, 'SYSTEM.md')) ||
      existsSync(join(full, 'SKILL.md')) ||
      findEntry(full, entries)
    ) {
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
 * 读取 SKILL.md（可选）—— 兼容 dsh / agent-skills 生态的可移植技能格式：
 * 可选 YAML frontmatter（name/description），正文即提示词/知识内容。
 * 无 frontmatter 时人格名回退为目录名，实现"丢一个文件夹就是插件"。
 */
export function readSkillFile(
  dir: string,
): { name: string; description?: string; systemPrompt: string } | undefined {
  const p = join(dir, 'SKILL.md');
  if (!existsSync(p)) return undefined;
  const raw = readFileSync(p, 'utf8');
  let name = basename(dir);
  let description: string | undefined;
  let body = raw.trim();
  const fm = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/.exec(raw);
  if (fm) {
    try {
      const meta = YAML.parse(fm[1] ?? '') as { name?: string; description?: string } | null;
      if (meta && typeof meta.name === 'string' && meta.name.trim()) name = meta.name.trim();
      if (meta && typeof meta.description === 'string') description = meta.description;
    } catch {
      /* 坏 frontmatter 按纯 Markdown 处理 */
    }
    body = raw.slice(fm[0].length).trim();
  }
  if (!body) return undefined;
  return { name, description, systemPrompt: body };
}

/**
 * 声明式人格来源统一入口：SYSTEM.md（原生格式，优先）→ SKILL.md（生态兼容格式）。
 * loader 的虚拟插件与 manager 的人格兜底共用，保证两处语义一致。
 */
export function readDeclarativePersona(
  dir: string,
): { name: string; description?: string; systemPrompt: string } | undefined {
  return readPersonaFile(dir) ?? readSkillFile(dir);
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
      // 纯声明式插件：人格文件（SYSTEM.md / SKILL.md）即全部内容，openaide.yaml 可选补充元数据。
      // 仅 manifest 而无人格来源视为残缺插件（多半是写了一半的目录），保持加载失败语义。
      const manifest = loadManifest(entry);
      const personaSource = readDeclarativePersona(entry);
      if (personaSource) {
        const virtual: OpenAIDePlugin = {
          name: manifest?.name ?? personaSource?.name ?? basename(entry),
          version: manifest?.version ?? '1.0.0',
          description:
            manifest?.description ?? personaSource?.description ?? 'declarative persona plugin',
          category: manifest?.category ?? 'persona',
          persona: {
            name: manifest?.persona ?? personaSource?.name ?? basename(entry),
            description: personaSource?.description ?? manifest?.description ?? '',
            systemPrompt: personaSource?.systemPrompt ?? '',
            toolAllowlist: manifest?.toolAllowlist,
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
