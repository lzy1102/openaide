/**
 * 示例 TS 插件 —— 同进程动态加载（无子进程）。
 * 导出 OpenAIDePlugin：工具 + 钩子 + 人格。
 */
import type { OpenAIDePlugin } from '@openaide/plugins';
import type { KernelEvent } from '@openaide/core';

const plugin: OpenAIDePlugin = {
  name: 'example',
  version: '0.1.0',
  description: '示例插件：演示动态加载、工具注册、事件钩子、人格注入',

  // 工具集：注册为 example__upper / example__random / example__time
  tools: [
    {
      name: 'upper',
      description: '把 text 参数转为大写',
      parameters: {
        type: 'object',
        properties: { text: { type: 'string' } },
        required: ['text'],
      },
      handler: async (args) => {
        const text = String(args.text ?? '');
        return { content: text.toUpperCase() };
      },
    },
    {
      name: 'random',
      description: '返回一个 1-100 的随机数',
      parameters: { type: 'object', properties: {} },
      handler: async () => {
        return { content: `random: ${Math.floor(Math.random() * 100) + 1}` };
      },
    },
  ],

  // 事件钩子：每次工具调用结束打印一条
  hooks: [
    {
      event: 'tool.call.ended',
      handler: async (event: Event) => {
        console.log(`\x1b[33m[hook:example] tool "${String(event.data?.tool)}" finished\x1b[0m`);
      },
    },
  ],

  // 人格：静态提供（也可由 SYSTEM.md 外置文件提供）
  persona: {
    name: 'example',
    description: '示例人格',
    systemPrompt:
      'You are the example persona. Answer briefly, prefer tools when applicable, and end with an ASCII rocket.',
  },

  activate: async (ctx) => {
    console.log(`[example-plugin] activated in ${ctx.dir}`);
  },
};

export default plugin;
