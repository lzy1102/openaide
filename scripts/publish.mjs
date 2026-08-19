/**
 * OpenAIDE 发布脚本 —— 按依赖顺序发布全部 @openaide/* 包并打 tag。
 * 用法：
 *   node scripts/publish.mjs                # 发布 + 打 git tag（tag = v<version>）
 *   node scripts/publish.mjs --no-tag       # 仅发布，不打 tag（CI 用，避免递归触发）
 *   node scripts/publish.mjs --tag=v0.3.0   # 发布 + 指定 tag
 *
 * 前提：已登录 npm（npm login），有 @openaide scope 发布权限。
 * 包为"源码形态"：main/exports 指向 src/*.ts，cli 的 bin 经 tsx 运行时加载，无需预编译。
 */
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));

/** 发布顺序：依赖在前的先发 */
const ORDER = [
  'core',
  'config',
  'llm',
  'plugins',
  'memory',
  'tools',
  'api',
  'cli',
];

function run(cmd, args, opts = {}) {
  const res = spawnSync(cmd, args, { cwd: root, stdio: 'inherit', shell: process.platform === 'win32', ...opts });
  if (res.status !== 0) process.exit(res.status ?? 1);
}

// 校验版本一致性（避免部分包版本不一致导致解析问题）
const versions = new Set(ORDER.map((p) => JSON.parse(readFileSync(resolve(root, 'packages', p, 'package.json'), 'utf8')).version));
if (versions.size > 1) {
  console.error(`[publish] inconsistent versions across packages: ${[...versions].join(', ')}`);
  process.exit(1);
}
const version = [...versions][0];

console.log(`[publish] publishing all packages @ ${version} in order:`);
for (const p of ORDER) {
  console.log(`  → @openaide/${p}@${version}`);
  run('npm', ['publish', '--workspace', `@openaide/${p}`, '--access', 'public']);
}
console.log(`[publish] all packages published: @openaide/*@${version}`);

// 可选：打 git tag（--no-tag 跳过，供 CI 使用）
if (process.argv.includes('--no-tag')) {
  console.log('[publish] skipped git tag (--no-tag)');
} else {
  const tagArg = process.argv.find((a) => a.startsWith('--tag='));
  const tag = tagArg ? tagArg.slice('--tag='.length) : `v${version}`;
  console.log(`[publish] creating git tag ${tag}...`);
  run('git', ['tag', tag]);
  run('git', ['push', 'origin', tag]);
}

console.log('\n[done] Users can now install globally:');
console.log('  npm install -g @openaide/cli');