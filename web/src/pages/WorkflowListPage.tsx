// 工作流列表页

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { useWorkflows } from '@/api/workflows';
import { CreateWorkflowDialog } from '@/features/workflow/CreateWorkflowDialog';

export function WorkflowListPage() {
  const [dialogOpen, setDialogOpen] = useState(false);
  const { data, isLoading, error } = useWorkflows();

  return (
    <div className="mx-auto flex h-full max-w-5xl flex-col px-6 py-8">
      <header className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">工作流</h1>
          <p className="mt-1 text-sm text-slate-500">
            管理你的运维工作流
          </p>
        </div>
        <Button onClick={() => setDialogOpen(true)}>+ 新建工作流</Button>
      </header>

      <main className="flex-1">
        {isLoading && <div className="text-sm text-slate-500">加载中...</div>}
        {error && (
          <div className="rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
            加载失败：{error.message}
          </div>
        )}
        {data && data.length === 0 && (
          <EmptyState onCreate={() => setDialogOpen(true)} />
        )}
        {data && data.length > 0 && (
          <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white">
            {data.map((wf) => (
              <li key={wf.id}>
                <Link
                  to={`/workflows/${wf.id}`}
                  className="flex items-center justify-between px-4 py-3 hover:bg-slate-50"
                >
                  <div>
                    <div className="text-sm font-medium text-slate-900">
                      {wf.name}
                    </div>
                    <div className="mt-0.5 font-mono text-xs text-slate-500">
                      {wf.id}
                    </div>
                    {wf.description && (
                      <div className="mt-1 text-xs text-slate-600">
                        {wf.description}
                      </div>
                    )}
                  </div>
                  <span className="text-slate-400">›</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </main>

      <CreateWorkflowDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-slate-300 bg-white px-6 py-16 text-center">
      <div className="text-sm text-slate-500">还没有工作流</div>
      <Button className="mt-4" onClick={onCreate}>
        创建第一个工作流
      </Button>
    </div>
  );
}
