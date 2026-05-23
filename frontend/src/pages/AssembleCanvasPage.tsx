// 集合画布页：顶栏 + 中间画布 + 右侧详情 + 添加节点弹窗

import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ReactFlowProvider } from '@xyflow/react';
import { useAssemble, useUpdateAssemble } from '@/api/assembles';
import {
  WorkflowCanvas,
  addNodeToGraph,
  addNodeWithEdge,
  type PendingConnection,
} from '@/features/workflow/WorkflowCanvas';
import { NodeDetailPanel } from '@/features/workflow/NodeDetailPanel';
import { AddNodeDialog } from '@/features/workflow/AddNodeDialog';
import { AssembleSidebar } from '@/features/assemble/AssembleSidebar';
import { Button } from '@/components/ui/Button';
import type { AssembleDef } from '@/types/assemble';
import type { NodeTypeDef } from '@/types/nodeType';
import { CenteredMessage } from '@/components/ui/CenteredMessage';
import { TabBar } from '@/features/tabs/TabBar';
import { useTabs } from '@/features/tabs/TabsContext';

export function AssembleCanvasPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: assemble, isLoading, error } = useAssemble(id);
  const update = useUpdateAssemble();
  const { openTab } = useTabs();

  // 进入页面 / 切到该路由时 upsert tab
  useEffect(() => {
    if (!id) return;
    openTab({ kind: 'assemble', id, name: assemble?.name ?? id });
  }, [id, assemble?.name, openTab]);

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

  const handleAssembleChange = useCallback(
    (next: AssembleDef) => {
      update.mutate(next);
    },
    [update],
  );

  function handleAddButtonClick() {
    setPendingConnection(null);
    setAddDialogOpen(true);
  }

  const handlePendingConnection = useCallback((pending: PendingConnection) => {
    setPendingConnection(pending);
    setAddDialogOpen(true);
  }, []);

  function handleNodeTypeSelected(
    typeDef: NodeTypeDef,
    matchedPortId: string | null,
  ) {
    if (!assemble) return;

    let result: { graph: AssembleDef; nodeId: string };

    if (pendingConnection && matchedPortId) {
      result = addNodeWithEdge(
        assemble,
        typeDef.type_id,
        pendingConnection.position,
        pendingConnection,
        matchedPortId,
      );
    } else {
      const offset = assemble.nodes.length * 20;
      result = addNodeToGraph(assemble, typeDef.type_id, {
        x: 400 + offset,
        y: 200 + offset,
      });
    }

    handleAssembleChange(result.graph);
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
  if (!assemble)
    return <CenteredMessage tone="error">集合不存在</CenteredMessage>;

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
            <Button size="sm" onClick={handleAddButtonClick}>
              + 添加节点
            </Button>
          </div>
        </header>

        <div className="flex flex-1 overflow-hidden">
          <AssembleSidebar
            assemble={assemble}
            onChange={handleAssembleChange}
          />
          <main className="flex-1">
            <WorkflowCanvas
              graph={assemble}
              selectedNodeId={selectedNodeId}
              onSelectNode={setSelectedNodeId}
              onGraphChange={handleAssembleChange}
              onPendingConnection={handlePendingConnection}
              onNodeDoubleClick={handleNodeDoubleClick}
            />
          </main>
          <NodeDetailPanel
            graph={assemble}
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
