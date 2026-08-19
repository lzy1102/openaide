/**
 * 跨平台测试运行器 —— 递归收集 packages 各包 test 目录下的 .test.ts 并交给 node --test。
 * 背景：Node 18 的 --test 不展开 glob，也不按默认规则发现 .ts 文件。
 * 用法：
 *   node scripts/test.mjs              # 运行所有包测试
 *   node scripts/test.mjs core         # 只运行 packages/core 的测试
 */
import { spawnSync } from 'node:child_process';
import { readdirSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const scope = process.argv[2];

/** 递归收集 *.test.ts */
function collect(dir, files = []) {
  let entries;
  try {
    entries = readdirSync(dir);
  } catch {
    return files;
  }
  for (const e of entries) {
    if (e.startsWith('.') || e === 'node_modules') continue;
    const full = join(dir, e);
    if (statSync(full).isDirectory()) collect(full, files);
    else if (e.endsWith('.test.ts')) files.push(full);
  }
  return files;
}

const base = scope ? join(root, 'packages', scope, 'test') : join(root, 'packages');
const files = collect(base);

if (files.length === 0) {
  console.log(`[test] no test files found under ${base}`);
  process.exit(0);
}

console.log(`[test] running ${files.length} test file(s)...`);
const res = spawnSync(process.execPath, ['--import', 'tsx', '--test', ...files], {
  stdio: 'inherit',
  cwd: root,
});
process.exit(res.status ?? 1);
