import { describe, it, expect } from 'vitest';
import {
  appendTailAssistant,
  settleRunningTool,
  relTime,
  mapHistory,
  nextUiId,
} from './ui';
import type { UiMsg, UiAssistant, UiTool } from './ui';
import type { SessionDto } from './types';

describe('appendTailAssistant', () => {
  it('尾部不是助手消息时创建新的', () => {
    const msgs: UiMsg[] = [{ kind: 'user', id: 'u1', content: 'hi' }];
    const next = appendTailAssistant(msgs, (m) => ({ ...m, content: 'a' }));
    expect(next).toHaveLength(2);
    expect(next[1]).toMatchObject({ kind: 'assistant', content: 'a' });
    expect(next[0]).toBe(msgs[0]); // 原消息不可变
  });

  it('尾部位助手消息时原地续写（流式追加）', () => {
    const a: UiAssistant = { kind: 'assistant', id: 'u2', content: 'hel' };
    const next = appendTailAssistant([{ kind: 'user', id: 'u1', content: 'q' }, a], (m) => ({
      ...m,
      content: m.content + 'lo',
    }));
    expect(next).toHaveLength(2);
    expect(next[1]).toMatchObject({ content: 'hello' });
  });

  it('reasoning 与 content 互不覆盖', () => {
    let draft: UiAssistant = { kind: 'assistant', id: nextUiId(), content: '' };
    draft = appendTailAssistant([draft], (m) => ({ ...m, reasoning: (m.reasoning ?? '') + 'think' }))[0] as UiAssistant;
    draft = appendTailAssistant([draft], (m) => ({ ...m, content: m.content + 'text' }))[0] as UiAssistant;
    draft = appendTailAssistant([draft], (m) => ({ ...m, reasoning: (m.reasoning ?? '') + ' more' }))[0] as UiAssistant;
    expect(draft.reasoning).toBe('think more');
    expect(draft.content).toBe('text');
  });
});

describe('settleRunningTool', () => {
  const running = (id: string, name: string): UiTool => ({ kind: 'tool', id, name, status: 'running' });

  it('结算最后一条同名 running 卡片为 done', () => {
    const msgs: UiMsg[] = [running('a', 'read_file'), { kind: 'user', id: 'u', content: 'x' }, running('b', 'read_file')];
    const next = settleRunningTool(msgs, 'read_file', 'file body', undefined);
    const card = next.find((m) => m.kind === 'tool' && m.id === 'b') as UiTool;
    expect(card.status).toBe('done');
    expect(card.result).toBe('file body');
    // 更早的同名卡不受影响
    expect((next[0] as UiTool).status).toBe('running');
  });

  it('带 error 时结算为 failed', () => {
    const next = settleRunningTool([running('a', 'cmd')], 'cmd', undefined, 'boom');
    expect((next[0] as UiTool)).toMatchObject({ status: 'failed', error: 'boom' });
  });

  it('无匹配时原样返回（同引用）', () => {
    const msgs: UiMsg[] = [running('a', 'other')];
    expect(settleRunningTool(msgs, 'nope', 'r', undefined)).toBe(msgs);
  });
});

describe('relTime', () => {
  it('分级显示 now/m/h/d', () => {
    const now = Date.now();
    expect(relTime(now - 10_000)).toBe('now');
    expect(relTime(now - 5 * 60_000)).toBe('5m');
    expect(relTime(now - 3 * 3_600_000)).toBe('3h');
    expect(relTime(now - 2 * 86_400_000)).toBe('2d');
  });
});

describe('mapHistory', () => {
  const session: SessionDto = {
    id: 's1',
    projectId: 'default',
    userId: 'web',
    createdAt: 0,
    updatedAt: 0,
    messages: [
      { role: 'user', content: '请读文件' },
      {
        role: 'assistant',
        content: '',
        toolCalls: [{ id: 'tc1', type: 'function', function: { name: 'read_file', arguments: '{"path":"a.txt"}' } }],
      },
      { role: 'tool', toolCallId: 'tc1', content: 'file content here' },
      { role: 'assistant', content: '文件内容是…' },
    ],
  };

  it('还原 user/assistant/toolCalls/tool 结果四类消息', () => {
    const out = mapHistory(session);
    expect(out.map((m) => m.kind)).toEqual(['user', 'tool', 'assistant', 'assistant']);
    const card = out[1] as UiTool;
    expect(card).toMatchObject({ name: 'read_file', args: '{"path":"a.txt"}', result: 'file content here', status: 'done' });
  });

  it('无对应结果的工具卡保持 running 态（中断会话的真实形态）', () => {
    const truncated: SessionDto = { ...session, messages: session.messages.slice(0, 2) };
    const out = mapHistory(truncated);
    expect((out[1] as UiTool).status).toBe('running');
  });

  it('孤儿 tool 消息（无匹配 toolCallId）被忽略', () => {
    const orphan: SessionDto = { ...session, messages: [...session.messages, { role: 'tool', toolCallId: 'ghost', content: '?' }] };
    expect(mapHistory(orphan)).toHaveLength(4);
  });
});
