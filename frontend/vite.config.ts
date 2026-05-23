import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// Wails 桌面应用：不再需要 HTTP proxy，前端通过 wailsjs 绑定直接调用 Go 方法
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@wails': path.resolve(__dirname, './wailsjs'),
    },
  },
});
