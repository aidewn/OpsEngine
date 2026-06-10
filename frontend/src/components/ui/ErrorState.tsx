// ErrorState 组件：统一可重试错误展示。
import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

interface ErrorStateProps {
  title?: string;
  message: string;
  action?: ReactNode;
  className?: string;
}

export function ErrorState({ title = '操作失败', message, action, className }: ErrorStateProps) {
  return (
    <div className={cn('rounded-md border border-ops-danger bg-ops-danger-soft px-4 py-3 text-sm', className)}>
      <div className="font-medium text-ops-danger">{title}</div>
      <div className="mt-1 text-ops-primary">{message}</div>
      {action ? <div className="mt-3">{action}</div> : null}
    </div>
  );
}
