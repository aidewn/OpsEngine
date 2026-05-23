// 工作流列表（嵌入首页 tab 内容区）

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { useWorkflows, useDeleteWorkflow } from '@/api/workflows';
import { CreateWorkflowDialog } from './CreateWorkflowDialog';
import { useTabs } from '@/features/tabs/TabsContext';

export function WorkflowList() {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialog, setDeleteDialog] = useState<{
    open: boolean;
    id: string;
    name: string;
  } | null>(null);
  const { data, isLoading, error } = useWorkflows();
  const deleteWorkflow = useDeleteWorkflow();
  const { closeTab } = useTabs();

  return (
    <>
      <header className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">工作流</h1>
          <p className="mt-1 text-sm text-slate-500">管理你的运维工作流</p>
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
                <div className="flex items-center justify-between px-4 py-3 hover:bg-slate-50">
                  <Link to={`/workflows/${wf.id}`} className="flex-1">
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
                  </Link>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={(e) => {
                        e.preventDefault();
                        setDeleteDialog({
                          open: true,
                          id: wf.id,
                          name: wf.name,
                        });
                      }}
                    >
                      删除
                    </Button>
                    <span className="text-slate-400">›</span>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </main>

      <CreateWorkflowDialog open={dialogOpen} onOpenChange={setDialogOpen} />

      {deleteDialog && (
        <Dialog
          open={deleteDialog.open}
          onOpenChange={(open) => {
            if (!open) setDeleteDialog(null);
          }}
          title="确认删除"
          description={`确定要删除工作流「${deleteDialog.name}」吗？此操作不可恢复。`}
          footer={
            <>
              <Button
                type="button"
                variant="secondary"
                onClick={() => setDeleteDialog(null)}
                disabled={deleteWorkflow.isPending}
              >
                取消
              </Button>
              <Button
                type="button"
                variant="danger"
                onClick={async () => {
                  try {
                    await deleteWorkflow.mutateAsync(deleteDialog.id);
                    // 同步关闭对应的 tab（如有）
                    closeTab('workflow', deleteDialog.id);
                    setDeleteDialog(null);
                  } catch (err) {
                    console.error('删除失败:', err);
                  }
                }}
                disabled={deleteWorkflow.isPending}
              >
                {deleteWorkflow.isPending ? '删除中...' : '确认删除'}
              </Button>
            </>
          }
        >
          <div className="text-sm text-slate-600">
            工作流 ID: <span className="font-mono">{deleteDialog.id}</span>
          </div>
        </Dialog>
      )}
    </>
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
