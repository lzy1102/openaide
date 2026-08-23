/**
 * ReAct 循环 —— 内核核心：思考 → 工具调用 → 观察 → 循环。
 * 以生成器形式逐轮产出 StreamChunk，process 聚合为 Response。
 */
import {
  KernelEvent,
  LLMResponse,
  Message,
  StreamChunk,
  StreamChunkType,
  TokenUsage,
  ToolCall,
  ToolDefinition,
  ToolResult,
} from './types.js';
import type { ContextCompressor, Interceptor, LLMProvider, PermissionChecker, ToolExecutor } from './interfaces.js';
import { runAfterToolCall, runBeforeLLM, runBeforeToolCall } from './intercept.js';
import { EventTypes } from './types.js';
import { truncateToolResult } from './prompt.js';
import { compressToBudget, estimateMessagesTokens } from './compress.js';

export interface ReactConfig {
  maxRounds: number;
  /** 工具结果单条字符上限（入 LLM 上下文前截断） */
  maxToolResultChars?: number;
}

export interface ReactContext {
  provider: LLMProvider;
  executor: ToolExecutor;
  messages: Message[];
  sessionId: string;
  publish: (event: KernelEvent) => void;
  signal?: AbortSignal;
  /** 覆盖 provider 的调用选项（如 temperature/route） */
  options?: Record<string, unknown>;
  /** 可选上下文压缩器:消息总量超过 maxTokens 90% 时渐进式压缩 */
  compressor?: ContextCompressor;
  /** 上下文 token 预算(压缩阈值 = maxTokens * 0.9),默认 200000 */
  maxTokens?: number;
  /** 可选权限检查器:工具执行前鉴权,拒绝时把原因作为错误结果回给模型 */
  permission?: PermissionChecker;
  /** 拦截器链:可否决/改写 LLM 请求与工具调用(审批/脱敏/限流等策略) */
  interceptors?: Interceptor[];
  /** 工具白名单(persona 声明):只暴露名单内的工具,支持"任务变身" */
  toolAllowlist?: string[];
}

/** 单轮工具执行结果 */
interface ToolOutcome {
  call: ToolCall;
  result: ToolResult;
}

/** ReAct 聚合结果 */
export interface ReactResult {
  content: string;
  reasoningContent?: string;
  rounds: number;
  usage?: unknown;
}

/**
 * ReAct 生成器：逐轮产出 chunk。
 * 生成顺序：content/thinking → tool_call → tool_done → … → done
 */
export async function* reactLoop(ctx: ReactContext, config: ReactConfig): AsyncGenerator<StreamChunk> {
  const { provider, executor, messages, sessionId, publish, signal, permission, toolAllowlist } = ctx;
  const maxToolChars = config.maxToolResultChars ?? 20_000;
  const maxRounds = config.maxRounds > 0 ? config.maxRounds : 10;
  // 上下文预算:压缩阈值 = 预算 * 0.9(对齐 Claude Code 的 92% 附近)
  const tokenBudget = ctx.maxTokens && ctx.maxTokens > 0 ? ctx.maxTokens : 200_000;
  const compressThreshold = (tokenBudget * 9) / 10;

  let finalContent = '';
  let finalReasoning = '';
  let rounds = 0;
  let usage: TokenUsage | undefined;

  const workingMessages = [...messages];

  for (let round = 1; round <= maxRounds; round++) {
    rounds = round;
    throwIfAborted(signal);

    // 上下文压缩:超过预算 90% 时渐进式压缩,防止长会话上下文爆炸
    if (ctx.compressor) {
      const total = estimateMessagesTokens(workingMessages);
      if (total > compressThreshold) {
        try {
          const { messages: compressed, compressed: didCompress } =
            await compressToBudget(ctx.compressor, workingMessages, tokenBudget);
          if (didCompress) {
            workingMessages.length = 0;
            workingMessages.push(...compressed);
            publish({
              type: EventTypes.ContextCompressed,
              source: 'react',
              data: { round, tokensBefore: total, tokensAfter: estimateMessagesTokens(compressed) },
              timestamp: Date.now(),
            });
          }
        } catch {
          // 压缩失败不阻断主流程 — 下轮重试
        }
      }
    }

    publish({ type: EventTypes.RoundStart, source: 'react', data: { round }, timestamp: Date.now() });

    // 当前轮工具定义(persona 白名单过滤:变身后的 agent 只见自己该用的工具)
    const allTools = executor.definitions();
    const tools = toolAllowlist && toolAllowlist.length > 0
      ? allTools.filter((t) => {
          const name = t.function.name;
          return toolAllowlist.some((allowed) => name === allowed || name.endsWith(`__${allowed}`) || name.startsWith(`${allowed}__`));
        })
      : allTools;

    // 1. 思考(流式优先:token 到达即产出;失败降级非流式)
    let resp;
    try {
      // LLM 请求前拦截链(可脱敏/成本否决);deny 视为策略终止本轮查询
      let llmMessages: Message[] = workingMessages;
      if (ctx.interceptors && ctx.interceptors.length > 0) {
        const gate = await runBeforeLLM(ctx.interceptors, { sessionId, messages: [...workingMessages] });
        if (!gate.ok) {
          const err = new Error(`blocked by policy: ${gate.reason}`);
          yield { type: StreamChunkType.Error, error: err };
          throw err;
        }
        llmMessages = gate.payload;
      }
      resp = await streamOrChat(provider, llmMessages, tools, ctx.options ?? {}, signal);
    } catch (err) {
      yield { type: StreamChunkType.Error, error: toError(err) };
      throw err;
    }

    if (resp.reasoningContent) {
      finalReasoning = resp.reasoningContent;
      yield { type: StreamChunkType.Thinking, reasoningContent: resp.reasoningContent, round };
    }
    if (resp.usage) {
      usage = resp.usage;
    }

    // 2. 无工具调用 → 结束
    if (!resp.toolCalls || resp.toolCalls.length === 0) {
      finalContent = resp.content ?? '';
      // LLM 空回复(限流/静默失败)按错误处理,避免"完成但无回复"和保存空消息
      if (!finalContent && !resp.reasoningContent) {
        const err = new Error('LLM returned no response (rate limit or empty reply)');
        yield { type: StreamChunkType.Error, error: err };
        throw err;
      }
      if (finalContent) {
        yield { type: StreamChunkType.Content, content: finalContent, round };
      }
      yield { type: StreamChunkType.Done, done: true, round, totalRounds: rounds, usage };
      publish({ type: EventTypes.RoundEnd, source: 'react', data: { round, done: true }, timestamp: Date.now() });
      return;
    }

    // 3. 有工具调用：记录 assistant 消息 + 工具调用声明
    const assistantMsg: Message = {
      role: 'assistant',
      content: resp.content ?? '',
      reasoningContent: resp.reasoningContent,
      toolCalls: resp.toolCalls,
    };
    workingMessages.push(assistantMsg);
    for (const tc of resp.toolCalls) {
      yield {
        type: StreamChunkType.ToolCall,
        toolCallId: tc.id,
        toolName: tc.function.name,
        toolArgs: tc.function.arguments,
        round,
      };
      publish({
        type: EventTypes.ToolCallStarted,
        source: 'react',
        data: { tool: tc.function.name, args: tc.function.arguments, sessionId },
        timestamp: Date.now(),
      });
    }

    // 4. 执行工具（串行；PermissionChecker → 拦截链 → executor）
    const outcomes: ToolOutcome[] = [];
    for (const tc of resp.toolCalls) {
      throwIfAborted(signal);
      let argsJson = tc.function.arguments;
      let denied: string | null = null;

      // 权限检查:拒绝时不执行工具,把原因作为结果返回给模型自行调整
      if (permission && !denied) {
        const verdict = permission.check(sessionId, tc.function.name, tc.function.name, {
          args: argsJson,
          round,
        });
        if (!verdict.allowed) denied = verdict.reason;
      }
      // 拦截链(可否决或改写参数)
      if (!denied && ctx.interceptors && ctx.interceptors.length > 0) {
        const gate = await runBeforeToolCall(ctx.interceptors, {
          sessionId,
          tool: tc.function.name,
          argsJson,
        });
        if (!gate.ok) denied = gate.reason;
        else argsJson = gate.payload;
      }

      let result: ToolResult;
      if (denied !== null) {
        result = { content: '', error: `permission denied: ${denied}`, errorCode: 'PERMISSION_DENIED' };
        yield {
          type: StreamChunkType.ToolDone,
          toolCallId: tc.id,
          toolName: tc.function.name,
          round,
        };
        workingMessages.push({
          role: 'tool',
          toolCallId: tc.id,
          content: `Permission denied: ${denied}. Choose a different approach.`,
        });
        outcomes.push({ call: tc, result });
        continue;
      }

      // 参数被拦截器改写时以新参数构造调用
      const call: ToolCall =
        argsJson === tc.function.arguments
          ? tc
          : { id: tc.id, type: tc.type, function: { name: tc.function.name, arguments: argsJson } };
      try {
        result = await executor.execute(call, sessionId, signal);
      } catch (err) {
        result = { content: '', error: toError(err).message, errorCode: 'EXEC_FAILED' };
      }
      // 执行后拦截链(可否决/脱敏结果)
      if (ctx.interceptors && ctx.interceptors.length > 0) {
        result = await runAfterToolCall(
          ctx.interceptors,
          { sessionId, tool: tc.function.name, argsJson },
          result,
        );
      }
      // 截断超大输出
      if (typeof result.content === 'string' && result.content.length > maxToolChars) {
        result = { ...result, content: truncateToolResult(result.content) };
      }
      outcomes.push({ call: tc, result });

      yield {
        type: StreamChunkType.ToolDone,
        toolCallId: tc.id,
        toolName: tc.function.name,
        toolResult: result,
        round,
      };
      publish({
        type: EventTypes.ToolCallEnded,
        source: 'react',
        data: { tool: tc.function.name, sessionId, result },
        timestamp: Date.now(),
      });

      // 追加 tool 消息（观察）
      const content =
        typeof result.content === 'string' ? result.content : JSON.stringify(result.content);
      workingMessages.push({ role: 'tool', toolCallId: tc.id, content });
    }

    publish({ type: EventTypes.RoundEnd, source: 'react', data: { round, done: false }, timestamp: Date.now() });
    yield { type: StreamChunkType.Progress, round, totalRounds: maxRounds };
  }

  // 达到最大轮数：返回最后已知内容
  yield { type: StreamChunkType.Done, done: true, round: rounds, totalRounds: rounds, usage };
  return;
}

export function toError(err: unknown): Error {
  return err instanceof Error ? err : new Error(String(err));
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) {
    throw new Error(signal.reason ? String(signal.reason) : 'aborted');
  }
}

/** 聚合 reactLoop 的 chunk 为最终结果（供 process 同步路径使用） */
export async function collectReact(ctx: ReactContext, config: ReactConfig): Promise<{
  content: string;
  reasoningContent?: string;
  rounds: number;
  usage?: TokenUsage;
}> {
  let content = '';
  let reasoning = '';
  let rounds = 0;
  let usage: TokenUsage | undefined;
  for await (const chunk of reactLoop(ctx, config)) {
    if (chunk.type === StreamChunkType.Content && chunk.content) {
      content += chunk.content;
    }
    if (chunk.type === StreamChunkType.Thinking && chunk.reasoningContent) {
      reasoning = chunk.reasoningContent;
    }
    if (chunk.round) rounds = chunk.round;
    if (chunk.usage) usage = chunk.usage;
  }
  return { content, reasoningContent: reasoning, rounds, usage };
}

/**
 * 流式优先的 LLM 调用:
 *  - provider.chatStream 可用时走流式,content/thinking 增量即时产出(实时反馈),
 *    工具调用增量按 id 合并(OpenAI 流式把参数拆进多个 delta);
 *  - chatStream 不存在或中途失败时降级为非流式 chat。
 */
async function streamOrChat(
  provider: LLMProvider,
  messages: Message[],
  tools: ToolDefinition[],
  options: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<LLMResponse> {
  // 无 chatStream 能力 → 直接非流式
  if (typeof (provider as { chatStream?: unknown }).chatStream !== 'function') {
    return provider.chat(messages, tools, options);
  }

  const contentParts: string[] = [];
  let reasoning = '';
  let toolCalls: ToolCall[] | undefined;
  let usage: TokenUsage | undefined;
  try {
    for await (const chunk of provider.chatStream(messages, tools, options)) {
      throwIfAborted(signal);
      if (chunk.type === StreamChunkType.Content && chunk.content) {
        contentParts.push(chunk.content);
      }
      if (chunk.type === StreamChunkType.Thinking && chunk.reasoningContent) {
        reasoning += chunk.reasoningContent;
      }
      // 流式工具调用增量按 id 合并(provider 已跨 chunk 累积参数,此处按 id 去重取最新)
      if (chunk.type === StreamChunkType.ToolCall && chunk.toolCallId) {
        const incoming: ToolCall = {
          id: chunk.toolCallId,
          type: 'function',
          function: { name: chunk.toolName ?? '', arguments: chunk.toolArgs ?? '' },
        };
        if (!toolCalls) toolCalls = [];
        const existing = toolCalls.find((t) => t.id === incoming.id || t.function.name === incoming.function.name);
        if (existing) {
          existing.function.arguments = incoming.function.arguments;
          existing.function.name = incoming.function.name || existing.function.name;
        } else {
          toolCalls.push(incoming);
        }
      }
      if (chunk.usage) usage = chunk.usage;
    }
  } catch (err) {
    // 流式中途失败且已收到部分内容 → 抛给上层;完全没内容 → 降级非流式重试一次
    if (contentParts.length > 0) throw err;
    return provider.chat(messages, tools, options);
  }

  const content = contentParts.join('');
  if (!content && !toolCalls?.length && !reasoning) {
    // 流结束但空(部分 provider 静默失败)→ 降级非流式
    return provider.chat(messages, tools, options);
  }
  return {
    content,
    reasoningContent: reasoning || undefined,
    toolCalls,
    usage,
  } as LLMResponse;
}
