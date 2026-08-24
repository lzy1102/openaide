/**
 * 示例 UI 插件 —— 约 40 行自研界面：极简问答循环。
 * 演示 ui 插件接缝：OPENAIDE_UI=mini（或 config.ui: mini）即可替换内置 TUI/REPL，
 * 宿主只交付 UiRuntime（kernel + registry + …），界面长什么样完全是插件自己的事。
 */
import type { OpenAIDePlugin, UiRuntime } from '@openaide/plugins';
import { StreamChunkType, newId } from '@openaide/core';
import { createInterface } from 'node:readline';

const plugin: OpenAIDePlugin = {
  name: 'example-mini-ui',
  version: '0.1.0',
  description: '示例界面插件：40 行极简问答 UI（OPENAIDE_UI=mini 启用）',
  category: 'ui',
  uis: [
    {
      name: 'mini',
      description: '极简问答界面',
      start: (host: UiRuntime) =>
        new Promise<void>((resolve) => {
          const rl = createInterface({ input: process.stdin, output: process.stdout });
          let sessionId: string | undefined;
          console.log('[mini-ui] 极简界面已启动。输入问题回车发送，/quit 退出。');

          const ask = (): void => {
            rl.question('> ', async (line) => {
              const text = line.trim();
              if (!text) return ask();
              if (text === '/quit' || text === '/exit') {
                rl.close();
                console.log('[mini-ui] bye.');
                return resolve();
              }
              sessionId ??= newId();
              process.stdout.write('… ');
              try {
                for await (const chunk of host.kernel.processStream({
                  sessionId,
                  projectId: 'default',
                  userId: 'mini-ui',
                  content: text,
                  options: {},
                })) {
                  if (chunk.type === StreamChunkType.Content && chunk.content) {
                    process.stdout.write(chunk.content);
                  }
                  if (chunk.type === StreamChunkType.Error && chunk.error) {
                    process.stdout.write(`\n[error] ${chunk.error.message ?? chunk.error}`);
                  }
                }
              } catch (err) {
                process.stdout.write(`\n[error] ${(err as Error).message}`);
              }
              process.stdout.write('\n');
              ask();
            });
          };
          rl.on('close', () => resolve());
          ask();
        }),
    },
  ],
};

export default plugin;
