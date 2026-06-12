// K8sWorkloadsCard / K8sPodsCard：k8s_workloads 与 k8s_pods 视图。
// 工作负载按 ready/replicas 比例上色；Pod 按 phase 上色；均按命名空间分组。
import { useMemo, useState } from 'react';
import { StatusDot, ViewCard } from './StatusDot';
import { Empty, MoreToggle } from './ContainerListCard';

interface Workload {
  kind: string;
  namespace: string;
  name: string;
  container: string;
  current_image: string;
  replicas: number;
  ready_replicas: number;
}

interface Pod {
  name: string;
  namespace: string;
  phase: string;
  node: string;
}

const COLLAPSE_AT = 8;

export function K8sWorkloadsCard({ title, data }: { title: string; data: Workload[] }) {
  const [expanded, setExpanded] = useState(false);
  if (data.length === 0) return <ViewCard title={title} count={0}><Empty /></ViewCard>;
  const shown = expanded ? data : data.slice(0, COLLAPSE_AT);
  return (
    <ViewCard title={title} count={data.length}>
      <div className="space-y-0.5 text-2xs">
        {shown.map((w, i) => {
          const ready = w.ready_replicas >= w.replicas && w.replicas > 0;
          const partial = w.ready_replicas > 0 && w.ready_replicas < w.replicas;
          return (
            <div key={`${w.namespace}/${w.name}/${i}`} className="flex items-center gap-2">
              <StatusDot tone={ready ? 'ok' : partial ? 'warn' : 'danger'} pulse={ready} />
              <span className="w-12 shrink-0 truncate text-ops-tertiary">{w.kind}</span>
              <span className="w-16 shrink-0 truncate text-ops-secondary">{w.namespace}</span>
              <span className="min-w-0 flex-1 truncate font-mono text-ops-primary">{w.name}</span>
              <span className="shrink-0 font-mono tabular-nums text-ops-secondary">
                {w.ready_replicas}/{w.replicas}
              </span>
            </div>
          );
        })}
      </div>
      {data.length > COLLAPSE_AT && <MoreToggle expanded={expanded} total={data.length} onToggle={() => setExpanded((v) => !v)} />}
    </ViewCard>
  );
}

// Pod phase → 色调
function podTone(phase: string): { tone: 'ok' | 'warn' | 'danger' | 'muted'; pulse?: boolean } {
  switch (phase) {
    case 'Running':
      return { tone: 'ok', pulse: true };
    case 'Succeeded':
      return { tone: 'ok' };
    case 'Pending':
      return { tone: 'warn' };
    case 'Failed':
      return { tone: 'danger' };
    default:
      return { tone: 'muted' };
  }
}

export function K8sPodsCard({ title, data }: { title: string; data: Pod[] }) {
  const [expanded, setExpanded] = useState(false);
  // 命名空间分组统计放标题
  const nsCount = useMemo(() => new Set(data.map((p) => p.namespace)).size, [data]);
  if (data.length === 0) return <ViewCard title={title} count={0}><Empty /></ViewCard>;
  const shown = expanded ? data : data.slice(0, COLLAPSE_AT);
  return (
    <ViewCard title={`${title} · ${nsCount} 命名空间`} count={data.length}>
      <div className="space-y-0.5 text-2xs">
        {shown.map((p, i) => {
          const { tone, pulse } = podTone(p.phase);
          return (
            <div key={`${p.namespace}/${p.name}/${i}`} className="flex items-center gap-2">
              <StatusDot tone={tone} pulse={pulse} />
              <span className="w-16 shrink-0 truncate text-ops-secondary">{p.namespace}</span>
              <span className="min-w-0 flex-1 truncate font-mono text-ops-primary">{p.name}</span>
              <span className="w-16 shrink-0 truncate text-ops-tertiary">{p.phase}</span>
            </div>
          );
        })}
      </div>
      {data.length > COLLAPSE_AT && <MoreToggle expanded={expanded} total={data.length} onToggle={() => setExpanded((v) => !v)} />}
    </ViewCard>
  );
}
