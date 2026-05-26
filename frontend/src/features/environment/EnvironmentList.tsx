// 环境列表（嵌入首页 tab 内容区）
// 与 WorkflowList 同构：header + 新建按钮 + loading/error/empty/list 四态 + 删除确认

import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { useEnvironments, useDeleteEnvironment } from '@/api/environments';
import { CreateEnvironmentDialog } from './CreateEnvironmentDialog';

export function EnvironmentList() {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialog, setDeleteDialog] = useState<{
    open: boolean;
    id: string;
    name: string;
  } | null>(null);
  const { data, isLoading, error } = useEnvironments();
  const deleteEnv = useDeleteEnvironment();

  return (
    <>
      <header className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">配置环境</h1>
          <p className="mt-1 text-sm text-slate-500">
            按业务/项目集中管理 SSH/Docker/K8s/Jenkins 凭证
          </p>
        </div>
        <Button onClick={() => setDialogOpen(true)}>+ 新建环境</Button>
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
            {data.map((env) => (
              <li key={env.id}>
                <div className="flex items-center justify-between px-4 py-3 hover:bg-slate-50">
                  <Link to={`/environments/${env.id}`} className="flex-1">
                    <div className="text-sm font-medium text-slate-900">
                      {env.name}
                    </div>
                    <div className="mt-0.5 font-mono text-xs text-slate-500">
                      {env.id}
                    </div>
                    {env.description && (
                      <div className="mt-1 text-xs text-slate-600">
                        {env.description}
                      </div>
                    )}
                    <div className="mt-1 text-xs text-slate-400">
                      {env.configs?.length ?? 0} 条配置
                    </div>
                  </Link>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={(e) => {
                        e.preventDefault();
                        setDeleteDialog({
                          open: true,
                          id: env.id,
                          name: env.name,
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

      <CreateEnvironmentDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />

      {deleteDialog && (
        <Dialog
          open={deleteDialog.open}
          onOpenChange={(open) => {
            if (!open) setDeleteDialog(null);
          }}
          title="确认删除"
          description={`确定要删除环境「${deleteDialog.name}」吗？此操作不可恢复。`}
          footer={
            <>
              <Button
                type="button"
                variant="secondary"
                onClick={() => setDeleteDialog(null)}
                disabled={deleteEnv.isPending}
              >
                取消
              </Button>
              <Button
                type="button"
                variant="danger"
                onClick={async () => {
                  try {
                    await deleteEnv.mutateAsync(deleteDialog.id);
                    setDeleteDialog(null);
                  } catch (err) {
                    console.error('删除失败:', err);
                  }
                }}
                disabled={deleteEnv.isPending}
              >
                {deleteEnv.isPending ? '删除中...' : '确认删除'}
              </Button>
            </>
          }
        >
          <div className="text-sm text-slate-600">
            环境 ID: <span className="font-mono">{deleteDialog.id}</span>
          </div>
        </Dialog>
      )}
    </>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-slate-300 bg-white px-6 py-16 text-center">
      <div className="text-sm text-slate-500">还没有环境</div>
      <Button className="mt-4" onClick={onCreate}>
        创建第一个环境
      </Button>
    </div>
  );
}
