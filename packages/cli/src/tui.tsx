/**
 * Ink TUI —— 参考 Claude Code / Gemini CLI 风格的交互式终端界面。
 * 布局（自上而下）：
 *  - Banner（Static 首项）：品牌 + 版本 + 模型 + 快捷键提示，只渲染一次进滚动缓冲区
 *  - 对话流（Static）：用户/助手(markdown 渲染)/思考/工具调用/错误/信息卡片
 *  - 补全弹层：输入 "/" 时过滤命令清单，↑↓ 选择、Tab 补全、Esc 关闭
 *  - 输入框：圆角边框 + 占位符（Gemini CLI 风格）
 *  - 状态栏：模型 · 会话 · tools: N · plugins: N（始终可见）
 */
import React, { useState, useRef, useCallback, useEffect } from 'react';
import { Box, Text, Static, useApp, useInput } from 'ink';
import Spinner from 'ink-spinner';
import { StreamChunkType, newId } from '@openaide/core';
import { DEFAULT_REGISTRY_URL, fetchRegistry, GITHUB_PLUGIN_TOPIC, installEntry, readPluginState, searchEverywhere, writePluginState } from '@openaide/plugins';
import { existsSync, rmSync } from 'node:fs';
import type { App } from './app.js';
import type { ApprovalRequest } from './approval.js';
import { PasteInput } from './components/PasteInput.js';

/** 渲染用消息（历史列表项；kind=banner 仅作 Static 首项占位） */
interface TuiMessage {
  id: number;
  kind: 'banner' | 'user' | 'assistant' | 'thinking' | 'tool' | 'error' | 'info';
  content: string;
}

/** 处理阶段（spinner 动词） */
type Phase = 'idle' | 'thinking' | 'reasoning' | 'tool' | 'responding';

/** 斜杠命令元数据 —— 帮助文本与自动补全共用一份来源 */
interface CommandMeta {
  name: string;
  args?: string;
  desc: string;
}

const COMMANDS: CommandMeta[] = [
  { name: '/help', desc: 'show this help' },
  { name: '/new', desc: 'start a new session (context reset)' },
  { name: '/sessions', desc: 'list recent sessions' },
  { name: '/use', args: '<id>', desc: 'resume a session by id' },
  { name: '/model', args: '<id>', desc: 'switch model at runtime (no arg = show current)' },
  { name: '/plugins', desc: 'list plugins (all states) and tools' },
  { name: '/plugins search', args: '<kw>', desc: 'search the plugin registry' },
  { name: '/plugins install', args: '<name>', desc: 'install a plugin from the registry' },
  { name: '/plugins uninstall', args: '<name>', desc: 'remove an installed plugin' },
  { name: '/plugins enable', args: '<name>', desc: 'enable a plugin (persisted)' },
  { name: '/plugins disable', args: '<name>', desc: 'disable a plugin (persisted)' },
  { name: '/plugins reload', args: '<name>', desc: 'hot-reload a plugin' },
  { name: '/persona', desc: 'list available personas' },
  { name: '/persona', args: '<name>', desc: 'switch persona ("default" resets to built-in)' },
  { name: '/exit', desc: 'quit (also /quit)' },
];

const HELP_TEXT = [
  'Commands:',
  ...COMMANDS.map((c) => `  ${c.name}${c.args ? ` ${c.args}` : ''}`.padEnd(34) + c.desc),
  '',
  'Keys:',
  '  ↑/↓ history · Tab complete · Esc clear input · Ctrl+C exit',
].join('\n');

let seq = 0;
const nextId = (): number => ++seq;

/* ---------------------------------- markdown 轻量渲染 ---------------------------------- */

/** 行内格式：**bold** / *italic* / `code` → React 节点序列 */
function renderInline(text: string, keyBase: string): React.ReactNode[] {
  const parts = text.split(/(\*\*[^*]+\*\*|\*[^*\s][^*]*\*|`[^`]+`)/g).filter(Boolean);
  return parts.map((p, i) => {
    const key = `${keyBase}-${i}`;
    if (p.startsWith('**') && p.endsWith('**')) {
      return <Text key={key} bold>{p.slice(2, -2)}</Text>;
    }
    if (p.startsWith('`') && p.endsWith('`')) {
      return <Text key={key} color="cyan">{p.slice(1, -1)}</Text>;
    }
    if (p.startsWith('*') && p.endsWith('*') && p.length > 2) {
      return <Text key={key} italic>{p.slice(1, -1)}</Text>;
    }
    return <Text key={key}>{p}</Text>;
  });
}

/**
 * 极简 markdown：代码块（保留原样、暗底）、标题、列表、行内格式。
 * 不追求完整 spec——终端对话场景覆盖常用语法即可。
 */
function Markdown({ text }: { text: string }): React.ReactElement {
  const lines = text.replace(/\r\n/g, '\n').split('\n');
  const blocks: React.ReactNode[] = [];
  let inCode = false;
  let codeLines: string[] = [];

  const flushCode = (key: string) => {
    blocks.push(
      <Box key={key} flexDirection="column" borderStyle="round" borderColor="gray" paddingX={1}>
        {codeLines.map((l, i) => (
          <Text key={i} color="green">{l || ' '}</Text>
        ))}
      </Box>,
    );
    codeLines = [];
  };

  lines.forEach((line, idx) => {
    if (line.trim().startsWith('```')) {
      if (inCode) flushCode(`code-${idx}`);
      inCode = !inCode;
      return;
    }
    if (inCode) {
      codeLines.push(line);
      return;
    }
    const header = /^(#{1,6})\s+(.*)$/.exec(line);
    if (header) {
      blocks.push(
        <Text key={`h-${idx}`} bold underline color="magenta">
          {header[2]}
        </Text>,
      );
      return;
    }
    const bullet = /^(\s*)[-*]\s+(.*)$/.exec(line);
    if (bullet) {
      blocks.push(
        <Text key={`b-${idx}`}>
          {`${bullet[1]}• `}
          {renderInline(bullet[2] ?? '', `bi-${idx}`)}
        </Text>,
      );
      return;
    }
    const ordered = /^(\s*)(\d+\.)\s+(.*)$/.exec(line);
    if (ordered) {
      blocks.push(
        <Text key={`o-${idx}`}>
          {`${ordered[1]}${ordered[2]} `}
          {renderInline(ordered[3] ?? '', `oi-${idx}`)}
        </Text>,
      );
      return;
    }
    if (line.trim() === '') {
      blocks.push(<Text key={`s-${idx}`}> </Text>);
      return;
    }
    blocks.push(<Text key={`p-${idx}`}>{renderInline(line, `pi-${idx}`)}</Text>);
  });
  if (codeLines.length > 0) flushCode('code-end');
  return <Box flexDirection="column">{blocks}</Box>;
}

/* ---------------------------------- 主组件 ---------------------------------- */

/** TUI 根组件 */
export function Tui({ app, initialSessionId }: { app: App; initialSessionId?: string }): React.ReactElement {
  const { exit } = useApp();
  const [history, setHistory] = useState<TuiMessage[]>([]);
  const [streamingContent, setStreamingContent] = useState('');
  const [streamingReasoning, setStreamingReasoning] = useState('');
  const [phase, setPhase] = useState<Phase>('idle');
  const [currentTool, setCurrentTool] = useState('');
  const [status, setStatus] = useState<string>('ready');
  const [input, setInput] = useState('');
  const [sessionId, setSessionId] = useState<string | undefined>(initialSessionId);
  const [busy, setBusy] = useState(false);

  // 工具审批：流式中拦截 → 卡片等待 y/n
  const [pendingApproval, setPendingApproval] = useState<ApprovalRequest | null>(null);
  const approvalResolve = useRef<((v: boolean) => void) | null>(null);
  useEffect(() => {
    if (typeof app.setApprovalHandler !== 'function') return;
    app.setApprovalHandler((req) => {
      setPendingApproval(req);
      return new Promise<boolean>((resolve) => {
        approvalResolve.current = resolve;
      });
    });
    return () => {
      approvalResolve.current?.(false);
    };
  }, [app]);

  // 输入历史（上/下键）
  const inputHistory = useRef<string[]>([]);
  const historyIndex = useRef(-1);

  // 自动补全：输入以 "/" 开头且无空格（还在敲命令名）时即时显示
  const suggestionMatch = input.startsWith('/') && !input.includes(' ') && !input.includes('\t')
    ? COMMANDS.filter((c) => c.name.startsWith(input))
    : [];
  const [suggestIndex, setSuggestIndex] = useState(-1);
  useEffect(() => setSuggestIndex(-1), [input]);

  // 终端括号化粘贴模式：粘贴以 ESC[200~…ESC[201~ 成块到达，才能整体识别
  useEffect(() => {
    process.stdout.write('[?2004h');
    return () => { process.stdout.write('[?2004l'); };
  }, []);
  const activeSuggest = suggestIndex < 0 ? 0 : Math.min(suggestIndex, suggestionMatch.length - 1);
  const showSuggestions = !busy && suggestionMatch.length > 0;

  const push = useCallback((msg: Omit<TuiMessage, 'id'>) => {
    setHistory((prev) => [...prev, { id: nextId(), ...msg }]);
  }, []);

  const pushUser = useCallback((content: string) => push({ kind: 'user', content }), [push]);

  /* ---------------- 斜杠命令 ---------------- */
  const executeCommand = useCallback(
    async (cmdLine: string) => {
      const tokens = cmdLine.split(/\s+/).filter(Boolean);
      const cmd = tokens[0];
      const arg = tokens.slice(1).join(' ');
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
        case '/model':
          if (!arg) {
            push({
              kind: 'info',
              content: `current model: ${app.llm?.getModelId?.() ?? '(unknown)'}\nusage: /model <model-id>`,
            });
          } else {
            app.llm?.setModelId?.(arg);
            push({ kind: 'info', content: `model switched to: ${arg}` });
          }
          return;
        case '/plugins': {
          const sub = tokens[1];
          const name = tokens[2];
          const kw = tokens.slice(2).join(' ');
          if (!sub) {
            const lines = app.plugins.list().map((p) => {
              const badge = p.status === 'active' ? '' : ` <${p.status}>`;
              return `  ${p.name}@${p.version ?? '0.0.0'}${badge} — ${p.description ?? ''}`;
            });
            lines.push(`  tools: ${app.registry.definitions().map((d) => d.function.name).join(', ')}`);
            push({ kind: 'info', content: lines.join('\n') || '(no plugins)' });
            return;
          }
          if (sub === 'search') {
            try {
              const hits = await searchEverywhere(kw, { registryUrl: app.config?.registryUrl });
              if (hits.length === 0) {
                push({ kind: 'info', content: `no matches on GitHub (topic:${GITHUB_PLUGIN_TOPIC}) or registry${kw ? ` for "${kw}"` : ''}` });
              } else {
                push({
                  kind: 'info',
                  content:
                    hits
                      .map((e) => `${e.from.includes('github') ? `[gh★${e.stars ?? 0}]` : '[registry]'} ${e.name}${e.version ? `@${e.version}` : ''} — ${e.description ?? ''}`)
                      .join('\n') +
                    `\npublish: repo + topic ${GITHUB_PLUGIN_TOPIC} · install: /plugins install <name>`,
                });
              }
            } catch (err) {
              push({ kind: 'error', content: `search error: ${(err as Error).message}` });
            }
            return;
          }
          if (sub === 'install') {
            if (!name) {
              push({ kind: 'error', content: 'usage: /plugins install <name>' });
              return;
            }
            try {
              const reg = await fetchRegistry(app.config?.registryUrl ?? DEFAULT_REGISTRY_URL);
              const entry = reg.plugins.find((p) => p.name === name);
              if (!entry) {
                push({ kind: 'error', content: `not in registry: ${name}  (try: /plugins search ${name})` });
                return;
              }
              setStatus(`installing ${name}…`);
              const res = await installEntry(entry, { pluginsDir: app.config.pluginsDir });
              await app.plugins.load(res.dir);
              push({ kind: 'info', content: `installed ${res.name} → ${res.dir}` });
            } catch (err) {
              push({ kind: 'error', content: `install failed: ${(err as Error).message}` });
            }
            return;
          }
          if (sub === 'uninstall') {
            if (!name) {
              push({ kind: 'error', content: 'usage: /plugins uninstall <name>' });
              return;
            }
            const dir = app.plugins.dirOf(name);
            if (!dir || !existsSync(dir)) {
              push({ kind: 'error', content: `not installed: ${name}` });
              return;
            }
            await app.plugins.unload(name);
            rmSync(dir, { recursive: true, force: true });
            const st = readPluginState(app.config.dataDir);
            if (st.disabled.includes(name)) {
              st.disabled = st.disabled.filter((n) => n !== name);
              writePluginState(app.config.dataDir, st);
            }
            push({ kind: 'info', content: `uninstalled ${name}` });
            return;
          }
          if (!name || !['enable', 'disable', 'reload'].includes(sub)) {
            push({ kind: 'error', content: 'usage: /plugins [search <kw> | install|uninstall|enable|disable|reload <name>]' });
            return;
          }
          if (sub === 'disable') {
            await app.plugins.disable(name);
            push({ kind: 'info', content: `disabled ${name} (unloaded now; skipped on startup)` });
          } else if (sub === 'enable') {
            const ok = await app.plugins.enable(name);
            push(
              ok
                ? { kind: 'info', content: `enabled ${name}` }
                : { kind: 'error', content: `not found (or directory missing): ${name}` },
            );
          } else {
            try {
              await app.plugins.reload(name);
              push({ kind: 'info', content: `reloaded ${name}` });
            } catch (err) {
              push({ kind: 'error', content: `reload failed: ${(err as Error).message}` });
            }
          }
          return;
        }
        case '/persona': {
          const names = app.plugins.getPersonas().map((p) => p.name);
          if (!arg) {
            push({
              kind: 'info',
              content: `personas: ${names.join(', ') || '(builtin only)'}\ncurrent: ${app.getActivePersona?.() ?? '(default)'}\nswitch: /persona <name>  |  reset: /persona default`,
            });
            return;
          }
          const target = arg === 'default' ? undefined : arg;
          if (target && !names.includes(target)) {
            push({ kind: 'error', content: `persona not found: ${arg}  (available: ${names.join(', ')})` });
            return;
          }
          app.setActivePersona(target);
          push({
            kind: 'info',
            content: target
              ? `persona switched to: ${target} (takes effect on next message)`
              : 'persona reset to built-in default',
          });
          return;
        }
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
      setInput('');
      if (line.startsWith('/')) {
        await executeCommand(line);
        return;
      }
      const sid = sessionId ?? newId();
      setSessionId(sid);
      setBusy(true);
      setPhase('thinking');
      setStatus('waiting for first token…');
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
              setPhase('reasoning');
              if (chunk.reasoningContent) {
                setStreamingReasoning((prev) => prev + chunk.reasoningContent);
              }
              break;
            case StreamChunkType.Content:
              if (chunk.content) {
                setPhase('responding');
                text += chunk.content;
                setStreamingContent(text);
              }
              break;
            case StreamChunkType.ToolCall:
              setPhase('tool');
              setCurrentTool(chunk.toolName ?? '');
              push({ kind: 'tool', content: `⚡ ${chunk.toolName}(${chunk.toolArgs})` });
              break;
            case StreamChunkType.ToolDone:
              if (chunk.toolResult?.error) {
                push({ kind: 'error', content: `✗ ${chunk.toolResult.error}` });
                // 失败时的输出摘要（末尾几行最接近错误现场）
                const raw = chunk.toolResult.content;
                const out = typeof raw === 'string' ? raw.replace(/\s+$/, '') : '';
                if (out) {
                  const tail = out.split('\n').slice(-4).join('\n').slice(0, 400);
                  push({ kind: 'tool', content: `↳ ${tail}` });
                }
              }
              break;
            case StreamChunkType.Error:
              push({ kind: 'error', content: `[error] ${chunk.error?.message ?? 'unknown'}` });
              break;
            default:
              break;
          }
          // 让出事件循环一拍:Ink 的 setState 批处理需要事件循环间隙提交渲染,
          // 否则连续到达的 chunk 会合并成一次渲染甚至全部丢帧(实测表现为界面卡死)
          await new Promise((r) => setImmediate(r));
        }
      } finally {
        if (text) {
          push({ kind: 'assistant', content: text });
        }
        setStreamingContent('');
        setStreamingReasoning('');
        setBusy(false);
        setPhase('idle');
        setCurrentTool('');
        setStatus('ready');
      }
    },
    [app, sessionId, pushUser, push, executeCommand],
  );

  // 键盘：审批 y/n 优先 → ↑/↓ 历史/补全选择 → Tab 补全 → Esc 清空 → Ctrl+C 退出
  useInput((char, key) => {
    if (key.ctrl && char === 'c') {
      exit();
      return;
    }
    // 审批等待中：y 允许 / n 或 Esc 拒绝（优先级最高，流式期间也生效）
    if (pendingApproval) {
      if (char === 'y' || char === 'Y') {
        approvalResolve.current?.(true);
        setPendingApproval(null);
      } else if (char === 'n' || char === 'N' || key.escape) {
        approvalResolve.current?.(false);
        setPendingApproval(null);
      }
      return;
    }
    if (busy) return;
    // Tab：补全当前高亮（或第一个）建议
    if (char === '\t') {
      const list = suggestionMatch;
      if (list.length > 0) {
        const pick = list[activeSuggest];
        setInput(`${pick?.name} `);
      }
      return;
    }
    if (key.escape) {
      setInput('');
      return;
    }
    if (input.startsWith('/') && suggestionMatch.length > 0) {
      // 弹层打开时 ↑↓ 在建议间移动
      if (key.upArrow) {
        setSuggestIndex((i) => (i <= 0 ? suggestionMatch.length - 1 : i - 1));
        return;
      }
      if (key.downArrow) {
        setSuggestIndex((i) => (i >= suggestionMatch.length - 1 ? 0 : i + 1));
        return;
      }
    } else {
      // 无弹层时 ↑↓ 浏览输入历史
      if (key.upArrow) {
        const hist = inputHistory.current;
        if (hist.length === 0) return;
        const idx = historyIndex.current < 0 ? hist.length - 1 : Math.max(0, historyIndex.current - 1);
        historyIndex.current = idx;
        setInput(hist[idx] ?? '');
        return;
      }
      if (key.downArrow) {
        const hist = inputHistory.current;
        if (historyIndex.current < 0) return;
        const idx = historyIndex.current + 1;
        historyIndex.current = idx >= hist.length ? -1 : idx;
        setInput(idx >= hist.length ? '' : hist[idx] ?? '');
        return;
      }
    }
  });

  /* ---------------- 派生展示数据 ---------------- */
  const toolCount = app.registry.definitions().length;
  const pluginCount = app.plugins.names().length;
  const modelId = app.llm?.getModelId?.() ?? '-';
  const sessionShort = sessionId ? `${sessionId.slice(0, 8)}…` : 'new';

  const phaseLabel =
    phase === 'thinking'
      ? 'Thinking…'
      : phase === 'reasoning'
        ? 'Reasoning…'
        : phase === 'tool'
          ? `Running ${currentTool}…`
          : phase === 'responding'
            ? 'Responding…'
            : status;

  const bannerMsg: TuiMessage = { id: 0, kind: 'banner', content: '' };

  return (
    <Box flexDirection="column" width="100%">
      {/* 对话流（Static：只增不减，写入终端滚动缓冲区；banner 为首项） */}
      <Static items={[bannerMsg, ...history]}>
        {(item) => {
          if (item.kind === 'banner') {
            return (
              <Box key="banner" flexDirection="column" paddingBottom={1}>
                <Text bold color="magenta">◆ OpenAIDE</Text>
                <Text dimColor>everything is a plugin · model {modelId}</Text>
                <Text dimColor>/help commands · Ctrl+C exit</Text>
              </Box>
            );
          }
          return (
            <Box key={item.id} flexDirection="column">
              {(item.kind === 'user' || item.kind === 'assistant') && (
                <Text dimColor>{item.kind === 'user' ? '❯ you' : '● agent'}</Text>
              )}
              <Box paddingLeft={item.kind === 'user' || item.kind === 'assistant' ? 2 : 0}>
                {item.kind === 'user' ? (
                  <Text bold>{item.content}</Text>
                ) : item.kind === 'assistant' ? (
                  <Markdown text={item.content} />
                ) : item.kind === 'thinking' ? (
                  <Text dimColor italic>✻ {item.content}</Text>
                ) : item.kind === 'tool' ? (
                  <Text color="cyan">{item.content}</Text>
                ) : item.kind === 'error' ? (
                  <Text color="red">{item.content}</Text>
                ) : (
                  <Text color="yellow">{item.content}</Text>
                )}
              </Box>
            </Box>
          );
        }}
      </Static>

      {/* 流式输出（实时变化，不放 Static） */}
      {busy && streamingReasoning ? (
        <Box paddingLeft={2}>
          <Text dimColor italic>✻ {streamingReasoning}</Text>
        </Box>
      ) : null}
      {busy && streamingContent ? (
        <Box flexDirection="column" paddingLeft={2}>
          <Text dimColor>● agent</Text>
          <Text>{streamingContent}</Text>
        </Box>
      ) : null}

      {/* 补全弹层 */}
      {showSuggestions ? (
        <Box flexDirection="column" borderStyle="round" borderColor="gray" paddingX={1}>
          {suggestionMatch.map((c, i) => (
            <Text key={c.name + i} inverse={i === suggestIndex}>
              {`${c.name}${c.args ? ` ${c.args}` : ''}`.padEnd(30)}
              {c.desc}
            </Text>
          ))}
        </Box>
      ) : null}

      {/* 工具审批卡 */}
      {pendingApproval ? (
        <Box flexDirection="column" borderStyle="round" borderColor="yellow" paddingX={1}>
          <Text color="yellow" bold>
            ⚠ approve tool call?  [y] allow / [n] deny
          </Text>
          <Text color="cyan">{pendingApproval.tool}</Text>
          <Text dimColor>{(pendingApproval.args || '').slice(0, 300)}</Text>
        </Box>
      ) : null}

      {/* 输入框 */}
      <Box marginTop={1}>
        <Box
          borderStyle="round"
          borderColor={busy ? 'gray' : 'magenta'}
          paddingX={1}
          flexGrow={1}
        >
          <Text color="magenta">❯ </Text>
          <PasteInput
            busy={busy}
            value={input}
            onChange={setInput}
            onSubmit={(finalText) => void onSubmit(finalText)}
          />
        </Box>
      </Box>

      {/* 状态栏 + spinner */}
      <Box justifyContent="space-between" paddingX={1}>
        <Box>
          {busy ? (
            <>
              <Spinner type="dots" />
              <Text> </Text>
              <Text color="cyan">{phaseLabel}</Text>
            </>
          ) : (
            <Text dimColor>{phaseLabel}</Text>
          )}
        </Box>
        <Text dimColor>
          {modelId} · session {sessionShort} · plugins: {pluginCount} · tools: {toolCount}
        </Text>
      </Box>
    </Box>
  );
}

/** 启动 Ink TUI（非 TTY 环境由调用方降级，见 runTui） */
export async function runTui(app: App, initialSessionId?: string): Promise<void> {
  const { render } = await import('ink');
  const { waitUntilExit } = render(<Tui app={app} initialSessionId={initialSessionId} />);
  await waitUntilExit();
}
