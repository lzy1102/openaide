/**
 * MCP 桥接验收 —— InMemoryTransport 注入工厂，免子进程全链路验证：
 * 连接 → 工具发现 → ctx.registerTool 动态注册 → 经桥调用 → 结果/错误回传。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { InMemoryTransport } from '@modelcontextprotocol/sdk/inMemory.js';
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from '@modelcontextprotocol/sdk/types.js';
import type { ToolDefinition, ToolHandler } from '@openaide/core';
import { createMcpBridgePlugin, expandEnv, type McpServerSpec } from '../src/index.js';

type Registry = Map<string, ToolHandler>;

/** 内存 MCP 服务器：echo（正常）与 boom（isError）两个工具 */
async function makeEchoServer(): Promise<{
  clientTransport: InMemoryTransport;
  dispose: () => Promise<void>;
}> {
  const server = new Server({ name: 'echo-srv', version: '1.0.0' }, { capabilities: { tools: {} } });
  const text = (t: string) => ({ content: [{ type: 'text', text: t }] });
  server.setRequestHandler(ListToolsRequestSchema, async () => ({
    tools: [
      {
        name: 'echo',
        description: '回显输入',
        inputSchema: { type: 'object', properties: { text: { type: 'string' } }, required: ['text'] },
      },
      { name: 'boom', description: '总是报错', inputSchema: { type: 'object', properties: {} } },
    ],
  }));
  server.setRequestHandler(CallToolRequestSchema, async (req) => {
    if (req.params.name === 'boom') return { isError: true, ...text('explosion requested') };
    const args = req.params.arguments as { text?: string };
    return text(`echo: ${args?.text ?? ''}`);
  });

  const client = new Client({ name: 'test-client', version: '0.0.0' });
  const [clientT, serverT] = InMemoryTransport.createLinkedPair();
  await Promise.all([server.connect(serverT), client.connect(clientT)]);
  void client;
  return {
    clientTransport: clientT,
    dispose: async () => {
      await client.close();
      await server.close();
    },
  };
}

async function activateBridge(): Promise<Registry> {
  const srv = await makeEchoServer();
  const registered: Registry = new Map();
  const plugin = await createMcpBridgePlugin({
    servers: { echoSrv: SPEC },
    transportFactory: () => srv.clientTransport,
  });
  await plugin.activate!({
    dir: '.',
    dataDir: '.',
    log: () => {},
    registerTool: (def: ToolDefinition, handler: ToolHandler) => {
      registered.set(def.function.name, handler);
    },
  });
  // 保持进程存活到断言结束：dispose 挂在测试尾部手动调用
  queueMicrotask(() => {});
  void srv;
  return registered;
}

const SPEC: McpServerSpec = { command: 'unused-in-memory' };

test('expandEnv：${VAR} 展开为宿主环境值，未定义置空', () => {
  process.env.OA_TEST_TOKEN = 'abc123';
  assert.equal(expandEnv('Bearer ${OA_TEST_TOKEN}'), 'Bearer abc123');
  assert.equal(expandEnv('${OA_MISSING_X}/path'), '/path');
  delete process.env.OA_TEST_TOKEN;
});

test('MCP 桥端到端：动态注册 <server>__<tool> 并成功调用', async () => {
  const registered = await activateBridge();
  assert.ok(registered.has('echoSrv__echo'), '工具应以 <server>__<tool> 命名注册');
  assert.ok(registered.has('echoSrv__boom'), 'server 的全部工具都应被发现');

  const result = await registered.get('echoSrv__echo')!(JSON.stringify({ text: 'hi via mcp' }), 's1');
  assert.equal(result.content, 'echo: hi via mcp');
  assert.equal(result.error, undefined);
});

test('MCP isError 结果映射为 OpenAIDE 错误结果', async () => {
  const registered = await activateBridge();
  const result = await registered.get('echoSrv__boom')!('{}', 's1');
  assert.match(String(result.error ?? ''), /explosion requested/);
  assert.equal(result.errorCode, 'EXEC_FAILED');
});
