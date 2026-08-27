/**
 * notify —— 任务完成/失败推送通知（长任务 + 无限重试场景的好搭档）。
 *
 * 后端按环境变量自动发现（零配置即用，缺省时返回配置指引）：
 *   NTFY_URL      完整 ntfy 地址（如 https://ntfy.sh/my-topic）
 *   NTFY_TOPIC    简写（展开为 https://ntfy.sh/<topic>）
 *   BARK_URL      Bark iOS 推送地址（如 https://api.day.app/xxx/）
 *   WEBHOOK_URL   任意 webhook（POST {title,message,level}）
 *
 * 全部后端均通过 fetch 直发，无额外依赖。
 */
import type { OpenAIDePlugin } from '@openaide/plugins';

interface Backend {
  name: string;
  url: string;
  send(title: string, message: string, level: string): Promise<void>;
}

function resolveBackends(env: NodeJS.ProcessEnv = process.env): Backend[] {
  const list: Backend[] = [];

  const ntfyUrl = env.NTFY_URL || (env.NTFY_TOPIC ? `https://ntfy.sh/${env.NTFY_TOPIC}` : '');
  if (ntfyUrl) {
    list.push({
      name: 'ntfy',
      url: ntfyUrl,
      async send(title, message, level) {
        const priority = level === 'error' ? '5' : level === 'warn' ? '4' : '3';
        const res = await fetch(ntfyUrl, {
          method: 'POST',
          headers: { Title: title, Priority: priority, Tags: level },
          body: message,
        });
        if (!res.ok) throw new Error(`ntfy ${res.status}: ${await res.text().catch(() => '')}`);
      },
    });
  }

  if (env.BARK_URL) {
    const base = env.BARK_URL.replace(/\/+$/, '');
    list.push({
      name: 'bark',
      url: base,
      async send(title, message) {
        const url = `${base}/${encodeURIComponent(title)}/${encodeURIComponent(message)}`;
        const res = await fetch(url);
        if (!res.ok) throw new Error(`bark ${res.status}`);
      },
    });
  }

  if (env.WEBHOOK_URL) {
    list.push({
      name: 'webhook',
      url: env.WEBHOOK_URL,
      async send(title, message, level) {
        const res = await fetch(env.WEBHOOK_URL!, {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ title, message, level }),
        });
        if (!res.ok) throw new Error(`webhook ${res.status}`);
      },
    });
  }

  return list;
}

export const notifyPlugin: OpenAIDePlugin = {
  name: 'notify',
  version: '0.3.0',
  description: '推送通知：ntfy / Bark / 任意 webhook（长任务完成提醒）',
  category: 'capability',

  tools: [
    {
      name: 'notify',
      description:
        '发送推送通知给用户（长任务、后台任务完成时使用）。后端由环境变量配置：NTFY_URL 或 NTFY_TOPIC（ntfy.sh 免费）、BARK_URL（iOS Bark）、WEBHOOK_URL（任意地址）。',
      parameters: {
        type: 'object',
        properties: {
          message: { type: 'string', description: '通知正文（一句话总结结果）' },
          title: { type: 'string', description: '通知标题（默认 OpenAIDE）' },
          level: { type: 'string', enum: ['info', 'warn', 'error'], description: '级别（影响优先级/图标）' },
        },
        required: ['message'],
      },
      handler: async (args) => {
        const message = String(args.message ?? '').trim();
        if (!message) return { content: '', error: 'message required', errorCode: 'INVALID_ARGS' };
        const title = String(args.title ?? 'OpenAIDE');
        const level = String(args.level ?? 'info');

        const backends = resolveBackends();
        if (backends.length === 0) {
          return {
            content: '',
            error:
              'no notify backend configured. Set NTFY_URL=https://ntfy.sh/<topic> | NTFY_TOPIC=<topic> | BARK_URL | WEBHOOK_URL',
            errorCode: 'NOT_CONFIGURED',
          };
        }

        const results: string[] = [];
        for (const b of backends) {
          try {
            await b.send(title, message, level);
            results.push(`${b.name}: ok`);
          } catch (err) {
            results.push(`${b.name}: ${(err as Error).message}`);
          }
        }
        return { content: results.join('\n') };
      },
    },
  ],
};
