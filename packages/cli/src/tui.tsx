/**
 * Ink TUI —— 交互式终端界面（React 渲染）。
 * 能力（与原 readline REPL 对齐 + TUI 增强）：
 *  - 上/下方向键浏览输入历史
 *  - 顶部标题栏（版本、会话、插件数）
 *  - 对话流：用户/助手消息、工具调用、流式输出
 *  - 斜杠命令：/help /new /sessions /use /plugins /persona /exit
 *  - 流式输出实时渲染
 */
import React, { useState, useRef, useCallback } from 'react';
import { Box, Text, Static, useApp, useInput } from 'ink';
import TextInput from 'ink-text-input';
import Spinner from 'ink-spinner';
import { KernelEvent, EventTypes, StreamChunkType, newId } from '@openaide/core';
import type { App } from './app.js';

/** 渲染用消息（历史列表项） */
interface TuiMessage {
  id: number;
  kind: 'user' | 'assistant' | 'tool' | 'error' | 'info';
  content: string;
}

const HELP_TEXT = [
  'Commands:',
  '  /help                  show this help',
  '  /new                   start a new session (context reset)',
  '  /sessions              list recent sessions',
  '  /use <id>              resume a session by id',
  '  /plugins               list active plugins and tools',
  '  /persona               list available personas',
  '  /exit, /quit           quit',
  '  anything else          send to the agent',
  '',
  'Keys:',
  '  ↑/↓  input history    Ctrl+C  exit',
].join('\n');

let seq = 0;
const nextId = (): number => ++seq;

/** TUI 根组件 */
export function Tui({ app }: { app: App }): React.ReactElement {
  const { exit } = useApp();
  const [history, setHistory] = useState<TuiMessage[]>([]);
  const [streaming, setStreaming] = useState('');
  const [status, setStatus] = useState<string>('ready');
  const [input, setInput] = useState('');
  const [sessionId, setSessionId] = useState<string | undefined>();
  const [busy, setBusy] = useState(false);

  // 输入历史（上/下键）
  const inputHistory = useRef<string[]>([]);
  const historyIndex = useRef(-1);

  const push = useCallback((msg: Omit<TuiMessage, 'id'>) => {
    setHistory((prev) => [...prev, { id: nextId(), ...msg }]);
  }, []);

  const pushUser = useCallback(
    (content: string) => push({ kind: 'user', content }),
    [push],
  );

  const executeCommand = useCallback(
    async (cmdLine: string) => {
      const [cmd, arg] = cmdLine.split(/\s+/, 2);
      switch (cmd) {
        case '/exit':
        case '/quit':
          exit();
          return;
        case '/help':
          push({ kind: 'info', content: HELP_TEXT });
          return;
        case '/new':
          setSessionId(undefined);
          push({ kind: 'info', content: 'new session started.' });
          return;
        case '/sessions': {
          const sessions = await app.sessions.list();
          if (sessions.length === 0) {
            push({ kind: 'info', content: '(no sessions yet)' });
          } else {
            const lines = ['recent sessions:'];
            for (const s of sessions.slice(0, 10)) {
              lines.push(
                `  ${s.id}  project=${s.projectId}  msgs=${s.messages.length}  ${new Date(s.updatedAt).toISOString()}`,
              );
            }
            push({ kind: 'info', content: lines.join('\n') });
          }
          return;
        }
        case '/use': {
          if (!arg) {
            push({ kind: 'info', content: 'usage: /use <session-id>' });
            return;
          }
          const s = await app.sessions.get(arg);
          if (!s) {
            push({ kind: 'error', content: `session not found: ${arg}` });
          } else {
            setSessionId(s.id);
            push({ kind: 'info', content: `resumed session ${s.id} (${s.messages.length} messages).` });
          }
          return;
        }
        case '/plugins':
          push({
            kind: 'info',
            content:
              `active plugins: ${app.plugins.names().join(', ') || '(none)'}\n` +
              `tools: ${app.registry.definitions().map((d) => d.function.name).join(', ')}`,
          });
          return;
        case '/persona':
          push({
            kind: 'info',
            content: `available personas: ${app.plugins.getPersonas().map((p) => p.name).join(', ') || '(builtin only)'}`,
          });
          return;
        default:
          push({ kind: 'error', content: `unknown command: ${cmd}  (try /help)` });
      }
    },
    [app, push, exit],
  );

  /** 提交输入：命令或对话 */
  const onSubmit = useCallback(
    async (value: string) => {
      const line = value.trim();
      if (!line) return;
      inputHistory.current.push(line);
      historyIndex.current = -1;
      if (line.startsWith('/')) {
        await executeCommand(line);
        return;
      }
      const sid = sessionId ?? newId();
      setSessionId(sid);
      setBusy(true);
      setStatus('thinking…');
      pushUser(line);
      let text = '';
      try {
        for await (const chunk of app.kernel.processStream({
          sessionId: sid,
          projectId: 'default',
          userId: 'local',
          content: line,
          options: {},
        })) {
          switch (chunk.type) {
            case StreamChunkType.Thinking:
              if (chunk.reasoningContent) {
                text += chunk.reasoningContent;
                setStreaming(text);
              }
              break;
            case StreamChunkType.Content:
              if (chunk.content) {
                text += chunk.content;
                setStreaming(text);
              }
              break;
            case StreamChunkType.ToolCall:
              push({
                kind: 'tool',
                content: `⚡ ${chunk.toolName}(${chunk.toolArgs})`,
              });
              break;
            case StreamChunkType.ToolDone:
              if (chunk.toolResult?.error) {
                push({ kind: 'error', content: `✗ ${chunk.toolResult.error}` });
              }
              break;
            case StreamChunkType.Error:
              push({ kind: 'error', content: `[error] ${chunk.error?.message ?? 'unknown'}` });
              break;
            default:
              break;
          }
        }
      } finally {
        if (text) {
          push({ kind: 'assistant', content: text });
        }
        setStreaming('');
        setBusy(false);
        setStatus('ready');
      }
    },
    [app, sessionId, pushUser, push, executeCommand],
  );

  // 键盘：↑/↓ 历史、Ctrl+C 退出
  useInput((char, key) => {
    if (key.ctrl && char === 'c') {
      exit();
      return;
    }
    if (busy) return;
    if (key.upArrow) {
      const hist = inputHistory.current;
      if (hist.length === 0) return;
      const idx = historyIndex.current < 0 ? hist.length - 1 : Math.max(0, historyIndex.current - 1);
      historyIndex.current = idx;
      setInput(hist[idx] ?? '');
    } else if (key.downArrow) {
      const hist = inputHistory.current;
      if (historyIndex.current < 0) return;
      const idx = historyIndex.current + 1;
      historyIndex.current = idx >= hist.length ? -1 : idx;
      setInput(idx >= hist.length ? '' : hist[idx] ?? '');
    }
  });

  // 订阅内核事件：展示工具执行（供验证插件钩子）
  const hookSeen = useRef(false);
  if (!hookSeen.current) {
    hookSeen.current = true;
    app.kernel.subscribe((event: KernelEvent) => {
      if (event.type === EventTypes.ToolCallStarted) {
        push({ kind: 'tool', content: `[hook] tool: ${String(event.data?.tool)}` });
      }
    });
  }

  const toolCount = app.registry.definitions().length;

  return (
    <Box flexDirection="column">
      {/* 标题栏 */}
      <Box borderStyle="round" paddingX={1} flexDirection="column">
        <Text bold color="magenta">
          OpenAIDE
        </Text>
        <Text dimColor>
          {`session: ${sessionId ?? '(new)'}   tools: ${toolCount}   plugins: ${app.plugins.names().join(', ') || '-'}`}
        </Text>
      </Box>

      {/* 对话流（Static：只增不减，避免输入时重渲染闪烁） */}
      <Box flexDirection="column">
        <Static items={history}>
          {(msg) => (
            <Box key={msg.id}>
              <Text
                color={
                  msg.kind === 'user'
                    ? 'green'
                    : msg.kind === 'assistant'
                      ? 'white'
                      : msg.kind === 'tool'
                        ? 'cyan'
                        : msg.kind === 'error'
                          ? 'red'
                          : 'yellow'
                }
              >
                {msg.kind === 'user' ? '› ' : msg.kind === 'assistant' ? '▸ ' : '  '}
                {msg.content}
              </Text>
            </Box>
          )}
        </Static>

        {/* 流式输出（实时变化，不放 Static） */}
        {streaming ? (
          <Text color="white">
            ▸ {streaming}
          </Text>
        ) : null}
      </Box>

      {/* 状态行 + 输入 */}
      <Box flexDirection="column" marginTop={1}>
        <Box>
          {busy ? <Spinner /> : <Text dimColor> </Text>}
          <Text dimColor>{status}</Text>
        </Box>
        <TextInput
          focus
          placeholder="Type a message or /help…"
          value={input}
          onChange={setInput}
          onSubmit={onSubmit}
        />
      </Box>
    </Box>
  );
}

/** 启动 Ink TUI（非 TTY 环境由调用方降级，见 runTui） */
export async function runTui(app: App): Promise<void> {
  const { render } = await import('ink');
  const { waitUntilExit } = render(<Tui app={app} />);
  await waitUntilExit();
}
