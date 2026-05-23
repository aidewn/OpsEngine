// 节点卡片骨架：彩色边框 + 标题 + 副标题
// 各具体节点组件复用此结构

import { type ReactNode } from 'react';
import { cn } from '@/lib/cn';

export type NodeTone = 'ready' | 'update' | 'over' | 'neutral';

const toneClass: Record<
  NodeTone,
  { border: string; selectedBorder: string; bg: string; chip: string }
> = {
  ready: {
    border: 'border-ready-300',
    selectedBorder: 'border-ready-600',
    bg: 'bg-ready-50',
    chip: 'text-ready-700 bg-white border-ready-300',
  },
  update: {
    border: 'border-update-300',
    selectedBorder: 'border-update-600',
    bg: 'bg-update-50',
    chip: 'text-update-700 bg-white border-update-300',
  },
  over: {
    border: 'border-over-300',
    selectedBorder: 'border-over-600',
    bg: 'bg-over-50',
    chip: 'text-over-700 bg-white border-over-300',
  },
  neutral: {
    border: 'border-slate-300',
    selectedBorder: 'border-slate-600',
    bg: 'bg-white',
    chip: 'text-slate-700 bg-slate-50 border-slate-300',
  },
};

interface BaseNodeProps {
  tone: NodeTone;
  selected?: boolean;
  category: string;
  title: string;
  subtitle?: string;
  children?: ReactNode;
}

export function BaseNode({
  tone,
  selected,
  category,
  title,
  subtitle,
  children,
}: BaseNodeProps) {
  const t = toneClass[tone];
  return (
    <div
      className={cn(
        'relative w-56 rounded-lg border-2 p-3 shadow-sm transition-shadow',
        t.bg,
        selected ? t.selectedBorder : t.border,
        selected && 'shadow-md',
      )}
    >
      <span
        className={cn(
          'inline-block rounded border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
          t.chip,
        )}
      >
        {category}
      </span>
      <div className="mt-2 text-sm font-semibold text-slate-900">{title}</div>
      {subtitle && (
        <div className="mt-1 text-xs text-slate-500">{subtitle}</div>
      )}
      {children}
    </div>
  );
}
