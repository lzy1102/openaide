import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

/** 用 ReadableStream 构造 SSE Response（分片可跨帧边界，验证缓冲逻辑） */
function sseResponse(pieces: string[], status = 200): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const p of pieces) controller.enqueue(encoder.encode(p));
      controller.close();
    },
  });
  return new Response(stream, { status });
}

afterEach(() => vi.unstubAllGlobals());

describe('api.streamChat（SSE 解析）', () => {
  it('解析 ready/chunk/done 帧，返回 sessionId', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        sseResponse([
          'event: ready\ndata: {"sessionId":"abc-1"}\n\n',
          'event: chunk\ndata: {"type":"content","content":"he"}\n\n',
          'event: chunk\ndata: {"type":"content","content":"llo"}\n\n',
          'event: done\ndata: {"sessionId":"abc-1"}\n\n',
        ]),
      ),
    );
    const seen: unknown[] = [];
    const sid = await api.streamChat({ content: 'q' }, (c) => seen.push(c));
    expect(sid).toBe('abc-1');
    expect(seen).toEqual([
      { type: 'content', content: 'he' },
      { type: 'content', content: 'llo' },
      { type: 'done', done: true },
    ]);
  });

  it('帧被网络分片截断时仍能正确拼装（缓冲边界）', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        sseResponse(['event: rea', 'dy\ndata: {"session', 'Id":"xyz"}\n\nevent: chunk\ndata: {"type":"think', 'ing","reasoningContent":"why"}\n\n']),
      ),
    );
    const seen: unknown[] = [];
    const sid = await api.streamChat({ content: 'q' }, (c) => seen.push(c));
    expect(sid).toBe('xyz');
    expect(seen).toEqual([{ type: 'thinking', reasoningContent: 'why' }]);
  });

  it('error 帧映射为 error chunk；HTTP 错误抛出', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => sseResponse(['event: error\ndata: {"message":"rate limited"}\n\n'])),
    );
    const seen: unknown[] = [];
    await api.streamChat({ content: 'q' }, (c) => seen.push(c));
    expect(seen).toEqual([{ type: 'error', error: { message: 'rate limited' } }]);

    vi.stubGlobal('fetch', vi.fn(async () => new Response('nope', { status: 400 })));
    await expect(api.streamChat({ content: 'q' }, () => {})).rejects.toThrow('HTTP 400');
  });

  it('请求体携带 session_id 与 content', async () => {
    const fetchMock = vi.fn(async () => sseResponse(['event: done\ndata: {}\n\n']));
    vi.stubGlobal('fetch', fetchMock);
    await api.streamChat({ sessionId: 's9', content: 'hello', projectId: 'p1' }, () => {});
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/v1/chat/stream');
    expect(JSON.parse(String(init.body))).toMatchObject({
      session_id: 's9',
      content: 'hello',
      project_id: 'p1',
      user_id: 'web',
    });
  });
});

describe('sessions API', () => {
  it('listSessions 解包 {sessions}', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify({ sessions: [{ id: 'a', messages: [] }] }), { status: 200 }),
      ),
    );
    const list = await api.listSessions();
    expect(list).toHaveLength(1);
  });

  it('后端错误体优先展示 error 字段', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"session not found"}', { status: 404 })));
    await expect(api.getSession('nope')).rejects.toThrow('session not found');
  });
});
