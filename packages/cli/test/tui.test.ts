import { test } from 'node:test';
import assert from 'node:assert/strict';
import React from 'react';
import { render } from 'ink-testing-library';
import { StreamChunk, StreamChunkType } from '@openaide/core';
import type { App } from '../src/app.js';
import { Tui } from '../src/tui.js';

const flush = (ms = 80) => new Promise((r) => setTimeout(r, ms));
/** Ink 的 useInput 通过 useEffect 注册 readable 监听，需等待挂载完成 */
const mount = (ms = 150) => new Promise((r) => setTimeout(r, ms));

/**
 * 轮询断言：Ink 渲染是异步批处理的（并行测试负载下尤其慢），
 * 固定 sleep 会产生时序 flake——改为反复取样直到条件成立或超时。
 */
async function waitForFrame(
  lastFrame: () => string | undefined,
  pred: (frame: string) => boolean,
  timeoutMs = 3000,
): Promise<string> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const frame = lastFrame() ?? '';
    if (pred(frame)) return frame;
    if (Date.now() > deadline) {
      assert.ok(pred(frame), `frame never matched; last:
${frame.slice(0, 500)}`);
      return frame;
    }
    await flush(40);
  }
}

/** 分步输入：等待挂载 → 写入文本 → 间隔 → 回车提交 */
async function typeAndSubmit(stdin: { write: (d: string) => void }, text: string) {
  await mount();
  stdin.write(text);
  await flush();
  stdin.write('\r');
  await flush(180);
}

/** 构造最小 fake App：内核产生固定流式 chunk */
function makeFakeApp(): App {
  const kernel = {
    async *processStream(): AsyncGenerator<StreamChunk> {
      yield { type: StreamChunkType.ToolCall, toolName: 'demo__upper', toolArgs: '{"text":"hi"}' };
      yield { type: StreamChunkType.Content, content: 'hello ' };
      yield { type: StreamChunkType.Content, content: 'world' };
      yield { type: StreamChunkType.Done, done: true, round: 1, totalRounds: 1 };
    },
    subscribe: () => 0,
  };
  const sessions = {
    list: async () => [],
    get: async () => undefined,
  };
  const plugins = {
    names: () => ['builtin'],
    getPersonas: () => [],
  };
  const registry = {
    definitions: () => [{ type: 'function', function: { name: 'builtin__echo', description: 'echo' } }],
  };
  return {
    kernel,
    sessions,
    plugins,
    registry,
  } as unknown as App;
}

test('TUI：提交消息 → 渲染用户消息/工具调用/流式助手内容', async () => {
  const { lastFrame, stdin, unmount } = render(React.createElement(Tui, { app: makeFakeApp() }));
  await typeAndSubmit(stdin, 'hello agent');

  await waitForFrame(lastFrame, (f) => f.includes('hello agent'));
  const frame = await waitForFrame(lastFrame, (f) => f.includes('demo__upper'));
  await waitForFrame(lastFrame, (f) => f.includes('hello world'));
  assert.ok(frame.includes('tools: 1'), '标题栏应显示工具数');
  unmount();
});

test('TUI：/help 显示命令帮助', async () => {
  const { lastFrame, stdin, unmount } = render(React.createElement(Tui, { app: makeFakeApp() }));
  await typeAndSubmit(stdin, '/help');

  const frame = lastFrame();
  assert.ok(frame.includes('/sessions'), '帮助应包含 /sessions');
  assert.ok(frame.includes('/plugins'), '帮助应包含 /plugins');
  assert.ok(frame.includes('/exit'), '帮助应包含 /exit');
  unmount();
});

test('TUI：/sessions 空列表提示', async () => {
  const { lastFrame, stdin, unmount } = render(React.createElement(Tui, { app: makeFakeApp() }));
  await typeAndSubmit(stdin, '/sessions');

  await waitForFrame(lastFrame, (f) => f.includes('no sessions yet'));
  unmount();
});
