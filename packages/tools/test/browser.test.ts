/**
 * browser 插件验收 —— 工具清单完整性；未安装 playwright 时的可操作指引。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { browserPlugin } from '../src/browser.js';

test('工具清单完整且命名规范', () => {
  const names = browserPlugin.tools!.map((t) => t.name);
  assert.deepEqual(names, [
    'browser_navigate',
    'browser_snapshot',
    'browser_click',
    'browser_type',
    'browser_search',
    'browser_close',
  ]);
  for (const t of browserPlugin.tools!) {
    assert.equal(t.parameters.type, 'object');
    assert.ok(t.description.length > 5);
  }
});

test('未安装 playwright → navigate 返回安装指引错误而非抛异常', async () => {
  const nav = browserPlugin.tools!.find((t) => t.name === 'browser_navigate')!;
  const close = browserPlugin.tools!.find((t) => t.name === 'browser_close')!;
  try {
    const r = await nav.handler({ url: 'https://example.com' }, 's1');
  // 本机若装了 playwright 则会真实导航——两种结果都合法
    if (typeof r.error === 'string') {
    assert.match(r.error, /playwright is not installed|browsers not downloaded/);
    assert.match(r.error, /npx playwright install chromium/);
    } else {
      assert.ok(String(r.content).length > 0, '已装 playwright 时应返回页面快照');
    }
  } finally {
    await close.handler({}, 's1');
  }
});
