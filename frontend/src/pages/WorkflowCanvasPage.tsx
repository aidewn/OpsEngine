// 工作流画布页：顶栏 + 中间画布 + 右侧详情 + 添加节点弹窗

import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ReactFlowProvider } from '@xyflow/react';
import { useUpdateWorkflow, useWorkflow } from '@/api/workflows';
import {
  WorkflowCanvas,
  addNodeToGraph,
  addNodeWithEdge,
  type PendingConnection,
} from '@/features/workflow/WorkflowCanvas';
import { NodeDetailPanel } from '@/features/workflow/NodeDetailPanel';
import { AddNodeDialog } from '@/features/workflow/AddNodeDialog';
import { WorkflowSidebar } from '@/features/workflow/WorkflowSidebar';
import { Button } from '@/components/ui/Button';
import type { WorkflowDef } from '@/types/workflow';
import { buildDefaultConfig, type NodeTypeDef } from '@/types/nodeType';
import { CenteredMessage } from '@/components/ui/CenteredMessage';
import { TabBar } from '@/features/tabs/TabBar';
import { useTabs } from '@/features/tabs/TabsContext';
import { useRunWorkflow } from '@/api/executions';
import { RunningBadge } from '@/features/execution/RunningBadge';

export function WorkflowCanvasPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: workflow, isLoading, error } = useWorkflow(id);
  const update = useUpdateWorkflow();
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
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [pendingConnection, setPendingConnection] =
    useState<PendingConnection | null>(null);

  const handleWorkflowChange = useCallback(
    (next: WorkflowDef) => {
      update.mutate(next);
    },
    [update],
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
    <ReactFlowProvider>
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
              onGraphChange={handleWorkflowChange}
              onPendingConnection={handlePendingConnection}
              onNodeDoubleClick={handleNodeDoubleClick}
            />
          </main>
          <NodeDetailPanel
            graph={workflow}
            selectedNodeId={selectedNodeId}
          />
        </div>

        <AddNodeDialog
          open={addDialogOpen}
          onOpenChange={setAddDialogOpen}
          pendingConnection={pendingConnection}
          onSelect={handleNodeTypeSelected}
        />
      </div>
    </ReactFlowProvider>
  );
}
