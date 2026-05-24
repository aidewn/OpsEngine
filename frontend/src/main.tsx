// 应用入口：挂载 QueryClient + Router
// Wails 桌面应用使用 HashRouter（无 HTTP server 做 fallback）

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { HashRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App } from './App';
import { TabsProvider } from '@/features/tabs/TabsContext';
import { ExecutionStoreProvider } from '@/features/execution/ExecutionStore';
import './index.css';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

const root = document.getElementById('root');
if (!root) throw new Error('找不到根元素 #root');

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <HashRouter>
        <TabsProvider>
          <ExecutionStoreProvider>
            <App />
          </ExecutionStoreProvider>
        </TabsProvider>
      </HashRouter>
    </QueryClientProvider>
  </StrictMode>,
);
