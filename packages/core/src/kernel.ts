/**
 * AgentKernel —— 内核装配点。
 * 收敛四大功能：消息/会话、ReAct 循环、提示词组装、事件总线。
 * 所有能力经接口注入，不感知具体实现（工具、记忆、人格皆可插拔）。
 */
import {
  EventTypes,
  KernelEvent,
  KernelState,
  LLMResponse,
  Message,
  Query,
  Response,
  StateIdle,
  StateProcessing,
  StreamChunk,
  StreamChunkType,
} from './types.js';
import type {
  ContextCompressor,
  EventHandler,
  LLMProvider,
  Memory,
  PersonaProvider,
  PermissionChecker,
  SessionStore,
  ToolExecutor,
} from './interfaces.js';
import { EventBus } from './events.js';
import { appendMessages, MemorySessionStore, newId } from './session.js';
import { buildMessages, buildSystemLayer } from './prompt.js';
import { collectReact, reactLoop, ReactContext, ReactConfig } from './react.js';

export interface KernelConfig {
  maxRounds?: number;
  maxTokens?: number;
  systemPrompt?: string;
}

export interface KernelDeps {
  llm: LLMProvider;
  tools: ToolExecutor;
  memory?: Memory;
  sessions?: SessionStore;
  persona?: PersonaProvider;
  compressor?: ContextCompressor;
  permission?: PermissionChecker;
  config?: KernelConfig;
  /** 外部注入的事件总线（默认内部新建）。与插件管理器共享同一总线时传入 */
  eventBus?: EventBus;
}

export class AgentKernel {
  readonly llm: LLMProvider;
  readonly tools: ToolExecutor;
  readonly memory?: Memory;
  readonly sessions: SessionStore;
  readonly persona?: PersonaProvider;
  readonly compressor?: ContextCompressor;
  readonly permission?: PermissionChecker;

  private readonly eventBus: EventBus;
  private state: KernelState = StateIdle;
  private systemPrompt: string;
  private maxRounds: number;
  private maxTokens: number;

  constructor(deps: KernelDeps) {
    this.llm = deps.llm;
    this.tools = deps.tools;
    this.memory = deps.memory;
    this.sessions = deps.sessions ?? new MemorySessionStore();
    this.persona = deps.persona;
    this.compressor = deps.compressor;
    this.permission = deps.permission;
    this.eventBus = deps.eventBus ?? new EventBus();
    this.systemPrompt = deps.config?.systemPrompt ?? '';
    this.maxRounds = deps.config?.maxRounds ?? 10;
    // 上下文 token 预算(用于压缩阈值/历史裁剪),非单次输出上限;默认对齐主流 200K 窗口
    this.maxTokens = deps.config?.maxTokens ?? 200_000;
  }

  // ── 状态 / 事件 ──────────────────────────────────────────
  getState(): KernelState {
    return this.state;
  }

  subscribe(handler: EventHandler): number {
    return this.eventBus.subscribe(handler);
  }

  unsubscribe(id: number): void {
    this.eventBus.unsubscribe(id);
  }

  getSlashCommands(): Map<string, string> {
    return new Map();
  }

  // ── 会话 ────────────────────────────────────────────────
  async getOrCreateSession(query: Query) {
    if (query.sessionId) {
      const existing = await this.sessions.get(query.sessionId);
      if (existing) return existing;
    }
    const session = await this.sessions.create(query.projectId, query.userId);
    this.publish({ type: EventTypes.SessionCreated, source: 'kernel', data: { session_id: session.id }, timestamp: Date.now() });
    return session;
  }

  // ── 主入口 ──────────────────────────────────────────────
  async process(query: Query, signal?: AbortSignal): Promise<Response> {
    this.setState(StateProcessing);
    this.publish({ type: EventTypes.QueryStart, source: 'kernel', data: { sessionId: query.sessionId }, timestamp: Date.now() });

    const session = await this.getOrCreateSession(query);
    const persona = await this.persona?.active();
    const systemLayer = buildSystemLayer({
      persona,
      customSystemPrompt: this.systemPrompt,
      projectContext: query.options.projectContext,
    });

    const history = this.memory
      ? await this.memory.load(session.id, this.maxRounds > 20 ? 200 : 20)
      : session.messages.slice(-20);

    const messages = buildMessages(systemLayer, history, query, {
      persona,
      customSystemPrompt: this.systemPrompt,
      projectContext: query.options.projectContext,
    });

    // 持久化用户消息
    appendMessages(session, [{ role: 'user', content: query.content }]);
    await this.sessions.update(session);
    await this.memory?.save(session.id, [{ role: 'user', content: query.content }]);

    const ctx: ReactContext = {
      provider: this.llm,
      executor: this.tools,
      messages,
      sessionId: session.id,
      publish: (e) => this.publish(e),
      signal,
      options: buildOptions(query.options),
    };
    const reactConfig: ReactConfig = { maxRounds: this.maxRounds };

    const result = await collectReact(ctx, reactConfig);

    // 持久化 assistant 消息
    appendMessages(session, [{ role: 'assistant', content: result.content, reasoningContent: result.reasoningContent }]);
    await this.sessions.update(session);
    await this.memory?.save(session.id, [{ role: 'assistant', content: result.content, reasoningContent: result.reasoningContent }]);

    this.setState(StateIdle);
    this.publish({ type: EventTypes.QueryEnd, source: 'kernel', data: { sessionId: session.id, rounds: result.rounds }, timestamp: Date.now() });

    return {
      content: result.content,
      reasoningContent: result.reasoningContent,
      sessionId: session.id,
      round: result.rounds,
      totalRounds: result.rounds,
      usage: result.usage,
    };
  }

  /** 流式入口：透传 ReAct 生成器 */
  async *processStream(query: Query, signal?: AbortSignal): AsyncGenerator<StreamChunk> {
    this.setState(StateProcessing);
    this.publish({ type: EventTypes.QueryStart, source: 'kernel', data: { sessionId: query.sessionId }, timestamp: Date.now() });

    const session = await this.getOrCreateSession(query);
    const persona = await this.persona?.active();
    const systemLayer = buildSystemLayer({
      persona,
      customSystemPrompt: this.systemPrompt,
      projectContext: query.options.projectContext,
    });
    const history = this.memory
      ? await this.memory.load(session.id, this.maxRounds > 20 ? 200 : 20)
      : session.messages.slice(-20);
    const messages = buildMessages(systemLayer, history, query, {
      persona,
      customSystemPrompt: this.systemPrompt,
      projectContext: query.options.projectContext,
    });

    appendMessages(session, [{ role: 'user', content: query.content }]);
    await this.sessions.update(session);
    await this.memory?.save(session.id, [{ role: 'user', content: query.content }]);

    const ctx: ReactContext = {
      provider: this.llm,
      executor: this.tools,
      messages,
      sessionId: session.id,
      publish: (e) => this.publish(e),
      signal,
      options: buildOptions(query.options),
    };

    let finalContent = '';
    let finalReasoning = '';
    try {
      for await (const chunk of reactLoop(ctx, { maxRounds: this.maxRounds })) {
        if (chunk.type === StreamChunkType.Content && chunk.content) finalContent += chunk.content;
        if (chunk.type === StreamChunkType.Thinking && chunk.reasoningContent) finalReasoning = chunk.reasoningContent;
        yield chunk;
      }
    } finally {
      if (finalContent || finalReasoning) {
        appendMessages(session, [{ role: 'assistant', content: finalContent, reasoningContent: finalReasoning }]);
        await this.sessions.update(session);
        await this.memory?.save(session.id, [{ role: 'assistant', content: finalContent, reasoningContent: finalReasoning }]);
      }
      this.setState(StateIdle);
      this.publish({ type: EventTypes.QueryEnd, source: 'kernel', data: { sessionId: session.id }, timestamp: Date.now() });
    }
  }

  // ── 内部 ────────────────────────────────────────────────
  private setState(s: KernelState): void {
    this.state = s;
    this.publish({ type: EventTypes.StateChanged, source: 'kernel', data: { state: s }, timestamp: Date.now() });
  }

  private publish(event: KernelEvent): void {
    this.eventBus.publish(event);
  }
}

function buildOptions(opts: Query['options']): Record<string, unknown> {
  const options: Record<string, unknown> = {};
  if (opts.temperature && opts.temperature > 0) options['temperature'] = opts.temperature;
  if (opts.maxTokens && opts.maxTokens > 0) options['max_tokens'] = opts.maxTokens;
  if (opts.responseFormat) options['response_format'] = opts.responseFormat;
  return options;
}
