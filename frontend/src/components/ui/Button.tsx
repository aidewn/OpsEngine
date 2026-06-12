// 通用按钮：统一暗色主题下的按钮形态。
import { forwardRef, type ButtonHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

type Variant = 'primary' | 'accent' | 'secondary' | 'ghost' | 'danger';
type Size = 'sm' | 'md';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
}

const variantClass: Record<Variant, string> = {
  primary:
    'bg-ops-accent text-ops-inverse hover:bg-ops-accent-hover hover:shadow-glow-accent disabled:bg-ops-border-subtle disabled:text-ops-tertiary disabled:shadow-none',
  accent:
    'bg-ops-accent text-ops-inverse hover:bg-ops-accent-hover hover:shadow-glow-accent disabled:bg-ops-border-subtle disabled:text-ops-tertiary disabled:shadow-none',
  secondary:
    'border border-ops-border-subtle bg-ops-surface text-ops-primary hover:bg-ops-elevated disabled:opacity-50',
  ghost: 'bg-transparent text-ops-secondary hover:bg-ops-surface hover:text-ops-primary',
  danger: 'bg-ops-danger text-ops-primary hover:bg-ops-danger/90 disabled:opacity-50',
};

const sizeClass: Record<Size, string> = {
  sm: 'h-8 px-3 text-xs',
  md: 'h-9 px-4 text-sm',
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'md', ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        'inline-flex items-center justify-center rounded-md font-medium transition-[color,background-color,border-color,box-shadow] duration-fast ease-ops',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ops-border-focus',
        'disabled:cursor-not-allowed',
        variantClass[variant],
        sizeClass[size],
        className,
      )}
      {...props}
    />
  ),
);
Button.displayName = 'Button';
