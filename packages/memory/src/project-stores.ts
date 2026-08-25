/**
 * 项目存储布局 —— 工作区 + 身份 → 具体目录，并负责历史布局迁移。
 *
 * 目录形态：
 *   <workspace>/sessions/<identity>/*.json   按人隔离的会话快照
 *   <workspace>/memory/<identity>/*.jsonl    按人隔离的记忆流水
 *   <workspace>/knowledge/*.md               团队共享知识（进 git）
 *   <workspace>/identity                     随机兜底身份的持久化
 *
 * 迁移规则（resolveStores 内自动执行）：
 *  1. 旧版平铺：<workspace>/sessions/*.json → sessions/<identity>/
 *  2. 身份变更：sessions/<old>/ 有内容、<new>/ 不存在且其余目录 ≤0 → 改名收敛
 *     （多人同仓时各自身份目录并存，绝不互相吞并）
 */
import { existsSync, mkdirSync, readdirSync, renameSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { FileMemory, FileSessionStore } from './file-store.js';

export interface ProjectStores {
  workspace: string;
  identity: string;
  sessions: FileSessionStore;
  memory: FileMemory;
  /** 团队共享知识目录（可能不存在） */
  knowledgeDir: string;
}

export function resolveStores(
  workspace: string,
  identity: string,
): ProjectStores {
  const sessionsRoot = join(workspace, 'sessions');
  const memoryRoot = join(workspace, 'memory');
  const targetSessions = join(sessionsRoot, identity);
  const targetMemory = join(memoryRoot, identity);

  // ── 迁移 1：旧版平铺（sessions/ 下直接是 *.json）──
  if (existsSync(sessionsRoot) && !existsSync(join(sessionsRoot, identity))) {
    const flat = readdirSync(sessionsRoot).filter((n) => n.endsWith('.json'));
    if (flat.length > 0 && !existsSync(targetSessions)) {
      mkdirSync(targetSessions, { recursive: true });
      for (const f of flat) {
        try {
          renameSync(join(sessionsRoot, f), join(targetSessions, f));
        } catch {
          /* 单文件迁移失败不阻塞启动 */
        }
      }
    }
  }
  if (existsSync(memoryRoot) && !existsSync(join(memoryRoot, identity))) {
    const flat = readdirSync(memoryRoot).filter((n) => n.endsWith('.jsonl'));
    if (flat.length > 0 && !existsSync(targetMemory)) {
      mkdirSync(targetMemory, { recursive: true });
      for (const f of flat) {
        try {
          renameSync(join(memoryRoot, f), join(targetMemory, f));
        } catch {
          /* 同上 */
        }
      }
    }
  }

  // ── 迁移 2：身份收敛（旧身份目录唯一且有内容、新目录缺失 → 改名）──
  if (existsSync(sessionsRoot)) {
    const others = readdirSync(sessionsRoot).filter(
      (n) => n !== identity && !n.endsWith('.json') && existsSync(join(sessionsRoot, n)),
    );
    if (others.length === 1 && !existsSync(targetSessions)) {
      try {
        renameSync(join(sessionsRoot, others[0]!), targetSessions);
        const oldMem = join(memoryRoot, others[0]!);
        if (existsSync(oldMem)) renameSync(oldMem, targetMemory);
      } catch {
        /* 迁移失败按新建处理 */
      }
    }
  }

  mkdirSync(targetSessions, { recursive: true });
  mkdirSync(targetMemory, { recursive: true });

  const sessions = new FileSessionStore(targetSessions);
  const memory = new FileMemory(targetMemory);
  sessions.onDelete = (sid) => {
    rmSync(join(targetMemory, `${sid}.jsonl`), { force: true });
  };

  return {
    workspace,
    identity,
    sessions,
    memory,
    knowledgeDir: join(workspace, 'knowledge'),
  };
}
