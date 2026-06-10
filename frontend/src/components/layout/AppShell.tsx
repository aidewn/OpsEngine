// AppShell 组件：阶段 2 的整体布局，包含侧栏和主内容区域。
import type { ReactNode } from 'react';
import { useLocation } from 'react-router-dom';
import { Sidebar } from './Sidebar';

interface AppShellProps {
  children: ReactNode;
}

// AppShell 在环境详情等旧路由中仍保留侧栏，保证 deep link 不丢。
export function AppShell({ children }: AppShellProps) {
  const location = useLocation();
  const isFullscreenSettings = location.pathname.startsWith('/settings/');

  return (
    <div className="flex h-full min-h-0 bg-ops-canvas text-ops-primary">
      {!isFullscreenSettings ? <Sidebar /> : null}
      <section className="min-w-0 flex-1 overflow-hidden">{children}</section>
    </div>
  );
}
