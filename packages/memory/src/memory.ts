/**
 * SQLite 记忆存储 —— kernel.Memory 契约的实现。
 *
 * 独立于 sessions 表:内核用它构建跨轮历史(buildMessages 的 history 来源)。
 * 教训(Go 版审计):role 必须持久化——丢失后 Load 全部回退 assistant,
 * 会导致恢复会话时用户消息被当作回答渲染。
 */
import Database from 'better-sqlite3';
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
  private db: Database.Database;

  constructor(dbPath: string) {
    if (dbPath !== ':memory:') mkdirSync(dirname(dbPath), { recursive: true });
    this.db = new Database(dbPath);
    this.db.pragma('journal_mode = WAL');
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

  private insertStmt: Database.Statement;
  private loadStmt: Database.Statement;

  async save(sessionId: string, messages: Message[]): Promise<void> {
    if (messages.length === 0) return;
    let now = Date.now();
    const insertMany = this.db.transaction((msgs: Message[]) => {
      for (const m of msgs) {
        // 跳过空内容(如带工具调用但无文本的 assistant 中间态)
        if (!(m.content ?? '').trim()) continue;
        this.insertStmt.run(sessionId, m.role, m.content ?? '', now);
        now += 1; // 保证同批消息 created_at 单调递增,Load 顺序稳定
      }
    });
    insertMany(messages);
  }

  async load(sessionId: string, limit: number): Promise<Message[]> {
    const rows = this.loadStmt.all(sessionId, limit > 0 ? limit : -1) as Array<{
      role: string;
      content: string;
    }>;
    return rows.map((r) => ({ role: r.role as Message['role'], content: r.content }));
  }
}
