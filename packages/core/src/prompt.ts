/**
 * 提示词组装 —— 分层结构，persona 可插拔。
 * 对齐 Go 版 core/kernel_prompt.go 的分层思想，实际落到两条装配路径：
 *
 * 稳定系统层 buildSystemLayer()（每次查询字节级一致 → 可命中前缀缓存）：
 *  - L0 身份（persona 的 systemPrompt，否则 DEFAULT_L0）+ 恒定安全底线 SAFETY_FLOOR
 *  - L1 自定义 system_prompt（追加，绝不覆盖 L0）
 *  - L2 工具清单（从运行时工具表生成，不硬编码工具名）
 *  - L3 运行环境注记（动态尾部：os/shell/cwd）
 *
 * 消息层 buildMessages()（system 全部排在 user 之前）：
 *  - L4 任务适配 [Task: xxx]（按 detectTaskType 规则式判定）
 *  - L5 项目知识 [ProjectKnowledge]（内核可传 projectContext；CLI 另经
 *    knowledge 拦截器注入 knowledge/*.md，两者互不冲突）
 *  - 历史（条数 + token 双预算）
 *  - 用户查询
 */
import { Message, Query, StreamChunk, StreamChunkType, ToolDefinition } from './types.js';
import type { PersonaProvider, Persona } from './interfaces.js';

/**
 * 内核安全底线 —— 恒定注入，任何 persona 都不可关闭。
 *
 * 旧实现里 L0 只有一个 `persona.systemPrompt ?? DEFAULT_L0` 三元式：
 * 一旦激活人格（内置 coder 默认就激活），安全底线连同 DEFAULT_L0 一起被
 * **整体替换**，等于"换个人格就卸掉全部护栏"。护栏属于内核，不属于人格。
 *
 * 只放"任何 agent 都必须遵守"的硬约束；领域/流程提示词归 persona。
 */
export const SAFETY_FLOOR = `[Safety floor — non-negotiable]
- Ground every claim in what you actually observe (file contents, command output). Never fabricate results.
- Do not perform destructive or irreversible actions without explicit user confirmation.
- Never print, log, or commit secrets, API keys, tokens, or credentials.
- Use only the tools listed in [Available Tools]; never invent tool names or arguments.
- If a step fails or is blocked, report the real error and stop instead of pretending it worked.
- If the same action fails twice, change approach or stop and ask — do not loop.`;

/**
 * 内核兜底身份 —— 仅在「无激活 persona」时使用。
 * 刻意极简：内核只拥有身份与安全底线，具体领域提示词一律来自人格插件
 * （内置 coder/architect 住在 @openaide/plugins 的 builtin-personas.ts，
 * 与内置工具同走插件注册路径——"一切皆插件"）。
 */
export const DEFAULT_L0 = `You are an autonomous agent running inside the OpenAIDE terminal.
Work toward the goal by actually using tools, not by describing what you would do.`;

/** L0 组装：身份（persona 或兜底）+ 恒定安全底线 */
export function buildL0(persona?: Persona): string {
  return `${persona?.systemPrompt ?? DEFAULT_L0}\n\n${SAFETY_FLOOR}`;
}

/** 提示词组装上下文 */
export interface PromptContext {
  persona?: Persona;
  customSystemPrompt?: string;
  /** L5 项目知识（内嵌 [ProjectKnowledge] 段；CLI 的 knowledge 拦截器走另一条路径） */
  projectContext?: string;
  /** L4 任务类型，见 detectTaskType() */
  taskType?: string;
  /**
   * 当前轮可调用的工具定义（已应用人格白名单）。
   * 用于生成 [Available Tools] 段——工具真名由运行时工具表决定，
   * 提示词里不得硬编码工具名（插件改名即失效，见 #2）。
   */
  tools?: ToolDefinition[];
}

/**
 * 工具清单段 —— 从运行时工具表生成。
 *
 * 插件工具按 `<插件>__<工具>` 命名空间注册（见 plugins/manager.ts），
 * 提示词若硬编码裸名（read_file / search_files…）必然与实际注册名漂移，
 * 模型照着调只会拿到 `unknown tool` 白烧一轮。唯一真相来源 = 工具表。
 */
export function buildToolInventory(tools: ToolDefinition[]): string {
  const names = tools
    .map((t) => t.function?.name)
    .filter((n): n is string => typeof n === 'string' && n.length > 0);
  if (names.length === 0) return '';
  // 排序 → 字节级稳定，工具集不变时前缀缓存不失效
  const unique = [...new Set(names)].sort();
  return `[Available Tools]\n${unique.join(', ')}\n调用工具时必须使用上面列出的完整名称，不要臆造工具名。`;
}

/**
 * 人格工具白名单过滤 —— 与 ReAct 循环共用同一口径，
 * 保证 [Available Tools] 段列出的工具与实际可调用的完全一致。
 */
export function filterToolsByAllowlist(
  tools: ToolDefinition[],
  allowlist?: string[],
): ToolDefinition[] {
  if (!allowlist || allowlist.length === 0) return tools;
  return tools.filter((t) => {
    const name = t.function?.name ?? '';
    return allowlist.some(
      (allowed) =>
        name === allowed || name.endsWith(`__${allowed}`) || name.startsWith(`${allowed}__`),
    );
  });
}

/** 组装系统层（L0 身份 + L1 自定义 system + L2 工具清单 + L3 运行环境注记） */
export function buildSystemLayer(ctx: PromptContext): string {
  const parts: string[] = [];

  // L0：身份（persona 优先，否则兜底）+ 恒定安全底线（persona 不可覆盖）
  parts.push(buildL0(ctx.persona));

  // L1：自定义 system_prompt 追加，绝不覆盖 L0
  if (ctx.customSystemPrompt) {
    parts.push(ctx.customSystemPrompt);
  }

  // L2：工具清单，从运行时工具表生成（不硬编码名字）
  const inventory = buildToolInventory(ctx.tools ?? []);
  if (inventory) {
    parts.push(inventory);
  }

  // 运行环境注记放最后：前缀字节稳定利于 prompt cache，动态信息只在尾部变化
  // （否则模型在 Windows 上第一枪总打向 pwd/ls，浪费一轮试错）
  const isWin = process.platform === 'win32';
  const shellHint = isWin
    ? 'shell=cmd.exe (Unix 命令 pwd/ls/grep 不可用,改用 cd/dir/git;勿用 powershell,复杂处理写脚本文件执行)'
    : 'shell=bash';
  parts.push(`[Environment] os=${process.platform} · ${shellHint} · cwd=${process.cwd()}`);

  return parts.join('\n\n');
}

/**
 * 组装完整的 messages 列表（稳定前缀 + 动态尾部）。
 *
 * 顺序铁律：所有 system 消息必须排在历史与用户消息之前。
 * 旧实现把 [Task] / [Previous-round reflection] / [ProjectKnowledge]
 * 追加在 user 之后 —— system 夹在 user 后面会被多数模型（尤其 DeepSeek）
 * 忽略甚至拒收，等于这三层白写。
 */
export function buildMessages(
  systemLayer: string,
  history: Message[],
  query: Query,
  ctx: PromptContext,
  options: { maxHistory?: number; historyTokenBudget?: number } = {},
): Message[] {
  const messages: Message[] = [];
  if (systemLayer) messages.push({ role: 'system', content: systemLayer });

  // L4 任务适配（紧邻系统层，逐查询变化；置于历史之前不破坏系统层前缀缓存）
  if (ctx.taskType) {
    messages.push({ role: 'system', content: taskAdapterPrompt(ctx.taskType) });
  }

  // L5 项目知识（内嵌段；CLI 的 knowledge 拦截器会在首条 system 后插入同标签消息）
  if (ctx.projectContext) {
    messages.push({ role: 'system', content: `[ProjectKnowledge]\n${ctx.projectContext}` });
  }

  // 历史（稳定前缀）— 条数与 token 双重预算,防止上下文膨胀
  const limit = options.maxHistory ?? 20;
  const budget = options.historyTokenBudget ?? 6000;
  const historySlice = trimHistoryToBudget(history.slice(-limit), budget);
  for (const m of historySlice) {
    messages.push({ ...m });
  }

  // 用户查询（本轮唯一非 system 尾消息）
  messages.push({ role: 'user', content: query.content });

  return messages;
}

/** 粗估文本 token 数：CJK 按 1 字符/token 保守估（宁可早压缩不溢出），其余按 4 字符/token */
export function estimateTextTokens(text: string): number {
  if (!text) return 0;
  const cjk = (text.match(/[\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uff00-\uffef]/g) ?? []).length;
  return Math.ceil(cjk + (text.length - cjk) / 4);
}

/** 按 token 预算截断历史:从旧到新累积,超预算丢弃更旧的消息,至少保留最近 2 条 */
export function trimHistoryToBudget(history: Message[], budget: number): Message[] {
  if (history.length <= 2) return history;
  let total = 0;
  let keep = 0;
  for (let i = 0; i < history.length; i++) {
    const msg = history[i];
    if (!msg) break;
    total += estimateTextTokens(msg.content ?? '') + 4;
    if (total > budget && i >= 2) {
      break;
    }
    keep = i + 1;
  }
  return keep < history.length ? history.slice(history.length - keep) : history;
}

/**
 * 规则式任务类型检测(零 LLM 调用)。
 * 供 L3 任务适配器选择提示;识别不出返回 general。
 *
 * 关键:JS 正则的 \b 只认 ASCII 单词字符 [A-Za-z0-9_]，CJK 两侧永远构不成
 * 词边界 —— `\b审查\b` 恒不匹配，中文查询会全部落到 general。
 * 故中英文关键词分表：英文带 \b，中文不带。
 */
const TASK_RULES: Array<{ type: string; en: RegExp; zh: RegExp }> = [
  {
    type: 'review',
    en: /\b(review|code review)\b/i,
    zh: /审查|评审|走查|检视代码/,
  },
  {
    type: 'debugging',
    en: /\b(bug|error|fail|crash|exception|debug|stack ?trace|reproduce)\b/i,
    zh: /报错|崩溃|排错|排查|异常|复现|定位问题|不生效/,
  },
  {
    type: 'coding',
    en: /\b(implement|add|create|write|refactor|fix|patch|edit|migrate)\b/i,
    zh: /实现|添加|新建|编写|重构|修复|修改|写一个|开发|支持/,
  },
  {
    type: 'think',
    en: /\b(explain|how does|what is|why|compare|design|discuss|evaluate)\b/i,
    zh: /解释|为什么|是什么|对比|区别|分析|设计|评估/,
  },
];

export function detectTaskType(query: string): string {
  const q = query.toLowerCase();
  for (const rule of TASK_RULES) {
    if (rule.en.test(q) || rule.zh.test(q)) return rule.type;
  }
  return 'general';
}

/** 按任务类型生成适配提示（L3）—— 不出现具体工具名（真名见 [Available Tools] 段） */
export function taskAdapterPrompt(taskType: string): string {
  const adapters: Record<string, string> = {
    coding:
      '[Task: coding]\nInspect the relevant code first (read the files involved, search for the call sites you must change), then make minimal, focused edits. Verify with tests/build when appropriate.',
    review:
      '[Task: review]\nReview for correctness, security, and maintainability. Cite concrete file/line evidence. Prioritize findings by severity.',
    debugging:
      '[Task: debugging]\nReproduce the issue, inspect logs and stack traces, identify root cause, then propose or apply the fix. Verify the fix.',
    think:
      '[Task: think]\nAnalyze the question deeply. Consider multiple perspectives and trade-offs before concluding.',
  };
  return adapters[taskType] ?? '[Task: general]\nHandle the request directly and concretely.';
}

/** 空工具参数兜底 */
export function ensureSchema(parameters?: Record<string, unknown>): Record<string, unknown> {
  if (parameters && parameters.type) return parameters;
  return { type: 'object', properties: {} };
}

/** 工具结果截断：单条超过上限时头尾保留 */
export const MAX_TOOL_RESULT_CHARS = 20_000;
export function truncateToolResult(content: string): string {
  if (content.length <= MAX_TOOL_RESULT_CHARS) return content;
  const headLen = Math.floor((MAX_TOOL_RESULT_CHARS * 2) / 5);
  const tailLen = Math.floor((MAX_TOOL_RESULT_CHARS * 2) / 5);
  const head = content.slice(0, headLen);
  const tail = content.slice(content.length - tailLen);
  const snipped = content.length - head.length - tail.length;
  return `${head}\n\n... [${snipped} chars truncated] ...\n\n${tail}`;
}

/** 由任意可迭代产生类型化 chunk 的工具函数 */
export function chunk(type: StreamChunkType, partial: Partial<StreamChunk>): StreamChunk {
  return { type, ...partial } as StreamChunk;
}
