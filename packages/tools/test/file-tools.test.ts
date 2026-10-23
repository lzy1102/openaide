/**
 * file-tools 验收 —— 三个曾经真实踩过的坑：
 *  1. diff_edit 用 String.replace 做替换，replacement 里的 $& / $1 / $` 会被当特殊模式展开，
 *     改模板串/正则/shell 变量时内容被静默改坏。
 *  2. Windows 文件是 CRLF，模型给的是 LF —— 不兼容时每次编辑都报 not found。
 *  3. grep_search 跳过所有点开头条目，.env / .gitignore / .github 永远搜不到。
 */
import { test, describe, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileToolsPlugin } from '../src/file-tools.js';

const tools = fileToolsPlugin.tools ?? [];
const grep = tools.find((t) => t.name === 'grep_search')!.handler;
const edit = tools.find((t) => t.name === 'diff_edit')!.handler;

describe('diff_edit', () => {
  let dir: string;
  before(() => {
    dir = mkdtempSync(join(tmpdir(), 'openaide-edit-'));
  });
  after(() => rmSync(dir, { recursive: true, force: true }));

  test('replacement 里的 $& / $1 / $` 原样写入，不被当替换模式', async () => {
    const f = join(dir, 'dollar.ts');
    writeFileSync(f, 'const a = 1;\n');
    const replacement = 'const a = $& + "$1" + `$x`;';
    const r = await edit({ file_path: f, search: 'const a = 1;', replace: replacement });
    assert.ok(!r.error, r.error);
    assert.equal(readFileSync(f, 'utf8'), `${replacement}\n`);
  });

  test('LF 的 search 块能匹配 CRLF 文件', async () => {
    const f = join(dir, 'crlf.txt');
    writeFileSync(f, 'line1\r\nline2\r\n');
    const r = await edit({ file_path: f, search: 'line1\nline2', replace: 'x\ny' });
    assert.ok(!r.error, r.error);
    assert.equal(readFileSync(f, 'utf8'), 'x\r\ny\r\n');
  });

  test('CRLF 的 search 块能匹配 LF 文件', async () => {
    const f = join(dir, 'lf.txt');
    writeFileSync(f, 'line1\nline2\n');
    const r = await edit({ file_path: f, search: 'line1\r\nline2', replace: 'x\r\ny' });
    assert.ok(!r.error, r.error);
    assert.equal(readFileSync(f, 'utf8'), 'x\ny\n');
  });

  test('未命中与歧义分别给出可区分错误码', async () => {
    const f = join(dir, 'amb.txt');
    writeFileSync(f, 'dup\ndup\n');
    const miss = await edit({ file_path: f, search: 'nope', replace: 'x' });
    assert.equal(miss.errorCode, 'NOT_FOUND');
    const amb = await edit({ file_path: f, search: 'dup', replace: 'x' });
    assert.equal(amb.errorCode, 'AMBIGUOUS');
    assert.equal(readFileSync(f, 'utf8'), 'dup\ndup\n', '歧义时不得改写文件');
  });
});

describe('grep_search', () => {
  let dir: string;
  before(() => {
    dir = mkdtempSync(join(tmpdir(), 'openaide-grep-'));
    writeFileSync(join(dir, '.env'), 'API_KEY=secret\n');
    writeFileSync(join(dir, '.gitignore'), 'node_modules\n');
    writeFileSync(join(dir, 'app.ts'), 'const marker = 1;\n');
    writeFileSync(join(dir, 'app.test.ts'), 'const marker = 2;\n');
    mkdirSync(join(dir, 'src'));
    writeFileSync(join(dir, 'src', 'deep.ts'), 'const marker = 3;\n');
    mkdirSync(join(dir, 'node_modules'));
    writeFileSync(join(dir, 'node_modules', 'noise.js'), 'const marker = 4;\n');
  });
  after(() => rmSync(dir, { recursive: true, force: true }));

  test('点开头的文件不再是盲区', async () => {
    const r = await grep({ pattern: 'API_KEY', path: dir });
    assert.match(String(r.content), /\.env:1:/);
  });

  test('include 支持 ** 与子目录', async () => {
    const r = await grep({ pattern: 'marker', path: dir, include: '**/*.ts' });
    const text = String(r.content);
    assert.match(text, /app\.ts/);
    assert.match(text, /src\/deep\.ts|src\\deep\.ts/);
    assert.ok(!text.includes('noise.js'), 'node_modules 应被跳过');
  });

  test('include 为 *.test.ts 时只匹配测试文件', async () => {
    const r = await grep({ pattern: 'marker', path: dir, include: '*.test.ts' });
    const text = String(r.content);
    assert.match(text, /app\.test\.ts/);
    assert.ok(!text.includes('app.ts:'), 'app.ts 不应命中');
  });

  test('无匹配时返回明确提示', async () => {
    const r = await grep({ pattern: 'no-such-symbol-xyz', path: dir });
    assert.equal(r.content, '(no matches)');
  });
});
