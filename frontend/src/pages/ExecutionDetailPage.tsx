// 执行详情页（只读）
// 顶栏: ← 返回 | TabBar | 状态指示器 + 重新运行/停止
// 左侧: ExecutionCallStack 调用栈侧栏（进入子 frame 的唯一入口）
// 中间: WorkflowCanvas readOnly，画布显示当前 frame 对应的 snapshot 节点 + 实时状态
// 右侧: NodeDetailPanel 显示选中节点的日志
// 画布上方面包屑: 主流 / 集合A / 集合B...

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ReactFlowProvider } from '@xyflow/react';
import {
  useExecution as useExecutionQuery,
  useRunWorkflow,
  useStopExecution,
} from '@/api/executions';
import {
  frameAt,
  recordToState,
  useExecution as useExecutionLive,
  useExecutionHydrator,
} from '@/features/execution/ExecutionStore';
import { WorkflowStatusIcon } from '@/features/execution/ExecutionStatus';
import { ExecutionCallStack } from '@/features/execution/ExecutionCallStack';
import { WorkflowCanvas } from '@/features/workflow/WorkflowCanvas';
import { NodeDetailPanel } from '@/features/workflow/NodeDetailPanel';
import { Button } from '@/components/ui/Button';
import { CenteredMessage } from '@/components/ui/CenteredMessage';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import { TabBar } from '@/features/tabs/TabBar';
import { useTabs } from '@/features/tabs/TabsContext';
import { FramePathContext } from '@/features/workflow/nodes/useNodeExecState';
import { cn } from '@/lib/cn';

// 记录不存在/加载失败时，停留 NOT_FOUND_REDIRECT_MS 后自动跳首页
const NOT_FOUND_REDIRECT_MS = 1500;

export function ExecutionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { openTab, closeTab } = useTabs();
  const { hydrate } = useExecutionHydrator();

  const { data: record, isLoading, error } = useExecutionQuery(id);
  useEffect(() => {
    if (record) hydrate(record);
  }, [record, hydrate]);

  const liveExec = useExecutionLive(id);

  // hydrate 还没跑完时用 record 兜底，避免「记录已到但 store 仍空 → 误报不存在」白屏竞态
  const exec = useMemo(
    () => liveExec ?? (record ? recordToState(record) : null),
    [liveExec, record],
  );

  const runMutation = useRunWorkflow();
  const stopMutation = useStopExecution();

  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  // 当前查看的 frame 路径（[] = 主流；["callA"] = 主流中调用 callA 后的子帧）
  const [framePath, setFramePath] = useState<string[]>([]);

  // 切换 framePath 时清掉选中节点（不同 frame 的节点 id 可能相同）
  useEffect(() => {
    setSelectedNodeId(null);
  }, [framePath]);

  useEffect(() => {
    if (!id || !exec) return;
    const shortID = id.slice(0, 4);
    openTab({
      kind: 'execution',
      id,
      name: `${exec.workflowName} #${shortID}`,
    });
  }, [id, exec, openTab]);

  // 加载失败 / 加载完成仍找不到记录 → 关掉对应 tab 并跳首页
  // （比如别处删了记录但仍停留在旧 URL）
  const notFound = !isLoading && (!!error || !exec);
  useEffect(() => {
    if (!notFound) return;
    const timer = window.setTimeout(() => {
      if (id) closeTab('execution', id);
      navigate('/', { replace: true });
    }, NOT_FOUND_REDIRECT_MS);
    return () => window.clearTimeout(timer);
  }, [notFound, id, navigate, closeTab]);

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

  // 根据 framePath 解析当前 frame 对应的图
  const graph = useMemo(() => {
    if (!exec?.snapshot) return null;
    const frame = frameAt(exec.rootFrame, framePath);
    if (framePath.length === 0 || !frame?.assemble_id) {
      const wf = exec.snapshot.workflow;
      return { id: exec.id, nodes: wf.nodes, edges: wf.edges };
    }
    const asm = exec.snapshot.assembles[frame.assemble_id];
    if (!asm) return null;
    return { id: exec.id, nodes: asm.nodes, edges: asm.edges };
  }, [exec, framePath]);

  if (isLoading && !exec) return <CenteredMessage>加载中...</CenteredMessage>;
  if (error)
    return (
      <CenteredMessage tone="error">
        加载失败：{error.message}（即将返回首页）
      </CenteredMessage>
    );
  if (!exec)
    return (
      <CenteredMessage tone="error">
        执行记录不存在（即将返回首页）
      </CenteredMessage>
    );
  if (!exec.snapshot || !graph)
    return <CenteredMessage tone="error">执行快照缺失</CenteredMessage>;

  const isRunning = exec.status === 'Running';

  // 计算面包屑各层级的标题
  const breadcrumbs: { label: string; path: string[] }[] = [
    { label: exec.workflowName, path: [] },
  ];
  if (exec.snapshot) {
    let cur = exec.rootFrame;
    for (let i = 0; i < framePath.length; i++) {
      const callerID = framePath[i]!;
      const child = cur.children?.[callerID];
      if (!child) break;
      const asmName =
        exec.snapshot.assembles[child.assemble_id]?.name ?? child.assemble_id;
      breadcrumbs.push({ label: asmName, path: framePath.slice(0, i + 1) });
      cur = child;
    }
  }

  return (
    <ReactFlowProvider>
      <FramePathContext.Provider value={framePath}>
        <div className="flex h-screen flex-col">
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

          {/* 面包屑 */}
          {breadcrumbs.length > 1 && (
            <div className="flex items-center gap-1 border-b border-slate-200 bg-slate-50 px-4 py-1.5 text-xs">
              {breadcrumbs.map((b, idx) => {
                const isLast = idx === breadcrumbs.length - 1;
                return (
                  <span key={idx} className="flex items-center gap-1">
                    {idx > 0 && <span className="text-slate-300">/</span>}
                    <button
                      type="button"
                      onClick={() => setFramePath(b.path)}
                      className={cn(
                        isLast
                          ? 'font-medium text-slate-900'
                          : 'text-slate-500 hover:text-slate-700',
                      )}
                      disabled={isLast}
                    >
                      {b.label}
                    </button>
                  </span>
                );
              })}
            </div>
          )}

          <ErrorBoundary>
            <div className="flex flex-1 overflow-hidden">
              <ExecutionCallStack
                exec={exec}
                framePath={framePath}
                selectedNodeId={selectedNodeId}
                onSelectFrame={setFramePath}
                onSelectNode={setSelectedNodeId}
              />
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
                framePath={framePath}
              />
            </div>
          </ErrorBoundary>
        </div>
      </FramePathContext.Provider>
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
