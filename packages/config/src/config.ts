/**
 * 配置加载 —— yaml + 环境变量覆盖。
 */
import { existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import YAML from 'yaml';

export interface LLMConfigOptions {
  apiKey: string;
  model: string;
  baseUrl: string;
  timeoutMs?: number;
  /** Provider 名（插件注册的自定义后端；缺省内置 openai-compatible） */
  provider?: string;
}

export interface Config {
  /** LLM 主配置 */
  llm: LLMConfigOptions;
  /** 内核配置 */
  kernel: {
    maxRounds: number;
    maxTokens: number;
    systemPrompt?: string;
    persona?: string;
    /** 工具审批模式：off=不审批（默认）；dangerous=危险工具需确认；always=所有工具需确认 */
    approval?: 'off' | 'dangerous' | 'always';
  };
  /** 数据目录（会话/记忆/插件） */
  dataDir: string;
  /** 插件目录 */
  pluginsDir: string;
  /** 插件市场索引 URL（未设时由调用方使用内置默认） */
  registryUrl?: string;
  /** 交互界面名（插件 uis 注册；缺省 TTY→ink / 非 TTY→readline） */
  ui?: string;
}

/** 默认配置目录（OPENAIDE_DATA_DIR 优先，保证数据与配置随目录整体迁移） */
export function defaultDataDir(): string {
  return process.env.OPENAIDE_DATA_DIR ?? join(homedir(), '.openaide');
}

/** 项目工作区标记目录名（openaide init 创建；存在即启用"会话随项目走"） */
export const WORKSPACE_DIRNAME = '.openaide';

/**
 * 从 startDir（默认 cwd）向上查找项目工作区。
 * 显式 opt-in：只有目录树里真的存在 .openaide/ 才启用，绝不悄悄写进用户的仓库。
 * @returns 工作区绝对路径（…/<project>/.openaide），未找到返回 null
 */
export function findProjectWorkspace(startDir?: string): string | null {
  let dir = startDir ? resolve(startDir) : process.cwd();
  const home = homedir();
  for (;;) {
    const candidate = join(dir, WORKSPACE_DIRNAME);
    if (existsSync(candidate) && statSync(candidate).isDirectory()) return candidate;
    if (dir === home || dirname(dir) === dir) return null; // 到家目录/根为止
    dir = dirname(dir);
  }
}

/** 默认配置路径 */
export function defaultConfigPath(): string {
  return join(defaultDataDir(), 'config.yaml');
}

interface RawConfig {
  llm?: {
    api_key?: string;
    model?: string;
    base_url?: string;
    timeout_ms?: number;
    provider?: string;
  };
  kernel?: {
    max_rounds?: number;
    max_tokens?: number;
    system_prompt?: string;
    persona?: string;
    approval?: 'off' | 'dangerous' | 'always';
  };
  data_dir?: string;
  plugins_dir?: string;
  registry_url?: string;
  ui?: string;
}

/**
 * 加载配置：读取 yaml，环境变量覆盖（OPENAIDE_API_KEY / OPENAIDE_MODEL / OPENAIDE_BASE_URL）。
 * 缺失文件时返回默认值（仅环境变量）。
 */
export function loadConfig(configPath?: string): Config {
  const path = configPath ?? defaultConfigPath();
  const raw: RawConfig = {};
  if (existsSync(path)) {
    try {
      Object.assign(raw, YAML.parse(readFileSync(path, 'utf8')) ?? {});
    } catch (err) {
      console.warn(`[config] failed to parse ${path}:`, err);
    }
  }

  const dataDir = process.env.OPENAIDE_DATA_DIR ?? raw.data_dir ?? defaultDataDir();
  const config: Config = {
    llm: {
      apiKey:
        process.env.OPENAIDE_API_KEY ?? raw.llm?.api_key ?? '',
      model: process.env.OPENAIDE_MODEL ?? raw.llm?.model ?? 'deepseek-v4-pro',
      baseUrl: process.env.OPENAIDE_BASE_URL ?? raw.llm?.base_url ?? 'https://api.deepseek.com/v1',
      timeoutMs: raw.llm?.timeout_ms,
      provider: process.env.OPENAIDE_PROVIDER ?? raw.llm?.provider,
    },
    kernel: {
      maxRounds: raw.kernel?.max_rounds ?? 10,
      maxTokens: raw.kernel?.max_tokens ?? 4000,
      systemPrompt: raw.kernel?.system_prompt,
      persona: raw.kernel?.persona,
      approval: raw.kernel?.approval ?? 'off',
    },
    dataDir,
    pluginsDir:
      process.env.OPENAIDE_PLUGINS_DIR ??
      raw.plugins_dir ??
      join(dataDir, 'plugins'),
    registryUrl: process.env.OPENAIDE_REGISTRY_URL ?? raw.registry_url,
    ui: process.env.OPENAIDE_UI ?? raw.ui,
  };

  mkdirSync(dataDir, { recursive: true });
  mkdirSync(config.pluginsDir, { recursive: true });
  return config;
}

/** 写回配置（setup 命令用） */
export function saveConfig(config: Config, path?: string): void {
  const target = path ?? defaultConfigPath();
  const raw: RawConfig = {
    llm: {
      api_key: config.llm.apiKey,
      model: config.llm.model,
      base_url: config.llm.baseUrl,
      timeout_ms: config.llm.timeoutMs,
      provider: config.llm.provider,
    },
    kernel: {
      max_rounds: config.kernel.maxRounds,
      max_tokens: config.kernel.maxTokens,
      system_prompt: config.kernel.systemPrompt,
      persona: config.kernel.persona,
      approval: config.kernel.approval ?? 'off',
    },
    data_dir: config.dataDir,
    plugins_dir: config.pluginsDir,
    registry_url: config.registryUrl,
    ui: config.ui,
  };
  const dir = target.replace(/[\\/][^\\/]*$/, '');
  mkdirSync(dir, { recursive: true });
  writeFileSync(target, YAML.stringify(raw));
}
