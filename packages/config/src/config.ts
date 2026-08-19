/**
 * 配置加载 —— yaml + 环境变量覆盖。
 */
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import YAML from 'yaml';

export interface LLMConfigOptions {
  apiKey: string;
  model: string;
  baseUrl: string;
  timeoutMs?: number;
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
  };
  /** 数据目录（会话/记忆/插件） */
  dataDir: string;
  /** 插件目录 */
  pluginsDir: string;
}

/** 默认配置目录 */
export function defaultDataDir(): string {
  return join(homedir(), '.openaide');
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
  };
  kernel?: {
    max_rounds?: number;
    max_tokens?: number;
    system_prompt?: string;
    persona?: string;
  };
  data_dir?: string;
  plugins_dir?: string;
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
    },
    kernel: {
      maxRounds: raw.kernel?.max_rounds ?? 10,
      maxTokens: raw.kernel?.max_tokens ?? 4000,
      systemPrompt: raw.kernel?.system_prompt,
      persona: raw.kernel?.persona,
    },
    dataDir,
    pluginsDir:
      process.env.OPENAIDE_PLUGINS_DIR ??
      raw.plugins_dir ??
      join(dataDir, 'plugins'),
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
    },
    kernel: {
      max_rounds: config.kernel.maxRounds,
      max_tokens: config.kernel.maxTokens,
      system_prompt: config.kernel.systemPrompt,
      persona: config.kernel.persona,
    },
    data_dir: config.dataDir,
    plugins_dir: config.pluginsDir,
  };
  const dir = target.replace(/[\\/][^\\/]*$/, '');
  mkdirSync(dir, { recursive: true });
  writeFileSync(target, YAML.stringify(raw));
}
