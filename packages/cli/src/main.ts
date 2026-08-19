/**
 * OpenAIDE 入口 —— TS 版。
 * 用法：
 *   openaide "query"     一次性问答
 *   openaide repl        交互式 REPL
 *   openaide plugins     列出已加载插件与工具
 *   openaide sessions    列出已持久化的历史会话
 *   openaide setup       配置向导（写入 ~/.openaide/config.yaml）
 *   openaide --version   版本
 *   openaide --help      帮助
 */
import { buildApp, printToolInventory } from './app.js';
import type { App } from './app.js';
import { runQuery, runRepl } from './repl.js';
import { runTui } from './tui.js';
import { runServe } from './serve.js';
import { loadConfig, saveConfig } from '@openaide/config';
import { readFileSync } from 'node:fs';

const version = (
  JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')) as { version: string }
).version ?? '0.0.0';

const HELP = `
OpenAIDE — everything is a plugin.

Usage:
  openaide <query>        one-shot: run a single query
  openaide repl           interactive REPL (default)
  openaide plugins        list loaded plugins and tools
  openaide sessions       list persisted sessions
  openaide serve          start HTTP/WS API server (OPENAIDE_PORT, default 8080)
  openaide setup          config wizard
  openaide --version      print version
  openaide --help         show this help
`;

async function setup(): Promise<void> {
  const config = loadConfig();
  console.log('OpenAIDE setup');
  console.log(`  data dir:  ${config.dataDir}`);
  console.log(`  plugins:   ${config.pluginsDir}`);
  const baseUrl = process.env.OPENAIDE_BASE_URL ?? config.llm.baseUrl;
  const model = process.env.OPENAIDE_MODEL ?? config.llm.model;
  const apiKey = process.env.OPENAIDE_API_KEY ?? config.llm.apiKey;
  config.llm.baseUrl = baseUrl;
  config.llm.model = model;
  config.llm.apiKey = apiKey;
  saveConfig(config);
  console.log(`Config written to ${config.dataDir}/config.yaml`);
  if (!apiKey) {
    console.log('\nNote: no API key set. Set OPENAIDE_API_KEY env or edit the config file.');
  }
}

/** 交互式入口：TTY 用 Ink TUI，非 TTY（管道/脚本）降级为 readline REPL */
async function runInteractive(app: App): Promise<void> {
  if (process.stdin.isTTY && process.stdout.isTTY) {
    await runTui(app);
  } else {
    await runRepl(app);
  }
}

async function main(): Promise<void> {
  const args = process.argv.slice(2);
  const cmd = args[0];

  if (cmd === '--version' || cmd === '-v') {
    console.log(`openaide ${version}`);
    return;
  }
  if (cmd === '--help' || cmd === '-h' || cmd === 'help') {
    console.log(HELP);
    return;
  }
  if (cmd === 'setup') {
    await setup();
    return;
  }

  const app = await buildApp();

  if (cmd === 'plugins') {
    printToolInventory(app);
    return;
  }
  if (cmd === 'sessions') {
    const sessions = await app.sessions.list();
    if (sessions.length === 0) {
      console.log('(no sessions yet)');
    } else {
      console.log(`${sessions.length} session(s):`);
      for (const s of sessions.slice(0, 20)) {
        console.log(
          `  ${s.id}  project=${s.projectId}  msgs=${s.messages.length}  updated=${new Date(s.updatedAt).toISOString()}`,
        );
      }
    }
    return;
  }
  if (cmd === 'repl') {
    await runInteractive(app);
    return;
  }
  if (cmd === 'serve') {
    await runServe(app);
    return;
  }

  // one-shot：以首个非命令参数作为查询
  if (cmd && !cmd.startsWith('-')) {
    await runQuery(app, cmd);
    return;
  }

  // 默认：交互式（TTY → Ink TUI，非 TTY → readline）
  await runInteractive(app);
}

main().catch((err) => {
  console.error('[fatal]', err);
  process.exit(1);
});
