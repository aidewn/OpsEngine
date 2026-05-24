// 节点/工作流执行状态可视化
// 用纯 SVG，避免 emoji 在不同平台渲染差异
// - Idle：灰色空心圆
// - Executing：蓝色半圆，旋转动画
// - Success：绿色实心圆
// - Failed：红色实心圆 + 错误标记
// - Skipped：灰色虚线圆
// - Terminated：橙色实心方块

import type { NodeState, WorkflowStatus } from '@/types/execution';
import { cn } from '@/lib/cn';

type Tone = 'idle' | 'executing' | 'success' | 'failed' | 'skipped' | 'terminated';

// 把 NodeState/WorkflowStatus 映射到统一的 tone
export function toneOfNodeState(state: NodeState | undefined): Tone {
  switch (state) {
    case 'Executing':
      return 'executing';
    case 'Success':
      return 'success';
    case 'Failed':
    case 'CheckFailed':
      return 'failed';
    case 'Skipped':
      return 'skipped';
    default:
      return 'idle';
  }
}

export function toneOfWorkflowStatus(status: WorkflowStatus | undefined): Tone {
  switch (status) {
    case 'Running':
      return 'executing';
    case 'Success':
      return 'success';
    case 'Failed':
      return 'failed';
    case 'Terminated':
      return 'terminated';
    default:
      return 'idle';
  }
}

const toneColor: Record<Tone, string> = {
  idle: '#94a3b8', // slate-400
  executing: '#3b82f6', // blue-500
  success: '#22c55e', // green-500
  failed: '#ef4444', // red-500
  skipped: '#94a3b8',
  terminated: '#f97316', // orange-500
};

interface Props {
  tone: Tone;
  size?: number; // 默认 12
  className?: string;
  title?: string;
}

// StatusIcon 单个状态图标
export function StatusIcon({ tone, size = 12, className, title }: Props) {
  const color = toneColor[tone];

  if (tone === 'idle') {
    return (
      <svg width={size} height={size} viewBox="0 0 12 12" className={className}>
        {title && <title>{title}</title>}
        <circle cx="6" cy="6" r="4.5" fill="none" stroke={color} strokeWidth="1.5" />
      </svg>
    );
  }
  if (tone === 'executing') {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 12 12"
        className={cn('animate-spin', className)}
        style={{ animationDuration: '1.2s' }}
      >
        {title && <title>{title}</title>}
        <circle cx="6" cy="6" r="4.5" fill="none" stroke={color} strokeWidth="2" strokeDasharray="20 8" />
      </svg>
    );
  }
  if (tone === 'skipped') {
    return (
      <svg width={size} height={size} viewBox="0 0 12 12" className={className}>
        {title && <title>{title}</title>}
        <circle
          cx="6"
          cy="6"
          r="4.5"
          fill="none"
          stroke={color}
          strokeWidth="1.5"
          strokeDasharray="2 2"
        />
      </svg>
    );
  }
  if (tone === 'terminated') {
    return (
      <svg width={size} height={size} viewBox="0 0 12 12" className={className}>
        {title && <title>{title}</title>}
        <rect x="2" y="2" width="8" height="8" rx="1" fill={color} />
      </svg>
    );
  }
  // success / failed：实心圆
  return (
    <svg width={size} height={size} viewBox="0 0 12 12" className={className}>
      {title && <title>{title}</title>}
      <circle cx="6" cy="6" r="5" fill={color} />
      {tone === 'success' && (
        <path
          d="M3.5 6 L5.3 7.8 L8.5 4.2"
          fill="none"
          stroke="white"
          strokeWidth="1.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      )}
      {tone === 'failed' && (
        <>
          <path d="M4 4 L8 8" stroke="white" strokeWidth="1.4" strokeLinecap="round" />
          <path d="M8 4 L4 8" stroke="white" strokeWidth="1.4" strokeLinecap="round" />
        </>
      )}
    </svg>
  );
}

// 便利组件：直接接受 NodeState / WorkflowStatus
export function NodeStatusIcon({
  state,
  size,
  className,
}: {
  state: NodeState | undefined;
  size?: number;
  className?: string;
}) {
  return (
    <StatusIcon
      tone={toneOfNodeState(state)}
      size={size}
      className={className}
      title={state ?? 'Idle'}
    />
  );
}

export function WorkflowStatusIcon({
  status,
  size,
  className,
}: {
  status: WorkflowStatus | undefined;
  size?: number;
  className?: string;
}) {
  return (
    <StatusIcon
      tone={toneOfWorkflowStatus(status)}
      size={size}
      className={className}
      title={status ?? 'Idle'}
    />
  );
}
