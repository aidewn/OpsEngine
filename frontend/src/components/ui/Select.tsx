// Select 组件：统一原生下拉框的暗色 token，与 Input 同构。
import { forwardRef, type SelectHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

export type SelectProps = SelectHTMLAttributes<HTMLSelectElement>;

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ className, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        'h-9 w-full rounded-md border border-ops-border-strong bg-ops-input px-3 text-sm text-ops-primary',
        'focus:border-ops-border-focus focus:outline-none focus:ring-1 focus:ring-ops-border-focus',
        'disabled:cursor-not-allowed disabled:text-ops-tertiary',
        className,
      )}
      {...props}
    />
  ),
);
Select.displayName = 'Select';
