/**
 * OpenAIDE 入口 —— TS 版。
 * 用法：
 *   openaide "query"            一次性问答
 *   openaide file.go "prompt"   文件作为上下文 + 问答
 *   openaide repl               交互式 REPL（默认）
 *   openaide -c                 恢复最近一次会话续聊
 *   openaide plugins            列出全部插件（三态）与工具；search/install/uninstall 管理市场插件
 *   openaide sessions           列出已持久化的历史会话
 *   openaide serve              启动 HTTP/WS API 服务
 *   openaide setup              配置向导
 *   openaide --model <id>       运行时指定模型（可配子命令/问答）
 *   openaide --output json      一次性问答输出 JSON
 *   openaide --version          版本
 *   openaide --help             帮助
 */
import { buildApp, printToolInventory } from './app.js';
import type { App } from './app.js';
import { runQuery } from './repl.js';
import { chooseUi } from './ui-select.js';
import { runServe } from './serve.js';
import { loadConfig, saveConfig } from '@openaide/config';
import {
  DEFAULT_REGISTRY_URL,
  fetchRegistry,
  GITHUB_PLUGIN_TOPIC,
  installEntry,
  readPluginState,
  searchEverywhere,
  writePluginState,
} from '@openaide/plugins';
import { existsSync, readFileSync, rmSync } from 'node:fs';
import { parseArgs, buildPrompt } from './args.js';

// 版本号：优先从 package.json 读取（源码/npm 形态）；二进制形态下虚拟路径不存在，
// 回退到编译期由 `bun build --define` 注入的常量（见 Makefile binary 目标）
declare const OPENAIDE_VERSION: string | undefined;
const version = (() => {
  try {
    return (
      (JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')) as { version?: string })
        .version ?? '0.0.0'
    );
  } catch {
    return typeof OPENAIDE_VERSION !== 'undefined' ? OPENAIDE_VERSION : '0.0.0';
  }
})();

const HELP = `
OpenAIDE — everything is a plugin.

Usage:
  openaide <query>        one-shot: run a single query
  openaide <file...> <query>
                          include files as context, then query
  openaide repl           interactive REPL (default)
  openaide -c             continue the most recent session
  openaide plugins        list all plugins (active/disabled/failed) + tools
  openaide plugins search <keyword>
                          search the plugin registry
  openaide plugins install <name>
                          install a plugin from the registry (git-based)
  openaide plugins uninstall <name>
                          remove an installed plugin
  openaide plugins enable|disable <name>
                          toggle a plugin (persisted to plugin-state.json)
  openaide plugins reload <name>
                          hot-reload a plugin (bust module cache)
  openaide sessions       list persisted sessions
  openaide serve          start HTTP/WS API server (OPENAIDE_PORT, default 8080)
  openaide init           create .openaide/ workspace — sessions travel with the repo
  openaide setup          config wizard
  openaide --model <id>   override model (works with any command)
  openaide --output json  one-shot output as JSON
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

/**
 * 交互式入口：界面即插件。
 * 选择优先级：OPENAIDE_UI / config.ui 显式指定 > TTY 默认（ink）/ 非 TTY（readline）> 回退链。
 * 内置 ui-ink / ui-readline 与用户自研界面插件走同一注册表（plugin.uis）。
 */
async function runInteractive(app: App, initialSessionId?: string): Promise<void> {
  const wanted = process.env.OPENAIDE_UI ?? app.config.ui;
  const isTTY = Boolean(process.stdin.isTTY && process.stdout.isTTY);
  const pick = chooseUi({ available: app.plugins.uiNames(), wanted, isTTY });
  if (pick.warning) console.warn(`[ui] ${pick.warning}`);
  if (!pick.name) throw new Error('no UI plugin registered (need ui-ink or a custom ui plugin)');
  if (initialSessionId) (app as { initialSessionId?: string }).initialSessionId = initialSessionId;
  const ui = app.plugins.getUi(pick.name)!;
  await ui.start(app as never);
}

async function main(): Promise<void> {
  const cli = parseArgs(process.argv.slice(2));
  const cmd = cli.cmd;

  if (cmd === '--help' || cmd === '-h' || cmd === 'help') {
    console.log(HELP);
    return;
  }
  if (cmd === '--version' || cmd === '-v' || cmd === 'version') {
    console.log(`openaide ${version}`);
    return;
  }
  if (cmd === 'setup') {
    await setup();
    return;
  }

  // init：在当前目录创建项目工作区（.openaide/）——会话随仓库走，换机器 git pull 后无缝续聊
  if (cmd === 'init') {
    const { mkdirSync, writeFileSync, existsSync } = await import('node:fs');
    const { join } = await import('node:path');
    const dir = join(process.cwd(), '.openaide');
    mkdirSync(dir, { recursive: true });
    const keep = join(dir, 'README.md');
    if (!existsSync(keep)) {
      writeFileSync(
        keep,
        [
          '# OpenAIDE 项目工作区',
          '',
          '本目录存放该项目的 agent 会话（sessions/）与跨轮记忆（memory/）。',
          '**建议提交进 git**——换一台电脑 clone/pull 后，`openaide -c` 即可继续之前的对话。',
          '',
          '敏感提示：会话内容会进入版本历史；若对话涉密请将本目录加入 .gitignore。',
        ].join('\n'),
        'utf8',
      );
    }
    console.log(`[init] project workspace ready: ${dir}`);
    console.log('[init] commit it to git — sessions now travel with the repo.');
    console.log('[init] next machine: git pull && openaide -c');
    return;
  }

  // --model 覆盖：加载配置并改模型
  const cfg = loadConfig();
  if (cli.model) cfg.llm.model = cli.model;

  const app = await buildApp(cfg);

  // -c / --continue：恢复最近一次会话并进入交互（续聊）
  if (cmd === '-c' || cmd === '--continue') {
    const sessions = await app.sessions.list();
    const last = sessions[0];
    if (last) {
      console.log(`\x1b[36m[resume] continuing session ${last.id} (${last.messages.length} messages)\x1b[0m`);
      await runInteractive(app, last.id);
    } else {
      console.log('(no previous session — starting a new one)');
      await runInteractive(app);
    }
    return;
  }

  if (cmd === 'plugins') {
    const [sub, name] = cli.pluginArgs;
    if (!sub || sub === 'list') {
      printToolInventory(app);
      return;
    }
    if (sub === 'enable') {
      if (!name) {
        console.log('usage: openaide plugins enable <name>');
        return;
      }
      const ok = await app.plugins.enable(name);
      console.log(ok ? `[plugins] enabled ${name}` : `[plugins] not found (or directory missing): ${name}`);
      return;
    }
    if (sub === 'disable') {
      if (!name) {
        console.log('usage: openaide plugins disable <name>');
        return;
      }
      await app.plugins.disable(name);
      console.log(`[plugins] disabled ${name} (unloaded now; skipped on startup)`);
      return;
    }
    if (sub === 'search') {
      const kw = cli.pluginArgs.slice(1).join(' ');
      const hits = await searchEverywhere(kw, { registryUrl: cfg.registryUrl });
      if (hits.length === 0) {
        console.log(`[plugins] no matches on GitHub (topic:${GITHUB_PLUGIN_TOPIC}) or registry${kw ? ` for "${kw}"` : ''}`);
        return;
      }
      console.log(`[plugins] ${hits.length} match(es) — GitHub topic:${GITHUB_PLUGIN_TOPIC} + registry:`);
      for (const e of hits) {
        const badge = e.from.includes('github') ? `[gh★${e.stars ?? 0}]` : '[registry]';
        console.log(
          `  ${badge} ${e.name}${e.version ? `@${e.version}` : ''}${e.category ? ` [${e.category}]` : ''} — ${e.description ?? ''}${e.author ? ` (by ${e.author})` : ''}`,
        );
      }
      console.log('publish: create a GitHub repo with topic openaide-plugin · install: openaide plugins install <name>');
      return;
    }
    if (sub === 'install') {
      if (!name) {
        console.log('usage: openaide plugins install <name>');
        return;
      }
      const reg = await fetchRegistry(cfg.registryUrl ?? DEFAULT_REGISTRY_URL);
      const entry = reg.plugins.find((p) => p.name === name);
      if (!entry) {
        console.log(`[plugins] not in registry: ${name}  (try: openaide plugins search ${name})`);
        return;
      }
      console.log(`[plugins] installing ${entry.name}@${entry.version ?? '?'} from ${entry.source.url}...`);
      const res = await installEntry(entry, { pluginsDir: cfg.pluginsDir });
      await app.plugins.load(res.dir);
      const info = app.plugins.list().find((p) => p.name === res.name);
      console.log(
        `[plugins] installed ${res.name} → ${res.dir} (tools: ${info?.tools.length ?? 0}, persona: ${info?.persona ? 'yes' : 'no'})`,
      );
      return;
    }
    if (sub === 'uninstall') {
      if (!name) {
        console.log('usage: openaide plugins uninstall <name>');
        return;
      }
      const dir = app.plugins.dirOf(name);
      if (!dir || !existsSync(dir)) {
        console.log(`[plugins] not installed: ${name}`);
        return;
      }
      await app.plugins.unload(name);
      rmSync(dir, { recursive: true, force: true });
      // 若在禁用名单里，一并清除（目录已删，记录已无意义）
      const st = readPluginState(cfg.dataDir);
      if (st.disabled.includes(name)) {
        st.disabled = st.disabled.filter((n) => n !== name);
        writePluginState(cfg.dataDir, st);
      }
      console.log(`[plugins] uninstalled ${name} (removed ${dir})`);
      return;
    }
    if (sub === 'reload') {
      if (!name) {
        console.log('usage: openaide plugins reload <name>');
        return;
      }
      try {
        await app.plugins.reload(name);
        console.log(`[plugins] reloaded ${name}`);
      } catch (err) {
        console.error(`[plugins] reload failed: ${(err as Error).message}`);
      }
      return;
    }
    console.log(
      `unknown plugins subcommand: ${sub}  (list | search <kw> | install <name> | uninstall <name> | enable/disable/reload <name>)`,
    );
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

  // one-shot：有 prompt 或上下文文件即执行
  if (cli.prompt || cli.contextFiles.length > 0) {
    const content = buildPrompt(cli.contextFiles, cli.prompt);
    const r = await runQuery(app, content);
    if (cli.outputJson) {
      console.log(JSON.stringify({ content: r.content, sessionId: r.sessionId }));
    }
    return;
  }

  // 默认：交互式（TTY → Ink TUI，非 TTY → readline）
  await runInteractive(app);
}

main().catch((err) => {
  console.error('[fatal]', err);
  process.exit(1);
});
