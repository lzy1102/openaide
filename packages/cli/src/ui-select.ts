/**
 * UI 选择器 —— 纯函数，便于测试。
 * 优先级：显式指定（OPENAIDE_UI / config.ui）> TTY 默认 > 回退链。
 */

export interface UiChoiceInput {
  /** 已注册的界面名 */
  available: string[];
  /** 显式指定（config.ui / OPENAIDE_UI），未设为 undefined */
  wanted?: string;
  /** 当前是否交互终端（stdin+stdout 均 TTY） */
  isTTY: boolean;
}

export interface UiChoiceResult {
  name: string | null;
  /** 非空时为回退原因提示（显式指定的界面不存在等） */
  warning?: string;
}

/** 默认偏好链：TTY → ink → readline；非 TTY → readline → ink */
export function chooseUi({ available, wanted, isTTY }: UiChoiceInput): UiChoiceResult {
  const has = (n: string): boolean => available.includes(n);

  if (wanted) {
    if (has(wanted)) return { name: wanted };
    // 显式指定但缺失：按默认链回退并告警
    const fb = fallback(isTTY, available);
    return {
      name: fb,
      warning: `ui '${wanted}' not registered (available: ${available.join(', ') || 'none'}), falling back to '${fb ?? 'none'}'`,
    };
  }

  return { name: fallback(isTTY, available) };

  function fallback(tty: boolean, avail: string[]): string | null {
    for (const n of tty ? ['ink', 'readline'] : ['readline', 'ink']) {
      if (has(n)) return n;
    }
    return avail[0] ?? null; // 用户自定义环境：有啥用啥
  }
}
