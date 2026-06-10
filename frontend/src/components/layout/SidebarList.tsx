// SidebarList 组件：统一侧栏滚动列表容器。
import type { ReactNode } from 'react';

interface SidebarListProps {
  children: ReactNode;
}

export function SidebarList({ children }: SidebarListProps) {
  return <div className="min-h-0 flex-1 overflow-y-auto py-2">{children}</div>;
}
