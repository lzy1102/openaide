/**
 * 应用装配 —— 一切皆插件。
 * 内核只依赖接口；工具、人格、钩子全部经插件体系注入。
 */
import { AgentKernel, EventBus } from '@openaide/core';
import type { LLMProvider, ModelSwitcher, SessionStore } from '@openaide/core';
import { loadConfig, Config } from '@openaide/config';
import { OpenAICompatibleProvider } from '@openaide/llm';
import { PluginManager, PluginPersonaProvider } from '@openaide/plugins';
import { ToolRegistry, builtinToolsPlugin, fileToolsPlugin } from '@openaide/tools';
import { SQLiteSessionStore, SqliteMemory } from '@openaide/memory';
import { join } from 'node:path';
import { createApprovalInterceptor } from './approval.js';
import type { ApprovalHandler } from './approval.js';

export interface App {
  config: Config;
  kernel: AgentKernel;
  registry: ToolRegistry;
  /** 可热替换的 Provider 委托（config.llm.provider 指向插件注册的后端时整体切换） */
  llm: LLMProvider & ModelSwitcher;
  plugins: PluginManager;
  persona: PluginPersonaProvider;
  bus: EventBus;
  sessions: SessionStore;
  memory: SqliteMemory;
  /** 运行时切换激活人格(undefined = 回到内置默认 L0);立即生效于下一次查询 */
  setActivePersona(name: string | undefined): void;
  /** 当前激活人格名 */
  getActivePersona(): string | undefined;
  /** 注册审批 UI（TUI 卡片 / REPL 行内输入）；未注册时 fail-closed 默认拒绝 */
  setApprovalHandler(h: ApprovalHandler): void;
}

/** 装配完整应用：内置插件 + 用户插件动态加载 + 内核 */
export async function buildApp(config?: Config): Promise<App> {
  const cfg = config ?? loadConfig();

  const registry = new ToolRegistry();
  // 共享事件总线：内核发布 → 插件钩子订阅（同进程）
  const bus = new EventBus();

  // 会话存储:SQLite 持久化（重启不丢）
  const sessions = new SQLiteSessionStore(join(cfg.dataDir, 'sessions.db'));

  // 记忆存储:内核构建历史消息的来源(role 保留,跨轮上下文)
  const memory = new SqliteMemory(join(cfg.dataDir, 'memory.db'));

  // ── Provider 委托：先造默认实现占位，插件加载后按 config.llm.provider 热替换 ──
  let currentProvider: LLMProvider & Partial<ModelSwitcher> = new OpenAICompatibleProvider({
    baseUrl: cfg.llm.baseUrl,
    apiKey: cfg.llm.apiKey,
    model: cfg.llm.model,
    timeoutMs: cfg.llm.timeoutMs,
  });
  const llmDelegate: LLMProvider & ModelSwitcher = {
    chat: (m, t, o) => currentProvider.chat(m, t, o),
    chatStream: (m, t, o) => currentProvider.chatStream(m, t, o),
    getModelId: () => currentProvider.getModelId(),
    setModelId: (id) => currentProvider.setModelId?.(id),
  };

  // 受限 LLM 访问(插件上下文):只暴露 chat 与当前模型 ID
  const pluginLLM = {
    chat: async (messages: Array<{ role: string; content: string }>, options?: Record<string, unknown>) => {
      const resp = await llmDelegate.chat(messages as never[], [], options ?? {});
      return resp.content ?? '';
    },
    model: () => llmDelegate.getModelId(),
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

  // 内置插件（文件工具/命令工具，以插件形态；可经 plugin-state.json 禁用）
  if (plugins.isDisabled(builtinToolsPlugin.name)) {
    console.log(`[app] plugin disabled by state: ${builtinToolsPlugin.name}`);
  } else {
    await plugins.add(builtinToolsPlugin, process.cwd());
  }
  // 文件检索与精确编辑插件(grep_search / diff_edit)
  if (plugins.isDisabled(fileToolsPlugin.name)) {
    console.log(`[app] plugin disabled by state: ${fileToolsPlugin.name}`);
  } else {
    await plugins.add(fileToolsPlugin, process.cwd());
  }

  // 用户插件：动态加载（同进程，无子进程）
  const loadedUserPlugins = await plugins.loadAll();
  if (loadedUserPlugins.length > 0) {
    console.log(`[app] loaded user plugins: ${loadedUserPlugins.join(', ')}`);
  }

  // ── Provider 选择：config.llm.provider 指向插件注册的后端时整体切换 ──
  if (cfg.llm.provider && cfg.llm.provider !== 'openai-compatible') {
    const factory = plugins.getProvider(cfg.llm.provider);
    if (!factory) {
      throw new Error(
        `llm.provider '${cfg.llm.provider}' is not registered by any plugin (registered: ${plugins.providerNames().join(', ') || 'none'})`,
      );
    }
    currentProvider = factory.create({
      apiKey: cfg.llm.apiKey,
      model: cfg.llm.model,
      baseUrl: cfg.llm.baseUrl,
      timeoutMs: cfg.llm.timeoutMs,
    });
    console.log(`[app] llm provider: ${cfg.llm.provider} (model ${llmDelegate.getModelId()})`);
  }

  // 人格提供者（插件收集的 persona 供内核 L0 层使用）
  // 激活状态是可变的:/persona <name> 运行时切换,无需改配置重启
  const personaState: { active: string | undefined } = { active: cfg.kernel.persona };
  const persona = new PluginPersonaProvider(plugins, () => personaState.active);
  const setActivePersona = (name: string | undefined) => {
    personaState.active = name;
  };

  // ── 审批拦截器：dangerous/always 模式下工具执行前请求 UI 确认 ──
  const approvalMode = cfg.kernel.approval ?? 'off';
  let approvalHandler: ApprovalHandler = async () => false; // fail-closed：未接 UI 时默认拒绝
  if (approvalMode !== 'off') {
    const isDangerous = (tool: string): boolean =>
      registry
        .definitions()
        .some((d) => d.function.name === tool && d.function.description.startsWith('[dangerous'));
    plugins.interceptors.unshift(
      createApprovalInterceptor(approvalMode, {
        ask: (q) => approvalHandler(q),
        isDangerous,
      }),
    );
    console.log(`[app] tool approval: ${approvalMode} mode`);
  }

  // 内核
  const kernel = new AgentKernel({
    llm: llmDelegate,
    tools: registry,
    memory,
    sessions,
    persona,
    eventBus: bus,
    interceptors: plugins.interceptors, // 活数组引用：插件启停即时生效
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
    llm: llmDelegate,
    plugins,
    persona,
    bus,
    sessions,
    memory,
    setActivePersona,
    getActivePersona: () => personaState.active,
    setApprovalHandler: (h) => {
      approvalHandler = h;
    },
  };
}

/** 打印装配后的插件与工具清单（诊断用；含 disabled/failed 三态） */
export function printToolInventory(app: App): void {
  const defs = app.registry.definitions();
  const plugins = app.plugins.list();
  const activeCount = plugins.filter((p) => p.status === 'active').length;
  console.log(
    `[app] ${plugins.length} plugins (${activeCount} active), ${defs.length} tools registered:`,
  );

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
      const badge = p.status === 'active' ? '' : ` <${p.status}>`;
      const detail =
        p.status === 'failed'
          ? ` — error: ${p.error ?? 'unknown'}`
          : p.status === 'active'
            ? ''
            : ' — not loaded';
      console.log(
        `    ${p.name}@${p.version ?? '0.0.0'}${badge} — ${p.description ?? ''}${detail}`,
      );
      if (p.status === 'active') {
        if (p.tools.length > 0) console.log(`      tools: ${p.tools.join(', ')}`);
        if (p.persona) console.log('      persona: yes');
      }
    }
  }
  if (defs.length > 0) {
    console.log('  tools:');
    for (const d of defs) {
      console.log(`    - ${d.function.name}: ${d.function.description}`);
    }
  }
}
