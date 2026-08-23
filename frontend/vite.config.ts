import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 开发模式：代理到本地 openaide serve（同源，无 CORS）；
// 生产模式：npm run build 产物由 openaide serve 直接静态托管（同源相对路径）。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/health': 'http://127.0.0.1:8080',
      '/sessions': 'http://127.0.0.1:8080',
      '/v1': 'http://127.0.0.1:8080',
    },
  },
});
