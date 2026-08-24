/**
 * 资源作用域 —— 插件激活期间申请的一切可回收资源的统一账本。
 *
 * 设计动机：此前每种扩展面（工具/钩子/Provider/拦截器/人格）都要在
 * activate 里收集、再在 unload 里手写对应的反向清理——成对逻辑靠人肉维护，
 * 漏一处就是资源残留 bug。Scope 把"安装"与"回收"收敛到同一个收集点：
 * 申请资源时就地登记 dispose 闭包，卸载 = 释放整个作用域。
 * 新增扩展面时只需在收集处多写一行 scope.add(...)，无需再动 unload。
 */
export interface ScopeItemInfo {
  tag: string;
  name: string | null;
}

export class Scope {
  private items: Array<{ tag: string; name: string | null; dispose: () => void }> = [];

  /**
   * 登记一项资源及其回收方式。
   * @param tag 资源类别（'tool' | 'hook' | 'provider' | 'interceptor' | 'persona' …）
   * @param name 资源名（可选；供 list()/日志等清单展示）
   * @param dispose 卸载时执行的回收闭包
   */
  add(tag: string, name: string | null, dispose: () => void): void {
    this.items.push({ tag, name, dispose });
  }

  /** 某类资源的名称清单（按登记顺序） */
  names(tag: string): string[] {
    return this.items.filter((i) => i.tag === tag && i.name !== null).map((i) => i.name as string);
  }

  /** 某类资源的数量 */
  count(tag: string): number {
    return this.items.filter((i) => i.tag === tag).length;
  }

  /**
   * 逆序释放全部资源（后申请的先回收）。
   * 单项失败不阻断其余项，错误经 onError 上报；释放后作用域清空、可复用。
   */
  dispose(onError?: (err: unknown, item: ScopeItemInfo) => void): void {
    for (let i = this.items.length - 1; i >= 0; i--) {
      const item = this.items[i]!;
      try {
        item.dispose();
      } catch (err) {
        onError?.(err, { tag: item.tag, name: item.name });
      }
    }
    this.items.length = 0;
  }
}
