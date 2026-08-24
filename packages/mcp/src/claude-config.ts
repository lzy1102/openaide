/**
 * Claude 配置导入 —— 读取 claude_desktop_config.json / .mcp.json 等同源格式，
 * 提取 mcpServers 映射。字段与 McpServerSpec 结构兼容，原样透传。
 */
import { existsSync, readFileSync } from 'node:fs';

export function loadClaudeMcpConfig(path: string): Record<string, import('./index.js').McpServerSpec> {
  if (!existsSync(path)) throw new Error(`mcp config not found: ${path}`);
  const raw = JSON.parse(readFileSync(path, 'utf8')) as {
    mcpServers?: Record<string, unknown>;
  };
  const out: Record<string, import('./index.js').McpServerSpec> = {};
  for (const [name, entry] of Object.entries(raw.mcpServers ?? {})) {
    const e = entry as { command?: string; args?: string[]; env?: Record<string, string>; timeoutMs?: number };
    if (!e || typeof e.command !== 'string') continue; // 跳过非 stdio 形态（如仅 url），v1 支持 stdio
    out[name] = { command: e.command, args: e.args, env: e.env, timeoutMs: e.timeoutMs };
  }
  return out;
}
