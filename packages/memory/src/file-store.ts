/**
 * 文件型存储 —— 会话随项目走（<workspace>/.openaide/ 内），git 同步即跨机器续聊。
 *
 * 布局：
 *   sessions/<sessionId>.json    会话整体快照（含全部消息，人类可读、可 diff）
 *   memory/<sessionId>.jsonl     逐条消息流水（append-only，内核跨轮历史来源）
 *
 * 设计取舍：放弃 SQLite 选文件，是为了 git 友好——文本可合并、按会话分文件把
 * 冲突面缩到单会话；updatedAt 写在内容里而非取文件 mtime，跨机克隆后排序仍正确。
 */
import { existsSync, mkdirSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import type { Message, Session, SessionStore } from '@openaide/core';
import { newId } from '@openaide/core';

/** 原子写：先写临时文件再改名，避免进程中断留下半个 JSON */
function atomicWrite(file: string, data: string): void {
  const tmp = `${file}.tmp`;
  writeFileSync(tmp, data, 'utf8');
  renameSync(tmp, file);
}

export class FileSessionStore implements SessionStore {
  private readonly dir: string;

  /**
   * @param sessionsDir 会话 JSON 的实际存放目录
   *   （项目工作区形态：<workspace>/sessions/<identity>/，按开发者隔离）
   */
  constructor(sessionsDir: string) {
    this.dir = sessionsDir;
    mkdirSync(this.dir, { recursive: true });
  }

  private fileOf(id: string): string {
    // id 由 newId() 生成（hex），仍做一次白名单防路径注入
    if (!/^[A-Za-z0-9_-]+$/.test(id)) throw new Error(`invalid session id: ${id}`);
    return join(this.dir, `${id}.json`);
  }

  async create(projectId: string, userId: string): Promise<Session> {
    const now = Date.now();
    const session: Session = {
      id: newId(),
      projectId,
      userId,
      messages: [],
      metadata: {},
      createdAt: now,
      updatedAt: now,
    };
    atomicWrite(this.fileOf(session.id), JSON.stringify(session, null, 2));
    return session;
  }

  async get(sessionId: string): Promise<Session | undefined> {
    const file = this.fileOf(sessionId);
    if (!existsSync(file)) return undefined;
    try {
      return JSON.parse(readFileSync(file, 'utf8')) as Session;
    } catch {
      return undefined; // 损坏文件视为不存在，不阻塞启动
    }
  }

  async update(session: Session): Promise<void> {
    session.updatedAt = Date.now();
    atomicWrite(this.fileOf(session.id), JSON.stringify(session, null, 2));
  }

  async list(projectId?: string): Promise<Session[]> {
    if (!existsSync(this.dir)) return [];
    const out: Session[] = [];
    for (const name of readdirSync(this.dir)) {
      if (!name.endsWith('.json') || name.endsWith('.tmp')) continue;
      try {
        out.push(JSON.parse(readFileSync(join(this.dir, name), 'utf8')) as Session);
      } catch {
        /* 跳过损坏文件 */
      }
    }
    // 按内容里的 updatedAt 排序（非 mtime）：克隆到新机器后顺序依然正确
    out.sort((a, b) => b.updatedAt - a.updatedAt);
    return projectId ? out.filter((s) => s.projectId === projectId) : out;
  }

  async delete(sessionId: string): Promise<void> {
    rmSync(this.fileOf(sessionId), { force: true });
    this.onDelete?.(sessionId);
  }

  /** 装配层注入：删除会话时联动清理记忆流水（目录布局由装配层决定） */
  onDelete?: (sessionId: string) => void;
}

/** 文件型记忆 —— 与 SqliteMemory 等价的 append-only 实现（kernel.Memory 契约） */
export class FileMemory {
  private readonly dir: string;

  /** @param memoryDir 记忆 JSONL 的实际存放目录（…/memory/<identity>/） */
  constructor(memoryDir: string) {
    this.dir = memoryDir;
    mkdirSync(this.dir, { recursive: true });
  }

  private fileOf(sessionId: string): string {
    if (!/^[A-Za-z0-9_-]+$/.test(sessionId)) throw new Error(`invalid session id: ${sessionId}`);
    return join(this.dir, `${sessionId}.jsonl`);
  }

  async save(sessionId: string, messages: Message[]): Promise<void> {
    if (messages.length === 0) return;
    let ts = Date.now();
    const lines = messages
      .filter((m) => (m.content ?? '').trim()) // 跳过空中间态（对齐 SqliteMemory）
      .map((m) => JSON.stringify({ role: m.role, content: m.content ?? '', ts: ts++ }));
    if (lines.length > 0) writeFileSync(this.fileOf(sessionId), lines.join('\n') + '\n', { flag: 'a', encoding: 'utf8' });
  }

  /** 撤回:删除会话记忆流水的末尾 n 条 */
  async truncateLast(sessionId: string, n: number): Promise<void> {
    const file = this.fileOf(sessionId);
    if (!existsSync(file) || n <= 0) return;
    const lines = readFileSync(file, 'utf8').split('\n').filter((l) => l.trim());
    const kept = lines.slice(0, Math.max(0, lines.length - n));
    writeFileSync(file, kept.length > 0 ? kept.join('\n') + '\n' : '', 'utf8');
  }

  async load(sessionId: string, limit: number): Promise<Message[]> {
    const file = this.fileOf(sessionId);
    if (!existsSync(file)) return [];
    const rows: Array<{ role: string; content: string }> = [];
    for (const line of readFileSync(file, 'utf8').split('\n')) {
      if (!line.trim()) continue;
      try {
        rows.push(JSON.parse(line));
      } catch {
        /* 忽略坏行 */
      }
    }
    // 文件本身按时间升序追加；取末尾 limit 条即为最近历史
    return (limit > 0 ? rows.slice(-limit) : rows).map((r) => ({
      role: r.role as Message['role'],
      content: r.content,
    }));
  }
}
