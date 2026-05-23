import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// 开发期：前端 5173，后端 8080。/api 与 /ws 转发到后端，避免 CORS 配置。
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:10001', changeOrigin: true },
      '/ws': { target: 'ws://localhost:10001', ws: true },
    },
  },
});
