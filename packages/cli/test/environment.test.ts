/**
 * 平台附注测试:
 *  - Windows 附注：引导脚本优先 + 专用工具 + 列出检测到的运行时
 *  - POSIX 附注：仅运行时清单
 *  - 拦截器：[Platform] 插入首条 system 之后，字节级稳定
 */
import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { buildPlatformNote, createEnvironmentInterceptor, detectRuntimes } from '../src/environment.js';
import type { Message } from '@openaide/core';

describe('buildPlatformNote', () => {
  const runtimes = [
    { cmd: 'python', version: 'Python 3.12.4' },
    { cmd: 'node', version: 'v22.4.0' },
  ];

  test('windows note steers to scripts and dedicated tools', async () => {
    const note = await buildPlatformNote('win32', runtimes);
    assert.ok(note.startsWith('[Platform]'));
    assert.ok(note.includes('search_files'), 'should steer text search to search_files');
    assert.ok(note.includes('write_file'), 'should steer file writes to write_file');
    assert.ok(note.includes('脚本'), 'should prefer scripts over shell one-liners');
    assert.ok(note.includes('不要用 powershell'), 'must steer away from PowerShell');
    assert.ok(note.includes('python (Python 3.12.4)'));
    assert.ok(note.includes('node (v22.4.0)'));
  });

  test('windows note without runtimes still guides tool usage', async () => {
    const note = await buildPlatformNote('win32', []);
    assert.ok(note.includes('未检测到'));
  });

  test('posix note is a plain runtime list', async () => {
    const note = await buildPlatformNote('linux', runtimes);
    assert.ok(note.includes('[Platform]'));
    assert.ok(!note.includes('cmd.exe'), 'no Windows-specific rules on POSIX');
  });
});

describe('createEnvironmentInterceptor', () => {
  test('injects [Platform] after first system message, byte-stable across calls', async () => {
    const ix = createEnvironmentInterceptor();
    const messages: Message[] = [
      { role: 'system', content: 'persona rules' },
      { role: 'user', content: 'hi' },
    ];

    const v1 = await ix.beforeLLM!({ sessionId: 's', messages });
    assert.equal(v1.action, 'modify');
    const out1 = v1.payload as Message[];
    assert.equal(out1[0].content, 'persona rules');
    assert.ok(out1[1].content.startsWith('[Platform]'), 'note lands right after system');
    assert.equal(out1[2].content, 'hi');

    const v2 = await ix.beforeLLM!({ sessionId: 's', messages });
    assert.equal((v2.payload as Message[])[1].content, out1[1].content, 'identical bytes each turn');
  });

  test('inserts at head when no system message exists', async () => {
    const ix = createEnvironmentInterceptor();
    const v = await ix.beforeLLM!({
      sessionId: 's',
      messages: [{ role: 'user', content: 'hi' }],
    });
    assert.ok((v.payload as Message[])[0].content.startsWith('[Platform]'));
  });
});

describe('detectRuntimes', () => {
  test('finds at least node in test environment, dedupes python entries', async () => {
    const runtimes = await detectRuntimes();
    assert.ok(runtimes.some((r) => r.cmd === 'node'), 'node runs the tests, must be found');
    // python/python3/py 同源只保留一个
    const pyEntries = runtimes.filter((r) => ['python', 'python3', 'py'].includes(r.cmd));
    assert.ok(pyEntries.length <= 1);
  });
});
