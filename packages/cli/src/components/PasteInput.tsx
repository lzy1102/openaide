/**
 * PasteInput —— 自管输入框：在 ink-text-input 基础能力上增加"粘贴折叠"。
 *
 * 为什么不直接用 ink-text-input：多行粘贴的换行会被当成回车把半截内容发出去，
 * 且它无法感知粘贴事件。这里自行解释按键，并利用终端的括号化粘贴模式
 * （ESC[200~ … ESC[201~）把大段粘贴识别为一个整体：
 *   - 短单行粘贴 → 直接内联进输入
 *   - 多行/超长粘贴 → 折叠为占位符 [粘贴#N +L行]（Claude Code 同款交互），
 *     回车提交时再还原成完整内容发给模型
 * 不支持括号化粘贴的老终端走启发式：输入块里出现换行即视为粘贴。
 */
import React, { useRef } from 'react';
import { Box, Text, useInput } from 'ink';

const PASTE_START = '\x1b[200~';
const PASTE_END = '\x1b[201~';
/** 超过该长度或含换行的粘贴折叠为占位符 */
const INLINE_LIMIT = 60;

interface Attachment {
  id: number;
  content: string;
}

export function PasteInput({
  value,
  onChange,
  onSubmit,
  busy,
  placeholder = 'Send a message or type / for commands…',
}: {
  value: string;
  onChange(next: string): void;
  /** 收到回车：参数为已还原粘贴内容的最终文本 */
  onSubmit(finalText: string): void;
  busy: boolean;
  placeholder?: string;
}): React.ReactElement {
  const attachments = useRef<Attachment[]>([]);
  const seq = useRef(0);
  const inPaste = useRef(false);
  const valueRef = useRef(value);
  valueRef.current = value;

  /** 把一段粘贴内容落进输入：短的直接内联，长的折叠为占位符 */
  const absorbPaste = (raw: string): void => {
    const text = raw.replace(/\r\n/g, '\n').replace(/\n+$/, '');
    if (!text) return;
    if (text.length <= INLINE_LIMIT && !text.includes('\n')) {
      onChange(valueRef.current + text);
      return;
    }
    const id = ++seq.current;
    const lines = text.split('\n').length;
    attachments.current.push({ id, content: text });
    onChange(`${valueRef.current}[粘贴#${id} +${lines}行]`);
  };

  /** 提交前还原全部占位符；未被引用的附件追加到末尾，保证内容零丢失 */
  const composeFinal = (): string => {
    let final = valueRef.current;
    for (const a of attachments.current) {
      const tokenRe = new RegExp(`\\[粘贴#${a.id}(?:[^\\]]*)\\]`);
      if (tokenRe.test(final)) final = final.replace(tokenRe, a.content);
      else final += (final ? '\n\n' : '') + a.content;
    }
    return final;
  };

  /** 解释一个输入事件（含括号化粘贴的状态机） */
  const handleEvent = (ch: string, key: { return?: boolean; backspace?: boolean; delete?: boolean }): void => {
    // ── 括号化粘贴 ──
    if (inPaste.current || ch.includes(PASTE_START)) {
      let rest = ch;
      if (rest.includes(PASTE_START)) rest = rest.slice(rest.indexOf(PASTE_START) + PASTE_START.length);
      inPaste.current = !rest.includes(PASTE_END);
      if (rest.includes(PASTE_END)) rest = rest.slice(0, rest.indexOf(PASTE_END));
      absorbPaste(rest);
      return;
    }
    // ── 回车：提交（先还原粘贴）──
    if (key.return) {
      const final = composeFinal().trim();
      if (!final) return;
      attachments.current = [];
      onChange('');
      onSubmit(final);
      return;
    }
    // ── 退格 ──
    if (key.backspace || key.delete) {
      onChange(valueRef.current.slice(0, -1));
      return;
    }
    // ── 无标记的多行粘贴（老终端启发式）：整块按粘贴处理 ──
    if (/[\n\r]/.test(ch)) {
      absorbPaste(ch.replace(/[\r\n]+$/, ''));
      return;
    }
    // ── 可打印字符 ──
    if (ch && !key.backspace && !key.delete && ch >= ' ' && ch !== '\x7f') {
      onChange(valueRef.current + ch);
    }
  };

  useInput((ch, key) => {
    if (busy) return;
    if (key.ctrl) return; // Ctrl+C 由外层处理
    handleEvent(ch, key);
  });

  const showPlaceholder = !value;

  return (
    <Box flexDirection="column" flexGrow={1}>
      <Text>
        {showPlaceholder ? (
          <>
            <Text dimColor>{placeholder}</Text>
            <Text inverse> </Text>
          </>
        ) : (
          <>
            {value}
            <Text inverse> </Text>
          </>
        )}
      </Text>
      {attachments.current.length > 0 && (
        <Text dimColor>
          {`📎 ${attachments.current.length} 段粘贴内容将随消息一并发送`}
        </Text>
      )}
    </Box>
  );
}
