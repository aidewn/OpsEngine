// 工作流画布页：顶栏 + 中间画布 + 右侧详情 + 添加节点弹窗

import { useCallback, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { ReactFlowProvider } from '@xyflow/react';
import { useUpdateWorkflow, useWorkflow } from '@/api/workflows';
import {
  WorkflowCanvas,
  addNodeToWorkflow,
  addNodeWithEdge,
  type PendingConnection,
} from '@/features/workflow/WorkflowCanvas';
import { NodeDetailPanel } from '@/features/workflow/NodeDetailPanel';
import { AddNodeDialog } from '@/features/workflow/AddNodeDialog';
import { Button } from '@/components/ui/Button';
import type { WorkflowDef } from '@/types/workflow';
import type { NodeTypeDef } from '@/types/nodeType';

export function WorkflowCanvasPage() {
  const { id } = useParams<{ id: string }>();
  const { data: workflow, isLoading, error } = useWorkflow(id);
  const update = useUpdateWorkflow();

  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  // 拖线到空白处的上下文，传给 AddNodeDialog 做端口过滤
  const [pendingConnection, setPendingConnection] =
    useState<PendingConnection | null>(null);

  // 统一的 workflow 更新入口
  const handleWorkflowChange = useCallback(
    (next: WorkflowDef) => {
      update.mutate(next);
    },
    [update],
  );

  // 顶部按钮点击：打开添加弹窗（无端口过滤）
  function handleAddButtonClick() {
    setPendingConnection(null);
    setAddDialogOpen(true);
  }

  // 拖线到空白处：打开添加弹窗（带端口过滤）
  const handlePendingConnection = useCallback((pending: PendingConnection) => {
    setPendingConnection(pending);
    setAddDialogOpen(true);
  }, []);

  // 弹窗中选择节点类型后的回调
  function handleNodeTypeSelected(
    typeDef: NodeTypeDef,
    matchedPortId: string | null,
  ) {
    if (!workflow) return;

    let result: { workflow: WorkflowDef; nodeId: string };

    if (pendingConnection && matchedPortId) {
      // 拖线创建：在松手位置创建节点并自动连线
      result = addNodeWithEdge(
        workflow,
        typeDef.type_id,
        pendingConnection.position,
        pendingConnection,
        matchedPortId,
      );
    } else {
      // 按钮创建：在画布中心偏移位置创建节点
      const offset = workflow.nodes.length * 20;
      result = addNodeToWorkflow(workflow, typeDef.type_id, {
        x: 400 + offset,
        y: 200 + offset,
      });
    }

    handleWorkflowChange(result.workflow);
    setSelectedNodeId(result.nodeId);
    setPendingConnection(null);
  }

  if (isLoading) {
    return <CenteredMessage>加载中...</CenteredMessage>;
  }
  if (error) {
    return (
      <CenteredMessage tone="error">
        加载失败：{error.message}
      </CenteredMessage>
    );
  }
  if (!workflow) {
    return <CenteredMessage tone="error">工作流不存在</CenteredMessage>;
  }

  return (
    <ReactFlowProvider>
      <div className="flex h-screen flex-col">
        <Header
          workflow={workflow}
          saving={update.isPending}
          onAddNode={handleAddButtonClick}
        />
        <div className="flex flex-1 overflow-hidden">
          <main className="flex-1">
            <WorkflowCanvas
              workflow={workflow}
              selectedNodeId={selectedNodeId}
              onSelectNode={setSelectedNodeId}
              onWorkflowChange={handleWorkflowChange}
              onPendingConnection={handlePendingConnection}
            />
          </main>
          <NodeDetailPanel
            workflow={workflow}
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

// ── 顶栏 ─────────────────────────────────────────────────

function Header({
  workflow,
  saving,
  onAddNode,
}: {
  workflow: WorkflowDef;
  saving: boolean;
  onAddNode: () => void;
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
      <div className="flex items-center gap-3">
        <span className="text-xs text-slate-500">
          {saving ? '保存中...' : '已保存'}
        </span>
        <Button size="sm" onClick={onAddNode}>
          + 添加节点
        </Button>
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
