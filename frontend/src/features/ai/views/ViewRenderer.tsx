// ViewRenderer：Agent 可视化载荷的注册表入口（docs/agent-visual-output-plan.md）。
// 按 view.kind 分发到精修组件；未知 kind 渲染可折叠 JSON 兜底，保证旧版本/新视图永不白屏。
import { useMemo, useState } from 'react';
import type { AIViewPayload } from '@/types/ai';
import { HostStatusCard } from './HostStatusCard';
import { TopologyView } from './TopologyView';
import { ContainerListCard } from './ContainerListCard';
import { K8sWorkloadsCard, K8sPodsCard } from './K8sWorkloadsCard';
import { JobListCard } from './JobListCard';
import { ExecutionSummaryCard } from './ExecutionSummaryCard';
import { DiagramView } from './DiagramView';
import { useViewActions } from './ViewActions';

export function ViewRenderer({ view }: { view: AIViewPayload }) {
  const actions = useViewActions();
  // data 统一在入口解析一次；解析失败走兜底
  const data = useMemo(() => {
    try {
      return JSON.parse(view.data) as unknown;
    } catch {
      return null;
    }
  }, [view.data]);

  if (data === null) {
    return <FallbackView view={view} />;
  }
  switch (view.kind) {
    case 'host_status':
      return <HostStatusCard title={view.title} data={data as never} />;
    case 'topology':
      return <TopologyView title={view.title} data={data as never} />;
    case 'container_list':
      return <ContainerListCard title={view.title} data={data as never} />;
    case 'k8s_workloads':
      return <K8sWorkloadsCard title={view.title} data={data as never} />;
    case 'k8s_pods':
      return <K8sPodsCard title={view.title} data={data as never} />;
    case 'job_list':
      return <JobListCard title={view.title} data={data as never} />;
    case 'execution_summary':
      return (
        <ExecutionSummaryCard title={view.title} data={data as never} onOpenExecution={actions.openExecution} />
      );
    case 'diagram':
      return <DiagramView title={view.title} data={data as never} />;
    default:
      return <FallbackView view={view} />;
  }
}

// FallbackView 未知 kind / 解析失败的兜底：标题 + 可折叠原始 JSON。
function FallbackView({ view }: { view: AIViewPayload }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="rounded-md border border-ops-border-subtle bg-ops-input p-2 text-xs">
      <button
        type="button"
        className="flex w-full items-center justify-between text-ops-secondary transition-colors duration-fast ease-ops hover:text-ops-primary"
        onClick={() => setOpen((v) => !v)}
      >
        <span>{view.title || view.kind}</span>
        <span>{open ? '收起' : '展开数据'}</span>
      </button>
      {open && (
        <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all font-mono text-2xs text-ops-tertiary">
          {view.data}
        </pre>
      )}
    </div>
  );
}
