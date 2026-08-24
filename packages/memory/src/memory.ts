/**
 * SQLite 记忆存储 —— kernel.Memory 契约的实现。
 *
 * 独立于 sessions 表:内核用它构建跨轮历史(buildMessages 的 history 来源)。
 * 教训(Go 版审计):role 必须持久化——丢失后 Load 全部回退 assistant,
 * 会导致恢复会话时用户消息被当作回答渲染。
 */
import { openSqliteDatabase } from './sqlite-driver.js';
import type { SqliteDatabase, SqliteStatement } from './sqlite-driver.js';
import { mkdirSync } from 'node:fs';
import { dirname } from 'node:path';
import type { Memory, Message } from '@openaide/core';

const SCHEMA = `
CREATE TABLE IF NOT EXISTS memory_messages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  role       TEXT NOT NULL,
  content    TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_memory_session ON memory_messages(session_id, created_at);
`;

export class SqliteMemory implements Memory {
  private db: SqliteDatabase;

  constructor(dbPath: string) {
    if (dbPath !== ':memory:') mkdirSync(dirname(dbPath), { recursive: true });
    this.db = openSqliteDatabase(dbPath);
    this.db.exec('PRAGMA journal_mode = WAL;');
    this.db.exec(SCHEMA);
    this.insertStmt = this.db.prepare(
      `INSERT INTO memory_messages (session_id, role, content, created_at) VALUES (?, ?, ?, ?)`,
    );
    this.loadStmt = this.db.prepare(
      `SELECT role, content FROM (
         SELECT role, content, created_at FROM memory_messages
         WHERE session_id = ? ORDER BY created_at DESC LIMIT ?
       ) ORDER BY created_at ASC`,
    );
  }

  private insertStmt: SqliteStatement;
  private loadStmt: SqliteStatement;

  async save(sessionId: string, messages: Message[]): Promise<void> {
    if (messages.length === 0) return;
    let now = Date.now();
    for (const m of messages) {
      // 跳过空内容(如带工具调用但无文本的 assistant 中间态)
      if (!(m.content ?? '').trim()) continue;
      this.insertStmt.run(sessionId, m.role, m.content ?? '', now);
      now += 1; // 保证同批消息 created_at 单调递增,Load 顺序稳定
    }
  }

  async load(sessionId: string, limit: number): Promise<Message[]> {
    const rows = this.loadStmt.all(sessionId, limit > 0 ? limit : -1) as Array<{
      role: string;
      content: string;
    }>;
    return rows.map((r) => ({ role: r.role as Message['role'], content: r.content }));
  }

  /** 撤回:删除会话记忆流水的末尾 n 条(按 created_at 降序取后删) */
  async truncateLast(sessionId: string, n: number): Promise<void> {
    if (n <= 0) return;
    this.db.prepare(
      `DELETE FROM memory_messages WHERE id IN (
         SELECT id FROM memory_messages WHERE session_id = ?
         ORDER BY created_at DESC LIMIT ?
       )`,
    ).run(sessionId, n);
  }
}
