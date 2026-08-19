/**
 * 应用装配 —— 一切皆插件。
 * 内核只依赖接口；工具、人格、钩子全部经插件体系注入。
 */
import { AgentKernel, EventBus } from '@openaide/core';
import type { SessionStore } from '@openaide/core';
import { loadConfig, Config } from '@openaide/config';
import { OpenAICompatibleProvider } from '@openaide/llm';
import { PluginManager, PluginPersonaProvider } from '@openaide/plugins';
import { ToolRegistry, builtinToolsPlugin } from '@openaide/tools';
import { SQLiteSessionStore } from '@openaide/memory';
import { join } from 'node:path';

export interface App {
  config: Config;
  kernel: AgentKernel;
  registry: ToolRegistry;
  llm: OpenAICompatibleProvider;
  plugins: PluginManager;
  persona: PluginPersonaProvider;
  bus: EventBus;
  sessions: SessionStore;
}

/** 装配完整应用：内置插件 + 用户插件动态加载 + 内核 */
export async function buildApp(config?: Config): Promise<App> {
  const cfg = config ?? loadConfig();

  const registry = new ToolRegistry();
  // 共享事件总线：内核发布 → 插件钩子订阅（同进程）
  const bus = new EventBus();

  // 插件管理器（统一接入：内置与用户插件同一套注册逻辑）
  const plugins = new PluginManager({
    pluginsDir: cfg.pluginsDir,
    dataDir: cfg.dataDir,
    executor: registry,
    eventBus: bus,
    autoActivate: false,
  });

  // 内置插件（文件工具/命令工具，以插件形态）
  await plugins.add(builtinToolsPlugin, process.cwd());

  // 用户插件：动态加载（同进程，无子进程）
  const loadedUserPlugins = await plugins.loadAll();
  if (loadedUserPlugins.length > 0) {
    console.log(`[app] loaded user plugins: ${loadedUserPlugins.join(', ')}`);
  }

  // LLM 网关
  const llm = new OpenAICompatibleProvider({
    baseUrl: cfg.llm.baseUrl,
    apiKey: cfg.llm.apiKey,
    model: cfg.llm.model,
    timeoutMs: cfg.llm.timeoutMs,
  });

  // 会话存储：SQLite 持久化（重启不丢）
  const sessions = new SQLiteSessionStore(join(cfg.dataDir, 'sessions.db'));

  // 人格提供者（插件收集的 persona 供内核 L0 层使用）
  const persona = new PluginPersonaProvider(plugins, () => cfg.kernel.persona);

  // 内核
  const kernel = new AgentKernel({
    llm,
    tools: registry,
    sessions,
    persona,
    eventBus: bus,
    config: {
      maxRounds: cfg.kernel.maxRounds,
      maxTokens: cfg.kernel.maxTokens,
      systemPrompt: cfg.kernel.systemPrompt,
    },
  });

  return { config: cfg, kernel, registry, llm, plugins, persona, bus, sessions };
}

/** 打印装配后的工具清单（诊断用） */
export function printToolInventory(app: App): void {
  const defs = app.registry.definitions();
  const plugins = app.plugins.list();
  console.log(`[app] ${plugins.length} plugins, ${defs.length} tools registered:`);

  // 按分类分组（保持加载顺序）
  const groups = new Map<string, typeof plugins>();
  for (const p of plugins) {
    const key = p.category;
    const arr = groups.get(key) ?? [];
    arr.push(p);
    groups.set(key, arr);
  }
  for (const [category, list] of groups) {
    console.log(`  [${category}]`);
    for (const p of list) {
      console.log(
        `    ${p.name}@${p.version ?? '0.0.0'} — ${p.description ?? ''} (tools: ${p.tools.length}, hooks: ${p.hooks}, persona: ${p.persona ? 'yes' : 'no'})`,
      );
    }
  }
  console.log('  tools:');
  for (const d of defs) {
    console.log(`    - ${d.function.name}: ${d.function.description}`);
  }
}
