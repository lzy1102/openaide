/**
 * 上下文压缩器测试:
 *  - LLM 摘要成功 → system 保留、旧消息折叠为摘要、最近消息保留
 *  - LLM 失败 → 确定性截断回退,不抛错
 *  - compressToBudget 渐进收敛,未超预算时不压缩
 */
import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { LLMCompressor, compressToBudget, estimateMessagesTokens } from '../src/compress.js';
import { Message } from '../src/types.js';
import type { LLMProvider, ToolDefinition } from '@openaide/core';

function msg(role: string, content: string): Message {
  return { role: role as Message['role'], content };
}

/** 可脚本化响应的 mock provider */
class MockProvider implements LLMProvider {
  fail = false;
  empty = false;
  calls = 0;

  async chat(
    _messages: Message[],
    _tools: ToolDefinition[],
    _options: Record<string, unknown>,
  ): Promise<{ content: string }> {
    this.calls++;
    if (this.fail) throw new Error('llm down');
    return { content: this.empty ? '' : '[User Intent] test\n[Current State] mid-task' };
  }
}

function bigHistory(n: number): Message[] {
  const msgs: Message[] = [msg('system', 'core rules')];
  for (let i = 0; i < n; i++) {
    msgs.push(msg(i % 2 === 0 ? 'user' : 'assistant', `message ${i} — ${'x'.repeat(500)}`));
  }
  return msgs;
}

describe('LLMCompressor', () => {
  test('compress keeps system messages and recent tail, folds old into summary', async () => {
    const compressor = new LLMCompressor(new MockProvider());
    const history = bigHistory(20);
    const before = estimateMessagesTokens(history);

    const { messages, saved } = await compressor.compress(history, before - 1);

    // system 全量保留
    assert.equal(messages[0].role, 'system');
    assert.ok(messages[0].content.includes('core rules'));
    // 摘要消息存在
    const summary = messages.find((m) => m.content.includes('[Conversation Summary]'));
    assert.ok(summary, 'expected a summary message');
    // 最近消息保留
    const last = history[history.length - 1];
    assert.ok(
      messages.some((m) => m.content === last.content),
      'recent tail message should survive',
    );
    assert.ok(saved > 0, 'should report saved tokens');
  });

  test('compress falls back to deterministic truncation when LLM fails', async () => {
    const provider = new MockProvider();
    provider.fail = true;
    const compressor = new LLMCompressor(provider);
    const history = bigHistory(20);

    const { messages } = await compressor.compress(history, estimateMessagesTokens(history) - 1);

    const summary = messages.find((m) => m.content.includes('[Conversation Summary]'));
    assert.ok(summary, 'fallback summary should exist');
    assert.ok(summary!.content.includes('[Truncated history]'), 'fallback marker present');
  });

  test('compress skips when within budget or too few messages', async () => {
    const compressor = new LLMCompressor(new MockProvider());
    const small = [msg('system', 'rules'), msg('user', 'hi'), msg('assistant', 'yo')];

    const r1 = await compressor.compress(small, 999_999);
    assert.equal(r1.saved, 0);
    assert.deepEqual(r1.messages, small);

    const r2 = await compressor.compress(small, 10); // 超预算但只有 3 条
    assert.equal(r2.saved, 0);
    assert.deepEqual(r2.messages, small);
  });

  test('compressToBudget does nothing when already within budget', async () => {
    const msgs = [msg('user', 'tiny')];
    const { compressed } = await compressToBudget(new LLMCompressor(new MockProvider()), msgs, 9999);
    assert.equal(compressed, false);
  });

  test('compressToBudget converges on oversized context', async () => {
    const provider = new MockProvider();
    const history = bigHistory(30);
    const budget = Math.floor(estimateMessagesTokens(history) / 2);

    const { messages, compressed } = await compressToBudget(
      new LLMCompressor(provider),
      history,
      budget,
    );
    assert.ok(compressed, 'expected compression to run');
    assert.ok(
      estimateMessagesTokens(messages) < estimateMessagesTokens(history),
      'compressed size should shrink',
    );
    assert.ok(provider.calls > 0, 'summary LLM should have been called');
  });
});
