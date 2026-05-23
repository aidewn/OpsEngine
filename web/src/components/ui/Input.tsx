import { forwardRef, type InputHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        'h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm',
        'placeholder:text-slate-400',
        'focus:border-slate-500 focus:outline-none focus:ring-1 focus:ring-slate-500',
        'disabled:cursor-not-allowed disabled:bg-slate-50 disabled:text-slate-500',
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = 'Input';
