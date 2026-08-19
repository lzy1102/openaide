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

export interface LLMConfig {
  /** 兼容 OpenAI 的 base URL，如 https://api.deepseek.com/v1 */
  baseUrl: string;
  apiKey: string;
  model: string;
  /** 请求超时（毫秒），默认 120s */
  timeoutMs?: number;
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

  private headers(): Record<string, string> {
    return {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${this.config.apiKey}`,
    };
  }

  private toWireMessages(messages: Message[], tools: ToolDefinition[]): ChatMessage[] {
    return messages.map((m) => {
      const wire: ChatMessage = {
        role: m.role,
        content: m.content ?? '',
      };
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
    options: Record<string, unknown>,
  ): Promise<LLMResponse> {
    const body: Record<string, unknown> = {
      model: this.config.model,
      messages: this.toWireMessages(messages, tools),
    };
    if (tools.length > 0) body['tools'] = tools;
    for (const key of ['temperature', 'max_tokens', 'response_format', 'thinking']) {
      if (options[key] !== undefined) body[key] = options[key];
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.config.timeoutMs ?? 120_000);
    try {
      const res = await fetch(this.url(), {
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
    options: Record<string, unknown>,
  ): AsyncGenerator<StreamChunk> {
    const body: Record<string, unknown> = {
      model: this.config.model,
      messages: this.toWireMessages(messages, tools),
      stream: true,
    };
    if (tools.length > 0) body['tools'] = tools;
    for (const key of ['temperature', 'max_tokens', 'response_format', 'thinking']) {
      if (options[key] !== undefined) body[key] = options[key];
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.config.timeoutMs ?? 120_000);
    try {
      const res = await fetch(this.url(), {
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
          const chunk = parseSSELine(line);
          if (!chunk) continue;
          yield* emitSseChunk(chunk);
        }
      }
      if (buffer.trim()) {
        const chunk = parseSSELine(buffer);
        if (chunk) yield* emitSseChunk(chunk);
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

function* emitSseChunk(data: Record<string, unknown>): Generator<StreamChunk, void> {
  const choices = data['choices'] as Array<Record<string, unknown>> | undefined;
  if (!choices || choices.length === 0) return;
  const delta = (choices[0]?.['delta'] ?? {}) as Record<string, unknown>;
  const content = (delta['content'] as string | undefined) ?? '';
  const reasoning = (delta['reasoning_content'] as string | undefined) ?? '';
  if (content) yield { type: StreamChunkType.Content, content };
  if (reasoning) yield { type: StreamChunkType.Thinking, reasoningContent: reasoning };
  // 工具调用增量（OpenAI 流式增量格式）
  const toolCalls = delta['tool_calls'] as Array<{ index?: number; id?: string; type?: string; function?: { name?: string; arguments?: string } }> | undefined;
  if (toolCalls && toolCalls.length > 0) {
    const first = toolCalls[0];
    yield {
      type: StreamChunkType.ToolCall,
      toolCallId: first?.id,
      toolName: first?.function?.name,
      toolArgs: first?.function?.arguments,
    };
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
