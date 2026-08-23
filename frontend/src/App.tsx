import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from './api';
import type { HealthInfo, SessionDto, StreamChunkDto } from './types';
import { appendTailAssistant, mapHistory, nextUiId, settleRunningTool } from './ui';
import type { UiMsg, UiAssistant } from './ui';
import { Sidebar } from './components/Sidebar';
import { MessageView } from './components/Message';

export default function App(): React.ReactElement {
  const [health, setHealth] = useState<HealthInfo | null>(null);
  const [sessions, setSessions] = useState<SessionDto[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [msgs, setMsgs] = useState<UiMsg[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [phase, setPhase] = useState('ready');
  const [error, setError] = useState<string | null>(null);

  const abortRef = useRef<AbortController | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  // 健康轮询：连接状态点 + 模型名
  useEffect(() => {
    let stop = false;
    const poll = () =>
      api
        .health()
        .then((h) => !stop && setHealth(h))
        .catch(() => !stop && setHealth(null));
    poll();
    const timer = setInterval(poll, 8000);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, []);

  const refreshSessions = useCallback(() => {
    api.listSessions().then(setSessions).catch(() => {});
  }, []);
  useEffect(refreshSessions, [refreshSessions]);

  // 自动滚动到底
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [msgs, phase]);

  const loadSession = useCallback(async (id: string) => {
    try {
      const s = await api.getSession(id);
      setActiveId(s.id);
      setMsgs(mapHistory(s));
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  }, []);

  const newChat = useCallback(async () => {
    try {
      const s = await api.createSession();
      setActiveId(s.id);
      setMsgs([]);
      setError(null);
      refreshSessions();
    } catch (err) {
      // 后端不可用也能本地起聊（stream 会自动建会话）
      setActiveId(null);
      setMsgs([]);
      setError((err as Error).message);
    }
  }, [refreshSessions]);

  const deleteSession = useCallback(
    async (id: string) => {
      await api.deleteSession(id).catch(() => {});
      setSessions((prev) => prev.filter((s) => s.id !== id));
      if (id === activeId) {
        setActiveId(null);
        setMsgs([]);
      }
    },
    [activeId],
  );

  const send = useCallback(async () => {
    const content = input.trim();
    if (!content || streaming) return;
    setInput('');
    setError(null);
    setMsgs((prev) => [...prev, { kind: 'user', id: nextUiId(), content }]);
    setStreaming(true);
    setPhase('Thinking…');

    const controller = new AbortController();
    abortRef.current = controller;
    let sawDone = false;

    const onChunk = (chunk: StreamChunkDto) => {
      switch (chunk.type) {
        case 'thinking':
          if (chunk.reasoningContent) {
            setPhase('Reasoning…');
            setMsgs((prev) =>
              appendTailAssistant(prev, (m: UiAssistant) => ({
                ...m,
                reasoning: (m.reasoning ?? '') + chunk.reasoningContent,
              })),
            );
          }
          break;
        case 'content':
          setPhase('Responding…');
          setMsgs((prev) =>
            appendTailAssistant(prev, (m) => ({ ...m, content: m.content + (chunk.content ?? '') })),
          );
          break;
        case 'tool_call': {
          setPhase(`Running ${chunk.toolName ?? 'tool'}…`);
          setMsgs((prev) => [
            ...prev,
            {
              kind: 'tool',
              id: nextUiId(),
              name: chunk.toolName ?? 'tool',
              args: chunk.toolArgs,
              status: 'running',
            },
          ]);
          break;
        }
        case 'tool_done':
          setMsgs((prev) =>
            settleRunningTool(
              prev,
              chunk.toolName,
              chunk.toolResult?.content,
              chunk.toolResult?.error,
            ),
          );
          break;
        case 'progress':
          setPhase(`Round ${chunk.round ?? '?'}/${chunk.totalRounds ?? '?'}`);
          break;
        case 'done':
          sawDone = true;
          break;
        case 'error':
          setError(chunk.error?.message ?? 'unknown error');
          break;
        default:
          break;
      }
    };

    try {
      const sid = await api.streamChat(
        { sessionId: activeId ?? undefined, content, signal: controller.signal },
        onChunk,
      );
      if (!activeId && sid) {
        setActiveId(sid);
        refreshSessions();
      }
      if (!sawDone) setError('connection closed before completion');
    } catch (err) {
      if ((err as Error).name !== 'AbortError') setError((err as Error).message);
    } finally {
      abortRef.current = null;
      setStreaming(false);
      setPhase('ready');
      refreshSessions();
    }
  }, [input, streaming, activeId, refreshSessions]);

  const stop = useCallback(() => abortRef.current?.abort(), []);

  return (
    <div className="app">
      <header className="topbar">
        <span className="logo">◆ OpenAIDE</span>
        <span className="model">{health?.model ?? '—'}</span>
        <span className="spacer" />
        <span className={`conn${health ? ' up' : ''}`}>
          <span className="dot" />
          {health ? `${health.state} · v${health.version}` : 'offline'}
        </span>
      </header>

      <div className="main">
        <Sidebar
          sessions={sessions}
          activeId={activeId}
          onSelect={loadSession}
          onNew={newChat}
          onDelete={deleteSession}
        />

        <section className="chat">
          <div className="messages" ref={scrollRef}>
            {msgs.length === 0 && !streaming ? (
              <div className="empty-hint">
                <span className="big">◆ OpenAIDE</span>
                <span>everything is a plugin</span>
                <span>
                  send a message, or run <code>openaide</code> in your terminal
                </span>
              </div>
            ) : (
              msgs.map((m) => <MessageView key={m.id} m={m} />)
            )}
            {streaming && (
              <div className="phase">
                <span className="spinner" />
                {phase}
              </div>
            )}
          </div>

          <div className="composer-wrap">
            <div className="composer">
              <textarea
                rows={1}
                placeholder="Send a message…"
                value={input}
                onChange={(e) => {
                  setInput(e.target.value);
                  e.target.style.height = 'auto';
                  e.target.style.height = `${Math.min(e.target.scrollHeight, 160)}px`;
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    void send();
                  }
                }}
              />
              {streaming ? (
                <button className="btn-send stop" onClick={stop}>
                  Stop
                </button>
              ) : (
                <button className="btn-send" onClick={() => void send()} disabled={!input.trim()}>
                  Send
                </button>
              )}
            </div>
            <div className="composer-hint">Enter send · Shift+Enter newline · SSE stream</div>
          </div>
        </section>
      </div>

      {error && (
        <div style={{ position: 'fixed', bottom: 18, left: '50%', transform: 'translateX(-50%)', zIndex: 9 }}>
          <div className="error-bar" style={{ margin: 0 }}>
            ✗ {error}
            <button
              style={{ marginLeft: 12, border: 0, background: 'none', color: 'inherit', cursor: 'pointer' }}
              onClick={() => setError(null)}
            >
              dismiss
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
