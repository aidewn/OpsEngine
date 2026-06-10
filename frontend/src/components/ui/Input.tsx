// Input 组件：统一表单输入框的暗色 token。
import { forwardRef, type InputHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        'h-9 w-full rounded-md border border-ops-border-strong bg-ops-input px-3 text-sm text-ops-primary',
        'placeholder:text-ops-tertiary',
        'focus:border-ops-border-focus focus:outline-none focus:ring-1 focus:ring-ops-border-focus',
        'disabled:cursor-not-allowed disabled:text-ops-tertiary',
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = 'Input';
