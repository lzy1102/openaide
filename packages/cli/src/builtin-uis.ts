/**
 * 内置界面插件 —— TUI/REPL 与工具、人格一样走统一插件注册路径。
 * 实现本体（tui.tsx / repl.ts）保持懒加载：start 被调用时才 import 重依赖，
 * 一次性问答与管道场景永远不付出 UI 的加载成本。
 */
import type { OpenAIDePlugin, UiRuntime } from '@openaide/plugins';

export const inkTuiUiPlugin: OpenAIDePlugin = {
  name: 'ui-ink',
  version: '0.3.0',
  description: '内置 Ink TUI（Claude Code 风格交互界面）',
  category: 'ui',
  uis: [
    {
      name: 'ink',
      description: 'Ink TUI（TTY 默认）',
      start: async (host: UiRuntime) => {
        const { runTui } = await import('./tui.js');
        await runTui(host as never, host.initialSessionId);
      },
    },
  ],
};

export const readlineUiPlugin: OpenAIDePlugin = {
  name: 'ui-readline',
  version: '0.3.0',
  description: '内置 readline REPL（非 TTY/管道默认）',
  category: 'ui',
  uis: [
    {
      name: 'readline',
      description: 'readline REPL',
      start: async (host: UiRuntime) => {
        const { runRepl } = await import('./repl.js');
        await runRepl(host as never, host.initialSessionId);
      },
    },
  ],
};

/** 全部内置界面插件（buildApp 按 plugin-state 禁用规则注册） */
export const builtinUiPlugins: OpenAIDePlugin[] = [inkTuiUiPlugin, readlineUiPlugin];
