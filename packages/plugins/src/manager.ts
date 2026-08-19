/**
 * 插件管理器 —— 统一接入内核。
 * 职责：加载/激活插件 → 注册工具（加 <插件>__ 前缀）→ 挂载钩子 → 收集人格 → 热重载。
 * 全部同进程运行，无子进程。
 */
import {
  EventBus,
  KernelEvent,
  EventHandler,
  Persona,
  ToolDefinition,
  ToolExecutor,
} from '@openaide/core';
import { discover, loadPlugin, readPersonaFile } from './loader.js';
import type { LoadedPlugin, OpenAIDePlugin, PluginContext, PluginHook } from './types.js';

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
}

/** 已激活插件的运行时状态 */
interface ActivePlugin {
  loaded: LoadedPlugin;
  registeredTools: string[]; // 注册到 executor 的工具全名
  hooks: PluginHook[];
  hookIds: number[]; // 事件总线订阅 id（卸载时注销）
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

  private active = new Map<string, ActivePlugin>();
  private personas: Persona[] = [];
  private loadedManifestNames = new Map<string, string>(); // dir -> plugin name

  constructor(options: PluginManagerOptions) {
    this.pluginsDir = options.pluginsDir;
    this.dataDir = options.dataDir ?? options.pluginsDir;
    this.executor = options.executor;
    this.eventBus = options.eventBus;
    if (options.autoActivate !== false) {
      void this.loadAll();
    }
  }

  /** 扫描并加载目录下所有插件（顺序执行，单个失败不阻塞其余） */
  async loadAll(): Promise<string[]> {
    const dirs = discover(this.pluginsDir);
    const names: string[] = [];
    for (const dir of dirs) {
      try {
        const name = await this.load(dir);
        names.push(name);
      } catch (err) {
        console.warn(`[plugins] failed to load ${dir}:`, err);
      }
    }
    return names;
  }

  /** 加载并激活单个插件目录 */
  async load(dir: string): Promise<string> {
    const loaded = await loadPlugin(dir);
    if (this.active.has(loaded.plugin.name)) {
      await this.unload(loaded.plugin.name);
    }
    this.loadedManifestNames.set(dir, loaded.plugin.name);
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
    this.active.delete(name);
    this.personas = this.personas.filter((p) => p.name !== name);
  }

  /** 激活插件：注册工具/钩子/人格 */
  private async activate(loaded: LoadedPlugin): Promise<void> {
    const { plugin, dir } = loaded;
    const ctx: PluginContext = { dir, dataDir: this.dataDir };

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
            description: tool.description,
            parameters: tool.parameters ?? { type: 'object', properties: {} },
          },
        };
        this.executor.register(def, (args, sessionId, signal) =>
          tool.handler(JSON.parse(args) as Record<string, unknown>, sessionId, signal),
        );
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
    this.active.set(plugin.name, { loaded, registeredTools, hooks, hookIds });

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

  /** 已收集的人格列表（供 PersonaProvider 使用） */
  getPersonas(): Persona[] {
    return [...this.personas];
  }

  /** 获取指定插件的人格 */
  getPersona(name: string): Persona | undefined {
    return this.personas.find((p) => p.name === name);
  }
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
