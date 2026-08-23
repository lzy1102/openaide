/**
 * 插件管理器 —— 统一接入内核。
 * 职责：加载/激活插件 → 注册工具（加 <插件>__ 前缀）→ 挂载钩子 → 收集人格 → 热重载。
 * 全部同进程运行，无子进程。
 */
import {
  EventBus,
  Interceptor,
  KernelEvent,
  LLMResponse,
  Message,
  Persona,
  ToolDefinition,
  ToolExecutor,
} from '@openaide/core';
import { basename } from 'node:path';
import type { PluginLLM, PluginProvider, PluginSessions, ProgressReporter } from './types.js';
import { discover, loadPlugin, loadManifest, readPersonaFile } from './loader.js';
import { readPluginState, writePluginState } from './state.js';
import type {
  LoadedPlugin,
  OpenAIDePlugin,
  PluginContext,
  PluginHook,
  PluginInfo,
  PluginManifest,
} from './types.js';

export interface PluginManagerOptions {
  /** 插件目录 */
  pluginsDir: string;
  /** 应用数据目录（注入插件上下文） */
  dataDir?: string;
  /** 工具注册器（内核的 ToolExecutor） */
  executor?: ToolExecutor;
  /** 事件总线（插件钩子订阅目标，如未传则不接钩子） */
  eventBus?: EventBus;
  /** 加载后自动激活 */
  autoActivate?: boolean;
  /** 受限 LLM 访问(注入插件上下文,可选) */
  llm?: PluginLLM;
  /** 只读会话访问(注入插件上下文,可选) */
  sessions?: PluginSessions;
}

/** 已激活插件的运行时状态 */
interface ActivePlugin {
  loaded: LoadedPlugin;
  registeredTools: string[]; // 注册到 executor 的工具全名
  hooks: PluginHook[];
  hookIds: number[]; // 事件总线订阅 id（卸载时注销）
  providerNames: string[]; // 本插件注册的 Provider 名
  interceptorsAdded: Interceptor[]; // 本插件加入的拦截器（卸载时按身份移除）
}

/** 人格的宽松形态（插件静态/函数/SYSTEM.md 三来源统一为 Persona 前的中间类型） */
interface PersonaLike {
  name: string;
  description?: string;
  systemPrompt: string;
  toolAllowlist?: string[];
}

/**
 * PluginManager：管理插件生命周期与内核集成。
 * - registerTools 命名空间：<插件名>__<工具名>，避免不同插件工具冲突
 * - 钩子直接挂在 EventBus 上（同进程回调）
 * - reload 支持运行时热更新（破坏模块缓存重新 import）
 */
export class PluginManager {
  readonly pluginsDir: string;
  readonly dataDir: string;
  readonly executor?: ToolExecutor;
  readonly eventBus?: EventBus;
  readonly llm?: PluginLLM;
  readonly sessions?: PluginSessions;
  /** 进度上报通道:装配层注入(把 note/percent 包装成事件发布) */
  reportProgress?: ProgressReporter['report'];
  /**
   * 拦截器活数组 —— 原地变更（同一引用），内核持有后无需重建即可感知插件启停。
   * 装配层把它注入 AgentKernel 的 interceptors。
   */
  readonly interceptors: Interceptor[] = [];
  /** 已注册的 LLM Provider 工厂（按名取用） */
  private providersMap = new Map<string, PluginProvider>();

  private active = new Map<string, ActivePlugin>();
  private personas: Persona[] = [];
  private loadedManifestNames = new Map<string, string>(); // dir -> plugin name
  /** 禁用名单（构造时从 plugin-state.json 读入；disable/enable 同步维护） */
  private disabledNames = new Set<string>();
  /** 已知插件目录：插件名/候选名 -> 目录（enable 时重新加载用） */
  private knownDirs = new Map<string, string>();
  /** 加载失败记录：候选名 -> 错误信息（list 展示 failed 态） */
  private failures = new Map<string, string>();

  constructor(options: PluginManagerOptions) {
    this.pluginsDir = options.pluginsDir;
    this.dataDir = options.dataDir ?? options.pluginsDir;
    this.executor = options.executor;
    this.eventBus = options.eventBus;
    this.llm = options.llm;
    this.sessions = options.sessions;
    this.disabledNames = new Set(readPluginState(this.dataDir).disabled);
    this.reportProgress = (note: string, percent?: number) => {
      this.eventBus?.publish({
        type: 'context.progress',
        source: 'plugin',
        data: { note, percent },
        timestamp: Date.now(),
      });
    };
    if (options.autoActivate !== false) {
      void this.loadAll();
    }
  }

  /** 扫描并加载目录下所有插件（顺序执行；禁用名单内跳过，单个失败不阻塞其余） */
  async loadAll(): Promise<string[]> {
    const dirs = discover(this.pluginsDir);
    const names: string[] = [];
    this.failures.clear();
    for (const dir of dirs) {
      this.knownDirs.set(this.candidateName(dir), dir);
      if (this.isDisabled(this.candidateName(dir)) || this.disabledNames.has(basename(dir))) continue;
      try {
        const name = await this.load(dir);
        names.push(name);
      } catch (err) {
        this.failures.set(this.candidateName(dir), (err as Error).message ?? String(err));
        console.warn(`[plugins] failed to load ${dir}:`, err);
      }
    }
    return names;
  }

  /** 加载前的候选名探测：manifest.name > 目录名（禁用匹配/enable 回查用） */
  private candidateName(dir: string): string {
    return loadManifest(dir)?.name ?? basename(dir);
  }

  /** 加载并激活单个插件目录 */
  async load(dir: string): Promise<string> {
    const loaded = await loadPlugin(dir);
    if (this.active.has(loaded.plugin.name)) {
      await this.unload(loaded.plugin.name);
    }
    this.loadedManifestNames.set(dir, loaded.plugin.name);
    this.knownDirs.set(loaded.plugin.name, dir);
    await this.activate(loaded);
    return loaded.plugin.name;
  }

  /**
   * 直接注册一个已实例化的插件（内置插件走同一套注册逻辑，体现“一切皆插件”）。
   * dir 用于解析资源文件（如内置插件可传项目目录）。
   */
  async add(plugin: OpenAIDePlugin, dir: string): Promise<string> {
    if (this.active.has(plugin.name)) {
      await this.unload(plugin.name);
    }
    const loaded: LoadedPlugin = { plugin, dir, loadedAt: Date.now() };
    await this.activate(loaded);
    return plugin.name;
  }

  /** 热重载单个插件（破坏缓存重新 import） */
  async reload(name: string): Promise<void> {
    const existing = this.active.get(name);
    if (!existing) throw new Error(`plugin not active: ${name}`);
    const dir = existing.loaded.dir;
    await this.unload(name);
    const fresh = await loadPlugin(dir, true);
    await this.activate(fresh);
  }

  /** 卸载插件：注销工具、移除钩子、调用 deactivate */
  async unload(name: string): Promise<void> {
    const existing = this.active.get(name);
    if (!existing) return;
    const { registeredTools, hooks, hookIds } = existing;
    for (const toolName of registeredTools) {
      try {
        this.executor?.unregister?.(toolName);
      } catch (err) {
        console.warn(`[plugins] unregister ${toolName}:`, err);
      }
    }
    for (const id of hookIds) {
      try {
        this.eventBus?.unsubscribe(id);
      } catch (err) {
        console.warn(`[plugins] unsubscribe hook ${id}:`, err);
      }
    }
    try {
      await existing.loaded.plugin.deactivate?.();
    } catch (err) {
      console.warn(`[plugins] deactivate ${name}:`, err);
    }
    // 反注册 Provider 与拦截器
    for (const pn of existing.providerNames) this.providersMap.delete(pn);
    for (const ix of existing.interceptorsAdded) {
      const idx = this.interceptors.indexOf(ix);
      if (idx >= 0) this.interceptors.splice(idx, 1);
    }
    this.active.delete(name);
    this.personas = this.personas.filter((p) => p.name !== name);
  }

  /** 激活插件：注册工具/钩子/人格 */
  private async activate(loaded: LoadedPlugin): Promise<void> {
    const { plugin, dir } = loaded;
    const reporter: ProgressReporter | undefined = this.reportProgress
      ? { report: (note: string, percent?: number) => this.reportProgress!(note, percent) }
      : undefined;
    const ctx: PluginContext = {
      dir,
      dataDir: this.dataDir,
      llm: this.llm,
      sessions: this.sessions,
      reportProgress: reporter?.report,
    };

    await plugin.activate?.(ctx);

    const registeredTools: string[] = [];
    if (this.executor) {
      const tools = plugin.tools ?? [];
      for (const tool of tools) {
        const fullName = `${plugin.name}__${tool.name}`;
        const def: ToolDefinition = {
          type: 'function',
          function: {
            name: fullName,
            // dangerous 工具在描述里显式标注,让 LLM 谨慎使用
            description: tool.dangerous
              ? `[dangerous — side effects, use with care] ${tool.description}`
              : tool.description,
            parameters: tool.parameters ?? { type: 'object', properties: {} },
          },
        };
        const runHandler = (args: string, sessionId: string, signal?: AbortSignal) =>
          tool.handler(JSON.parse(args) as Record<string, unknown>, sessionId, signal);
        this.executor.register(def, (args, sessionId, signal) => {
          // dangerous 工具执行前发提示事件(可观测;装配层可据此接审批)
          if (tool.dangerous) {
            this.eventBus?.publishSync({
              type: 'tool.permission',
              source: 'plugin',
              data: { plugin: plugin.name, tool: tool.name, sessionId },
              timestamp: Date.now(),
            });
          }
          return runHandler(args, sessionId, signal);
        });
        registeredTools.push(fullName);
      }
    }

    const hooks = plugin.hooks ?? [];
    const hookIds: number[] = [];
    if (this.eventBus) {
      for (const hook of hooks) {
        const id = this.eventBus.subscribe((event: KernelEvent) => {
          if (event.type === hook.event) {
            void hook.handler(event);
          }
        });
        hookIds.push(id);
      }
    }
    // LLM Provider 注册（同名后者覆盖；内核按 config.llm.provider 取用）
    const providerNames: string[] = [];
    for (const p of plugin.providers ?? []) {
      this.providersMap.set(p.name, p);
      providerNames.push(p.name);
    }

    // 拦截器加入活数组（原地 push，内核持引用即时生效）
    const interceptorsAdded: Interceptor[] = [...(plugin.interceptors ?? [])];
    for (const ix of interceptorsAdded) this.interceptors.push(ix);

    this.active.set(plugin.name, { loaded, registeredTools, hooks, hookIds, providerNames, interceptorsAdded });

    // 人格收集：静态 persona / 函数 / SYSTEM.md 外置文件 → 统一归一化为 Persona
    const candidate: PersonaLike | undefined =
      plugin.persona
        ? typeof plugin.persona === 'function'
          ? await plugin.persona()
          : plugin.persona
        : readPersonaFile(dir);
    if (candidate && candidate.name && candidate.systemPrompt) {
      const persona: Persona = {
        name: candidate.name,
        description: candidate.description ?? '',
        systemPrompt: candidate.systemPrompt,
        toolAllowlist: candidate.toolAllowlist,
      };
      this.personas = this.personas.filter((p) => p.name !== persona.name);
      this.personas.push(persona);
    }

    console.log(
      `[plugins] loaded ${plugin.name}@${plugin.version ?? '0.0.0'} (tools: ${registeredTools.length}, hooks: ${hooks.length}, persona: ${candidate ? 'yes' : 'no'})`,
    );
  }

  /** 已激活插件名 */
  names(): string[] {
    return [...this.active.keys()];
  }

  /** 插件是否在禁用名单 */
  isDisabled(name: string): boolean {
    return this.disabledNames.has(name);
  }

  /** 插件所在目录（激活中取运行时目录，否则回查已知目录；未知返回 undefined） */
  dirOf(name: string): string | undefined {
    return this.active.get(name)?.loaded.dir ?? this.knownDirs.get(name);
  }

  /** 取已注册的 LLM Provider 工厂 */
  getProvider(name: string): PluginProvider | undefined {
    return this.providersMap.get(name);
  }

  /** 已注册的 Provider 名列表 */
  providerNames(): string[] {
    return [...this.providersMap.keys()];
  }

  /**
   * 禁用插件：立即卸载（若在跑）+ 持久化到 plugin-state.json（重启后不再加载）。
   * 对不在目录里的插件（如内置）同样生效——只影响持久化状态与本次卸载。
   */
  async disable(name: string): Promise<void> {
    await this.unload(name);
    const state = readPluginState(this.dataDir);
    if (!state.disabled.includes(name)) state.disabled.push(name);
    writePluginState(this.dataDir, state);
    this.disabledNames.add(name);
    this.failures.delete(name);
  }

  /**
   * 启用插件：从禁用名单移除；若插件目录仍存在则立即重新加载。
   * @returns 是否成功加载（名单移除总是发生；目录缺失时返回 false）
   */
  async enable(name: string): Promise<boolean> {
    const state = readPluginState(this.dataDir);
    state.disabled = state.disabled.filter((n) => n !== name);
    writePluginState(this.dataDir, state);
    this.disabledNames.delete(name);
    this.failures.delete(name);
    const dir =
      this.knownDirs.get(name) ??
      discover(this.pluginsDir).find((d) => this.candidateName(d) === name || basename(d) === name);
    if (!dir) return false;
    try {
      await this.load(dir);
      return true;
    } catch (err) {
      this.failures.set(name, (err as Error).message ?? String(err));
      return false;
    }
  }

  /** 全部已知插件信息（active + disabled + failed 三态，供展示/管理） */
  list(): PluginInfo[] {
    const infos: PluginInfo[] = [];
    for (const a of this.active.values()) {
      const p = a.loaded.plugin;
      infos.push({
        name: p.name,
        version: p.version,
        description: p.description,
        category: resolveCategory(p, a.loaded.dir),
        status: 'active',
        tools: a.registeredTools,
        hooks: a.hooks.length,
        persona: this.personas.some((x) => x.name === p.name),
      });
    }
    for (const name of this.disabledNames) {
      if ([...this.active.values()].some((a) => a.loaded.plugin.name === name)) continue;
      const dir = this.knownDirs.get(name);
      const manifest = dir ? loadManifest(dir) : undefined;
      infos.push({
        name,
        version: manifest?.version,
        description: manifest?.description ?? '(disabled)',
        category: manifest?.category ?? 'uncategorized',
        status: 'disabled',
        tools: [],
        hooks: 0,
        persona: false,
      });
    }
    for (const [name, error] of this.failures) {
      const dir = this.knownDirs.get(name);
      const manifest = dir ? loadManifest(dir) : undefined;
      infos.push({
        name,
        version: manifest?.version,
        description: manifest?.description ?? '(failed to load)',
        category: manifest?.category ?? 'uncategorized',
        status: 'failed',
        tools: [],
        hooks: 0,
        persona: false,
        error,
      });
    }
    return infos;
  }

  /** 已收集的人格列表（供 PersonaProvider 使用） */
  getPersonas(): Persona[] {
    return [...this.personas];
  }

  /** 获取指定插件的人格 */
  getPersona(name: string): Persona | undefined {
    return this.personas.find((p) => p.name === name);
  }
}

/** 插件分类解析：代码声明优先，其次 manifest，缺省 'uncategorized' */
export function resolveCategory(plugin: OpenAIDePlugin, dir: string): string {
  if (plugin.category) return plugin.category;
  const manifest: PluginManifest | undefined = loadManifest(dir);
  return manifest?.category ?? 'uncategorized';
}

/**
 * PersonaProvider 适配器：把 PluginManager 收集的人格暴露给内核。
 */
export class PluginPersonaProvider {
  constructor(
    private readonly manager: PluginManager,
    private readonly activeName: () => string | undefined,
  ) {}

  async active(): Promise<Persona | undefined> {
    const name = this.activeName();
    if (!name) return undefined;
    return this.manager.getPersona(name);
  }
}
