// 顶部 tab 渲染组件
// 激活 tab 由当前路由决定；关闭 tab 时按 VSCode 风格只切左侧

import { useLocation, useNavigate } from 'react-router-dom';
import { cn } from '@/lib/cn';
import {
  activeTabFromPath,
  routeFor,
  useTabs,
  type TabItem,
} from './TabsContext';

export function TabBar() {
  const { tabs, closeTab } = useTabs();
  const navigate = useNavigate();
  const location = useLocation();

  const active = activeTabFromPath(location.pathname);

  function handleClose(e: React.MouseEvent, tab: TabItem, index: number) {
    e.stopPropagation();
    const isActive = active?.kind === tab.kind && active?.id === tab.id;
    closeTab(tab.kind, tab.id);

    if (!isActive) return;
    // 关闭激活 tab：只切左侧（VSCode 风格），无左侧则回首页
    const leftNeighbor = index > 0 ? tabs[index - 1] : undefined;
    if (leftNeighbor) {
      navigate(routeFor(leftNeighbor));
    } else {
      navigate('/');
    }
  }

  if (tabs.length === 0) {
    return <div className="flex-1" />;
  }

  return (
    <div className="flex flex-1 items-center gap-0.5 overflow-x-auto">
      {tabs.map((tab, idx) => {
        const isActive =
          active?.kind === tab.kind && active?.id === tab.id;
        return (
          <div
            key={`${tab.kind}:${tab.id}`}
            onClick={() => {
              if (!isActive) navigate(routeFor(tab));
            }}
            className={cn(
              'group flex h-9 cursor-pointer items-center gap-1.5 border-b-2 px-3 text-xs transition-colors',
              isActive
                ? 'border-blue-500 bg-slate-50 font-medium text-slate-900'
                : 'border-transparent text-slate-500 hover:bg-slate-50',
            )}
            title={`${tab.kind === 'workflow' ? '工作流' : '集合'}: ${tab.name}`}
          >
            <span>{tab.kind === 'workflow' ? '📄' : '📦'}</span>
            <span className="max-w-[160px] truncate">{tab.name}</span>
            <span
              role="button"
              tabIndex={-1}
              onClick={(e) => handleClose(e, tab, idx)}
              className={cn(
                'ml-1 inline-flex size-4 items-center justify-center rounded text-slate-400 hover:bg-slate-200 hover:text-slate-700',
                !isActive && 'opacity-0 group-hover:opacity-100',
              )}
              title="关闭"
            >
              ×
            </span>
          </div>
        );
      })}
    </div>
  );
}
