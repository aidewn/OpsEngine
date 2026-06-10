// ProgressTimeline 组件：折叠展示长流程进度和工具调用。
import { useMemo, useState } from 'react';
import { cn } from '@/lib/cn';

interface ProgressTimelineProps {
  items: string[];
}

export function ProgressTimeline({ items }: ProgressTimelineProps) {
  const [open, setOpen] = useState(false);
  const summary = useMemo(() => {
    const toolCount = items.filter((item) => isToolLine(item)).length;
    return `${items.length} 步进度 · ${toolCount} 个工具调用`;
  }, [items]);

  if (items.length === 0) return null;

  return (
    <div className="mt-2 border-t border-ops-border-subtle pt-2 text-xs">
      <button
        type="button"
        className="flex w-full items-center justify-between rounded px-2 py-1 text-left text-ops-secondary hover:bg-ops-surface hover:text-ops-primary"
        onClick={() => setOpen((value) => !value)}
      >
        <span>{summary}</span>
        <span>{open ? '收起' : '展开'}</span>
      </button>
      {open ? (
        <ol className="mt-2 space-y-1">
          {items.map((item, index) => (
            <li
              key={`${item}-${index}`}
              className={cn(
                'rounded border border-ops-border-subtle bg-ops-input px-2 py-1',
                progressLineClass(item),
              )}
            >
              {item}
            </li>
          ))}
        </ol>
      ) : null}
    </div>
  );
}

function isToolLine(text: string) {
  return text.startsWith('🔧') || text.includes('工具');
}

function progressLineClass(text: string): string {
  if (text.startsWith('🔧')) return 'text-ops-info';
  if (text.startsWith('✓')) return 'text-ops-success';
  if (text.startsWith('✗')) return 'text-ops-danger';
  return 'text-ops-secondary';
}
