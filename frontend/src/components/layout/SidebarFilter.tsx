// SidebarFilter 组件：渲染侧栏内的轻量 segment/chip 筛选。
import { cn } from '@/lib/cn';

interface SidebarFilterProps<TValue extends string> {
  value: TValue;
  options: Array<{ value: TValue; label: string }>;
  onChange: (value: TValue) => void;
}

export function SidebarFilter<TValue extends string>({
  value,
  options,
  onChange,
}: SidebarFilterProps<TValue>) {
  return (
    <div className="flex rounded-md bg-ops-input p-1">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          className={cn(
            'min-w-0 flex-1 rounded px-2 py-1 text-xs transition-colors',
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
