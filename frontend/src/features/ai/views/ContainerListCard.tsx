// ContainerListCard：docker_list_containers 视图。状态点 + 镜像 + 状态文本。
import { useState } from 'react';
import { StatusDot, ViewCard } from './StatusDot';

interface Container {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
}

// 容器 state → 状态点色调（running 呼吸，restarting 琥珀，其余灰）
function toneOf(state: string): { tone: 'ok' | 'warn' | 'danger' | 'muted'; pulse?: boolean } {
  const s = state.toLowerCase();
  if (s === 'running') return { tone: 'ok', pulse: true };
  if (s.includes('restart')) return { tone: 'warn' };
  if (s === 'exited' || s === 'dead') return { tone: 'muted' };
  if (s.includes('unhealthy') || s === 'paused') return { tone: 'danger' };
  return { tone: 'muted' };
}

const COLLAPSE_AT = 8;

export function ContainerListCard({ title, data }: { title: string; data: Container[] }) {
  const [expanded, setExpanded] = useState(false);
  if (data.length === 0) {
    return <ViewCard title={title} count={0}><Empty /></ViewCard>;
  }
  const shown = expanded ? data : data.slice(0, COLLAPSE_AT);
  return (
    <ViewCard title={title} count={data.length}>
      <div className="space-y-0.5 text-2xs">
        {shown.map((c) => {
          const { tone, pulse } = toneOf(c.state);
          return (
            <div key={c.id} className="flex items-center gap-2">
              <StatusDot tone={tone} pulse={pulse} />
              <span className="w-32 truncate font-mono text-ops-primary">{c.name}</span>
              <span className="min-w-0 flex-1 truncate text-ops-secondary">{c.image}</span>
              <span className="shrink-0 truncate text-ops-tertiary" style={{ maxWidth: 120 }}>
                {c.status}
              </span>
            </div>
          );
        })}
      </div>
      {data.length > COLLAPSE_AT && <MoreToggle expanded={expanded} total={data.length} onToggle={() => setExpanded((v) => !v)} />}
    </ViewCard>
  );
}

export function Empty() {
  return <div className="px-1 py-2 text-2xs text-ops-tertiary">（无数据）</div>;
}

export function MoreToggle({ expanded, total, onToggle }: { expanded: boolean; total: number; onToggle: () => void }) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="mt-1 w-full rounded py-1 text-2xs text-ops-secondary transition-colors duration-fast ease-ops hover:bg-ops-surface hover:text-ops-primary"
    >
      {expanded ? '收起' : `展开全部 ${total} 项`}
    </button>
  );
}
