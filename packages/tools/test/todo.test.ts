/**
 * todo_plan 验收 —— 校验、会话隔离、广播、全量覆写。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createTodoPlugin, getTodos } from '../src/todo.js';
import type { EventBus } from '@openaide/core';
import { EventBus as RealBus } from '@openaide/core';

test('全量覆写：空数组/超长/非法状态 均被拒绝', async () => {
  const plugin = createTodoPlugin();
  const handler = plugin.tools!.find((t) => t.name === 'todo_write')!.handler;
  assert.match(String((await handler({ todos: [] }, 's1')).error ?? ''), /non-empty/);
  assert.match(String((await handler({ todos: Array(25).fill({ content: 'x', status: 'pending' }) }, 's1')).error ?? ''), /too many/);
  assert.match(String((await handler({ todos: [{ content: '', status: 'pending' }] }, 's1')).error ?? ''), /empty/);
  assert.match(String((await handler({ todos: [{ content: 'x', status: 'weird' as never }] }, 's1')).error ?? ''), /invalid status/);
});

test('会话隔离：不同 session 的计划互不影响', async () => {
  const bus = new RealBus();
  const plugin = createTodoPlugin(bus as EventBus);
  const handler = plugin.tools!.find((t) => t.name === 'todo_write')!.handler;

  await handler({ todos: [{ content: 'task A', status: 'pending' }] }, 'sA');
  await handler({ todos: [{ content: 'task B', status: 'completed' }] }, 'sB');

  assert.equal(getTodos('sA')[0]?.content, 'task A');
  assert.equal(getTodos('sB')[0]?.content, 'task B');
});

test('广播：写计划后 bus 发布 todo.updated 事件', async () => {
  const bus = new RealBus();
  let seen: unknown = null;
  bus.subscribe((event) => {
    if (event.type === 'todo.updated') seen = event.data;
  });
  const plugin = createTodoPlugin(bus as EventBus);
  const handler = plugin.tools!.find((t) => t.name === 'todo_write')!.handler;

  await handler({ todos: [{ content: 'do X', status: 'in_progress' }] }, 's9');

  const data = seen as { sessionId: string; todos: Array<{ content: string; status: string }> };
  assert.equal(data?.sessionId, 's9');
  assert.equal(data?.todos[0]?.status, 'in_progress');
});

test('完成计数：返回文案含 done/total', async () => {
  const plugin = createTodoPlugin();
  const handler = plugin.tools!.find((t) => t.name === 'todo_write')!.handler;
  const r = await handler(
    {
      todos: [
        { content: 'a', status: 'completed' },
        { content: 'b', status: 'in_progress' },
        { content: 'c', status: 'pending' },
      ],
    },
    's1',
  );
  assert.match(r.content ?? '', /1\/3 completed/);
});
