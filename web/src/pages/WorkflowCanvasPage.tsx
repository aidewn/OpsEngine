// 工作流画布页：左侧导航 + 中间画布 + 右侧详情

import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useUpdateWorkflow, useWorkflow } from '@/api/workflows';
import { WorkflowCanvas } from '@/features/workflow/WorkflowCanvas';
import { NodeDetailPanel } from '@/features/workflow/NodeDetailPanel';
import type { WorkflowDef } from '@/types/workflow';

export function WorkflowCanvasPage() {
  const { id } = useParams<{ id: string }>();
  const { data: workflow, isLoading, error } = useWorkflow(id);
  const update = useUpdateWorkflow();

  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  if (isLoading) {
    return <CenteredMessage>加载中...</CenteredMessage>;
  }
  if (error) {
    return (
      <CenteredMessage tone="error">加载失败：{error.message}</CenteredMessage>
    );
  }
  if (!workflow) {
    return <CenteredMessage tone="error">工作流不存在</CenteredMessage>;
  }

  function handlePositionsChange(next: WorkflowDef) {
    // 后台保存，不阻塞 UI；失败时让 TanStack Query 的 error state 显示出来即可
    update.mutate(next);
  }

  return (
    <div className="flex h-screen flex-col">
      <Header workflow={workflow} saving={update.isPending} />
      <div className="flex flex-1 overflow-hidden">
        <main className="flex-1">
          <WorkflowCanvas
            workflow={workflow}
            selectedNodeId={selectedNodeId}
            onSelectNode={setSelectedNodeId}
            onNodePositionsChange={handlePositionsChange}
          />
        </main>
        <NodeDetailPanel
          workflow={workflow}
          selectedNodeId={selectedNodeId}
        />
      </div>
    </div>
  );
}

function Header({
  workflow,
  saving,
}: {
  workflow: WorkflowDef;
  saving: boolean;
}) {
  return (
    <header className="flex h-12 items-center justify-between border-b border-slate-200 bg-white px-4">
      <div className="flex items-center gap-3">
        <Link
          to="/"
          className="text-sm text-slate-500 hover:text-slate-900"
        >
          ← 工作流列表
        </Link>
        <span className="text-slate-300">|</span>
        <span className="text-sm font-medium text-slate-900">
          {workflow.name}
        </span>
        <span className="font-mono text-xs text-slate-400">{workflow.id}</span>
      </div>
      <div className="text-xs text-slate-500">
        {saving ? '保存中...' : '已保存'}
      </div>
    </header>
  );
}

function CenteredMessage({
  children,
  tone = 'info',
}: {
  children: React.ReactNode;
  tone?: 'info' | 'error';
}) {
  return (
    <div className="flex h-screen items-center justify-center">
      <div
        className={
          tone === 'error'
            ? 'text-sm text-red-600'
            : 'text-sm text-slate-500'
        }
      >
        {children}
      </div>
    </div>
  );
}
