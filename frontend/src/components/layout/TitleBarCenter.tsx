// 标题栏中间区域：根据当前路由显示页面标题。
import { useLocation } from 'react-router-dom';

// getTitleByPath 函数：保持阶段 0 的标题逻辑简单稳定。
function getTitleByPath(pathname: string) {
  if (pathname.startsWith('/workflows/')) return '工作流 · OpsEngine';
  if (pathname.startsWith('/assembles/')) return '集合 · OpsEngine';
  if (pathname.startsWith('/executions/')) return '执行详情 · OpsEngine';
  if (pathname.startsWith('/environments/')) return '环境详情 · OpsEngine';
  return 'OpsEngine';
}

// TitleBarCenter 组件：中间留作可拖拽区域。
export function TitleBarCenter() {
  const { pathname } = useLocation();

  return (
    <div className="titlebar-drag flex min-w-0 flex-1 items-center justify-center px-4 text-xs text-[#A8A6A1]">
      <span className="truncate">{getTitleByPath(pathname)}</span>
    </div>
  );
}
