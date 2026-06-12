// StatusDot：数据卡通用状态点。running/healthy 呼吸，异常态用语义色。
import { cn } from '@/lib/cn';

type Tone = 'ok' | 'warn' | 'danger' | 'muted';

const toneClass: Record<Tone, string> = {
  ok: 'bg-ops-success',
  warn: 'bg-ops-warning',
  danger: 'bg-ops-danger',
  muted: 'bg-ops-tertiary',
};

export function StatusDot({ tone, pulse }: { tone: Tone; pulse?: boolean }) {
  return (
    <span
      className={cn('inline-block h-1.5 w-1.5 shrink-0 rounded-full', toneClass[tone], pulse && 'animate-pulse')}
      aria-hidden
    />
  );
}

// ViewCard：数据卡统一外壳——标题栏（可放右侧操作）+ 内容区。
export function ViewCard({
  title,
  count,
  action,
  children,
}: {
  title: string;
  count?: number;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-md border border-ops-border-subtle bg-ops-input">
      <div className="flex items-center justify-between gap-2 border-b border-ops-border-subtle px-3 py-1.5">
        <span className="min-w-0 truncate text-xs font-medium text-ops-primary">
          {title}
          {count != null && (
            <span className="ml-1 font-mono tabular-nums text-ops-tertiary">{count}</span>
          )}
        </span>
        {action}
      </div>
      <div className="p-2">{children}</div>
    </div>
  );
}
