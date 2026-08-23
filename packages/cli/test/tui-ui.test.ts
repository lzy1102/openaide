/**
 * TUI 重设计验收 —— 补全弹层 / Tab 补全 / 状态栏常驻。
 * 复用 tui.test.ts 的 fake App 构造方式。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import React from 'react';
import { render } from 'ink-testing-library';
import { StreamChunk, StreamChunkType } from '@openaide/core';
import type { App } from '../src/app.js';
import { Tui } from '../src/tui.js';

const flush = (ms = 80) => new Promise((r) => setTimeout(r, ms));
const mount = (ms = 150) => new Promise((r) => setTimeout(r, ms));

function makeFakeApp(): App {
  const kernel = {
    async *processStream(): AsyncGenerator<StreamChunk> {
      yield { type: StreamChunkType.Content, content: 'ok' };
      yield { type: StreamChunkType.Done, done: true, round: 1, totalRounds: 1 };
    },
    subscribe: () => 0,
  };
  const sessions = { list: async () => [], get: async () => undefined };
  const plugins = { names: () => ['builtin', 'file-tools'], getPersonas: () => [] };
  const registry = {
    definitions: () => [
      { type: 'function', function: { name: 'a__one', description: 'one' } },
      { type: 'function', function: { name: 'b__two', description: 'two' } },
    ],
  };
  return { kernel, sessions, plugins, registry } as unknown as App;
}

test('TUI：输入 "/" 即弹出命令补全', async () => {
  const { lastFrame, stdin, unmount } = render(React.createElement(Tui, { app: makeFakeApp() }));
  await mount();
  stdin.write('/');
  await flush();

  const frame = lastFrame();
  assert.ok(frame?.includes('/sessions'), '弹层应列出 /sessions');
  assert.ok(frame?.includes('/persona'), '弹层应列出 /persona');
  assert.ok(frame?.includes('list plugins'), '弹层应带描述');
  unmount();
});

test('TUI：过滤前缀 + Tab 补全为完整命令', async () => {
  const { lastFrame, stdin, unmount } = render(React.createElement(Tui, { app: makeFakeApp() }));
  await mount();
  stdin.write('/pers');
  await flush();
  stdin.write('\t');
  await flush();

  const frame = lastFrame();
  assert.ok(frame?.includes('/persona '), 'Tab 后应补全为 "/persona "');
  unmount();
});

test('TUI：状态栏常驻显示 model/session/plugins/tools', async () => {
  const { lastFrame, unmount } = render(React.createElement(Tui, { app: makeFakeApp() }));
  await mount();

  const frame = lastFrame();
  assert.ok(frame?.includes('plugins: 2'), '应显示插件数');
  assert.ok(frame?.includes('tools: 2'), '应显示工具数');
  assert.ok(frame?.includes('session'), '应显示会话标识');
  unmount();
});

test('TUI：工具失败时展示输出摘要（↳ 行）', async () => {
  const kernel = {
    async *processStream(): AsyncGenerator<StreamChunk> {
      yield { type: StreamChunkType.ToolCall, toolName: 'demo__cmd', toolArgs: '{"x":1}' };
      yield {
        type: StreamChunkType.ToolDone,
        toolName: 'demo__cmd',
        toolResult: { content: 'build log line\nError: cannot find module', error: 'exit error: failed' },
      };
      yield { type: StreamChunkType.Done, done: true, round: 1, totalRounds: 1 };
    },
    subscribe: () => 0,
  };
  const app = makeFakeApp() as unknown as Record<string, unknown>;
  app.kernel = kernel;
  const { lastFrame, stdin, unmount } = render(
    React.createElement(Tui, { app: app as unknown as App }),
  );
  await mount();
  stdin.write('run it');
  await flush();
  stdin.write('\r');
  await flush(200);

  const frame = lastFrame();
  assert.ok(frame?.includes('exit error'), '应显示错误信息');
  assert.ok(frame?.includes('cannot find module'), '应展示 stdout/stderr 摘要');
  unmount();
});
