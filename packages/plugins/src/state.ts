/**
 * 插件状态持久化 —— <dataDir>/plugin-state.json。
 * 记录被禁用的插件名单（按插件名）；loadAll 跳过名单内的插件。
 * 文件损坏/缺失一律回退为空状态，不阻塞启动。
 */
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';

export interface PluginState {
  version: 1;
  disabled: string[];
}

export function pluginStatePath(dataDir: string): string {
  return join(dataDir, 'plugin-state.json');
}

export function emptyPluginState(): PluginState {
  return { version: 1, disabled: [] };
}

export function readPluginState(dataDir: string): PluginState {
  const p = pluginStatePath(dataDir);
  if (!existsSync(p)) return emptyPluginState();
  try {
    const raw = JSON.parse(readFileSync(p, 'utf8')) as Partial<PluginState>;
    if (!Array.isArray(raw.disabled)) return emptyPluginState();
    return { version: 1, disabled: raw.disabled.filter((x): x is string => typeof x === 'string') };
  } catch {
    return emptyPluginState();
  }
}

export function writePluginState(dataDir: string, state: PluginState): void {
  mkdirSync(dataDir, { recursive: true });
  writeFileSync(pluginStatePath(dataDir), `${JSON.stringify(state, null, 2)}\n`);
}
