/**
 * HTTP/WebSocket API 服务 —— 依赖注入式（不依赖 cli 装配）。
 * 能力：
 *  - REST：/health、/sessions（增删查）、/v1/chat（非流式问答）
 *  - SSE：POST /v1/chat/stream（流式问答，text/event-stream）
 *  - WebSocket：/ws（双向流式对话）
 * 全部同进程，零子进程。
 */
import { createServer, IncomingMessage, Server, ServerResponse } from 'node:http';
import { randomUUID } from 'node:crypto';
import { WebSocketServer, WebSocket } from 'ws';
import {
  AgentKernel,
  Message,
  Session,
  SessionStore,
  StreamChunk,
  StreamChunkType,
} from '@openaide/core';

/** API 服务依赖（内核 + 会话存储 + 服务信息）—— 由装配方注入 */
export interface ApiContext {
  kernel: Pick<AgentKernel, 'process' | 'processStream' | 'getState'>;
  sessions: SessionStore;
  service: { name: string; version: string; model: string };
}

/** 请求体解析（JSON） */
function readBody(req: IncomingMessage): Promise<Record<string, unknown>> {
  return new Promise((resolve, reject) => {
    let data = '';
    req.on('data', (c: Buffer) => {
      data += c.toString('utf8');
      if (data.length > 1_000_000) {
        req.destroy();
        reject(new Error('payload too large'));
      }
    });
    req.on('end', () => {
      if (!data) return resolve({});
      try {
        resolve(JSON.parse(data) as Record<string, unknown>);
      } catch (err) {
        reject(new Error(`invalid JSON: ${(err as Error).message}`));
      }
    });
    req.on('error', reject);
  });
}

function sendJson(res: ServerResponse, status: number, body: unknown): void {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    'content-type': 'application/json; charset=utf-8',
    'content-length': Buffer.byteLength(payload),
  });
  res.end(payload);
}

function getPath(req: IncomingMessage): string {
  const url = req.url ?? '/';
  return url.split('?')[0]!;
}

function sseSend(res: ServerResponse, event: string, data: unknown): void {
  res.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
}

function toSessionDto(s: Session): Record<string, unknown> {
  return {
    id: s.id,
    projectId: s.projectId,
    userId: s.userId,
    messages: s.messages,
    createdAt: s.createdAt,
    updatedAt: s.updatedAt,
  };
}

/** 创建 API 服务（返回 http.Server，绑定 ws 升级） */
export function createApiServer(ctx: ApiContext): Server {
  const { kernel, sessions, service } = ctx;

  const wss = new WebSocketServer({ noServer: true });

  // WebSocket 会话状态：连接 → { currentSessionId }
  const wsState = new WeakMap<WebSocket, { sessionId?: string }>();

  wss.on('connection', (ws: WebSocket) => {
    wsState.set(ws, {});
    ws.on('message', (raw: Buffer) => {
      void handleWsMessage(ws, raw.toString('utf8')).catch((err) => {
        ws.send(JSON.stringify({ type: 'error', message: (err as Error).message }));
      });
    });
    ws.send(JSON.stringify({ type: 'ready' }));
  });

  async function handleWsMessage(ws: WebSocket, text: string): Promise<void> {
    const msg = JSON.parse(text) as {
      type?: string;
      content?: string;
      sessionId?: string;
      projectId?: string;
    };
    if (msg.type === 'ping') {
      ws.send(JSON.stringify({ type: 'pong' }));
      return;
    }
    if (msg.type !== 'chat' || typeof msg.content !== 'string') {
      ws.send(JSON.stringify({ type: 'error', message: 'expected {type:"chat",content:"..."}' }));
      return;
    }
    const state = wsState.get(ws) ?? {};
    const sessionId = msg.sessionId ?? state.sessionId ?? randomUUID();
    state.sessionId = sessionId;
    wsState.set(ws, state);

    const query = {
      sessionId,
      projectId: msg.projectId ?? 'default',
      userId: 'ws',
      content: msg.content,
      options: {},
    };
    for await (const chunk of kernel.processStream(query)) {
      ws.send(JSON.stringify({ type: 'chunk', data: chunk }));
    }
    ws.send(JSON.stringify({ type: 'done', sessionId }));
  }

  const server = createServer(async (req, res) => {
    try {
      await route(req, res);
    } catch (err) {
      sendJson(res, 400, { error: (err as Error).message });
    }
  });

  async function route(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const path = getPath(req);
    const method = req.method ?? 'GET';

    // ── 健康检查 ──────────────────────────────
    if (method === 'GET' && path === '/health') {
      sendJson(res, 200, {
        ok: true,
        name: service.name,
        version: service.version,
        model: service.model,
        state: kernel.getState(),
        uptime: process.uptime(),
      });
      return;
    }

    // ── 会话管理 ──────────────────────────────
    if (path === '/sessions' && method === 'GET') {
      const all = await sessions.list();
      sendJson(res, 200, { sessions: all.map(toSessionDto) });
      return;
    }
    if (path === '/sessions' && method === 'POST') {
      const body = await readBody(req);
      const session = await sessions.create(
        String(body.project_id ?? 'default'),
        String(body.user_id ?? 'api'),
      );
      sendJson(res, 201, { session: toSessionDto(session) });
      return;
    }
    if (path.startsWith('/sessions/') && method === 'DELETE') {
      const id = path.slice('/sessions/'.length);
      await sessions.delete(id);
      sendJson(res, 200, { ok: true, deleted: id });
      return;
    }
    if (path.startsWith('/sessions/') && method === 'GET') {
      const id = path.slice('/sessions/'.length);
      const session = await sessions.get(id);
      if (!session) {
        sendJson(res, 404, { error: 'session not found' });
      } else {
        sendJson(res, 200, { session: toSessionDto(session) });
      }
      return;
    }

    // ── 流式问答（SSE）────────────────────────
    if (path === '/v1/chat/stream' && method === 'POST') {
      const body = await readBody(req);
      const content = String(body.content ?? '');
      if (!content) {
        sendJson(res, 400, { error: 'content required' });
        return;
      }
      const sessionId = String(body.session_id ?? randomUUID());
      res.writeHead(200, {
        'content-type': 'text/event-stream; charset=utf-8',
        'cache-control': 'no-cache',
        connection: 'keep-alive',
      });
      sseSend(res, 'ready', { sessionId });
      try {
        for await (const chunk of kernel.processStream({
          sessionId,
          projectId: String(body.project_id ?? 'default'),
          userId: String(body.user_id ?? 'api'),
          content,
          options: {
            temperature: body.temperature as number | undefined,
            maxTokens: body.max_tokens as number | undefined,
          },
        })) {
          sseSend(res, 'chunk', chunk);
        }
      } catch (err) {
        sseSend(res, 'error', { message: (err as Error).message });
      }
      sseSend(res, 'done', { sessionId });
      res.end();
      return;
    }

    // ── 非流式问答 ────────────────────────────
    if (path === '/v1/chat' && method === 'POST') {
      const body = await readBody(req);
      const content = String(body.content ?? '');
      if (!content) {
        sendJson(res, 400, { error: 'content required' });
        return;
      }
      const sessionId = String(body.session_id ?? randomUUID());
      const response = await kernel.process({
        sessionId,
        projectId: String(body.project_id ?? 'default'),
        userId: String(body.user_id ?? 'api'),
        content,
        options: {
          temperature: body.temperature as number | undefined,
          maxTokens: body.max_tokens as number | undefined,
        },
      });
      sendJson(res, 200, { sessionId: response.sessionId, content: response.content, usage: response.usage });
      return;
    }

    sendJson(res, 404, { error: `not found: ${method} ${path}` });
  }

  // WebSocket 升级
  server.on('upgrade', (req, socket, head) => {
    const path = getPath(req);
    if (path === '/ws') {
      wss.handleUpgrade(req, socket, head, (ws) => {
        wss.emit('connection', ws, req);
      });
    } else {
      socket.destroy();
    }
  });

  return server;
}

/** 便捷启动：绑定端口后返回 server 与地址 */
export function startApiServer(ctx: ApiContext, port: number, host = '127.0.0.1'): Promise<Server> {
  const server = createApiServer(ctx);
  return new Promise((resolve) => {
    server.listen(port, host, () => resolve(server));
  });
}

/** 给 SSE chunk 判类型用（导出复用） */
export function isDoneChunk(chunk: StreamChunk): boolean {
  return chunk.type === StreamChunkType.Done;
}

export type { StreamChunk, Message };
