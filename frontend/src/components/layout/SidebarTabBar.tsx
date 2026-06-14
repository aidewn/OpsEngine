// SidebarTabBar 组件：渲染侧栏顶部的一级导航。
import {
  Activity,
  FileText,
  GitBranch,
  MessageSquare,
  type LucideIcon,
} from 'lucide-react';
import { cn } from '@/lib/cn';

export type SidebarTab = 'chat' | 'workflow' | 'monitor' | 'reports';

interface SidebarTabBarProps {
  activeTab: SidebarTab;
  collapsed: boolean;
  onTabChange: (tab: SidebarTab) => void;
}

const tabs: Array<{ value: SidebarTab; label: string; Icon: LucideIcon }> = [
  { value: 'chat', label: 'Chat', Icon: MessageSquare },
  { value: 'workflow', label: '工作流', Icon: GitBranch },
  { value: 'monitor', label: '监控', Icon: Activity },
  { value: 'reports', label: '报告', Icon: FileText },
];

export function SidebarTabBar({
  activeTab,
  collapsed,
  onTabChange,
}: SidebarTabBarProps) {
  return (
    <nav className="space-y-1 p-2">
      {tabs.map((tab) => {
        const active = activeTab === tab.value;
        return (
          <button
            key={tab.value}
            type="button"
            className={cn(
              'flex h-9 w-full items-center gap-2 rounded-md border-l-2 px-2 text-sm transition-colors',
              collapsed && 'justify-center px-0',
              active
                ? 'border-ops-accent bg-ops-surface text-ops-primary'
                : 'border-transparent text-ops-secondary hover:bg-ops-surface/60 hover:text-ops-primary',
            )}
            title={tab.label}
            onClick={() => onTabChange(tab.value)}
          >
            <span className="flex w-5 justify-center">
              <tab.Icon size={16} />
            </span>
            {!collapsed ? <span>{tab.label}</span> : null}
          </button>
        );
      })}
    </nav>
  );
}
