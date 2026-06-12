// Textarea 组件：统一多行输入框的暗色 token。
import { forwardRef, type TextareaHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn(
        'w-full rounded-md border border-ops-border-strong bg-ops-input px-3 py-2 text-sm text-ops-primary',
        'transition-[border-color,box-shadow] duration-fast ease-ops',
        'placeholder:text-ops-tertiary',
        'focus:border-ops-border-focus focus:outline-none focus:ring-1 focus:ring-ops-border-focus',
        'disabled:cursor-not-allowed disabled:text-ops-tertiary',
        className,
      )}
      {...props}
    />
  ),
);
Textarea.displayName = 'Textarea';
