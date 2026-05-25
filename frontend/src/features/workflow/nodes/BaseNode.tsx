// 节点卡片骨架：图标 + 名称 + 端口列表
// 紧凑型 UE Blueprint 风格，详情见右侧详情面板
// 右上角可选显示执行状态角标（执行详情页用）

import { type ReactNode } from 'react';
import { cn } from '@/lib/cn';
import { NodeStatusIcon } from '@/features/execution/ExecutionStatus';
import type { NodeState } from '@/types/execution';

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
  // 副标题（config 摘要，灰色小字）；空 → 不渲染，header 不变高
  subtitle?: string;
  // 节点 body 内容（端口行）
  children?: ReactNode;
  // 执行状态：传入时右上角显示状态角标 + 失败/执行中边框高亮
  execState?: NodeState;
}

export function BaseNode({
  tone,
  selected,
  icon,
  title,
  subtitle,
  children,
  execState,
}: BaseNodeProps) {
  const t = toneClass[tone];
  // 执行中/失败状态覆盖默认边框
  const stateBorder =
    execState === 'Executing'
      ? 'border-blue-500'
      : execState === 'Failed'
        ? 'border-red-500'
        : execState === 'Success'
          ? 'border-green-500'
          : null;
  return (
    <div
      className={cn(
        'relative min-w-[140px] rounded-md border-2 bg-white shadow-sm transition-shadow',
        stateBorder ?? (selected ? t.selectedBorder : t.border),
        selected && 'shadow-md',
      )}
    >
      {/* 标题区：icon + 名字 */}
      {/* 注意：外层不能用 overflow-hidden（会裁掉 Handle 半圆），所以这里需要手动加圆角让 headerBg 贴合外层圆角 */}
      <div
        className={cn(
          'flex items-center gap-1.5 rounded-t-[4px] px-2 py-1 text-xs font-semibold',
          t.headerBg,
          t.headerText,
        )}
      >
        {icon && <span className="shrink-0">{icon}</span>}
        <div className="min-w-0 flex-1">
          <div className="truncate">{title}</div>
          {subtitle && (
            <div
              className="truncate text-[10px] font-normal text-slate-500"
              title={subtitle}
            >
              {subtitle}
            </div>
          )}
        </div>
        {execState && (
          <NodeStatusIcon state={execState} size={12} className="shrink-0" />
        )}
      </div>
      {/* 端口区 */}
      {children && <div className="py-1">{children}</div>}
    </div>
  );
}
