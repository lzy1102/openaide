/**
 * 工作区边界验收 —— git 仓库边界即项目边界：
 * 嵌套仓库不越界、子目录归位、已有工作区复用、外层 .openaide 不穿透内层仓库。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { resolveProjectWorkspace } from '@openaide/config';

const GIT = ['-c', 'user.email=t@t', '-c', 'user.name=t'];
function gitRepo(parent: string, name: string): string {
  const dir = join(parent, name);
  mkdirSync(dir, { recursive: true });
  execFileSync('git', ['-C', dir, ...GIT, 'init'], { stdio: 'ignore' });
  return dir;
}
const newRoot = (): string => mkdtempSync(join(tmpdir(), 'openaide-boundary-'));

test('嵌套仓库：内层仓库的工作区不越界到外层', () => {
  const root = newRoot();
  try {
    const outer = gitRepo(root, 'outer');
    mkdirSync(join(outer, '.openaide'), { recursive: true }); // 外层已有工作区
    const inner = gitRepo(outer, 'inner');

    const ws = resolveProjectWorkspace(join(inner, 'src'));
    assert.equal(ws, join(inner, '.openaide'), '内层仓库自建工作区，绝不复用外层的');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('子目录启动：归位到所在仓库根的工作区', () => {
  const root = newRoot();
  try {
    const repo = gitRepo(root, 'repo');
    mkdirSync(join(repo, 'src', 'deep'), { recursive: true });
    const ws = resolveProjectWorkspace(join(repo, 'src', 'deep'));
    assert.equal(ws, join(repo, '.openaide'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('仓库根已有 .openaide → 子目录启动复用而非新建', () => {
  const root = newRoot();
  try {
    const repo = gitRepo(root, 'repo');
    mkdirSync(join(repo, '.openaide'), { recursive: true });
    const ws = resolveProjectWorkspace(join(repo, 'src'));
    assert.equal(ws, join(repo, '.openaide'));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test('外层 .openaide 不穿透内层 git 仓库（dogfood 越界场景回归）', () => {
  const root = newRoot();
  try {
    // 复刻事故现场：开发仓库根有 .openaide，游戏项目是内层独立 git 仓库
    const devRepo = gitRepo(root, 'dev-repo');
    mkdirSync(join(devRepo, '.openaide'), { recursive: true });
    const game = join(devRepo, 'games', 'watermelon');
    mkdirSync(game, { recursive: true });
    execFileSync('git', ['-C', game, ...GIT, 'init'], { stdio: 'ignore' });

    const ws = resolveProjectWorkspace(game);
    assert.equal(ws, join(game, '.openaide'), '游戏项目应有自己的工作区');
    assert.ok(existsSync(join(devRepo, '.openaide')), '外层工作区原样保留、互不污染');
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
