/**
 * 会话存储 —— 第一阶段内存实现。
 * 唯一 ID 生成：纳秒时间戳 + 递增序号，避免 Go 版时间戳冲突 bug。
 */
import { Message, Session } from './types.js';
import type { SessionStore } from './interfaces.js';

let seq = 0;
const HEX = '0123456789abcdef';

/** 生成全局唯一会话/消息 ID（16 位 hex） */
export function newId(): string {
  const n = (BigInt(Date.now()) * 1000n + BigInt(++seq)) & 0xffffffffffffffffn;
  let s = '';
  let v = n;
  for (let i = 0; i < 16; i++) {
    s = HEX[Number(v & 0xfn)] + s;
    v >>= 4n;
  }
  return s;
}

/** 内存会话存储（进程内，满足 SessionStore 契约） */
export class MemorySessionStore implements SessionStore {
  private sessions = new Map<string, Session>();

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
    this.sessions.set(session.id, session);
    return session;
  }

  async get(sessionId: string): Promise<Session | undefined> {
    return this.sessions.get(sessionId);
  }

  async update(session: Session): Promise<void> {
    session.updatedAt = Date.now();
    this.sessions.set(session.id, session);
  }

  async list(projectId?: string): Promise<Session[]> {
    const all = [...this.sessions.values()].sort((a, b) => b.updatedAt - a.updatedAt);
    return projectId ? all.filter((s) => s.projectId === projectId) : all;
  }

  async delete(sessionId: string): Promise<void> {
    this.sessions.delete(sessionId);
  }
}

/** 把消息追加进会话并持久化（会话内消息与 memory 双写由内核统一处理） */
export function appendMessages(session: Session, messages: Message[]): void {
  session.messages.push(...messages);
  if (session.messages.length > 500) {
    session.messages = session.messages.slice(-500);
  }
}
