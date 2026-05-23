// 节点卡片骨架：图标 + 名称 + 端口列表
// 紧凑型 UE Blueprint 风格，详情见右侧详情面板

import { type ReactNode } from 'react';
import { cn } from '@/lib/cn';

export type NodeTone = 'ready' | 'update' | 'over' | 'neutral';

// 不同分类节点的色调
const toneClass: Record<
  NodeTone,
  { border: string; selectedBorder: string; headerBg: string; headerText: string }
> = {
  ready: {
    border: 'border-ready-300',
    selectedBorder: 'border-ready-600',
    headerBg: 'bg-ready-100',
    headerText: 'text-ready-800',
  },
  update: {
    border: 'border-update-300',
    selectedBorder: 'border-update-600',
    headerBg: 'bg-update-100',
    headerText: 'text-update-800',
  },
  over: {
    border: 'border-over-300',
    selectedBorder: 'border-over-600',
    headerBg: 'bg-over-100',
    headerText: 'text-over-800',
  },
  neutral: {
    border: 'border-slate-300',
    selectedBorder: 'border-slate-600',
    headerBg: 'bg-slate-100',
    headerText: 'text-slate-800',
  },
};

interface BaseNodeProps {
  tone: NodeTone;
  selected?: boolean;
  icon?: string;
  title: string;
  // 节点 body 内容（端口行）
  children?: ReactNode;
}

export function BaseNode({
  tone,
  selected,
  icon,
  title,
  children,
}: BaseNodeProps) {
  const t = toneClass[tone];
  return (
    <div
      className={cn(
        'min-w-[140px] overflow-hidden rounded-md border-2 bg-white shadow-sm transition-shadow',
        selected ? t.selectedBorder : t.border,
        selected && 'shadow-md',
      )}
    >
      {/* 标题区：icon + 名字 */}
      <div
        className={cn(
          'flex items-center gap-1.5 px-2 py-1 text-xs font-semibold',
          t.headerBg,
          t.headerText,
        )}
      >
        {icon && <span>{icon}</span>}
        <span className="truncate">{title}</span>
      </div>
      {/* 端口区 */}
      {children && <div className="py-1">{children}</div>}
    </div>
  );
}
