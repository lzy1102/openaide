/**
 * CLI 参数解析测试 —— --model / --output json / 子命令 / 文件上下文。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { parseArgs, buildPrompt } from '../src/args.js';

test('parseArgs：--model 与 --output json', () => {
  const r = parseArgs(['--model', 'deepseek-v4', 'hi']);
  assert.equal(r.model, 'deepseek-v4');
  assert.equal(r.prompt, 'hi');
  assert.equal(r.outputJson, false);

  const r2 = parseArgs(['hello', '--output=json', '-m', 'gpt']);
  assert.equal(r2.model, 'gpt');
  assert.equal(r2.outputJson, true);
  assert.equal(r2.prompt, 'hello');
});

test('parseArgs：子命令与 -c', () => {
  assert.equal(parseArgs(['-c']).cmd, '-c');
  assert.equal(parseArgs(['plugins', '--model', 'x']).cmd, 'plugins');
  assert.equal(parseArgs(['plugins', '--model', 'x']).model, 'x');
  assert.equal(parseArgs(['repl']).cmd, 'repl');
  assert.equal(parseArgs(['sessions']).cmd, 'sessions');
});

test('parseArgs + buildPrompt：存在的文件作为上下文', () => {
  const dir = mkdtempSync(join(tmpdir(), 'openaide-args-'));
  const f = join(dir, 'a.txt');
  writeFileSync(f, 'file content');
  try {
    const r = parseArgs([f, '总结它']);
    assert.deepEqual(r.contextFiles, [f]);
    assert.equal(r.prompt, '总结它');
    const prompt = buildPrompt(r.contextFiles, r.prompt);
    assert.ok(prompt.includes('file content'), 'prompt 应包含文件内容');
    assert.ok(prompt.includes('总结它'), 'prompt 应包含用户问题');
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('buildPrompt：无文件时原样返回 prompt', () => {
  assert.equal(buildPrompt([], 'just a question'), 'just a question');
});
