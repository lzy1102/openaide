/**
 * 应用装配 —— 一切皆插件。
 * 内核只依赖接口；工具、人格、钩子全部经插件体系注入。
 */
import { AgentKernel, EventBus } from '@openaide/core';
import type { LLMProvider, Memory, ModelSwitcher, SessionStore } from '@openaide/core';
import { loadConfig, Config, resolveIdentity, resolveProjectWorkspace } from '@openaide/config';
import { OpenAICompatibleProvider } from '@openaide/llm';
import { PluginManager, PluginPersonaProvider, builtinPersonasPlugin } from '@openaide/plugins';
import { closeBrowser, ToolRegistry, builtinToolsPlugin, fileToolsPlugin, webSearchPlugin, browserPlugin } from '@openaide/tools';
import { resolveStores } from '@openaide/memory';
import { SQLiteSessionStore, SqliteMemory } from '@openaide/memory';
import { createMcpBridgePlugin, loadClaudeMcpConfig } from '@openaide/mcp';
import { join } from 'node:path';
import { builtinUiPlugins } from './builtin-uis.js';
import { createApprovalInterceptor } from './approval.js';
import { createSubAgentsPlugin } from './subagents.js';
import { createKnowledgeInterceptor, syncSessions } from './workspace.js';
import type { ApprovalHandler } from './approval.js';

export interface App {
  config: Config;
  /** 项目工作区目录（workspace off 时为 undefined） */
  workspaceDir?: string;
  /** 当前开发者身份（会话子目录名） */
  identity?: string;
  /** 手动触发一次会话同步（sessionSync 策略）；返回描述文本 */
  syncNow?(): Promise<string>;
  /** 每轮对话结束后由 UI 调用（内部 30s 防抖） */
  autoSyncSessions?(): void;
  /** 退出前等待未完成的同步落盘 */
  dispose?(): Promise<void>;
  kernel: AgentKernel;
  registry: ToolRegistry;
  /** 可热替换的 Provider 委托（config.llm.provider 指向插件注册的后端时整体切换） */
  llm: LLMProvider & ModelSwitcher;
  plugins: PluginManager;
  persona: PluginPersonaProvider;
  bus: EventBus;
  sessions: SessionStore;
  memory: Memory;
  /** 运行时切换激活人格(undefined = 回到内置默认 L0);立即生效于下一次查询 */
  setActivePersona(name: string | undefined): void;
  /** 当前激活人格名 */
  getActivePersona(): string | undefined;
  /** 注册审批 UI（TUI 卡片 / REPL 行内输入）；未注册时 fail-closed 默认拒绝 */
  setApprovalHandler(h: ApprovalHandler): void;
}

/** 装配期元数据（buildApp 返回前挂到 App） */
const appMeta: {
  workspaceDir?: string;
  identity?: string;
  sync?: () => Promise<string>;
  autoSync?: () => void;
  lastSync?: Promise<string>;
} = {};

/** 装配完整应用：内置插件 + 用户插件动态加载 + 内核 */
export async function buildApp(config?: Config): Promise<App> {
  const cfg = config ?? loadConfig();

  const registry = new ToolRegistry();
  // 共享事件总线：内核发布 → 插件钩子订阅（同进程）
  const bus = new EventBus();

  // ── 存储：会话按开发者身份隔离在 <项目>/.openaide/ 内（git 同步即跨机续聊）。
  //    workspace: off 或家目录护栏命中 → 退回全局 SQLite（脚本/CI 场景）。
  let sessions: SessionStore;
  let memory: Memory;
  const wsMode = process.env.OPENAIDE_WORKSPACE ?? cfg.workspace ?? 'on';
  if (wsMode === 'off') {
    sessions = new SQLiteSessionStore(join(cfg.dataDir, 'sessions.db'));
    memory = new SqliteMemory(join(cfg.dataDir, 'memory.db'));
    console.log('[app] workspace off — global sqlite storage');
  } else {
    const workspace = resolveProjectWorkspace();
    const identityInfo = resolveIdentity(workspace);
    const stores = resolveStores(workspace, identityInfo.name);
    sessions = stores.sessions;
    memory = stores.memory;
    appMeta.workspaceDir = workspace;
    appMeta.identity = identityInfo.name;
    console.log(
      `[app] workspace ${workspace} · identity ${identityInfo.name} (${identityInfo.from}) — sessions travel with the repo`,
    );
  }

  // ── Provider 委托：先造默认实现占位，插件加载后按 config.llm.provider 热替换 ──
  let currentProvider: LLMProvider & Partial<ModelSwitcher> = new OpenAICompatibleProvider({
    baseUrl: cfg.llm.baseUrl,
    apiKey: cfg.llm.apiKey,
    model: cfg.llm.model,
    timeoutMs: cfg.llm.timeoutMs,
    reasoningEffort: cfg.llm.reasoningEffort,
    retries: cfg.llm.retries,
    retryDelayMs: cfg.llm.retryDelayMs,
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
  // 联网搜索（Tavily/Brave/SearXNG 后端按环境变量自动选择）
  if (plugins.isDisabled(webSearchPlugin.name)) {
    console.log(`[app] plugin disabled by state: ${webSearchPlugin.name}`);
  } else if (process.env.TAVILY_API_KEY || process.env.BRAVE_API_KEY || process.env.SEARXNG_URL) {
    await plugins.add(webSearchPlugin, process.cwd());
  }

  // 浏览器操作（需 npm i -D playwright；未装时调用会返回安装指引）
  if (plugins.isDisabled(browserPlugin.name)) {
    console.log(`[app] plugin disabled by state: ${browserPlugin.name}`);
  } else {
    await plugins.add(browserPlugin, process.cwd());
  }
  // 内置人格包（coder/architect）——提示词内容住插件，不住内核
  if (plugins.isDisabled(builtinPersonasPlugin.name)) {
    console.log(`[app] plugin disabled by state: ${builtinPersonasPlugin.name}`);
  } else {
    await plugins.add(builtinPersonasPlugin, process.cwd());
  }

  // MCP 桥（配置了 mcp_servers 或 mcp_config_file 时启用；Claude 配置可直接导入）
  const mcpFile = process.env.OPENAIDE_MCP_CONFIG ?? (cfg as { mcp_config_file?: string }).mcp_config_file;
  const mcpServers: Record<string, NonNullable<typeof cfg.mcpServers>[string]> = {
    ...(cfg.mcpServers ?? {}),
    ...(mcpFile ? loadClaudeMcpConfig(mcpFile) : {}),
  };
  if (Object.keys(mcpServers).length > 0) {
    const bridge = await createMcpBridgePlugin({ servers: mcpServers });
    await plugins.add(bridge, process.cwd());
  }

  // 内置界面（ink TUI / readline）——同样可被 plugin-state 禁用或被自定义 UI 插件取代
  for (const uiPlugin of builtinUiPlugins) {
    if (plugins.isDisabled(uiPlugin.name)) {
      console.log(`[app] plugin disabled by state: ${uiPlugin.name}`);
    } else {
      await plugins.add(uiPlugin, process.cwd());
    }
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
  // 缺省激活 coder（内置人格包提供）——保持"开箱即编码助手"的行为；
  // /persona default 回到内核极简安全底线（不再是完整 coder 提示词）
  const personaState: { active: string | undefined } = {
    active: cfg.kernel.persona ?? (plugins.getPersona('coder') ? 'coder' : undefined),
  };
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

  // 团队知识注入（knowledge/*.md → L1），常驻拦截链
  if (appMeta.workspaceDir) {
    plugins.interceptors.push(createKnowledgeInterceptor(appMeta.workspaceDir));
  }

  // 子代理编排（config.kernel.subagents 声明；人格懒解析，用户插件人格同样可用）
  const subSpecs = cfg.kernel.subagents ?? [];
  if (subSpecs.length > 0) {
    const plugin = createSubAgentsPlugin(subSpecs, {
      llm: llmDelegate,
      registry,
      getPersona: (n) => plugins.getPersona(n),
      eventBus: bus,
      maxRounds: cfg.kernel.maxRounds,
    });
    if (plugin) await plugins.add(plugin, process.cwd());
  }

  // 会话自动同步（sessionSync 策略；workspace off 时不启用）
  if (appMeta.workspaceDir) {
    const mode = cfg.sessionSync ?? 'commit';
    let lastAt = 0;
    appMeta.sync = async () => {
      const r = syncSessions(appMeta.workspaceDir!, { name: appMeta.identity! }, mode);
      if (r.status === 'failed') return `sync failed: ${r.reason}`;
      if (r.status === 'noop') return 'already up to date';
      return `synced (commit: ${r.committed}, push: ${r.pushed})`;
    };
    let inFlight = false;
    appMeta.autoSync = () => {
      const now = Date.now();
      if (now - lastAt < 30_000 || inFlight) return; // 防抖 + 重入保护
      lastAt = now;
      inFlight = true;
      appMeta.lastSync = appMeta
        .sync!()
        .catch(() => 'sync failed')
        .finally(() => {
          inFlight = false;
        });
    };
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
    workspaceDir: appMeta.workspaceDir,
    identity: appMeta.identity,
    syncNow: async () => {
      if (!appMeta.sync) return 'sync unavailable';
      return appMeta.sync();
    },
    autoSyncSessions: () => appMeta.autoSync?.(),
    dispose: async () => {
      await appMeta.lastSync;
      await closeBrowser();
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
