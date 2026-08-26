/**
 * OpenAI 兼容 LLM gateway —— 零依赖，基于全局 fetch。
 * 支持 chat / chatStream（SSE 解析）/ 模型切换。
 */
import {
  LLMProvider,
  LLMResponse,
  Message,
  StreamChunk,
  StreamChunkType,
  ToolCall,
  ToolDefinition,
  TokenUsage,
} from '@openaide/core';

// 推理型模型可能数分钟才返回响应头——undici 默认 300s headersTimeout 会先炸。
// 全局 dispatcher 只在此模块首次使用时设置一次（CLI 单进程场景可接受）。
import { Agent, setGlobalDispatcher } from 'undici';
let longTimeoutDispatched = false;
function ensureLongTimeouts(): void {
  if (longTimeoutDispatched) return;
  longTimeoutDispatched = true;
  try {
    setGlobalDispatcher(new Agent({ headersTimeout: 900_000, bodyTimeout: 0 }));
  } catch {
    /* 设置失败则沿用默认（短任务不受影响） */
  }
}

/** 可重试的瞬态 HTTP 状态码 */
const RETRYABLE_STATUS = new Set([429, 500, 502, 503, 504]);

export interface LLMConfig {
  /** 兼容 OpenAI 的 base URL，如 https://api.deepseek.com/v1 */
  baseUrl: string;
  apiKey: string;
  model: string;
  /** 请求超时（毫秒），默认 120s */
  timeoutMs?: number;
  /**
   * 推理强度，原样透传给网关（reasoning_effort 字段）。
   * 常见取值：low / medium / high / max——不同模型档位名不同，不做枚举限制。
   */
  reasoningEffort?: string;
  /**
   * 瞬态失败自动重试次数（网络错误/超时/429/5xx）。
   * 默认 -1 = 无限重试直到成功或用户 Ctrl+C；0 = 不重试；正数 = N 次。
   * 鉴权(401/403)与参数(400)错误永不重试。
   */
  retries?: number;
  /** 重试基础延迟毫秒（指数退避 ×2），默认 1500 */
  retryDelayMs?: number;
}

interface ChatMessage {
  role: string;
  content: string;
  reasoning_content?: string;
  name?: string;
  tool_call_id?: string;
  tool_calls?: unknown[];
}

interface ChatCompletionResponse {
  id: string;
  model: string;
  choices: Array<{
    message?: {
      content: string;
      reasoning_content?: string;
      name?: string;
      tool_call_id?: string;
      tool_calls?: ToolCall[];
    };
  }>;
  usage?: {
    prompt_tokens?: number;
    completion_tokens?: number;
    total_tokens?: number;
    prompt_cache_hit_tokens?: number;
    prompt_cache_miss_tokens?: number;
  };
}

export class OpenAICompatibleProvider implements LLMProvider {
  private config: LLMConfig;

  constructor(config: LLMConfig) {
    this.config = config;
  }

  setConfig(config: LLMConfig): void {
    this.config = config;
  }

  getModelId(): string {
    return this.config.model;
  }

  setModelId(modelId: string): void {
    this.config.model = modelId;
  }

  private url(): string {
    const base = this.config.baseUrl.replace(/\/+$/, '');
    return `${base}/chat/completions`;
  }

  private async sleep(ms: number): Promise<void> {
    await new Promise((r) => setTimeout(r, ms));
  }

  /**
   * 带重试的 fetch：网络错误/超时/429/5xx 按指数退避重试（封顶 30s）；
   * retries<0 表示无限重试直到成功或用户 Ctrl+C；4xx 立即失败不重试。
   */
  private async fetchWithRetry(url: string, init: RequestInit): Promise<Response> {
    const maxRetries = this.config.retries ?? -1; // 默认无限
    const baseDelay = this.config.retryDelayMs ?? 1_500;
    const MAX_DELAY = 30_000;
    for (let attempt = 0; ; attempt++) {
      const canRetry = maxRetries < 0 || attempt < maxRetries;
      let res: Response;
      try {
        res = await fetch(url, init);
      } catch (err) {
        // 网络层错误（含超时 abort）→ 可重试
        if (canRetry) {
          const delay = Math.min(baseDelay * 2 ** attempt, MAX_DELAY);
          process.stderr.write(`[llm] attempt ${attempt + 1} network error, retry in ${delay}ms: ${(err as Error).message}
`);
          await this.sleep(delay);
          continue;
        }
        throw err;
      }
      if (RETRYABLE_STATUS.has(res.status) && canRetry) {
        const delay = Math.min(baseDelay * 2 ** attempt, MAX_DELAY);
        process.stderr.write(`[llm] attempt ${attempt + 1} got ${res.status}, retry in ${delay}ms
`);
        await this.sleep(delay);
        continue;
      }
      return res;
    }
  }

  private headers(): Record<string, string> {
    return {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${this.config.apiKey}`,
      // 防御性加固（非已观察故障）：部分网关的 Cloudflare 会按 UA 封禁
      // 默认 fetch 签名——实测 Python-urllib 被 opencode.ai 网关以
      // error 1010 拒绝；OpenAIDE 自身的 Node fetch 路径历史上未触发，
      // 显式 UA 消除这一类潜在风险
      'User-Agent': 'openaide-cli',
    };
  }

  private toWireMessages(messages: Message[], tools: ToolDefinition[]): ChatMessage[] {
    return messages.map((m) => {
      const wire: ChatMessage = {
        role: m.role,
        content: m.content ?? '',
      };
      // system 消息标记前缀缓存(DeepSeek/OpenAI 兼容):多轮共享 system 前缀时大幅降价
      if (m.role === 'system') {
        (wire as unknown as Record<string, unknown>)['cache_control'] = { type: 'ephemeral' };
      }
      if (m.reasoningContent) wire.reasoning_content = m.reasoningContent;
      if (m.name) wire.name = m.name;
      if (m.toolCallId) wire.tool_call_id = m.toolCallId;
      if (m.toolCalls && m.toolCalls.length > 0) wire.tool_calls = m.toolCalls;
      return wire;
    });
  }

  async chat(
    messages: Message[],
    tools: ToolDefinition[],
    options: Record<string, unknown> = {},
  ): Promise<LLMResponse> {
    ensureLongTimeouts();
    const body: Record<string, unknown> = {
      model: this.config.model,
      messages: this.toWireMessages(messages, tools),
    };
    if (this.config.reasoningEffort) body['reasoning_effort'] = this.config.reasoningEffort;
    if (tools.length > 0) body['tools'] = tools;
    for (const key of ['temperature', 'max_tokens', 'response_format', 'thinking']) {
      if (options[key] !== undefined) body[key] = options[key];
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.config.timeoutMs ?? 300_000);
    try {
      const res = await this.fetchWithRetry(this.url(), {
        method: 'POST',
        headers: this.headers(),
        body: JSON.stringify(body),
        signal: controller.signal,
      });
      if (!res.ok) {
        const text = await res.text().catch(() => '');
        throw new Error(`LLM ${res.status}: ${text.slice(0, 500)}`);
      }
      const data = (await res.json()) as ChatCompletionResponse;
      const choice = data.choices[0];
      return {
        id: data.id,
        model: data.model,
        content: choice?.message?.content ?? '',
        reasoningContent: choice?.message?.reasoning_content ?? undefined,
        toolCalls: (choice?.message?.tool_calls ?? undefined) as unknown as ToolCall[] | undefined,
        usage: toUsage(data.usage),
      };
    } finally {
      clearTimeout(timer);
    }
  }

  async *chatStream(
    messages: Message[],
    tools: ToolDefinition[],
    options: Record<string, unknown> = {},
  ): AsyncGenerator<StreamChunk> {
    ensureLongTimeouts();
    const body: Record<string, unknown> = {
      model: this.config.model,
      messages: this.toWireMessages(messages, tools),
      stream: true,
    };
    if (this.config.reasoningEffort) body['reasoning_effort'] = this.config.reasoningEffort;
    if (tools.length > 0) body['tools'] = tools;
    for (const key of ['temperature', 'max_tokens', 'response_format', 'thinking']) {
      if (options[key] !== undefined) body[key] = options[key];
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.config.timeoutMs ?? 300_000);
    // 工具调用增量累积:OpenAI 流式把参数拆进多个 delta(按 index 分片),
    // 必须跨 chunk 合并,否则拿到的是残缺的调用。
    const pendingToolCalls = new Map<number, { id: string; name: string; arguments: string }>();
    try {
      const res = await this.fetchWithRetry(this.url(), {
        method: 'POST',
        headers: this.headers(),
        body: JSON.stringify(body),
        signal: controller.signal,
      });
      if (!res.ok) {
        const text = await res.text().catch(() => '');
        throw new Error(`LLM ${res.status}: ${text.slice(0, 500)}`);
      }
      if (!res.body) throw new Error('no response body');

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';
        for (const line of lines) {
          const data = parseSSELine(line);
          if (!data) continue;
          const delta = sseDelta(data);
          if (delta.content) {
            yield { type: StreamChunkType.Content, content: delta.content };
          }
          if (delta.reasoning) {
            yield { type: StreamChunkType.Thinking, reasoningContent: delta.reasoning };
          }
          if (delta.toolCalls) {
            accumulateToolCalls(pendingToolCalls, delta.toolCalls);
            // 每个增量都产出当前合并后的完整列表,消费方可实时展示
            for (const tc of pendingToolCalls.values()) {
              yield {
                type: StreamChunkType.ToolCall,
                toolCallId: tc.id,
                toolName: tc.name,
                toolArgs: tc.arguments,
              };
            }
          }
        }
      }
      if (buffer.trim()) {
        const data = parseSSELine(buffer);
        if (data) {
          const delta = sseDelta(data);
          if (delta.content) yield { type: StreamChunkType.Content, content: delta.content };
          if (delta.reasoning) {
            yield { type: StreamChunkType.Thinking, reasoningContent: delta.reasoning };
          }
          if (delta.toolCalls) accumulateToolCalls(pendingToolCalls, delta.toolCalls);
        }
      }
    } finally {
      clearTimeout(timer);
    }
  }
}

function parseSSELine(line: string): Record<string, unknown> | undefined {
  const trimmed = line.trim();
  if (!trimmed.startsWith('data:')) return undefined;
  const payload = trimmed.slice(5).trim();
  if (payload === '[DONE]') return undefined;
  try {
    return JSON.parse(payload) as Record<string, unknown>;
  } catch {
    return undefined;
  }
}

/** 从 SSE data 中提取本轮 delta */
function sseDelta(data: Record<string, unknown>): {  content?: string;
  reasoning?: string;
  toolCalls?: Array<{ index?: number; id?: string; function?: { name?: string; arguments?: string } }>;
} {
  const choices = data['choices'] as Array<Record<string, unknown>> | undefined;
  if (!choices || choices.length === 0) return {};
  const delta = (choices[0]?.['delta'] ?? {}) as Record<string, unknown>;
  return {
    content: (delta['content'] as string | undefined) || undefined,
    reasoning: (delta['reasoning_content'] as string | undefined) || undefined,
    toolCalls: delta['tool_calls'] as Array<{
      index?: number;
      id?: string;
      function?: { name?: string; arguments?: string };
    }> | undefined,
  };
}

/** 把流式增量合并进累积表:index 相同的分片追加 arguments、补齐 id/name */
function accumulateToolCalls(
  pending: Map<number, { id: string; name: string; arguments: string }>,
  deltas: Array<{ index?: number; id?: string; function?: { name?: string; arguments?: string } }>,
): void {
  for (const d of deltas) {
    const idx = d.index ?? 0;
    const existing = pending.get(idx) ?? { id: '', name: '', arguments: '' };
    if (d.id) existing.id = d.id;
    if (d.function?.name) existing.name = d.function.name;
    if (d.function?.arguments) existing.arguments += d.function.arguments;
    pending.set(idx, existing);
  }
}

function toUsage(usage?: ChatCompletionResponse['usage']): TokenUsage | undefined {
  if (!usage) return undefined;
  return {
    promptTokens: usage.prompt_tokens ?? 0,
    completionTokens: usage.completion_tokens ?? 0,
    totalTokens: usage.total_tokens ?? 0,
    promptCacheHitTokens: usage.prompt_cache_hit_tokens,
    promptCacheMissTokens: usage.prompt_cache_miss_tokens,
  };
}
