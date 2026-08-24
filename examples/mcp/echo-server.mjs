/**
 * 演示用 MCP stdio 服务器 —— 一个 echo 工具。
 * 用途：测试 OpenAIDE 的 MCP 桥（claude_desktop_config.json 同款配置即可挂载）。
 * 启动：node examples/mcp/echo-server.mjs
 */
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from '@modelcontextprotocol/sdk/types.js';

const server = new Server({ name: 'demo-echo', version: '1.0.0' }, { capabilities: { tools: {} } });
const text = (t) => ({ content: [{ type: 'text', text: t }] });

server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: [
    {
      name: 'echo',
      description: '回显输入文本',
      inputSchema: {
        type: 'object',
        properties: { text: { type: 'string', description: '要回显的文本' } },
        required: ['text'],
      },
    },
  ],
}));

server.setRequestHandler(CallToolRequestSchema, async (req) => {
  const args = req.params.arguments ?? {};
  return text(`echo: ${String(args.text ?? '')}`);
});

await server.connect(new StdioServerTransport());
