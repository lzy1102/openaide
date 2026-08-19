/**
 * serve 命令 —— 启动 HTTP/WebSocket API 服务。
 * 依赖注入：把装配好的 App 适配为 ApiContext。
 * 端口：环境变量 OPENAIDE_PORT，默认 8080；宿主：OPENAIDE_HOST，默认 127.0.0.1。
 */
import { startApiServer } from '@openaide/api';
import type { App } from './app.js';

export async function runServe(app: App): Promise<void> {
  const port = Number(process.env.OPENAIDE_PORT ?? 8080);
  const host = process.env.OPENAIDE_HOST ?? '127.0.0.1';
  const server = await startApiServer(
    {
      kernel: app.kernel,
      sessions: app.sessions,
      service: { name: 'openaide', version: '0.3.0', model: app.config.llm.model },
    },
    port,
    host,
  );
  console.log(`OpenAIDE API listening on http://${host}:${port}`);
  console.log(`  GET /health   POST /v1/chat   POST /v1/chat/stream   WS /ws`);
  console.log('  Ctrl+C to stop.');

  await new Promise<void>((resolve) => {
    const shutdown = () => {
      server.close(() => resolve());
    };
    process.on('SIGINT', shutdown);
    process.on('SIGTERM', shutdown);
  });
}
