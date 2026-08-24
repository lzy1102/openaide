/**
 * UI 选择器验收 —— 显式指定优先、TTY 默认链、缺失回退告警。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chooseUi } from '../src/ui-select.js';

test('显式指定存在 → 直接命中', () => {
  const r = chooseUi({ available: ['ink', 'readline', 'mini'], wanted: 'mini', isTTY: true });
  assert.equal(r.name, 'mini');
  assert.equal(r.warning, undefined);
});

test('无指定：TTY 偏好 ink，非 TTY 偏好 readline', () => {
  assert.equal(chooseUi({ available: ['ink', 'readline'], isTTY: true }).name, 'ink');
  assert.equal(chooseUi({ available: ['ink', 'readline'], isTTY: false }).name, 'readline');
});

test('默认偏好缺席时沿回退链取可用项', () => {
  // 用户禁用了两个内置 UI，只装了自己的
  const r = chooseUi({ available: ['fancy'], isTTY: true });
  assert.equal(r.name, 'fancy');
});

test('显式指定但未注册 → 回退并给出告警', () => {
  const r = chooseUi({ available: ['ink'], wanted: 'ghost', isTTY: false });
  assert.equal(r.name, 'ink');
  assert.match(r.warning ?? '', /'ghost' not registered/);
});

test('什么都没有 → null（调用方报错）', () => {
  assert.equal(chooseUi({ available: [], isTTY: true }).name, null);
});
