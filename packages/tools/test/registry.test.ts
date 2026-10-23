/**
 * ToolRegistry 危险标记 —— 标记与 ToolDefinition 分离：
 * 危险标记若塞进 function 对象，会被原样发给 LLM（provider 直接透传 tools），
 * 部分网关会拒收未知字段。故单独存放，经 isDangerous() 查询。
 */
import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { ToolRegistry } from '../src/registry.js';
import type { ToolDefinition } from '@openaide/core';

function def(name: string): ToolDefinition {
  return { type: 'function', function: { name, description: name, parameters: { type: 'object' } } };
}

const noop = async () => ({ content: '' });

describe('ToolRegistry dangerous flag', () => {
  test('defaults to safe', () => {
    const reg = new ToolRegistry();
    reg.register(def('a__safe'), noop);
    assert.equal(reg.isDangerous('a__safe'), false);
  });

  test('marks dangerous tools via register options', () => {
    const reg = new ToolRegistry();
    reg.register(def('a__rm'), noop, { dangerous: true });
    assert.equal(reg.isDangerous('a__rm'), true);
  });

  test('unregister clears the flag', () => {
    const reg = new ToolRegistry();
    reg.register(def('a__rm'), noop, { dangerous: true });
    reg.unregister('a__rm');
    assert.equal(reg.isDangerous('a__rm'), false);
  });

  test('dangerous flag never leaks into the LLM-facing definition', () => {
    const reg = new ToolRegistry();
    reg.register(def('a__rm'), noop, { dangerous: true });
    const [d] = reg.definitions();
    assert.ok(d);
    assert.deepEqual(Object.keys(d.function).sort(), ['description', 'name', 'parameters']);
  });
});
