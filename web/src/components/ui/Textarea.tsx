import { forwardRef, type TextareaHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn(
        'w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm',
        'placeholder:text-slate-400',
        'focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500',
        'disabled:cursor-not-allowed disabled:bg-slate-50',
        className,
      )}
      {...props}
    />
  ),
);
Textarea.displayName = 'Textarea';
