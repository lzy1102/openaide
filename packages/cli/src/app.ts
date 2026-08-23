/**
 * 应用装配 —— 一切皆插件。
 * 内核只依赖接口；工具、人格、钩子全部经插件体系注入。
 */
import { AgentKernel, EventBus } from '@openaide/core';
import type { SessionStore } from '@openaide/core';
import { loadConfig, Config } from '@openaide/config';
import { OpenAICompatibleProvider } from '@openaide/llm';
import { PluginManager, PluginPersonaProvider } from '@openaide/plugins';
import { ToolRegistry, builtinToolsPlugin, fileToolsPlugin } from '@openaide/tools';
import { SQLiteSessionStore, SqliteMemory } from '@openaide/memory';
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
  memory: SqliteMemory;
  /** 运行时切换激活人格(undefined = 回到内置默认 L0);立即生效于下一次查询 */
  setActivePersona(name: string | undefined): void;
  /** 当前激活人格名 */
  getActivePersona(): string | undefined;
}

/** 装配完整应用：内置插件 + 用户插件动态加载 + 内核 */
export async function buildApp(config?: Config): Promise<App> {
  const cfg = config ?? loadConfig();

  const registry = new ToolRegistry();
  // 共享事件总线：内核发布 → 插件钩子订阅（同进程）
  const bus = new EventBus();

  // LLM 网关
  const llm = new OpenAICompatibleProvider({
    baseUrl: cfg.llm.baseUrl,
    apiKey: cfg.llm.apiKey,
    model: cfg.llm.model,
    timeoutMs: cfg.llm.timeoutMs,
  });

  // 会话存储:SQLite 持久化（重启不丢）
  const sessions = new SQLiteSessionStore(join(cfg.dataDir, 'sessions.db'));

  // 记忆存储:内核构建历史消息的来源(role 保留,跨轮上下文)
  const memory = new SqliteMemory(join(cfg.dataDir, 'memory.db'));

  // 受限 LLM 访问(插件上下文):只暴露 chat 与当前模型 ID
  const pluginLLM = {
    chat: async (messages: Array<{ role: string; content: string }>, options?: Record<string, unknown>) => {
      const resp = await llm.chat(messages as never[], [], options ?? {});
      return resp.content ?? '';
    },
    model: () => llm.getModelId(),
  };

  // 只读会话访问(插件上下文):不含消息正文之外的写通道
  const pluginSessions = {
    get: async (sessionId: string) => {
      const s = await sessions.get(sessionId);
      return s
        ? { id: s.id, messages: s.messages.map((m) => ({ role: m.role, content: m.content })) }
        : undefined;
    },
    list: async (projectId?: string) =>
      (await sessions.list(projectId)).map((s) => ({
        id: s.id,
        projectId: s.projectId,
        messageCount: s.messages.length,
        updatedAt: s.updatedAt,
      })),
  };

  // 插件管理器（统一接入：内置与用户插件同一套注册逻辑）
  const plugins = new PluginManager({
    pluginsDir: cfg.pluginsDir,
    dataDir: cfg.dataDir,
    executor: registry,
    eventBus: bus,
    autoActivate: false,
    llm: pluginLLM,
    sessions: pluginSessions,
  });
  // 进度上报走共享事件总线(context.progress)
  plugins.reportProgress = (note: string, percent?: number) => {
    bus.publish({
      type: 'context.progress',
      source: 'plugin',
      data: { note, percent },
      timestamp: Date.now(),
    });
  };

  // 内置插件（文件工具/命令工具，以插件形态）
  await plugins.add(builtinToolsPlugin, process.cwd());
  // 文件检索与精确编辑插件(grep_search / diff_edit)
  await plugins.add(fileToolsPlugin, process.cwd());

  // 用户插件：动态加载（同进程，无子进程）
  const loadedUserPlugins = await plugins.loadAll();
  if (loadedUserPlugins.length > 0) {
    console.log(`[app] loaded user plugins: ${loadedUserPlugins.join(', ')}`);
  }

  // 人格提供者（插件收集的 persona 供内核 L0 层使用）
  // 激活状态是可变的:/persona <name> 运行时切换,无需改配置重启
  const personaState: { active: string | undefined } = { active: cfg.kernel.persona };
  const persona = new PluginPersonaProvider(plugins, () => personaState.active);
  const setActivePersona = (name: string | undefined) => {
    personaState.active = name;
  };

  // 内核
  const kernel = new AgentKernel({
    llm,
    tools: registry,
    memory,
    sessions,
    persona,
    eventBus: bus,
    config: {
      maxRounds: cfg.kernel.maxRounds,
      maxTokens: cfg.kernel.maxTokens,
      systemPrompt: cfg.kernel.systemPrompt,
    },
  });

  return {
    config: cfg,
    kernel,
    registry,
    llm,
    plugins,
    persona,
    bus,
    sessions,
    memory,
    setActivePersona,
    getActivePersona: () => personaState.active,
  };
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
