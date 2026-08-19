/**
 * OpenAIDE 安装脚本 —— 跨平台（Windows / macOS / Linux）。
 * 用法：
 *   node scripts/install.mjs          # npm install + 全局 link openaide 命令
 *   node scripts/install.mjs --dev    # 仅 npm install（开发模式，不 link 全局命令）
 */
import { spawnSync } from 'node:child_process';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const devOnly = process.argv.includes('--dev');

function run(cmd, args, opts = {}) {
  const res = spawnSync(cmd, args, { cwd: root, stdio: 'inherit', shell: process.platform === 'win32', ...opts });
  if (res.status !== 0) {
    console.error(`[install] failed: ${cmd} ${args.join(' ')}`);
    process.exit(res.status ?? 1);
  }
  return res;
}

console.log('[install] installing dependencies...');
run('npm', ['install']);

if (devOnly) {
  console.log('[install] dev mode done. Run `npm run dev` to start.');
  process.exit(0);
}

console.log('[install] linking global `openaide` command...');
run('npm', ['link', '--workspace', '@openaide/cli']);

console.log('\n[install] done. `openaide` is available globally.');
console.log('  openaide                interactive REPL');
console.log('  openaide --version      version');
console.log('  openaide --help         help');
console.log('\nConfig: edit ~/.openaide/config.yaml (or run `openaide setup`).');