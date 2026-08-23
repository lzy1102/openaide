/**
 * SQLite 驱动适配层 —— 同步 API 统一两种运行时形态：
 *  - Bun 二进制形态（bun build --compile）：内置 bun:sqlite，无需任何文件依赖；
 *    better-sqlite3 的运行时 bindings 探测在编译产物的虚拟 FS 中必然失败。
 *  - Node 源码/npm 形态：维持 better-sqlite3。
 * 两驱动 API 同形（exec/prepare/run/get/all/@命名绑定/对象行返回），调用方零感知。
 */
import { createRequire } from 'node:module';

export interface SqliteStatement {
  run(...params: unknown[]): unknown;
  get(...params: unknown[]): unknown;
  all(...params: unknown[]): unknown[];
}

export interface SqliteDatabase {
  /** 执行原始 SQL（建表/PRAGMA 等） */
  exec(sql: string): void;
  prepare(sql: string): SqliteStatement;
  close(): void;
}

export function openSqliteDatabase(filename: string): SqliteDatabase {
  const isBun = typeof (globalThis as { Bun?: unknown }).Bun !== 'undefined';
  if (isBun) {
    // bun:sqlite 为 Bun 内置模块，字面量引用即可被 compile 辨识且不会打进产物
    const req = createRequire(import.meta.url);
    const { Database } = req('bun:sqlite') as {
      Database: new (filename: string) => SqliteDatabase;
    };
    return new Database(filename);
  }
  // Node：模块名拼接防止打包器静态追踪原生模块（其 .node 路径靠运行时探测）
  const moduleName = 'better-sqlite' + '3';
  const req = createRequire(import.meta.url);
  const Better = req(moduleName) as new (
    filename: string,
    options?: Record<string, unknown>,
  ) => SqliteDatabase;
  return new Better(filename);
}
