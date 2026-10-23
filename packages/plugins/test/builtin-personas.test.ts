/**
 * 内置人格验收 —— coder 的操作原则必须是"可回滚但绝不代替用户提交"。
 *
 * 历史坑:旧提示词要求 agent 编辑前执行 `git add -A && git commit -m "wip: ..."`。
 * 后果:(1) 在全仓库范围制造无意义的提交;(2) `add -A` 会把未跟踪的密钥/垃圾一并
 * 收进历史;(3) 与同文件"先问再做""不擅自做不可逆操作"自相矛盾。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { builtinPersonasPlugin } from '../src/builtin-personas.js';

function coderPrompt(): string {
  const coder = builtinPersonasPlugin.personas?.find((p) => p.name === 'coder');
  assert.ok(coder, 'coder persona must exist');
  return coder.systemPrompt;
}

test('coder 不再指示自动提交', () => {
  const p = coderPrompt();
  assert.ok(!p.includes('git add -A && git commit'), 'must not auto-commit on the user behalf');
  assert.ok(!p.includes('wip: before openaide edits'));
});

test('coder 明确禁止未授权的 git 写操作', () => {
  const p = coderPrompt();
  assert.match(p, /never commit on the user'?s behalf/i);
  assert.match(p, /git status/);
  assert.match(p, /unless explicitly asked/i);
  assert.match(p, /git add -A/);
});

test('架构师人格保持只读评审定位', () => {
  const architect = builtinPersonasPlugin.personas?.find((p) => p.name === 'architect');
  assert.ok(architect);
  assert.match(architect.systemPrompt, /architect/i);
});
