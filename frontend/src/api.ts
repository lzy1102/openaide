/**
 * API 客户端 —— 与 packages/api 的真实路由一一对应：
 *   GET /health · GET/POST /sessions · GET/DELETE /sessions/:id
 *   POST /v1/chat/stream (SSE: ready/chunk/error/done)
 * 全部走相对路径：开发由 Vite 代理，生产与 openaide serve 同源。
 */
import type { HealthInfo, SessionDto, StreamChunkDto } from './types';

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* 非 JSON 错误体 */
    }
    throw new Error(msg);
  }
  return res.json() as Promise<T>;
}

export const api = {
  async health(): Promise<HealthInfo> {
    return fetch('/health').then((r) => json<HealthInfo>(r));
  },

  async listSessions(): Promise<SessionDto[]> {
    const data = await fetch('/sessions').then((r) => json<{ sessions: SessionDto[] }>(r));
    return data.sessions ?? [];
  },

  async createSession(projectId = 'default'): Promise<SessionDto> {
    const data = await fetch('/sessions', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ project_id: projectId, user_id: 'web' }),
    }).then((r) => json<{ session: SessionDto }>(r));
    return data.session;
  },

  async getSession(id: string): Promise<SessionDto> {
    const data = await fetch(`/sessions/${encodeURIComponent(id)}`).then((r) =>
      json<{ session: SessionDto }>(r),
    );
    return data.session;
  },

  async deleteSession(id: string): Promise<void> {
    await fetch(`/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' });
  },

  /**
   * 流式问答。返回 sessionId（来自 ready 事件）；
   * onChunk 收到除 ready 外的全部事件；abort 可中断生成。
   */
  async streamChat(
    opts: { sessionId?: string; content: string; projectId?: string; signal?: AbortSignal },
    onChunk: (chunk: StreamChunkDto) => void,
  ): Promise<string> {
    const res = await fetch('/v1/chat/stream', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        content: opts.content,
        session_id: opts.sessionId,
        project_id: opts.projectId ?? 'default',
        user_id: 'web',
      }),
      signal: opts.signal,
    });
    if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let sessionId = opts.sessionId ?? '';

    // SSE 解析：按空行分帧，帧内 event:/data: 两行
    const handleFrame = (frame: string) => {
      let event = 'message';
      let data = '';
      for (const line of frame.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim();
        else if (line.startsWith('data:')) data += line.slice(5).trim();
      }
      if (!data) return;
      let parsed: unknown;
      try {
        parsed = JSON.parse(data);
      } catch {
        return;
      }
      switch (event) {
        case 'ready': {
          const r = parsed as { sessionId?: string };
          if (r.sessionId) sessionId = r.sessionId;
          break;
        }
        case 'chunk':
          onChunk(parsed as StreamChunkDto);
          break;
        case 'error':
          onChunk({ type: 'error', error: { message: (parsed as { message?: string }).message } });
          break;
        case 'done':
          onChunk({ type: 'done', done: true });
          break;
        default:
          break;
      }
    };

    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const frames = buffer.split('\n\n');
      buffer = frames.pop() ?? '';
      for (const frame of frames) {
        if (frame.trim()) handleFrame(frame);
      }
    }
    if (buffer.trim()) handleFrame(buffer);
    return sessionId;
  },
};
