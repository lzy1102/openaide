/**
 * 控制台 REPL —— 交互式使用入口（零重依赖，原生 readline）。
 * 能力：
 *  - 多轮对话共用同一会话（上下文延续）
 *  - 会话持久化：/sessions 列出、/use <id> 恢复历史会话
 *  - 命令历史：跨进程保存到 dataDir/history
 *  - 斜杠命令：/help /new /sessions /use /plugins /persona /exit
 */
import { createInterface } from 'node:readline';
import { stdin as input, stdout as output } from 'node:process';
import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { KernelEvent, EventTypes, StreamChunkType, newId } from '@openaide/core';
import type { App } from './app.js';

/** 一次查询的上下文（REPL 维护当前会话） */
export interface QueryContext {
  sessionId?: string;
  projectId?: string;
}

/** 一次查询的结果（One-shot 与 REPL 共用） */
export interface QueryResult {
  sessionId: string;
  /** 助手输出的纯文本（不含工具调用，供 --output json 使用） */
  content: string;
}

/**
 * 流式处理一条查询并输出（One-shot 与 REPL 共用）。
 * 返回实际使用的 sessionId 与累计的助手文本。
 */
export async function runQuery(app: App, content: string, ctx: QueryContext = {}): Promise<QueryResult> {
  const projectId = ctx.projectId ?? 'default';
  const sessionId = ctx.sessionId ?? newId();
  let full = '';
  process.stdout.write('\n');
  for await (const chunk of app.kernel.processStream(
    { sessionId, projectId, userId: 'local', content, options: {} },
  )) {
    switch (chunk.type) {
      case StreamChunkType.Thinking:
        if (chunk.reasoningContent) process.stdout.write(`\x1b[90m${chunk.reasoningContent}\x1b[0m`);
        break;
      case StreamChunkType.Content:
        if (chunk.content) {
          process.stdout.write(chunk.content);
          full += chunk.content;
        }
        break;
      case StreamChunkType.ToolCall:
        process.stdout.write(`\n\x1b[36m  ⚡ ${chunk.toolName}(${chunk.toolArgs})\x1b[0m\n`);
        break;
      case StreamChunkType.ToolDone:
        if (chunk.toolResult?.error) {
          process.stdout.write(`\x1b[31m  ✗ ${chunk.toolResult.error}\x1b[0m\n`);
        }
        break;
      case StreamChunkType.Error:
        process.stdout.write(`\x1b[31m[error] ${chunk.error?.message ?? 'unknown'}\x1b[0m\n`);
        break;
      default:
        break;
    }
  }
  process.stdout.write('\n\n');
  return { sessionId, content: full };
}

const HISTORY_LIMIT = 500;

function loadHistory(dataDir: string): string[] {
  const p = join(dataDir, 'history');
  if (!existsSync(p)) return [];
  try {
    return readFileSync(p, 'utf8').split('\n').filter(Boolean).slice(-HISTORY_LIMIT);
  } catch {
    return [];
  }
}

function saveHistory(dataDir: string, history: string[]): void {
  try {
    mkdirSync(dataDir, { recursive: true });
    writeFileSync(join(dataDir, 'history'), history.slice(-HISTORY_LIMIT).join('\n') + '\n');
  } catch {
    /* 历史文件写失败不阻塞使用 */
  }
}

const HELP = `
Commands:
  /help                  show this help
  /new                   start a new session (context reset)
  /sessions              list recent sessions
  /use <id>              resume a session by id
  /model <id>            switch model at runtime (no arg = show current)
  /plugins               list active plugins and tools
  /persona               list available personas
  /exit, /quit           quit
  anything else          send to the agent
`;

/** 交互式 REPL */
export async function runRepl(app: App, initialSessionId?: string): Promise<void> {
  console.log('\x1b[35mOpenAIDE\x1b[0m — everything is a plugin. Type /help for commands, /exit to quit.');
  if (initialSessionId) {
    console.log(`\x1b[36m  resuming session ${initialSessionId}\x1b[0m`);
  }

  const rl = createInterface({ input, output, historySize: HISTORY_LIMIT });
  // readline 的 history 属性在 @types/node 中未声明（运行时存在），此处类型断言访问
  const rlHistory = (rl as unknown as { history: string[] }).history;
  for (const line of loadHistory(app.config.dataDir)) rlHistory.push(line);

  // readline 回调版封装为 Promise
  const question = (prompt: string) => new Promise<string>((resolve) => rl.question(prompt, resolve));

  // 订阅内核事件：展示工具执行（供验证插件钩子）
  app.kernel.subscribe((event: KernelEvent) => {
    if (event.type === EventTypes.ToolCallStarted) {
      console.log(`\x1b[36m  ⚡ [hook] tool: ${String(event.data?.tool)} ⚡\x1b[0m`);
    }
  });

  // 当前会话：undefined 时每条消息新建；-c 续聊时用传入的 initialSessionId
  let sessionId: string | undefined = initialSessionId;

  for (;;) {
    const line = (await question('> ')).trim();
    if (!line) continue;

    if (line.startsWith('/')) {
      const [cmd, arg] = line.split(/\s+/, 2);
      switch (cmd) {
        case '/exit':
        case '/quit':
          saveHistory(app.config.dataDir, rlHistory);
          rl.close();
          console.log('bye.');
          return;
        case '/help':
          console.log(HELP);
          continue;
        case '/new':
          sessionId = undefined;
          console.log('  new session started.');
          continue;
        case '/model':
          if (!arg) {
            console.log(`  current model: ${app.llm.getModelId()}`);
            console.log('  usage: /model <model-id>');
          } else {
            app.llm.setModelId(arg);
            console.log(`  model switched to: ${arg}`);
          }
          continue;
        case '/sessions': {
          const sessions = await app.sessions.list();
          if (sessions.length === 0) {
            console.log('  (no sessions yet)');
          } else {
            console.log('  recent sessions:');
            for (const s of sessions.slice(0, 10)) {
              const mark = s.id === sessionId ? ' *' : '  ';
              console.log(
                `   ${mark} ${s.id}  project=${s.projectId}  msgs=${s.messages.length}  ${new Date(s.updatedAt).toISOString()}`,
              );
            }
          }
          continue;
        }
        case '/use': {
          if (!arg) {
            console.log('  usage: /use <session-id>');
            continue;
          }
          const s = await app.sessions.get(arg);
          if (!s) {
            console.log(`  session not found: ${arg}`);
          } else {
            sessionId = s.id;
            console.log(`  resumed session ${s.id} (${s.messages.length} messages).`);
          }
          continue;
        }
        case '/plugins':
          console.log(`  active plugins: ${app.plugins.names().join(', ') || '(none)'}`);
          console.log(`  tools: ${app.registry.definitions().map((d) => d.function.name).join(', ')}`);
          continue;
        case '/persona': {
          const personas = app.plugins.getPersonas().map((p) => p.name);
          console.log(`  available personas: ${personas.join(', ') || '(builtin only)'}`);
          continue;
        }
        default:
          console.log(`  unknown command: ${cmd}  (try /help)`);
          continue;
      }
    }

    const r = await runQuery(app, line, { sessionId });
    sessionId = r.sessionId;
  }
}
