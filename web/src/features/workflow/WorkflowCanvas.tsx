// 工作流画布（只读编辑：当前 MVP 只支持节点拖动改 position，不支持新增/连线）

import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  type Node as RfNode,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { useEffect, useRef } from 'react';
import type { WorkflowDef } from '@/types/workflow';
import { nodeTypeMap } from './nodes/nodeTypeMap';
import {
  mergeNodePositions,
  type RfNodeData,
  workflowToRf,
} from './canvasMapping';

interface WorkflowCanvasProps {
  workflow: WorkflowDef;
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
  // 节点 position 变更后回调（拖拽结束）
  onNodePositionsChange: (workflow: WorkflowDef) => void;
}

export function WorkflowCanvas({
  workflow,
  selectedNodeId,
  onSelectNode,
  onNodePositionsChange,
}: WorkflowCanvasProps) {
  const initial = workflowToRf(workflow);
  const [nodes, setNodes, onNodesChange] = useNodesState<RfNode<RfNodeData>>(
    initial.nodes,
  );
  const [edges, , onEdgesChange] = useEdgesState(initial.edges);

  // 外部 workflow 变化时（如刷新数据）重置画布
  // 用 ref 比较 nodes 数量与 id 集，避免每次 render 都重置
  const lastWorkflowKey = useRef<string>(buildKey(workflow));
  useEffect(() => {
    const key = buildKey(workflow);
    if (key !== lastWorkflowKey.current) {
      const next = workflowToRf(workflow);
      setNodes(next.nodes);
      lastWorkflowKey.current = key;
    }
  }, [workflow, setNodes]);

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onNodeClick={(_, n) => onSelectNode(n.id)}
      onPaneClick={() => onSelectNode(null)}
      onNodeDragStop={() => {
        // 拖拽结束时把当前 RF nodes 的 position 合并回 workflow 并通知父组件
        onNodePositionsChange(mergeNodePositions(workflow, nodes));
      }}
      nodeTypes={nodeTypeMap}
      // 选中状态由父组件控制，画布只负责报告点击事件
      // 但 RF 自己也会维护 selected 标记，需要同步
      selectionOnDrag={false}
      fitView
      proOptions={{ hideAttribution: true }}
    >
      <Background gap={20} size={1} />
      <Controls />
      <MiniMap pannable zoomable />
      {/* selectedNodeId 同步到 RF 的 selected 标志 */}
      <SyncSelection
        selectedNodeId={selectedNodeId}
        nodes={nodes}
        setNodes={setNodes}
      />
    </ReactFlow>
  );
}

// 将父组件传入的 selectedNodeId 同步到每个 RF 节点的 selected 标志
function SyncSelection({
  selectedNodeId,
  nodes,
  setNodes,
}: {
  selectedNodeId: string | null;
  nodes: RfNode<RfNodeData>[];
  setNodes: (
    nodes: RfNode<RfNodeData>[] | ((nodes: RfNode<RfNodeData>[]) => RfNode<RfNodeData>[]),
  ) => void;
}) {
  useEffect(() => {
    setNodes((prev) =>
      prev.map((n) => ({ ...n, selected: n.id === selectedNodeId })),
    );
    // 仅当 selectedNodeId 变化时同步
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedNodeId]);
  // 避免未使用变量警告
  void nodes;
  return null;
}

// 简单 key：节点 id 拼接，用于检测"工作流换了一份"
function buildKey(workflow: WorkflowDef): string {
  return `${workflow.id}|${workflow.nodes.map((n) => n.instance_id).join(',')}`;
}
