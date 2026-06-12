// ProgressTimeline 组件：展示 Agent 思考/工具调用进度；默认折叠，用户可手动展开查看完整过程。
import { useEffect, useMemo, useState } from 'react';
import { cn } from '@/lib/cn';

interface ProgressTimelineProps {
  items: string[];
  /** 流式进行中：高亮最后一步，但不强制展开 */
  live?: boolean;
  /** 是否默认展开 */
  defaultOpen?: boolean;
}

export function ProgressTimeline({
  items,
  live = false,
  defaultOpen = false,
}: ProgressTimelineProps) {
  const [open, setOpen] = useState(defaultOpen);
  const summary = useMemo(() => {
    const toolCount = items.filter((item) => isToolLine(item)).length;
    return `${items.length} 步 · ${toolCount} 个工具调用`;
  }, [items]);
  const latest = items[items.length - 1];

  useEffect(() => {
    setOpen(defaultOpen);
  }, [defaultOpen]);

  if (items.length === 0) return null;

  return (
    <div className="mt-2 border-t border-ops-border-subtle pt-2 text-xs">
      {live ? (
        <div className="mb-1 flex items-center gap-1.5 px-1 text-[11px] font-medium text-ops-info">
          <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-ops-accent" />
          Agent 思考过程
        </div>
      ) : null}
      {!open && latest ? (
        <div className="mb-1 truncate px-2 text-[11px] text-ops-secondary">{latest}</div>
      ) : null}
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
          {items.map((item, index) => {
            const isLatest = live && index === items.length - 1;
            return (
              <li
                key={`${item}-${index}`}
                className={cn(
                  'rounded border px-2 py-1',
                  // live 模式下新步骤底部滑入；历史展开不重复入场（keyed 不重挂载）
                  live && 'animate-in fade-in slide-in-from-bottom-1 duration-base',
                  isLatest
                    ? 'border-ops-accent/40 bg-ops-accent-soft text-ops-primary'
                    : 'border-ops-border-subtle bg-ops-input',
                  'transition-colors duration-base ease-ops',
                  progressLineClass(item),
                )}
              >
                {item}
              </li>
            );
          })}
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
