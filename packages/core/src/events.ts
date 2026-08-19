/**
 * 事件总线 —— 内核事件发布/订阅。
 * 插件钩子、TUI、API 都通过它感知内核状态。
 */
import { KernelEvent } from './types.js';

/** 事件处理器 */
export type EventHandler = (event: KernelEvent) => void;

/** 带 ID 的跟踪处理器，用于安全取消订阅 */
interface TrackedHandler {
  id: number;
  handler: EventHandler;
}

/**
 * 事件总线：线程无关（Node 单线程），使用微任务队列分发，
 * 单个 handler 抛错不影响其他 handler。
 */
export class EventBus {
  private handlers: TrackedHandler[] = [];
  private seq = 0;

  /** 订阅事件，返回 handler ID */
  subscribe(handler: EventHandler): number {
    this.handlers.push({ id: ++this.seq, handler });
    return this.seq;
  }

  /** 通过 ID 取消订阅 */
  unsubscribe(id: number): void {
    this.handlers = this.handlers.filter((h) => h.id !== id);
  }

  /** 发布事件：异步分发，逐 handler 捕获异常 */
  publish(event: KernelEvent): void {
    for (const th of [...this.handlers]) {
      queueMicrotask(() => {
        try {
          th.handler(event);
        } catch (err) {
          console.warn('[event] handler panicked', event.type, th.id, err);
        }
      });
    }
  }

  /** 同步发布（内部场景：需要严格顺序时） */
  publishSync(event: KernelEvent): void {
    for (const th of [...this.handlers]) {
      try {
        th.handler(event);
      } catch (err) {
        console.warn('[event] handler panicked', event.type, th.id, err);
      }
    }
  }

  handlerCount(): number {
    return this.handlers.length;
  }
}
