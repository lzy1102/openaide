/**
 * 示例 TS 插件 —— 同进程动态加载（无子进程）。
 * 导出 OpenAIDePlugin：工具 + 钩子 + 人格 + 受限上下文(LLM/会话/进度)。
 */
import type { OpenAIDePlugin } from '@openaide/plugins';
import type { KernelEvent } from '@openaide/core';

const plugin: OpenAIDePlugin = {
  name: 'example',
  version: '0.3.0',
  description: '示例插件：动态加载、工具注册、dangerous 标记、事件钩子、拦截器、人格注入、LLM/进度访问',

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
    {
      // dangerous 标记示例:执行前内核会发布 tool.permission 提示事件,
      // 且工具描述会被加上 [dangerous] 前缀让 LLM 谨慎使用
      name: 'cleanup',
      description: '清理临时缓存(演示 dangerous 标记)',
      dangerous: true,
      parameters: { type: 'object', properties: {} },
      handler: async (_args, _sessionId, signal) => {
        // 长任务可用 activate 注入的 reportProgress 上报进展
        return { content: 'cleaned (demo — nothing was actually removed)' };
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

  // 拦截器：可否决/改写工具调用与 LLM 请求（策略插件示例）
  // 这里演示：拦下任何试图执行 rm -rf / Remove-Item -Recurse 的调用
  interceptors: [
    {
      name: 'rm-rf-guard',
      beforeToolCall(info) {
        if (/rm\s+-rf|Remove-Item\s+-Recurse/i.test(info.argsJson)) {
          return {
            action: 'deny',
            reason: 'destructive command blocked by rm-rf-guard (example policy)',
          };
        }
        return { action: 'allow' };
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

    // 新扩展点演示:
    // ctx.llm     — 受限 LLM 访问(摘要/自检类插件用)
    // ctx.sessions— 只读会话查询
    // ctx.reportProgress — 长任务进度上报(发布 context.progress 事件)
    if (ctx.llm) {
      console.log(`[example-plugin] llm available, model: ${ctx.llm.model()}`);
    }
    if (ctx.reportProgress) {
      ctx.reportProgress('example plugin activated', 100);
    }
  },
};

export default plugin;
