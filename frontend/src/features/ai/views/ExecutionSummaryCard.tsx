// ExecutionSummaryCard：get_execution 视图。状态横幅 + 失败节点列表。
// 点击「查看执行」跳执行详情页（回调由 ViewRenderer 透传）。
import { cn } from '@/lib/cn';
import { StatusDot, ViewCard } from './StatusDot';

interface FailedNode {
  instance_id: string;
  type_id: string;
  frame_path: string;
  last_log?: string;
}

interface ExecutionView {
  id: string;
  workflow_id: string;
  workflow_name: string;
  status: string;
  error?: string;
  failed_nodes?: FailedNode[];
}

// status → 横幅样式
function statusTone(status: string): { tone: 'ok' | 'warn' | 'danger' | 'muted'; label: string; bar: string } {
  switch (status) {
    case 'Success':
      return { tone: 'ok', label: '成功', bar: 'bg-ops-success-soft text-ops-success' };
    case 'Running':
      return { tone: 'warn', label: '运行中', bar: 'bg-ops-info-soft text-ops-info' };
    case 'Failed':
      return { tone: 'danger', label: '失败', bar: 'bg-ops-danger-soft text-ops-danger' };
    case 'Terminated':
      return { tone: 'danger', label: '已终止', bar: 'bg-ops-danger-soft text-ops-danger' };
    default:
      return { tone: 'muted', label: status, bar: 'bg-ops-surface text-ops-secondary' };
  }
}

export function ExecutionSummaryCard({
  title,
  data,
  onOpenExecution,
}: {
  title: string;
  data: ExecutionView;
  onOpenExecution?: (executionID: string) => void;
}) {
  const st = statusTone(data.status);
  return (
    <ViewCard
      title={title}
      action={
        onOpenExecution ? (
          <button
            type="button"
            onClick={() => onOpenExecution(data.id)}
            className="shrink-0 rounded px-1.5 py-0.5 text-2xs text-ops-secondary transition-colors duration-fast ease-ops hover:bg-ops-surface hover:text-ops-primary"
          >
            查看执行 →
          </button>
        ) : undefined
      }
    >
      <div className={cn('mb-2 flex items-center gap-2 rounded px-2 py-1 text-2xs', st.bar)}>
        <StatusDot tone={st.tone} pulse={data.status === 'Running'} />
        <span className="font-medium">{st.label}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-ops-secondary">{data.workflow_name}</span>
      </div>
      {data.error ? (
        <div className="mb-1 truncate font-mono text-2xs text-ops-danger">{data.error}</div>
      ) : null}
      {data.failed_nodes && data.failed_nodes.length > 0 ? (
        <div className="space-y-0.5">
          <div className="text-2xs text-ops-tertiary">失败节点 {data.failed_nodes.length}</div>
          {data.failed_nodes.map((n) => (
            <div key={n.instance_id} className="rounded bg-ops-surface px-2 py-1 text-2xs">
              <div className="flex items-center gap-2">
                <StatusDot tone="danger" />
                <span className="font-mono text-ops-primary">{n.type_id}</span>
                <span className="truncate text-ops-tertiary">{n.frame_path}</span>
              </div>
              {n.last_log ? (
                <div className="mt-0.5 truncate pl-3.5 font-mono text-ops-danger">{n.last_log}</div>
              ) : null}
            </div>
          ))}
        </div>
      ) : (
        <div className="text-2xs text-ops-tertiary">无失败节点</div>
      )}
    </ViewCard>
  );
}
