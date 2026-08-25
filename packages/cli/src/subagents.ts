/**
 * Agent-as-Tool —— 子代理编排。
 *
 * 一个子代理 = (persona, toolAllowlist) + 独立的临时 ReAct 循环。
 * 把它包装成普通 PluginTool 注册进内核后，父 agent 即可像调工具一样
 * "委派专项任务"，拿到子代理的最终结论作为工具结果——多级编排由此组合而生。
 *
 * 设计要点：
 *  - persona 懒解析（调用时经 getPersona 查询）：用户插件人格在 loadAll 之后
 *    才存在，工厂阶段拿不到也不需要拿到
 *  - 子会话完全隔离：每次调用用全新 MemorySessionStore + 随机 sessionId，
 *    不污染父会话与持久记忆
 *  - 工具面由 persona.toolAllowlist 决定（react 层过滤），父注册表共享但可见性受限
 *  - signal 透传：父级中断即级联中止子循环
 */
import { AgentKernel, MemorySessionStore, StreamChunkType, newId } from '@openaide/core';
import type { EventBus, LLMProvider, Persona, SessionStore, ToolExecutor } from '@openaide/core';
import type { OpenAIDePlugin, PluginTool } from '@openaide/plugins';

/** 子代理声明（config.kernel.subagents 条目） */
export interface SubAgentSpec {
  /** 工具名（subagent__<name>）；自动清洗为 [a-z0-9_-] */
  name: string;
  /** 引用的人格名（内置或任意插件提供） */
  persona: string;
  description?: string;
  /** 子循环最大轮数（缺省继承全局 max_rounds） */
  maxRounds?: number;
}

export interface SubAgentDeps {
  llm: LLMProvider;
  registry: ToolExecutor;
  /** 人格懒解析（装配层接 PluginManager.getPersona） */
  getPersona(name: string): Persona | undefined;
  eventBus?: EventBus;
  maxRounds: number;
}

function sanitizeName(raw: string): string {
  const n = raw.trim().toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-|-$/g, '');
  if (!n) throw new Error(`invalid subagent name: ${raw}`);
  return n;
}

/** 由声明生成一组工具（每个子代理一个）。人格缺失等错误延迟到调用期返回。 */
export function createSubAgentTools(specs: SubAgentSpec[], deps: SubAgentDeps): PluginTool[] {
  return specs.map((spec) => {
    const name = sanitizeName(spec.name);
    return {
      name,
      description:
        spec.description ??
        `子代理「${spec.persona}」——委派一项专项任务，参数 task 为任务描述，返回其最终答复`,
      parameters: {
        type: 'object',
        properties: { task: { type: 'string', description: '委派给该子代理的任务描述' } },
        required: ['task'],
      },
      handler: async (args: Record<string, unknown>, _sessionId: string, signal?: AbortSignal) => {
        const persona = deps.getPersona(spec.persona);
        if (!persona) {
          return {
            content: '',
            error: `subagent '${name}': persona '${spec.persona}' not found`,
            errorCode: 'NOT_FOUND',
          };
        }
        // 完全隔离的临时会话：不落盘、不复用
        const sessions: SessionStore = new MemorySessionStore();
        const kernel = new AgentKernel({
          llm: deps.llm,
          tools: deps.registry,
          sessions,
          persona: { active: () => Promise.resolve(persona) },
          eventBus: deps.eventBus,
          config: { maxRounds: spec.maxRounds ?? deps.maxRounds },
        });
        let out = '';
        try {
          for await (const chunk of kernel.processStream(
            {
              sessionId: newId(),
              projectId: `subagent:${name}`,
              userId: 'subagent',
              content: String(args.task ?? ''),
              options: {},
            },
            signal,
          )) {
            if (chunk.type === StreamChunkType.Content && chunk.content) out += chunk.content;
          }
        } catch (err) {
          if ((err as Error).name === 'AbortError') {
            return { content: '', error: 'subagent aborted', errorCode: 'TIMEOUT' };
          }
          return { content: '', error: `subagent failed: ${(err as Error).message}`, errorCode: 'EXEC_FAILED' };
        }
        return { content: out || '(subagent returned nothing)' };
      },
    };
  });
}

/** 内置子代理插件工厂：config.kernel.subagents → 单个 'subagents' 插件 */
export function createSubAgentsPlugin(
  specs: SubAgentSpec[],
  deps: SubAgentDeps,
): OpenAIDePlugin | undefined {
  if (specs.length === 0) return undefined;
  return {
    name: 'subagents',
    version: '0.3.0',
    description: `子代理编排（${specs.length} 个）：${specs.map((s) => s.name).join(', ')}`,
    category: 'workflow',
    tools: createSubAgentTools(specs, deps),
  };
}
