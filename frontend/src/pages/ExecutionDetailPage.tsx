// 执行详情页（只读）
// 顶栏: ← 返回 | TabBar | 状态指示器 + 重新运行/停止
// 中间: WorkflowCanvas readOnly，画布显示 snapshot 的节点 + 实时状态
// 右侧: NodeDetailPanel 显示选中节点的日志

import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ReactFlowProvider } from '@xyflow/react';
import {
  useExecution as useExecutionQuery,
  useRunWorkflow,
  useStopExecution,
} from '@/api/executions';
import {
  useExecution as useExecutionLive,
  useExecutionHydrator,
} from '@/features/execution/ExecutionStore';
import { WorkflowStatusIcon } from '@/features/execution/ExecutionStatus';
import { WorkflowCanvas } from '@/features/workflow/WorkflowCanvas';
import { NodeDetailPanel } from '@/features/workflow/NodeDetailPanel';
import { Button } from '@/components/ui/Button';
import { CenteredMessage } from '@/components/ui/CenteredMessage';
import { TabBar } from '@/features/tabs/TabBar';
import { useTabs } from '@/features/tabs/TabsContext';
import { snapshotGraph } from '@/types/execution';

export function ExecutionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { openTab } = useTabs();
  const { hydrate } = useExecutionHydrator();

  // 初次拉一份 ExecutionRecord，注入到 store
  const { data: record, isLoading, error } = useExecutionQuery(id);
  useEffect(() => {
    if (record) hydrate(record);
  }, [record, hydrate]);

  // 后续从 store 读实时状态（事件驱动）
  const exec = useExecutionLive(id);

  const runMutation = useRunWorkflow();
  const stopMutation = useStopExecution();

  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  // 进入页面时 upsert 当前 tab（带状态标题）
  useEffect(() => {
    if (!id || !exec) return;
    const shortID = id.slice(0, 4);
    openTab({
      kind: 'execution',
      id,
      name: `${exec.workflowName} #${shortID}`,
    });
  }, [id, exec, openTab]);

  // 重新运行：用同样的 workflowID 启动新执行，跳到新 tab
  const handleRerun = useCallback(async () => {
    if (!exec) return;
    try {
      const newID = await runMutation.mutateAsync(exec.workflowID);
      openTab({
        kind: 'execution',
        id: newID,
        name: `${exec.workflowName} #${newID.slice(0, 4)}`,
      });
      navigate(`/executions/${newID}`);
    } catch (err) {
      console.error('重新运行失败:', err);
    }
  }, [exec, runMutation, openTab, navigate]);

  const handleStop = useCallback(() => {
    if (!id) return;
    stopMutation.mutate(id);
  }, [id, stopMutation]);

  if (isLoading) return <CenteredMessage>加载中...</CenteredMessage>;
  if (error)
    return (
      <CenteredMessage tone="error">加载失败：{error.message}</CenteredMessage>
    );
  if (!exec)
    return <CenteredMessage tone="error">执行记录不存在</CenteredMessage>;
  if (!exec.snapshot)
    return <CenteredMessage tone="error">执行快照缺失</CenteredMessage>;

  const { nodes, edges } = snapshotGraph(exec.snapshot);
  const graph = {
    id: exec.id,
    nodes,
    edges,
  };

  const isRunning = exec.status === 'Running';

  return (
    <ReactFlowProvider>
      <div className="flex h-screen flex-col">
        {/* 顶栏 */}
        <header className="flex h-12 items-center border-b border-slate-200 bg-white px-4">
          <Link
            to="/"
            className="mr-3 text-sm text-slate-500 hover:text-slate-900"
          >
            ← 返回
          </Link>
          <span className="mr-2 text-slate-300">|</span>
          <TabBar />
          <div className="ml-3 flex items-center gap-3">
            <span className="flex items-center gap-1.5 text-xs text-slate-600">
              <WorkflowStatusIcon status={exec.status} size={14} />
              {statusLabel(exec.status)}
            </span>
            {isRunning ? (
              <Button
                size="sm"
                variant="danger"
                onClick={handleStop}
                disabled={stopMutation.isPending}
              >
                ■ 停止
              </Button>
            ) : (
              <Button
                size="sm"
                onClick={handleRerun}
                disabled={runMutation.isPending}
              >
                ↻ 重新运行
              </Button>
            )}
          </div>
        </header>

        <div className="flex flex-1 overflow-hidden">
          <main className="flex-1">
            <WorkflowCanvas
              graph={graph}
              selectedNodeId={selectedNodeId}
              onSelectNode={setSelectedNodeId}
              readOnly
            />
          </main>
          <NodeDetailPanel
            graph={graph}
            selectedNodeId={selectedNodeId}
            executionID={exec.id}
          />
        </div>
      </div>
    </ReactFlowProvider>
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
