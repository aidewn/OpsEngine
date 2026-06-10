// StatusBadge 组件：统一状态与类型标签的颜色。
import { cn } from '@/lib/cn';

type StatusTone = 'success' | 'warning' | 'danger' | 'info' | 'neutral';

interface StatusBadgeProps {
  children: string;
  tone?: StatusTone;
  className?: string;
}

const toneClass: Record<StatusTone, string> = {
  success: 'bg-ops-success-soft text-ops-success',
  warning: 'bg-ops-warning-soft text-ops-warning',
  danger: 'bg-ops-danger-soft text-ops-danger',
  info: 'bg-ops-info-soft text-ops-info',
  neutral: 'bg-ops-surface text-ops-secondary',
};

export function StatusBadge({ children, tone = 'neutral', className }: StatusBadgeProps) {
  return (
    <span className={cn('inline-flex items-center rounded px-1.5 py-0.5 text-2xs font-medium', toneClass[tone], className)}>
      {children}
    </span>
  );
}
