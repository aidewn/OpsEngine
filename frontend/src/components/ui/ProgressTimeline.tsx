// ProgressTimeline 组件：展示 Agent 思考/工具调用进度；进行中默认展开，完成后可折叠。
import { useEffect, useMemo, useState } from 'react';
import { cn } from '@/lib/cn';

interface ProgressTimelineProps {
  items: string[];
  /** 流式进行中：始终展开并高亮最后一步 */
  live?: boolean;
  /** 非 live 时是否默认展开（历史消息） */
  defaultOpen?: boolean;
}

export function ProgressTimeline({
  items,
  live = false,
  defaultOpen = false,
}: ProgressTimelineProps) {
  const [open, setOpen] = useState(defaultOpen || live);
  const summary = useMemo(() => {
    const toolCount = items.filter((item) => isToolLine(item)).length;
    return `${items.length} 步 · ${toolCount} 个工具调用`;
  }, [items]);
  const latest = items[items.length - 1];

  useEffect(() => {
    if (live) setOpen(true);
  }, [live]);

  if (items.length === 0) return null;

  return (
    <div className="mt-2 border-t border-ops-border-subtle pt-2 text-xs">
      {live ? (
        <div className="mb-1 flex items-center gap-1.5 px-1 text-[11px] font-medium text-ops-info">
          <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-ops-accent" />
          Agent 思考过程
        </div>
      ) : null}
      {!live && !open && latest ? (
        <div className="mb-1 truncate px-2 text-[11px] text-ops-secondary">{latest}</div>
      ) : null}
      {!live ? (
        <button
          type="button"
          className="flex w-full items-center justify-between rounded px-2 py-1 text-left text-ops-secondary hover:bg-ops-surface hover:text-ops-primary"
          onClick={() => setOpen((value) => !value)}
        >
          <span>{summary}</span>
          <span>{open ? '收起' : '展开'}</span>
        </button>
      ) : (
        <div className="px-2 py-0.5 text-ops-secondary">{summary}</div>
      )}
      {open ? (
        <ol className="mt-2 space-y-1">
          {items.map((item, index) => {
            const isLatest = live && index === items.length - 1;
            return (
              <li
                key={`${item}-${index}`}
                className={cn(
                  'rounded border px-2 py-1',
                  isLatest
                    ? 'border-ops-accent/40 bg-ops-accent-soft text-ops-primary'
                    : 'border-ops-border-subtle bg-ops-input',
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
