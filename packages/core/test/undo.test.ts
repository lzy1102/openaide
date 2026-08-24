import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { AgentKernel } from '../src/kernel.js';
import { MemorySessionStore } from '../src/session.js';
import type { Message } from '../src/types.js';

function makeKernel(history: Array<[string, string]>) {
  const store = new MemorySessionStore();
  const seed = async () => {
    const s = await store.create('p', 'u');
    s.messages = history.map(([role, content]) => ({ role: role as Message['role'], content }));
    await store.update(s);
    return s.id;
  };
  return { store, seed };
}

describe('undoLastRound', () => {
  test('removes the last user+assistant pair and keeps earlier rounds', async () => {
    const { store, seed } = makeKernel([
      ['user', 'q1'], ['assistant', 'a1'], ['user', 'q2'], ['assistant', 'a2'],
    ]);
    const sid = await seed();
    const kernel = new AgentKernel({
      llm: {} as never, tools: {} as never, sessions: store,
      config: { maxRounds: 5 },
    });
    const ok = await kernel.undoLastRound(sid);
    assert.equal(ok, true);
    const s = (await store.get(sid))!;
    assert.equal(s.messages.length, 2);
    assert.equal(s.messages.at(-1)?.content, 'a1');
  });

  test('returns false when there is no user message to undo', async () => {
    const { store, seed } = makeKernel([['assistant', 'only']]);
    const sid = await seed();
    const kernel = new AgentKernel({
      llm: {} as never, tools: {} as never, sessions: store,
      config: { maxRounds: 5 },
    });
    assert.equal(await kernel.undoLastRound(sid), false);
  });

  test('returns false for empty or missing session', async () => {
    const store = new MemorySessionStore();
    const kernel = new AgentKernel({
      llm: {} as never, tools: {} as never, sessions: store,
      config: { maxRounds: 5 },
    });
    const s = await store.create('p', 'u');
    assert.equal(await kernel.undoLastRound(s.id), false);
    assert.equal(await kernel.undoLastRound('missing'), false);
  });
});
