/**
 * 配置加载 —— yaml + 环境变量覆盖。
 */
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { userInfo } from 'node:os';
import { homedir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import YAML from 'yaml';

export interface LLMConfigOptions {
  apiKey: string;
  model: string;
  baseUrl: string;
  timeoutMs?: number;
  /** 推理强度，原样透传网关 reasoning_effort（low/medium/high/max…按模型支持） */
  reasoningEffort?: string;
  /**
   * 瞬态失败自动重试次数：-1=无限直到成功或用户取消（默认）；0=不重试；正数=N 次。
   * 仅针对网络/超时/429/5xx；鉴权与参数错误不重试。
   */
  retries?: number;
  /** 重试基础延迟（毫秒），指数退避 ×2，默认 1500 */
  retryDelayMs?: number;
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
    subagents?: Array<{ name: string; persona: string; description?: string; maxRounds?: number }>;
  };
  /** 数据目录（会话/记忆/插件） */
  dataDir: string;
  /** 插件目录 */
  pluginsDir: string;
  /** 插件市场索引 URL（未设时由调用方使用内置默认） */
  registryUrl?: string;
  /** 交互界面名（插件 uis 注册；缺省 TTY→ink / 非 TTY→readline） */
  ui?: string;
  /**
   * 项目工作区模式：on(默认,会话随项目走) / off(脚本/CI 场景退回全局 SQLite)
   * 环境变量 OPENAIDE_WORKSPACE 可覆盖
   */
  workspace?: 'on' | 'off';
  /**
   * 会话同步策略：commit(默认,自动提交 .openaide/) / push(再自动推送) / off
   */
  sessionSync?: 'off' | 'commit' | 'push';
  /** MCP 服务器映射（schema 与 Claude mcpServers 兼容：command/args/env/timeoutMs） */
  mcpServers?: Record<string, {
    command: string;
    args?: string[];
    env?: Record<string, string>;
    timeoutMs?: number;
  }>;
}

function _num(v: string | undefined): number | undefined {
  if (!v) return undefined;
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
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

/**
 * 解析项目工作区（会话一律随项目走）:
 *  - cwd 及其祖先已有 .openaide/ → 复用它（子目录启动也能找回项目根的会话）
 *  - 没有 → 在 cwd 下创建 .openaide/（启动目录即项目根）
 *  - 家目录护栏：cwd 即家目录时不创建（避免"~/.openaide 即工作区"的语义混淆），
 *    此时回退全局数据目录作为工作区
 */
export function resolveProjectWorkspace(startDir?: string): string {
  const cwd = startDir ? resolve(startDir) : process.cwd();
  const home = homedir();
  if (cwd === home) return defaultDataDir(); // 家目录护栏

  let dir = cwd;
  for (;;) {
    // 家目录的 .openaide 是全局配置目录，绝不能被当作项目工作区命中
    const candidate = join(dir, WORKSPACE_DIRNAME);
    if (dir !== home && existsSync(candidate) && statSync(candidate).isDirectory()) return candidate;
    if (dir === home || dirname(dir) === dir) break; // 到家目录/根为止
    dir = dirname(dir);
  }
  const created = join(cwd, WORKSPACE_DIRNAME);
  mkdirSync(created, { recursive: true });
  return created;
}

/* ────────────────────────── 开发者身份 ────────────────────────── */

/** 身份解析结果（name 已清洗为可作目录名的形态） */
export interface Identity {
  name: string;
  /** 命中的来源：env / git-name / git-email / os-user / random */
  from: 'env' | 'git-name' | 'git-email' | 'os-user' | 'random';
}

function sanitize(raw: string): string {
  const cleaned = raw
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');
  return cleaned;
}

function gitConfig(key: 'user.name' | 'user.email'): string | undefined {
  try {
    const out = execFileSync('git', ['config', '--get', key], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    });
    return out.trim() || undefined;
  } catch {
    return undefined;
  }
}

let identityCache: Identity | null = null;

/**
 * 解析当前开发者身份（用于会话按人分目录）。
 * 链式降级：OPENAIDE_USER > git user.name > git user.email(@前缀) >
 * 操作系统用户名 > 随机 id（持久化到 <workspace>/identity，保证同机稳定）。
 * 每次启动实时解析（不缓存 git 结果）：中途配置好 user.name 后，
 * 调用方检测到身份变化可自动迁移旧目录，跨机会自然收敛。
 */
export function resolveIdentity(workspaceDir?: string): Identity {
  const pick = (raw: string | undefined, from: Identity['from']): Identity | undefined => {
    if (!raw) return undefined;
    const name = sanitize(raw);
    return name ? { name, from } : undefined;
  };

  const env = pick(process.env.OPENAIDE_USER, 'env');
  if (env) return env;

  const gn = pick(gitConfig('user.name'), 'git-name');
  if (gn) return gn;

  const ge = pick(gitConfig('user.email')?.split('@')[0] ?? '', 'git-email');
  if (ge) return ge;

  const osu = pick(userInfo().username, 'os-user');
  if (osu) return osu;

  // 最终兜底：随机 id 持久化到工作区（无 workspaceDir 时仅进程内稳定）
  if (workspaceDir && existsSync(workspaceDir)) {
    const f = join(workspaceDir, 'identity');
    if (existsSync(f)) {
      const cached = sanitize(readFileSync(f, 'utf8'));
      if (cached) return { name: cached, from: 'random' };
    }
    const id = `u-${Math.random().toString(36).slice(2, 6)}`;
    try {
      writeFileSync(f, id, 'utf8');
    } catch {
      /* 只读环境接受进程内不稳定 */
    }
    return { name: id, from: 'random' };
  }
  return { name: 'local', from: 'random' };
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
    reasoning_effort?: string;
    retries?: number;
    retry_delay_ms?: number;
  };
  kernel?: {
    max_rounds?: number;
    max_tokens?: number;
    system_prompt?: string;
    persona?: string;
    approval?: 'off' | 'dangerous' | 'always';
    /** 子代理声明：每个条目打包 (persona, 可选 maxRounds) 为一个可委派工具 */
    subagents?: Array<{ name: string; persona: string; description?: string; max_rounds?: number }>;
  };
  data_dir?: string;
  plugins_dir?: string;
  registry_url?: string;
  ui?: string;
  workspace?: 'on' | 'off';
  session_sync?: 'off' | 'commit' | 'push';
  mcp_servers?: Record<string, { command: string; args?: string[]; env?: Record<string, string>; timeout_ms?: number }>;
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
      reasoningEffort: process.env.OPENAIDE_REASONING_EFFORT ?? raw.llm?.reasoning_effort,
      retries: raw.llm?.retries ?? _num(process.env.OPENAIDE_RETRIES),
      retryDelayMs: raw.llm?.retry_delay_ms ?? _num(process.env.OPENAIDE_RETRY_DELAY_MS),
    },
    kernel: {
      maxRounds: raw.kernel?.max_rounds ?? 10,
      maxTokens: raw.kernel?.max_tokens ?? 4000,
      systemPrompt: raw.kernel?.system_prompt,
      persona: raw.kernel?.persona,
      approval: raw.kernel?.approval ?? 'off',
      subagents: raw.kernel?.subagents?.map((x) => ({ name: x.name, persona: x.persona, description: x.description, maxRounds: x.max_rounds })),
    },
    dataDir,
    pluginsDir:
      process.env.OPENAIDE_PLUGINS_DIR ??
      raw.plugins_dir ??
      join(dataDir, 'plugins'),
    registryUrl: process.env.OPENAIDE_REGISTRY_URL ?? raw.registry_url,
    ui: process.env.OPENAIDE_UI ?? raw.ui,
    workspace: (process.env.OPENAIDE_WORKSPACE as 'on' | 'off' | undefined) ?? raw.workspace ?? 'on',
    sessionSync: raw.session_sync ?? 'commit',
    mcpServers: raw.mcp_servers
      ? Object.fromEntries(
          Object.entries(raw.mcp_servers).map(([k, v]) => [
            k,
            { command: v.command, args: v.args, env: v.env, timeoutMs: v.timeout_ms },
          ]),
        )
      : undefined,
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
      reasoning_effort: config.llm.reasoningEffort,
      retries: config.llm.retries,
      retry_delay_ms: config.llm.retryDelayMs,
    },
    kernel: {
      max_rounds: config.kernel.maxRounds,
      max_tokens: config.kernel.maxTokens,
      system_prompt: config.kernel.systemPrompt,
      persona: config.kernel.persona,
      approval: config.kernel.approval ?? 'off',
      subagents: config.kernel.subagents,
    },
    data_dir: config.dataDir,
    plugins_dir: config.pluginsDir,
    registry_url: config.registryUrl,
    ui: config.ui,
    workspace: config.workspace ?? 'on',
    session_sync: config.sessionSync ?? 'commit',
    mcp_servers: config.mcpServers
      ? Object.fromEntries(
          Object.entries(config.mcpServers).map(([k, v]) => [
            k,
            { command: v.command, args: v.args, env: v.env, timeout_ms: v.timeoutMs },
          ]),
        )
      : undefined,
  };
  const dir = target.replace(/[\\/][^\\/]*$/, '');
  mkdirSync(dir, { recursive: true });
  writeFileSync(target, YAML.stringify(raw));
}
