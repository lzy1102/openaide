/**
 * Scope（资源作用域）验收 —— 通用加载/卸载账本：
 * 分类清单、逆序回收、单项失败隔离、释放后可复用。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Scope } from '../src/scope.js';

test('names/count 按类别登记与查询', () => {
  const s = new Scope();
  s.add('tool', 'a__x', () => {});
  s.add('tool', 'a__y', () => {});
  s.add('persona', 'coder', () => {});
  s.add('hook', null, () => {}); // 匿名资源不计入 names
  assert.deepEqual(s.names('tool'), ['a__x', 'a__y']);
  assert.deepEqual(s.names('persona'), ['coder']);
  assert.equal(s.count('hook'), 1);
  assert.equal(s.count('nope'), 0);
});

test('dispose 逆序执行；单项抛错不阻断其余', () => {
  const order: string[] = [];
  const s = new Scope();
  s.add('t', 'first', () => order.push('first'));
  s.add('t', 'boom', () => {
    order.push('boom');
    throw new Error('release failed');
  });
  s.add('t', 'last', () => order.push('last'));

  const errors: string[] = [];
  s.dispose((err, item) => errors.push(`${item.name}:${(err as Error).message}`));

  assert.deepEqual(order, ['last', 'boom', 'first'], '后登记的先回收');
  assert.equal(errors.length, 1);
  assert.match(errors[0]!, /^boom:release failed$/);
});

test('dispose 后作用域清空、可复用；重复 dispose 安全', () => {
  let n = 0;
  const s = new Scope();
  s.add('r', null, () => n++);
  s.dispose();
  assert.equal(n, 1);
  s.dispose(); // 再释放无资源、无异常
  s.add('r', 'again', () => n++);
  s.dispose();
  assert.equal(n, 2);
});
