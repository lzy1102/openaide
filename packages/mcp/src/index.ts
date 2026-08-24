/**
 * MCP bridge —— 把 Model Context Protocol server 暴露为 OpenAIDE 原生工具。
 *
 * 兼容性设计：
 *  - 传输/协议与 Claude Desktop、Claude Code、dsh 同源（官方 MCP SDK，stdio 传输）
 *  - 配置 schema 与 Claude 的 mcpServers 一致：{ command, args?, env? }，
 *    因此 claude_desktop_config.json / .mcp.json 里的服务器条目可直接复制过来
 *  - 工具命名 <server>__<tool>，inputSchema 直通 MCP 的 JSON Schema
 */
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js';
import type { OpenAIDePlugin, PluginTool } from '@openaide/plugins';
import type { ToolResult } from '@openaide/core';

/** 单个 MCP 服务器条目 —— 字段与 Claude mcpServers 兼容 */
export interface McpServerSpec {
  /** 启动命令（如 npx / node / uvx） */
  command: string;
  /** 命令参数 */
  args?: string[];
  /** 注入子进程的环境变量（支持 ${ENV_VAR} 引用宿主环境） */
  env?: Record<string, string>;
  /** 工具调用超时（毫秒，默认 60s） */
  timeoutMs?: number;
}

export interface McpBridgeOptions {
  servers: Record<string, McpServerSpec>;
  clientName?: string;
  /**
   * 传输工厂（缺省 stdio 子进程）。测试/高级场景可注入内存传输等自定义实现，
   * 使桥接逻辑无需真实子进程即可全链路验证。
   */
  transportFactory?: (serverName: string, spec: McpServerSpec) => Promise<Transport> | Transport;
}

interface ConnectedServer {
  name: string;
  client: Client;
  spec: McpServerSpec;
}

/** 展开配置里的 ${ENV_VAR} 引用（Claude 配置常见写法），未定义的引用替换为空串 */
export function expandEnv(value: string): string {
  return value.replace(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g, (_, name: string) => process.env[name] ?? '');
}

function expandSpec(spec: McpServerSpec): McpServerSpec {
  return {
    ...spec,
    command: expandEnv(spec.command),
    args: spec.args?.map(expandEnv),
    env: spec.env
      ? Object.fromEntries(Object.entries(spec.env).map(([k, v]) => [k, expandEnv(v)]))
      : undefined,
  };
}

/** MCP content 数组 → 纯文本（text 拼接；其余类型标注占位） */
function toToolResult(content: unknown): { content: string } {
  const parts = (content as Array<{ type?: string; text?: string }> | undefined) ?? [];
  const texts = parts.map((p) => {
    if (p?.type === 'text') return p.text ?? '';
    if (p?.type) return `[${p.type} content omitted]`;
    return String(p ?? '');
  });
  return { content: texts.filter(Boolean).join('\n') || '(empty tool output)' };
}

export { loadClaudeMcpConfig } from './claude-config.js';

/**
 * 创建 MCP 桥接插件：逐台连接服务器 → listTools → 以 <server>__<tool> 注册。
 * 连接在插件 activate 阶段进行（经 PluginContext.registerTool 动态注册进注册表，
 * 卸载由 Scope 账本统一回收）；单台失败只告警不阻塞其余服务器。
 */
export async function createMcpBridgePlugin(opts: McpBridgeOptions): Promise<OpenAIDePlugin> {
  const connected: ConnectedServer[] = [];

  const plugin: OpenAIDePlugin = {
    name: 'mcp',
    version: '0.3.0',
    description: `MCP bridge (${Object.keys(opts.servers).length} server(s))`,
    category: 'capability',

    activate: async (ctx) => {
      for (const [name, rawSpec] of Object.entries(opts.servers)) {
        const spec = expandSpec(rawSpec);
        const client = new Client({ name: opts.clientName ?? 'openaide-mcp', version: '0.3.0' });
        try {
          const transport = opts.transportFactory
            ? await opts.transportFactory(name, spec)
            : new StdioClientTransport({
                command: spec.command,
                args: spec.args ?? [],
                env: { ...(spec.env ?? {}) },
              });
          await client.connect(transport);
          const { tools } = await client.listTools();
          ctx.log?.(`[mcp] ${name}: connected, ${tools.length} tool(s)`);
          connected.push({ name, client, spec });

          for (const t of tools) {
            const fullName = `${name}__${t.name}`;
            const tool: PluginTool = {
              name: fullName,
              description: t.description ?? `MCP tool ${t.name} from server ${name}`,
              parameters: (t.inputSchema as Record<string, unknown>) ?? { type: 'object', properties: {} },
              handler: async (args: Record<string, unknown>, _sessionId: string, _signal?: AbortSignal) => {
                const result = await client.callTool(
                  { name: t.name, arguments: args },
                  undefined,
                  { timeout: spec.timeoutMs ?? 60_000 },
                );
                if (result.isError) {
                  const errText = toToolResult(result.content).content;
                  const failed: ToolResult = {
                    content: '',
                    error: errText || 'MCP tool reported an error',
                    errorCode: 'EXEC_FAILED',
                  };
                  return failed;
                }
                return toToolResult(result.content);
              },
            };
            // 动态注册：走 ctx 提供的通用通道，卸载由 manager 的 Scope 统一回收
            ctx.registerTool!(
              {
                type: 'function',
                function: { name: fullName, description: tool.description, parameters: tool.parameters },
              },
              (argsJson, sessionId, signal) => tool.handler(JSON.parse(argsJson), sessionId, signal),
            );
          }
        } catch (err) {
          console.warn(`[mcp] server '${name}' failed to start — skipped:`, (err as Error).message);
        }
      }
      if (connected.length === 0 && Object.keys(opts.servers).length > 0) {
        console.warn('[mcp] no MCP server could be reached; bridge registered zero tools');
      }
    },

    deactivate: async () => {
      for (const c of connected) {
        try {
          await c.client.close();
        } catch {
          /* 关闭失败忽略——子进程随 stdio 断开退出 */
        }
      }
      connected.length = 0;
    },
  };

  return plugin;
}
