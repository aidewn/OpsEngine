// 工作流画布：节点拖动、连线、删除、拖线到空白创建节点

import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  type Node as RfNode,
  type Connection,
  type Edge as RfEdge,
  type OnConnectEnd,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { useCallback, useEffect, useRef } from 'react';
import type { WorkflowDef, EdgeConfig, NodeInstance } from '@/types/workflow';
import type { PortType } from '@/types/nodeType';
import { useNodeTypes } from '@/api/nodeTypes';
import { nodeTypeMap } from './nodes/nodeTypeMap';
import {
  mergeNodePositions,
  type RfNodeData,
  workflowToRf,
  toRfEdge,
} from './canvasMapping';
import { resolvePortType } from '@/types/nodeType';
import { newUUID } from '@/lib/uuid';

// 拖线到空白时记录的上下文信息
export interface PendingConnection {
  // 拖出端口的信息
  sourcePortType: PortType;
  sourceDirection: 'output' | 'input';
  // 源节点 + 端口 ID
  nodeId: string;
  portId: string;
  // 松手位置（画布坐标）
  position: { x: number; y: number };
}

interface WorkflowCanvasProps {
  workflow: WorkflowDef;
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
  onWorkflowChange: (workflow: WorkflowDef) => void;
  // 拖线到空白处时触发，通知父组件弹出节点创建框
  onPendingConnection: (pending: PendingConnection) => void;
}

export function WorkflowCanvas({
  workflow,
  selectedNodeId,
  onSelectNode,
  onWorkflowChange,
  onPendingConnection,
}: WorkflowCanvasProps) {
  const { data: nodeTypes } = useNodeTypes();
  const initial = workflowToRf(workflow);
  const [nodes, setNodes, onNodesChange] = useNodesState<RfNode<RfNodeData>>(
    initial.nodes,
  );
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges);

  // 最新 workflow 引用，供回调中使用
  const workflowRef = useRef(workflow);
  workflowRef.current = workflow;

  // 外部 workflow 变化时重置画布
  const lastWorkflowKey = useRef<string>(buildKey(workflow));
  useEffect(() => {
    const key = buildKey(workflow);
    if (key !== lastWorkflowKey.current) {
      const next = workflowToRf(workflow);
      setNodes(next.nodes);
      setEdges(next.edges);
      lastWorkflowKey.current = key;
    }
  }, [workflow, setNodes, setEdges]);

  // ── 连线校验 ──────────────────────────────────────────
  const isValidConnection = useCallback(
    (connection: Connection | RfEdge): boolean => {
      if (!nodeTypes || !connection.source || !connection.target) return false;
      if (connection.source === connection.target) return false;

      const wf = workflowRef.current;
      const sourceNode = wf.nodes.find(
        (n) => n.instance_id === connection.source,
      );
      const targetNode = wf.nodes.find(
        (n) => n.instance_id === connection.target,
      );
      if (!sourceNode || !targetNode) return false;

      const sourceDef = nodeTypes.find(
        (t) => t.type_id === sourceNode.type_id,
      );
      const targetDef = nodeTypes.find(
        (t) => t.type_id === targetNode.type_id,
      );
      if (!sourceDef || !targetDef) return true; // 未注册类型，允许连接

      const sourcePort = sourceDef.output_ports.find(
        (p) => p.id === connection.sourceHandle,
      );
      const targetPort = targetDef.input_ports.find(
        (p) => p.id === connection.targetHandle,
      );
      if (!sourcePort || !targetPort) return false;

      // 解析动态端口类型后比较
      const srcType = resolvePortType(sourcePort, sourceNode.config);
      const tgtType = resolvePortType(targetPort, targetNode.config);
      return srcType === tgtType;
    },
    [nodeTypes],
  );

  // ── 连线完成 ──────────────────────────────────────────
  const onConnect = useCallback(
    (connection: Connection) => {
      if (!connection.sourceHandle || !connection.targetHandle) return;

      const newEdge: EdgeConfig = {
        from: { node: connection.source, port: connection.sourceHandle },
        to: { node: connection.target, port: connection.targetHandle },
      };

      // 检查是否已存在相同连线
      const wf = workflowRef.current;
      const exists = wf.edges.some(
        (e) =>
          e.from.node === newEdge.from.node &&
          e.from.port === newEdge.from.port &&
          e.to.node === newEdge.to.node &&
          e.to.port === newEdge.to.port,
      );
      if (exists) return;

      // 更新画布
      const rfEdge = toRfEdge(newEdge);
      setEdges((prev) => [...prev, rfEdge]);

      // 持久化
      const updated: WorkflowDef = {
        ...wf,
        edges: [...wf.edges, newEdge],
      };
      onWorkflowChange(updated);
    },
    [setEdges, onWorkflowChange],
  );

  // ── 拖线到空白处 ──────────────────────────────────────
  const onConnectEnd: OnConnectEnd = useCallback(
    (event, connectionState) => {
      // 只在松手时没连接到任何端口时触发
      if (connectionState.isValid) return;

      const fromNodeId = connectionState.fromNode?.id;
      const fromHandleId = connectionState.fromHandle?.id;
      const fromHandleType = connectionState.fromHandle?.type; // 'source' | 'target'
      if (!fromNodeId || !fromHandleId || !fromHandleType || !nodeTypes) return;

      const wf = workflowRef.current;
      const fromNode = wf.nodes.find((n) => n.instance_id === fromNodeId);
      if (!fromNode) return;

      const fromDef = nodeTypes.find((t) => t.type_id === fromNode.type_id);
      if (!fromDef) return;

      // 确定拖出端口的类型
      const isOutput = fromHandleType === 'source';
      const portList = isOutput ? fromDef.output_ports : fromDef.input_ports;
      const portDef = portList.find((p) => p.id === fromHandleId);
      if (!portDef) return;

      const realType = resolvePortType(portDef, fromNode.config);

      // 计算松手位置的画布坐标
      const reactFlowBounds = (
        event.target as HTMLElement
      ).closest('.react-flow')?.getBoundingClientRect();
      if (!reactFlowBounds) return;

      // 区分 MouseEvent 和 TouchEvent
      let clientX: number;
      let clientY: number;
      if ('changedTouches' in event) {
        const touch = (event as TouchEvent).changedTouches[0];
        if (!touch) return;
        clientX = touch.clientX;
        clientY = touch.clientY;
      } else {
        clientX = (event as MouseEvent).clientX;
        clientY = (event as MouseEvent).clientY;
      }

      onPendingConnection({
        sourcePortType: realType,
        sourceDirection: isOutput ? 'output' : 'input',
        nodeId: fromNodeId,
        portId: fromHandleId,
        position: {
          x: clientX - reactFlowBounds.left,
          y: clientY - reactFlowBounds.top,
        },
      });
    },
    [nodeTypes, onPendingConnection],
  );

  // ── 删除节点 ──────────────────────────────────────────
  const onNodesDelete = useCallback(
    (deleted: RfNode<RfNodeData>[]) => {
      const deletedIds = new Set(deleted.map((n) => n.id));
      const wf = workflowRef.current;

      const updated: WorkflowDef = {
        ...wf,
        nodes: wf.nodes.filter((n) => !deletedIds.has(n.instance_id)),
        edges: wf.edges.filter(
          (e) => !deletedIds.has(e.from.node) && !deletedIds.has(e.to.node),
        ),
      };
      onWorkflowChange(updated);
    },
    [onWorkflowChange],
  );

  // ── 删除连线 ──────────────────────────────────────────
  const onEdgesDelete = useCallback(
    (deleted: RfEdge[]) => {
      const deletedIds = new Set(deleted.map((e) => e.id));
      const wf = workflowRef.current;

      const updated: WorkflowDef = {
        ...wf,
        edges: wf.edges.filter((e) => {
          const rfId = `${e.from.node}:${e.from.port}->${e.to.node}:${e.to.port}`;
          return !deletedIds.has(rfId);
        }),
      };
      onWorkflowChange(updated);
    },
    [onWorkflowChange],
  );

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      onConnectEnd={onConnectEnd}
      onNodesDelete={onNodesDelete}
      onEdgesDelete={onEdgesDelete}
      isValidConnection={isValidConnection}
      onNodeClick={(_, n) => onSelectNode(n.id)}
      onPaneClick={() => onSelectNode(null)}
      onNodeDragStop={() => {
        onWorkflowChange(mergeNodePositions(workflowRef.current, nodes));
      }}
      nodeTypes={nodeTypeMap}
      deleteKeyCode={['Backspace', 'Delete']}
      selectionOnDrag={false}
      fitView
      proOptions={{ hideAttribution: true }}
    >
      <Background gap={20} size={1} />
      <Controls />
      <MiniMap pannable zoomable />
      <SyncSelection
        selectedNodeId={selectedNodeId}
        nodes={nodes}
        setNodes={setNodes}
      />
    </ReactFlow>
  );
}

// ── 辅助：添加节点（供父组件调用） ─────────────────────────
// 创建节点后同步更新画布和 workflow
export function addNodeToWorkflow(
  workflow: WorkflowDef,
  typeId: string,
  position: { x: number; y: number },
): { workflow: WorkflowDef; nodeId: string } {
  const node: NodeInstance = {
    instance_id: newUUID(),
    type_id: typeId,
    config: {},
    position,
  };
  return {
    workflow: {
      ...workflow,
      nodes: [...workflow.nodes, node],
    },
    nodeId: node.instance_id,
  };
}

// ── 辅助：添加节点并自动连线 ─────────────────────────────
export function addNodeWithEdge(
  workflow: WorkflowDef,
  typeId: string,
  position: { x: number; y: number },
  pending: PendingConnection,
  targetPortId: string,
): { workflow: WorkflowDef; nodeId: string } {
  const nodeId = newUUID();
  const node: NodeInstance = {
    instance_id: nodeId,
    type_id: typeId,
    config: {},
    position,
  };

  // 根据拖线方向确定 edge 的 from/to
  const edge: EdgeConfig =
    pending.sourceDirection === 'output'
      ? {
          from: { node: pending.nodeId, port: pending.portId },
          to: { node: nodeId, port: targetPortId },
        }
      : {
          from: { node: nodeId, port: targetPortId },
          to: { node: pending.nodeId, port: pending.portId },
        };

  return {
    workflow: {
      ...workflow,
      nodes: [...workflow.nodes, node],
      edges: [...workflow.edges, edge],
    },
    nodeId,
  };
}

// ── 内部组件 ─────────────────────────────────────────────

// 将父组件传入的 selectedNodeId 同步到 RF 节点的 selected 标志
function SyncSelection({
  selectedNodeId,
  nodes,
  setNodes,
}: {
  selectedNodeId: string | null;
  nodes: RfNode<RfNodeData>[];
  setNodes: (
    updater:
      | RfNode<RfNodeData>[]
      | ((prev: RfNode<RfNodeData>[]) => RfNode<RfNodeData>[]),
  ) => void;
}) {
  useEffect(() => {
    setNodes((prev) =>
      prev.map((n) => ({ ...n, selected: n.id === selectedNodeId })),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedNodeId]);
  void nodes;
  return null;
}

// 简单 key：节点/边 id 拼接，用于检测工作流数据变化
function buildKey(workflow: WorkflowDef): string {
  const nodeKeys = workflow.nodes.map((n) => n.instance_id).join(',');
  const edgeKeys = workflow.edges
    .map((e) => `${e.from.node}:${e.from.port}-${e.to.node}:${e.to.port}`)
    .join(',');
  return `${workflow.id}|${nodeKeys}|${edgeKeys}`;
}
