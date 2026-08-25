/**
 * 会话同步与团队知识 —— 多人协作的两个支撑件。
 *
 * - createKnowledgeInterceptor(): 把 <workspace>/knowledge/*.md 注入 L1 上下文
 *   （团队共享的"项目约定"，对标 CLAUDE.md；走拦截链，零内核改动）
 * - syncSessions(): 按 sessionSync 策略自动提交/推送 .openaide/。
 *   身份未配置时用 -c 临时注入解析出的兜底身份，本地提交永远成功；
 *   pathspec 只限 .openaide，绝不触碰用户的工作区暂存。
 */
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import type { Interceptor } from '@openaide/core';

export type SyncMode = 'off' | 'commit' | 'push';
export type SyncResult = { status: 'synced'; committed: boolean; pushed: boolean } | { status: 'noop' } | { status: 'failed'; reason: string };

function git(cwd: string, args: string[], identity?: { name: string; email?: string }): string {
  const finalArgs = identity
    ? ['-c', `user.name=${identity.name}`, '-c', `user.email=${identity.email ?? `${identity.name}@openaide.local`}`, ...args]
    : args;
  return execFileSync('git', finalArgs, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
}

export function isGitRepo(dir: string): boolean {
  try {
    git(dir, ['rev-parse', '--is-inside-work-tree']);
    return true;
  } catch {
    return false;
  }
}

/** 同步 .openaide/ 到项目仓库。幂等：无变化即 noop。 */
export function syncSessions(
  workspace: string,
  identity: { name: string },
  mode: SyncMode,
): SyncResult {
  if (mode === 'off') return { status: 'noop' };
  const projectRoot = join(workspace, '..');
  try {
    if (!isGitRepo(projectRoot)) return { status: 'failed', reason: 'not a git repository' };
    const dirty = git(projectRoot, ['status', '--porcelain', '--', '.openaide']).trim().length > 0;
    if (!dirty && mode !== 'push') return { status: 'noop' };

    let committed = false;
    if (dirty) {
      git(projectRoot, ['add', '--', '.openaide'], { name: identity.name });
      git(projectRoot, ['commit', '-m', 'chore(openaide): sync sessions'], { name: identity.name });
      committed = true;
    }
    let pushed = false;
    if (mode === 'push') {
      // 无 upstream 时让 git 报错并向上返回 failed（不静默吞掉）
      git(projectRoot, ['push']);
      pushed = true;
    }
    return { status: 'synced', committed, pushed };
  } catch (err) {
    return { status: 'failed', reason: (err as Error).message.split('\n')[0] ?? String(err) };
  }
}

/** 知识目录内容（*.md 文件名 → 正文），目录不存在时为空 */
export function readKnowledge(knowledgeDir: string): Array<{ file: string; content: string }> {
  if (!existsSync(knowledgeDir)) return [];
  const out: Array<{ file: string; content: string }> = [];
  for (const f of readdirSync(knowledgeDir)) {
    const full = join(knowledgeDir, f);
    if (!f.endsWith('.md') || !statSync(full).isFile()) continue;
    try {
      out.push({ file: f, content: readFileSync(full, 'utf8').trim() });
    } catch {
      /* 跳过不可读文件 */
    }
  }
  return out.sort((a, b) => a.file.localeCompare(b.file));
}

/**
 * 团队知识注入拦截器：把 knowledge/*.md 作为 [ProjectKnowledge] 系统消息
 * 插入到首条 system 之后。按目录 mtime 缓存，文件未变不重复读盘。
 */
export function createKnowledgeInterceptor(workspace: string): Interceptor {
  const knowledgeDir = join(workspace, 'knowledge');
  let cacheMTime = -1;
  let cacheText = '';

  const refresh = (): void => {
    let mtime = 0;
    if (existsSync(knowledgeDir)) mtime = statSync(knowledgeDir).mtimeMs;
    if (mtime !== cacheMTime) {
      cacheMTime = mtime;
      const files = readKnowledge(knowledgeDir);
      cacheText =
        files.length > 0
          ? files.map((f) => `## ${f.file}\n${f.content}`).join('\n\n')
          : '';
    }
  };

  return {
    name: 'knowledge',
    beforeLLM(info) {
      refresh();
      if (!cacheText) return { action: 'allow' };
      const messages = [...info.messages];
      const sysIdx = messages.findIndex((m) => m.role === 'system');
      const knowledgeMsg = {
        role: 'system' as const,
        content: `[ProjectKnowledge]\n${cacheText}`,
      };
      if (sysIdx >= 0) messages.splice(sysIdx + 1, 0, knowledgeMsg);
      else messages.unshift(knowledgeMsg);
      return { action: 'modify', payload: messages };
    },
  };
}
