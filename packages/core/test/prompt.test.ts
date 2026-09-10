/**
 * token 估算测试：CJK 保守估（约 1 字符/token，宁可早压缩不溢出），
 * 拉丁文本按 4 字符/token；空文本为 0。
 */
import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { estimateTextTokens } from '../src/prompt.js';

describe('estimateTextTokens', () => {
  test('empty text is zero', () => {
    assert.equal(estimateTextTokens(''), 0);
  });

  test('ascii text approximates chars/4', () => {
    assert.equal(estimateTextTokens('a'.repeat(400)), 100);
  });

  test('cjk text is not underestimated (old impl gave 10 for 40 hanzi)', () => {
    assert.ok(estimateTextTokens('汉'.repeat(40)) >= 40);
  });

  test('mixed text counts both parts', () => {
    assert.equal(estimateTextTokens('汉'.repeat(10) + 'a'.repeat(40)), 20);
  });
});
