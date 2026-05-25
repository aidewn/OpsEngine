// 工作流画布页：顶栏 + 中间画布 + 右侧详情 + 添加节点弹窗

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ReactFlowProvider } from '@xyflow/react';
import { useQueryClient } from '@tanstack/react-query';
import { useUpdateWorkflow, useWorkflow } from '@/api/workflows';
import type { WorkflowDef } from '@/types/workflow';
import {
  WorkflowCanvas,
  addNodeToGraph,
  addNodeWithEdge,
  type PendingConnection,
} from '@/features/workflow/WorkflowCanvas';
import { NodeDetailPanel } from '@/features/workflow/NodeDetailPanel';
import { AddNodeDialog } from '@/features/workflow/AddNodeDialog';
import { WorkflowSidebar } from '@/features/workflow/WorkflowSidebar';
import { cleanupParallelEdges } from '@/features/workflow/cleanupParallel';
import { Button } from '@/components/ui/Button';
import { buildDefaultConfig, type NodeTypeDef } from '@/types/nodeType';
import { CenteredMessage } from '@/components/ui/CenteredMessage';
import { TabBar } from '@/features/tabs/TabBar';
import { useTabs } from '@/features/tabs/TabsContext';
import { useRunWorkflow } from '@/api/executions';
import { RunningBadge } from '@/features/execution/RunningBadge';
import { useCopyPaste } from '@/features/clipboard/useCopyPaste';

// 外壳组件：仅负责挂载 ReactFlowProvider
// 内部 hook（如 useCopyPaste → useReactFlow）必须位于 Provider 子树才能正常工作
export function WorkflowCanvasPage() {
  const { id } = useParams<{ id: string }>();
  return (
    <ReactFlowProvider>
      <WorkflowCanvasInner workflowId={id} />
    </ReactFlowProvider>
  );
}

// 内核组件：承载页面的全部逻辑与渲染
function WorkflowCanvasInner({ workflowId: id }: { workflowId: string | undefined }) {
  const navigate = useNavigate();
  const { data: workflow, isLoading, error } = useWorkflow(id);
  const update = useUpdateWorkflow();
  const queryClient = useQueryClient();
  const { openTab } = useTabs();
  const runMutation = useRunWorkflow();

  // 进入页面 / 切到该路由时 upsert 当前 tab
  // 直接访问 URL 时先用 id 占位，doc 加载后用 name 覆盖
  useEffect(() => {
    if (!id) return;
    openTab({ kind: 'workflow', id, name: workflow?.name ?? id });
  }, [id, workflow?.name, openTab]);

  // 双击集合调用节点 → openTab + 跳转
  const handleNodeDoubleClick = useCallback(
    (typeId: string) => {
      if (typeId.startsWith('assemble:')) {
        const assembleId = typeId.slice('assemble:'.length);
        openTab({ kind: 'assemble', id: assembleId, name: assembleId });
        navigate(`/assembles/${assembleId}`);
      }
    },
    [navigate, openTab],
  );

  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set());
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [pendingConnection, setPendingConnection] =
    useState<PendingConnection | null>(null);

  const handleWorkflowChange = useCallback(
    (next: WorkflowDef) => {
      // 副作用：parallel 节点 branch_count 改小后清理超范围 exec_out 边
      const cleaned = cleanupParallelEdges(next);
      update.mutate(cleaned);
    },
    [update],
  );

  // 节点 config 修改回调（由 NodeDetailPanel.ConfigForm 触发）
  const handleConfigChange = useCallback(
    (nodeId: string, config: Record<string, unknown>) => {
      if (!id) return;
      const detailKey = ['workflows', id] as const;
      const prev = queryClient.getQueryData<WorkflowDef>(detailKey);
      if (!prev) return;
      const next: WorkflowDef = {
        ...prev,
        nodes: prev.nodes.map((n) =>
          n.instance_id === nodeId ? { ...n, config } : n,
        ),
      };
      handleWorkflowChange(next);
    },
    [id, queryClient, handleWorkflowChange],
  );

  function handleAddButtonClick() {
    setPendingConnection(null);
    setAddDialogOpen(true);
  }

  // 启动工作流执行：调后端 Run → openTab(execution) → 跳转
  async function handleRun() {
    if (!workflow) return;
    try {
      const execID = await runMutation.mutateAsync(workflow.id);
      openTab({
        kind: 'execution',
        id: execID,
        name: `${workflow.name} #${execID.slice(0, 4)}`,
      });
      navigate(`/executions/${execID}`);
    } catch (err) {
      console.error('启动执行失败:', err);
    }
  }

  const handlePendingConnection = useCallback((pending: PendingConnection) => {
    setPendingConnection(pending);
    setAddDialogOpen(true);
  }, []);

  function handleNodeTypeSelected(
    typeDef: NodeTypeDef,
    matchedPortId: string | null,
  ) {
    if (!workflow) return;

    // 从 ConfigSchema 收集默认值作为新节点的初始 config
    const config = buildDefaultConfig(typeDef);
    let result: { graph: WorkflowDef; nodeId: string };

    if (pendingConnection && matchedPortId) {
      result = addNodeWithEdge(
        workflow,
        typeDef.type_id,
        pendingConnection.position,
        pendingConnection,
        matchedPortId,
        config,
      );
    } else {
      const offset = workflow.nodes.length * 20;
      result = addNodeToGraph(
        workflow,
        typeDef.type_id,
        { x: 400 + offset, y: 200 + offset },
        config,
      );
    }

    handleWorkflowChange(result.graph);
    setSelectedNodeId(result.nodeId);
    setPendingConnection(null);
  }

  // 复制粘贴：Ctrl/Cmd + C/X/V
  // Hook 必须在所有早返回之前调用（保持调用顺序）
  // workflow 未加载时用稳定的兜底对象 + ":none" editorKey 让 hook 内部短路
  const fallbackWorkflow = useMemo(
    () => ({ id: '', nodes: [], edges: [] }) as unknown as WorkflowDef,
    [],
  );
  useCopyPaste<WorkflowDef>({
    editorKey: workflow ? `workflow:${workflow.id}` : 'workflow:none',
    graph: workflow ?? fallbackWorkflow,
    selectedNodeIds,
    onGraphChange: handleWorkflowChange,
  });

  if (isLoading) return <CenteredMessage>加载中...</CenteredMessage>;
  if (error)
    return (
      <CenteredMessage tone="error">
        加载失败：{error.message}
      </CenteredMessage>
    );
  if (!workflow)
    return <CenteredMessage tone="error">工作流不存在</CenteredMessage>;

  return (
    <div className="flex h-screen flex-col">
      {/* 顶栏：返回 | tab 列表 | 右侧操作 */}
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
          <span className="text-xs text-slate-500">
            {update.isPending ? '保存中...' : '已保存'}
          </span>
          <RunningBadge workflowID={workflow.id} />
          <Button
            size="sm"
            onClick={handleRun}
            disabled={runMutation.isPending}
          >
            ▶ 运行
          </Button>
          <Button size="sm" variant="secondary" onClick={handleAddButtonClick}>
            + 添加节点
          </Button>
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden">
        <WorkflowSidebar
          workflow={workflow}
          onChange={handleWorkflowChange}
        />
        <main className="flex-1">
          <WorkflowCanvas
            graph={workflow}
            selectedNodeId={selectedNodeId}
            onSelectNode={setSelectedNodeId}
            onSelectedNodesChange={setSelectedNodeIds}
            onGraphChange={handleWorkflowChange}
            onPendingConnection={handlePendingConnection}
            onNodeDoubleClick={handleNodeDoubleClick}
          />
        </main>
        <NodeDetailPanel
          graph={workflow}
          selectedNodeId={selectedNodeId}
          onConfigChange={handleConfigChange}
        />
      </div>

      <AddNodeDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        pendingConnection={pendingConnection}
        onSelect={handleNodeTypeSelected}
      />
    </div>
  );
}
