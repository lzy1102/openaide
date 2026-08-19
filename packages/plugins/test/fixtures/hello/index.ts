/**
 * 测试用插件 fixture —— 验证动态加载、工具注册、事件钩子、人格注入、热重载。
 * 同进程 import() 加载，无子进程。
 */
import type { OpenAIDePlugin } from '@openaide/plugins';
import type { KernelEvent } from '@openaide/core';

/** 可观测状态（模块级，热重载后重新初始化） */
export const state = {
  activated: false,
  deactivated: false,
  hookEvents: [] as string[],
};

const plugin: OpenAIDePlugin = {
  name: 'hello',
  version: '1.0.0',
  description: '测试插件：工具/钩子/人格',
  tools: [
    {
      name: 'greet',
      description: '问候 name',
      parameters: {
        type: 'object',
        properties: { name: { type: 'string' } },
        required: ['name'],
      },
      handler: async (args) => {
        return { content: `hello, ${String(args.name)}` };
      },
    },
  ],
  hooks: [
    {
      event: 'tool.call.ended',
      handler: async (event: KernelEvent) => {
        state.hookEvents.push(String(event.data?.tool ?? ''));
      },
    },
  ],
  persona: {
    name: 'hello',
    description: '测试人格',
    systemPrompt: 'You are the hello persona.',
  },
  activate: async () => {
    state.activated = true;
  },
  deactivate: async () => {
    state.deactivated = true;
  },
};

export default plugin;
