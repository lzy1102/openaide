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

const waitForFrame = async (
  lastFrame: () => string | undefined,
  pred: (f: string) => boolean,
  timeoutMs = 3000,
): Promise<void> => {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (pred(lastFrame() ?? '')) return;
    if (Date.now() > deadline) return;
    await flush(40);
  }
};

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

test('TUI：多行粘贴折叠为占位符，提交时还原全文', async () => {
  const sent: string[] = [];
  const kernel = {
    async *processStream(q: { content: string }): AsyncGenerator<StreamChunk> {
      sent.push(q.content);
      yield { type: StreamChunkType.Content, content: 'ok' };
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

  // 括号化粘贴：两行内容成块到达
  stdin.write('\x1b[200~第一行粘贴\n第二行粘贴\x1b[201~');
  await flush();
  await waitForFrame(lastFrame, (f) => f.includes('[粘贴#1 +2行]'));

  // 补一句话并提交
  stdin.write(' 看看这个');
  await flush();
  stdin.write('\r');
  await flush(200);

  await waitForFrame(lastFrame, (f) => f.includes('看看这个'));
  // 用户消息应为还原后的完整内容（占位符位置展开）
  assert.ok(sent[0]?.includes('第一行粘贴'), '粘贴内容应随消息发出');
  assert.ok(sent[0]?.includes('看看这个'), '手打部分保留');
  unmount();
});
