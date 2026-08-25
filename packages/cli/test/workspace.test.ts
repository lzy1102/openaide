/**
 * 同步与团队知识注入验收 —— commit 身份注入、noop 幂等、knowledge L1 注入。
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createKnowledgeInterceptor, syncSessions } from '../src/workspace.js';

const GIT = ['-c', 'user.email=t@t', '-c', 'user.name=t'];
function makeGitRepo(): string {
  const repo = mkdtempSync(join(tmpdir(), 'openaide-sync-'));
  execFileSync('git', ['-C', repo, ...GIT, 'init'], { stdio: 'ignore' });
  writeFileSync(join(repo, 'README.md'), 'demo\n');
  execFileSync('git', ['-C', repo, ...GIT, 'add', 'README.md'], { stdio: 'ignore' });
  execFileSync('git', ['-C', repo, ...GIT, 'commit', '-m', 'init'], { stdio: 'ignore' });
  return repo;
}

test('syncSessions：提交 .openaide 时注入兜底身份；无变化 noop；off 直接跳过', () => {
  const repo = makeGitRepo();
  const noCfg = join(repo, 'empty-gitconfig');
    writeFileSync(noCfg, '');
    process.env.GIT_CONFIG_GLOBAL = noCfg; // 模拟"没配过 user.name"
  try {
    const ws = join(repo, '.openaide');
    mkdirSync(join(ws, 'sessions', 'dev-pc'), { recursive: true });
    writeFileSync(join(ws, 'sessions', 'dev-pc', 's.json'), '{}');

    const r1 = syncSessions(ws, { name: 'dev-pc' }, 'commit');
    assert.equal(r1.status, 'synced');

    const log = execFileSync('git', ['-C', repo, 'log', '-1', '--format=%an %s'], { encoding: 'utf8' });
    assert.match(log.trim(), /^dev-pc chore\(openaide\): sync sessions$/);

    assert.equal(syncSessions(ws, { name: 'dev-pc' }, 'commit').status, 'noop');
    writeFileSync(join(ws, 'sessions', 'dev-pc', 's2.json'), '{}');
    assert.equal(syncSessions(ws, { name: 'dev-pc' }, 'off').status, 'noop', 'off 不产生任何 git 动作');
  } finally {
    delete process.env.GIT_CONFIG_GLOBAL;
    rmSync(repo, { recursive: true, force: true });
  }
});

test('knowledge 注入：有文件 modify 插入 [ProjectKnowledge]；空目录 allow', () => {
  const ws = mkdtempSync(join(tmpdir(), 'openaide-kn-'));
  try {
    const ix = createKnowledgeInterceptor(ws);
    const msgs = [
      { role: 'system' as const, content: 'base L0' },
      { role: 'user' as const, content: 'hi' },
    ];
    assert.equal(ix.beforeLLM!({ sessionId: 's', messages: [...msgs] }).action, 'allow');

    mkdirSync(join(ws, 'knowledge'), { recursive: true });
    writeFileSync(join(ws, 'knowledge', 'PROJECT.md'), '# 约定\n使用 TypeScript');
    const v = ix.beforeLLM!({ sessionId: 's', messages: [...msgs] });
    assert.equal(v.action, 'modify');
    const out = v.payload!;
    assert.equal(out.length, 3);
    assert.match(String(out[1]?.content), /\[ProjectKnowledge\][\s\S]*使用 TypeScript/);
    assert.equal(out[2]?.role, 'user');
  } finally {
    rmSync(ws, { recursive: true, force: true });
  }
});
