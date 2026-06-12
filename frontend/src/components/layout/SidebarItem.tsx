// SidebarItem 组件：统一侧栏列表项的选中、悬停和元信息样式。
// 外层用 div + role=button 而不是 <button>，方便嵌一个 hover-revealed 删除按钮
// （HTML 不允许 button 嵌套 button）。
import type { KeyboardEvent, ReactNode } from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/cn';

interface SidebarItemProps {
  title: string;
  meta?: ReactNode;
  selected?: boolean;
  collapsed?: boolean;
  icon?: ReactNode;
  onClick: () => void;
  // onDelete 可选；提供时显示 hover-revealed × 按钮，click 时阻止冒泡到主点击区。
  onDelete?: () => void;
}

export function SidebarItem({
  title,
  meta,
  selected = false,
  collapsed = false,
  icon = '•',
  onClick,
  onDelete,
}: SidebarItemProps) {
  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onClick();
    }
  }
  return (
    <div
      role="button"
      tabIndex={0}
      className={cn(
        'group relative flex w-full items-start gap-2 border-l-2 px-3 py-2 text-left transition-colors duration-fast ease-ops cursor-pointer outline-none',
        collapsed && 'justify-center px-0',
        selected
          ? 'border-ops-accent bg-ops-surface text-ops-primary'
          : 'border-transparent text-ops-secondary hover:bg-ops-surface/60 hover:text-ops-primary',
      )}
      title={title}
      onClick={onClick}
      onKeyDown={handleKeyDown}
    >
      <span className="mt-0.5 flex w-5 shrink-0 justify-center text-xs">{icon}</span>
      {!collapsed ? (
        <span className="min-w-0 flex-1">
          <span className="block truncate pr-5 text-sm">{title}</span>
          {meta ? <span className="mt-1 block truncate text-2xs text-ops-tertiary">{meta}</span> : null}
        </span>
      ) : null}
      {onDelete && !collapsed ? (
        <button
          type="button"
          className="absolute right-2 top-2 rounded p-1 text-ops-tertiary opacity-0 transition-opacity hover:bg-ops-elevated hover:text-ops-danger focus:opacity-100 group-hover:opacity-100"
          title="删除"
          aria-label="删除"
          onClick={(event) => {
            event.stopPropagation();
            onDelete();
          }}
        >
          <X size={12} />
        </button>
      ) : null}
    </div>
  );
}
