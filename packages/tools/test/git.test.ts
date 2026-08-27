import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { gitPlugin } from '../src/git.js';

function safeRm(dir: string) {
  try { rmSync(dir, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 } as any); } catch {}
}

function makeRepo(): string {
  const dir = mkdtempSync(join(tmpdir(), 'openaide-git-test-'));
  execFileSync('git', ['init'], { cwd: dir });
  execFileSync('git', ['config', 'user.email', 'test@test.com'], { cwd: dir });
  execFileSync('git', ['config', 'user.name', 'Test'], { cwd: dir });
  writeFileSync(join(dir, 'a.txt'), 'hello\n');
  execFileSync('git', ['add', '.'], { cwd: dir });
  execFileSync('git', ['commit', '-m', 'init'], { cwd: dir });
  return dir;
}

test('git_status returns branch and files', async () => {
  const dir = makeRepo();
  try {
    const handler = gitPlugin.tools.find(t => t.name === 'git_status')!.handler;
    let r = await handler({ cwd: dir }, 's1');
    const data = JSON.parse(r.content as string);
    assert.ok(data.branch);
    assert.equal(data.files.length, 0);

    writeFileSync(join(dir, 'b.txt'), 'new\n');
    r = await handler({ cwd: dir }, 's1');
    const data2 = JSON.parse(r.content as string);
    assert.equal(data2.files[0].path, 'b.txt');
    assert.equal(data2.files[0].status, 'untracked');
  } finally {
    safeRm(dir);
  }
});

test('git_diff returns diff content', async () => {
  const dir = makeRepo();
  try {
    writeFileSync(join(dir, 'a.txt'), 'hello world\n');
    const handler = gitPlugin.tools.find(t => t.name === 'git_diff')!.handler;
    const r = await handler({ cwd: dir }, 's1');
    const data = JSON.parse(r.content as string);
    assert.match(data.diff, /hello world/);
  } finally {
    safeRm(dir);
  }
});

test('git_log returns commits', async () => {
  const dir = makeRepo();
  try {
    const handler = gitPlugin.tools.find(t => t.name === 'git_log')!.handler;
    const r = await handler({ cwd: dir, count: 5 }, 's1');
    const data = JSON.parse(r.content as string);
    assert.equal(data.commits.length, 1);
    assert.match(data.commits[0].message, /init/);
  } finally {
    safeRm(dir);
  }
});

test('not a git repo returns NOT_FOUND', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'openaide-git-norepo-'));
  try {
    const handler = gitPlugin.tools.find(t => t.name === 'git_status')!.handler;
    const r = await handler({ cwd: dir }, 's1');
    assert.equal(r.errorCode, 'NOT_FOUND');
  } finally {
    safeRm(dir);
  }
});
