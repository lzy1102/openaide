/**
 * 工具注册表 —— 内核 ToolExecutor 实现。
 * 线程无关（Node 单线程），工具按全名（含插件前缀）注册/分发/注销。
 */
import { ToolCall, ToolDefinition, ToolHandler, ToolResult, ToolExecutor } from '@openaide/core';

export class ToolRegistry implements ToolExecutor {
  private defs = new Map<string, ToolDefinition>();
  private handlers = new Map<string, ToolHandler>();

  register(def: ToolDefinition, handler: ToolHandler): void {
    const name = def.function.name;
    if (!name) throw new Error('tool name is empty');
    if (this.defs.has(name)) {
      throw new Error(`tool already registered: ${name}`);
    }
    this.defs.set(name, def);
    this.handlers.set(name, handler);
  }

  unregister(name: string): void {
    this.defs.delete(name);
    this.handlers.delete(name);
  }

  definitions(): ToolDefinition[] {
    return [...this.defs.values()];
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
