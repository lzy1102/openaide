/**
 * 工具注册表 —— 内核 ToolExecutor 实现。
 * 线程无关（Node 单线程），工具按全名（含插件前缀）注册/分发/注销。
 */
import { ToolCall, ToolDefinition, ToolHandler, ToolResult, ToolExecutor, ToolRegisterOptions } from '@openaide/core';

export class ToolRegistry implements ToolExecutor {
  private defs = new Map<string, ToolDefinition>();
  private handlers = new Map<string, ToolHandler>();
  private dangerous = new Set<string>();

  register(def: ToolDefinition, handler: ToolHandler, options?: ToolRegisterOptions): void {
    const name = def.function.name;
    if (!name) throw new Error('tool name is empty');
    if (this.defs.has(name)) {
      throw new Error(`tool already registered: ${name}`);
    }
    this.defs.set(name, def);
    this.handlers.set(name, handler);
    if (options?.dangerous) this.dangerous.add(name);
  }

  unregister(name: string): void {
    this.defs.delete(name);
    this.handlers.delete(name);
    this.dangerous.delete(name);
  }

  definitions(): ToolDefinition[] {
    return [...this.defs.values()];
  }

  /** 危险标记单独存放：不进 ToolDefinition，避免污染发给 LLM 的 tools 载荷 */
  isDangerous(name: string): boolean {
    return this.dangerous.has(name);
  }

  getHandler(name: string): ToolHandler | undefined {
    return this.handlers.get(name);
  }

  async execute(toolCall: ToolCall, sessionId: string, signal?: AbortSignal): Promise<ToolResult> {
    const name = toolCall.function.name;
    const handler = this.handlers.get(name);
    if (!handler) {
      return { content: '', error: `unknown tool: ${name}`, errorCode: 'NOT_FOUND' };
    }
    try {
      return await handler(toolCall.function.arguments, sessionId, signal);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      const isTimeout = message.toLowerCase().includes('timeout');
      return {
        content: '',
        error: message,
        errorCode: isTimeout ? 'TIMEOUT' : 'EXEC_FAILED',
        isRetryable: isTimeout,
      };
    }
  }
}
