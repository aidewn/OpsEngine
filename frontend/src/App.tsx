// 路由配置

import { Navigate, Route, Routes } from 'react-router-dom';
import { HomePage } from '@/pages/HomePage';
import { WorkflowCanvasPage } from '@/pages/WorkflowCanvasPage';
import { AssembleCanvasPage } from '@/pages/AssembleCanvasPage';
import { ExecutionDetailPage } from '@/pages/ExecutionDetailPage';
import { EnvironmentDetailPage } from '@/pages/EnvironmentDetailPage';
import { SettingsEnvironmentsPage } from '@/pages/SettingsEnvironmentsPage';
import { TitleBar } from '@/components/layout/TitleBar';
import { AppShell } from '@/components/layout/AppShell';
import { CommandPalette } from '@/components/layout/CommandPalette';
import { ExecutionFinishedNotifier } from '@/components/layout/ExecutionFinishedNotifier';
import { Toaster } from 'sonner';

// App 组件：在现有路由外层挂载阶段 0 的自绘标题栏。
export function App() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-[#1A1A19]">
      <TitleBar />
      <CommandPalette />
      <ExecutionFinishedNotifier />
      <main className="min-h-0 flex-1 overflow-hidden">
        <AppShell>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/workflows/:id" element={<WorkflowCanvasPage />} />
            <Route path="/assembles/:id" element={<AssembleCanvasPage />} />
            <Route path="/executions/:id" element={<ExecutionDetailPage />} />
            <Route path="/environments/:id" element={<EnvironmentDetailPage />} />
            <Route path="/settings/environments" element={<SettingsEnvironmentsPage />} />
            <Route
              path="/settings/environments/:id"
              element={<EnvironmentDetailPage backTo="/settings/environments" />}
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </AppShell>
      </main>
      <Toaster
        theme="dark"
        richColors
        toastOptions={{
          classNames: {
            toast: 'border border-ops-border-subtle bg-ops-elevated text-ops-primary',
            description: 'text-ops-secondary',
            actionButton: 'bg-ops-accent text-ops-inverse hover:bg-ops-accent-hover',
          },
        }}
      />
    </div>
  );
}
