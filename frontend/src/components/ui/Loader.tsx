// Loader 组件：统一加载状态。
import { cn } from '@/lib/cn';

interface LoaderProps {
  label?: string;
  className?: string;
}

export function Loader({ label = '加载中…', className }: LoaderProps) {
  return (
    <div className={cn('flex items-center justify-center gap-2 text-sm text-ops-secondary', className)}>
      <span className="size-3 animate-spin rounded-full border-2 border-ops-border-strong border-t-ops-accent" />
      <span>{label}</span>
    </div>
  );
}
