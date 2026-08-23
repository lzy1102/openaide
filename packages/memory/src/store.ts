/**
 * SQLite 会话存储 —— 基于 sqlite-driver 的持久化 SessionStore 实现。
 * 会话与消息整体序列化（JSON）存储，满足 core 的 SessionStore 契约。
 * 使用同步驱动但以 async 包装，接口签名与内存实现保持一致。
 */
import { openSqliteDatabase } from './sqlite-driver.js';
import type { SqliteDatabase, SqliteStatement } from './sqlite-driver.js';
import { mkdirSync } from 'node:fs';
import { dirname } from 'node:path';
import type { Message, Session, SessionStore } from '@openaide/core';
import { newId } from '@openaide/core';

const SCHEMA = `
CREATE TABLE IF NOT EXISTS sessions (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL,
  user_id     TEXT NOT NULL,
  messages    TEXT NOT NULL DEFAULT '[]',
  metadata    TEXT NOT NULL DEFAULT '{}',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC);
`;

interface SessionRow {
  id: string;
  project_id: string;
  user_id: string;
  messages: string;
  metadata: string;
  created_at: number;
  updated_at: number;
}

function parseRow(row: SessionRow): Session {
  return {
    id: row.id,
    projectId: row.project_id,
    userId: row.user_id,
    messages: JSON.parse(row.messages) as Message[],
    metadata: JSON.parse(row.metadata) as Record<string, unknown>,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

/**
 * SQLite 会话存储。
 * @param dbPath 数据库文件路径；':memory:' 表示纯内存（测试用）
 */
export class SQLiteSessionStore implements SessionStore {
  private db: SqliteDatabase;
  private getStmt: SqliteStatement;
  private listStmt: SqliteStatement;
  private listByProjectStmt: SqliteStatement;
  private insertStmt: SqliteStatement;
  private updateStmt: SqliteStatement;
  private deleteStmt: SqliteStatement;

  constructor(dbPath: string) {
    if (dbPath !== ':memory:') {
      mkdirSync(dirname(dbPath), { recursive: true });
    }
    this.db = openSqliteDatabase(dbPath);
    this.db.exec('PRAGMA journal_mode = WAL;');
    this.db.exec(SCHEMA);
    this.getStmt = this.db.prepare('SELECT * FROM sessions WHERE id = ?');
    this.listStmt = this.db.prepare('SELECT * FROM sessions ORDER BY updated_at DESC');
    this.listByProjectStmt = this.db.prepare(
      'SELECT * FROM sessions WHERE project_id = ? ORDER BY updated_at DESC',
    );
    this.insertStmt = this.db.prepare(
      `INSERT INTO sessions (id, project_id, user_id, messages, metadata, created_at, updated_at)
       VALUES (@id, @projectId, @userId, @messages, @metadata, @createdAt, @updatedAt)`,
    );
    this.updateStmt = this.db.prepare(
      `UPDATE sessions
       SET messages = @messages, metadata = @metadata, updated_at = @updatedAt
       WHERE id = @id`,
    );
    this.deleteStmt = this.db.prepare('DELETE FROM sessions WHERE id = ?');
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
    this.insertStmt.run({
      id: session.id,
      projectId,
      userId,
      messages: JSON.stringify(session.messages),
      metadata: JSON.stringify(session.metadata),
      createdAt: now,
      updatedAt: now,
    });
    return session;
  }

  async get(sessionId: string): Promise<Session | undefined> {
    const row = this.getStmt.get(sessionId) as SessionRow | undefined;
    return row ? parseRow(row) : undefined;
  }

  async update(session: Session): Promise<void> {
    const updatedAt = Date.now();
    this.updateStmt.run({
      id: session.id,
      messages: JSON.stringify(session.messages),
      metadata: JSON.stringify(session.metadata),
      updatedAt,
    });
    session.updatedAt = updatedAt;
  }

  async list(projectId?: string): Promise<Session[]> {
    const rows = (projectId ? this.listByProjectStmt.all(projectId) : this.listStmt.all()) as SessionRow[];
    return rows.map(parseRow);
  }

  async delete(sessionId: string): Promise<void> {
    this.deleteStmt.run(sessionId);
  }

  /** 关闭数据库连接 */
  close(): void {
    this.db.close();
  }
}
