// Segment 组件：用于少量互斥选项切换。
import { cn } from '@/lib/cn';

export interface SegmentOption<TValue extends string> {
  value: TValue;
  label: string;
}

interface SegmentProps<TValue extends string> {
  value: TValue;
  options: Array<SegmentOption<TValue>>;
  onChange: (value: TValue) => void;
  className?: string;
}

export function Segment<TValue extends string>({ value, options, onChange, className }: SegmentProps<TValue>) {
  return (
    <div className={cn('inline-flex rounded-md bg-ops-input p-1', className)}>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          className={cn(
            'rounded px-3 py-1 text-xs font-medium transition-colors',
            value === option.value
              ? 'bg-ops-surface text-ops-primary'
              : 'text-ops-secondary hover:text-ops-primary',
          )}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}
