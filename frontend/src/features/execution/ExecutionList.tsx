// 执行列表（首页第三 tab 内容）
// MVP 阶段从 ExecutionStore 读内存数据（不持久化）；Phase 7 加历史加载

import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { useExecutionHydrator } from './ExecutionStore';
import { WorkflowStatusIcon } from './ExecutionStatus';
import {
  useDeleteExecution,
  useExecutions,
  useStopExecution,
} from '@/api/executions';
import { useTabs } from '@/features/tabs/TabsContext';

export function ExecutionList() {
  const navigate = useNavigate();
  const { data, isLoading, error } = useExecutions();
  // Wails 空列表可能返回 null；解构默认值只对 undefined 生效
  const execs = Array.isArray(data) ? data : [];
  const stopMutation = useStopExecution();
  const deleteMutation = useDeleteExecution();
  const { remove: removeFromStore } = useExecutionHydrator();
  const { openTab, closeTab } = useTabs();

  function handleOpen(execID: string, name: string) {
    openTab({
      kind: 'execution',
      id: execID,
      name: `${name} #${execID.slice(0, 4)}`,
    });
    navigate(`/executions/${execID}`);
  }

  async function handleDelete(execID: string) {
    try {
      await deleteMutation.mutateAsync(execID);
      removeFromStore(execID);
      closeTab('execution', execID);
    } catch (err) {
      console.error('删除失败:', err);
    }
  }

  return (
    <>
      <header className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">执行</h1>
          <p className="mt-1 text-sm text-slate-500">
            正在运行的工作流和历史执行记录
          </p>
        </div>
      </header>

      <main className="flex-1">
        {isLoading && execs.length === 0 && (
          <div className="text-sm text-slate-500">加载中...</div>
        )}
        {error && (
          <div className="rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
            加载失败：{error.message}
          </div>
        )}
        {!isLoading && !error && execs.length === 0 && (
          <div className="rounded-lg border border-dashed border-slate-300 bg-white px-6 py-16 text-center">
            <p className="text-sm text-slate-500">还没有任何执行记录</p>
            <p className="mt-2 text-xs text-slate-400">
              在工作流编辑页点击「运行」后，记录会出现在这里
            </p>
            <Button
              variant="ghost"
              size="sm"
              className="mt-4"
              onClick={() => navigate('/')}
            >
              返回工作流列表
            </Button>
          </div>
        )}
        {!error && execs.length > 0 && (
          <ul className="divide-y divide-slate-200 rounded-lg border border-slate-200 bg-white">
            {execs.map((e) => (
              <li key={e.id}>
                <div className="flex items-center justify-between px-4 py-3 hover:bg-slate-50">
                  <button
                    type="button"
                    onClick={() => handleOpen(e.id, e.workflow_name)}
                    className="flex flex-1 items-center gap-3 text-left"
                  >
                    <WorkflowStatusIcon status={e.status} size={16} />
                    <div className="min-w-0 flex-1">
                      <div className="text-sm font-medium text-slate-900">
                        {e.workflow_name}
                        <span className="ml-2 font-mono text-xs text-slate-400">
                          #{e.id.slice(0, 6)}
                        </span>
                      </div>
                      <div className="mt-0.5 text-xs text-slate-500">
                        {statusLabel(e.status)} · {formatTime(e.started_at)}
                        {e.error && (
                          <span className="ml-2 text-red-600">
                            {e.error}
                          </span>
                        )}
                      </div>
                    </div>
                  </button>
                  <div className="flex items-center gap-2">
                    {e.status === 'Running' ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => stopMutation.mutate(e.id)}
                        disabled={stopMutation.isPending}
                      >
                        停止
                      </Button>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(e.id)}
                        disabled={deleteMutation.isPending}
                      >
                        删除
                      </Button>
                    )}
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </main>
    </>
  );
}

function statusLabel(s: string): string {
  switch (s) {
    case 'Running':
      return '运行中';
    case 'Success':
      return '成功';
    case 'Failed':
      return '失败';
    case 'Terminated':
      return '已终止';
    default:
      return s;
  }
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString('zh-CN', { hour12: false });
  } catch {
    return iso;
  }
}
