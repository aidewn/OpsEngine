// EmptyState 组件：统一空状态展示。
import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

interface EmptyStateProps {
  title: string;
  description?: string;
  icon?: ReactNode;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ title, description, icon, action, className }: EmptyStateProps) {
  return (
    <div className={cn('px-6 py-14 text-center', className)}>
      {icon ? <div className="mb-3 flex justify-center text-ops-tertiary">{icon}</div> : null}
      <div className="text-sm font-medium text-ops-primary">{title}</div>
      {description ? <p className="mt-1 text-sm text-ops-secondary">{description}</p> : null}
      {action ? <div className="mt-4 flex justify-center">{action}</div> : null}
    </div>
  );
}
