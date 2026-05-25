// 集合画布页：顶栏 + 中间画布 + 右侧详情 + 添加节点弹窗

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ReactFlowProvider } from '@xyflow/react';
import { useQueryClient } from '@tanstack/react-query';
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
import { AssembleProvider } from '@/features/assemble/AssembleContext';
import { cleanupParallelEdges } from '@/features/workflow/cleanupParallel';
import { useCopyPaste } from '@/features/clipboard/useCopyPaste';
import { Button } from '@/components/ui/Button';
import type { AssembleDef } from '@/types/assemble';
import { buildDefaultConfig, type NodeTypeDef } from '@/types/nodeType';
import { CenteredMessage } from '@/components/ui/CenteredMessage';
import { TabBar } from '@/features/tabs/TabBar';
import { useTabs } from '@/features/tabs/TabsContext';

// 外壳组件：仅负责挂载 ReactFlowProvider
// 内部 hook（如 useCopyPaste → useReactFlow）必须位于 Provider 子树才能正常工作
export function AssembleCanvasPage() {
  const { id } = useParams<{ id: string }>();
  return (
    <ReactFlowProvider>
      <AssembleCanvasInner assembleId={id} />
    </ReactFlowProvider>
  );
}

// 内核组件：承载页面的全部逻辑与渲染
function AssembleCanvasInner({ assembleId: id }: { assembleId: string | undefined }) {
  const navigate = useNavigate();
  const { data: assemble, isLoading, error } = useAssemble(id);
  const update = useUpdateAssemble();
  const queryClient = useQueryClient();
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
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set());
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [pendingConnection, setPendingConnection] =
    useState<PendingConnection | null>(null);

  const handleAssembleChange = useCallback(
    (next: AssembleDef) => {
      const cleaned = cleanupParallelEdges(next);
      update.mutate(cleaned);
    },
    [update],
  );

  const handleConfigChange = useCallback(
    (nodeId: string, config: Record<string, unknown>) => {
      if (!id) return;
      const detailKey = ['assembles', id] as const;
      const prev = queryClient.getQueryData<AssembleDef>(detailKey);
      if (!prev) return;
      const next: AssembleDef = {
        ...prev,
        nodes: prev.nodes.map((n) =>
          n.instance_id === nodeId ? { ...n, config } : n,
        ),
      };
      handleAssembleChange(next);
    },
    [id, queryClient, handleAssembleChange],
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

    const config = buildDefaultConfig(typeDef);
    let result: { graph: AssembleDef; nodeId: string };

    if (pendingConnection && matchedPortId) {
      result = addNodeWithEdge(
        assemble,
        typeDef.type_id,
        pendingConnection.position,
        pendingConnection,
        matchedPortId,
        config,
      );
    } else {
      const offset = assemble.nodes.length * 20;
      result = addNodeToGraph(
        assemble,
        typeDef.type_id,
        { x: 400 + offset, y: 200 + offset },
        config,
      );
    }

    handleAssembleChange(result.graph);
    setSelectedNodeId(result.nodeId);
    setPendingConnection(null);
  }

  // 复制粘贴：Ctrl/Cmd + C/X/V
  // assemble 未加载时用稳定兜底对象 + ":none" editorKey 让 hook 内部短路
  const fallbackAssemble = useMemo(
    () =>
      ({
        id: '',
        params: [],
        returns: [],
        variables: [],
        nodes: [],
        edges: [],
      }) as unknown as AssembleDef,
    [],
  );
  useCopyPaste<AssembleDef>({
    editorKey: assemble ? `assemble:${assemble.id}` : 'assemble:none',
    graph: assemble ?? fallbackAssemble,
    selectedNodeIds,
    onGraphChange: handleAssembleChange,
  });

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
    <AssembleProvider value={assemble}>
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
              onSelectedNodesChange={setSelectedNodeIds}
              onGraphChange={handleAssembleChange}
              onPendingConnection={handlePendingConnection}
              onNodeDoubleClick={handleNodeDoubleClick}
            />
          </main>
          <NodeDetailPanel
            graph={assemble}
            selectedNodeId={selectedNodeId}
            onConfigChange={handleConfigChange}
            onParamsChange={(params) =>
              handleAssembleChange({ ...assemble, params })
            }
            onReturnsChange={(returns) =>
              handleAssembleChange({ ...assemble, returns })
            }
          />
        </div>

        <AddNodeDialog
          open={addDialogOpen}
          onOpenChange={setAddDialogOpen}
          pendingConnection={pendingConnection}
          onSelect={handleNodeTypeSelected}
        />
      </div>
    </AssembleProvider>
  );
}
