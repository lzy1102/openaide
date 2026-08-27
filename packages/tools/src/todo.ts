/**
 * todo —— agent 自管理任务计划（长任务防跑偏，Claude Code TodoWrite 同款模式）。
 *
 * 模型经 todo_write 全量覆写当前会话的计划清单；
 * 状态经 EventBus 以 'todo.updated' 事件广播，UI（TUI/REPL）订阅渲染。
 * 存储为进程内按会话隔离的内存态（进程退出即清空——计划是过程产物，无需持久化）。
 */
import type { OpenAIDePlugin, PluginTool } from '@openaide/plugins';
import type { EventBus } from '@openaide/core';

export type TodoStatus = 'pending' | 'in_progress' | 'completed';

export interface TodoItem {
  content: string;
  status: TodoStatus;
}

const STATUSES: TodoStatus[] = ['pending', 'in_progress', 'completed'];
const MAX_ITEMS = 20;
const MAX_CONTENT = 300;

/** 会话 → 当前清单 */
const store = new Map<string, TodoItem[]>();

export function getTodos(sessionId: string): TodoItem[] {
  return store.get(sessionId) ?? [];
}

function publish(bus: EventBus | undefined, sessionId: string, todos: TodoItem[]): void {
  bus?.publish({
    type: 'todo.updated',
    source: 'plugin',
    data: { sessionId, todos },
    timestamp: Date.now(),
  } as never);
}

export function createTodoPlugin(bus?: EventBus): OpenAIDePlugin {
  return {
    name: 'todo',
    version: '0.3.0',
    description: '任务计划自管理（长任务防跑偏）',
    category: 'workflow',

    tools: [
      {
        name: 'todo_write',
        description:
          '写入/更新你的任务计划清单（全量覆写）。使用纪律：收到多步任务先列计划；' +
          '开始某项前将其置为 in_progress；完成后立即置为 completed 再继续下一项；' +
          '计划变化时随时重写。单步小任务不必使用。',
        parameters: {
          type: 'object',
          properties: {
            todos: {
              type: 'array',
              description: '完整计划清单（全量覆写）',
              items: {
                type: 'object',
                properties: {
                  content: { type: 'string', description: '任务内容（一句话，祈使式）' },
                  status: { type: 'string', enum: STATUSES, description: 'pending/in_progress/completed' },
                },
                required: ['content', 'status'],
              },
            },
          },
          required: ['todos'],
        },
        handler: async (args, sessionId) => {
          const raw = (args.todos ?? []) as Array<{ content?: unknown; status?: unknown }>;
          if (!Array.isArray(raw) || raw.length === 0) {
            return { content: '', error: 'todos must be a non-empty array', errorCode: 'INVALID_ARGS' };
          }
          if (raw.length > MAX_ITEMS) {
            return { content: '', error: `too many items (max ${MAX_ITEMS})`, errorCode: 'INVALID_ARGS' };
          }
          const items: TodoItem[] = [];
          for (const t of raw) {
            const content = String(t?.content ?? '').trim();
            const status = String(t?.status ?? 'pending') as TodoStatus;
            if (!content) return { content: '', error: 'empty todo content', errorCode: 'INVALID_ARGS' };
            if (content.length > MAX_CONTENT) content.slice(0, MAX_CONTENT);
            if (!STATUSES.includes(status)) {
              return { content: '', error: `invalid status "${status}"`, errorCode: 'INVALID_ARGS' };
            }
            items.push({ content: content.slice(0, MAX_CONTENT), status });
          }
          store.set(sessionId, items);
          publish(bus, sessionId, items);
          const done = items.filter((i) => i.status === 'completed').length;
          return { content: `plan saved (${done}/${items.length} completed)` };
        },
      },
    ],
  };
}
