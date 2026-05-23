// 通用画布组件：节点拖动、连线、删除、拖线到空白创建节点
// 同时被工作流和集合编辑页复用

import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  useReactFlow,
  type Node as RfNode,
  type Connection,
  type Edge as RfEdge,
  type OnConnectEnd,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { useCallback, useEffect, useRef } from 'react';
import type { EdgeConfig, NodeInstance } from '@/types/workflow';
import type { PortType } from '@/types/nodeType';
import { useNodeTypes } from '@/api/nodeTypes';
import { nodeTypeMap } from './nodes/nodeTypeMap';
import {
  mergeNodePositions,
  type GraphDef,
  type RfNodeData,
  graphToRf,
  toRfEdge,
} from './canvasMapping';
import { resolvePortType } from '@/types/nodeType';
import { newUUID } from '@/lib/uuid';
import { readDragPayload } from './dragNode';

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

// 画布接受任何含 nodes + edges 的结构
interface CanvasProps<T extends GraphDef> {
  graph: T;
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
  onGraphChange: (graph: T) => void;
  // 拖线到空白处时触发，通知父组件弹出节点创建框
  onPendingConnection: (pending: PendingConnection) => void;
  // 节点双击：用于集合调用节点跳转到集合编辑页
  onNodeDoubleClick?: (typeId: string) => void;
}

export function WorkflowCanvas<T extends GraphDef>({
  graph,
  selectedNodeId,
  onSelectNode,
  onGraphChange,
  onPendingConnection,
  onNodeDoubleClick,
}: CanvasProps<T>) {
  const { data: nodeTypes } = useNodeTypes();
  const rf = useReactFlow();
  const initial = graphToRf(graph);
  const [nodes, setNodes, onNodesChange] = useNodesState<RfNode<RfNodeData>>(
    initial.nodes,
  );
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges);

  // 最新 graph 引用，供回调中使用
  const graphRef = useRef(graph);
  graphRef.current = graph;

  // 外部 graph 变化时重置画布
  const lastGraphKey = useRef<string>(buildKey(graph));
  useEffect(() => {
    const key = buildKey(graph);
    if (key !== lastGraphKey.current) {
      const next = graphToRf(graph);
      setNodes(next.nodes);
      setEdges(next.edges);
      lastGraphKey.current = key;
    }
  }, [graph, setNodes, setEdges]);

  // ── 连线校验 ──────────────────────────────────────────
  const isValidConnection = useCallback(
    (connection: Connection | RfEdge): boolean => {
      if (!nodeTypes || !connection.source || !connection.target) return false;
      if (connection.source === connection.target) return false;

      const g = graphRef.current;
      const sourceNode = g.nodes.find(
        (n) => n.instance_id === connection.source,
      );
      const targetNode = g.nodes.find(
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
      const g = graphRef.current;
      const exists = g.edges.some(
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
      onGraphChange({ ...g, edges: [...g.edges, newEdge] } as T);
    },
    [setEdges, onGraphChange],
  );

  // ── 拖线到空白处 ──────────────────────────────────────
  const onConnectEnd: OnConnectEnd = useCallback(
    (event, connectionState) => {
      if (connectionState.isValid) return;

      const fromNodeId = connectionState.fromNode?.id;
      const fromHandleId = connectionState.fromHandle?.id;
      const fromHandleType = connectionState.fromHandle?.type;
      if (!fromNodeId || !fromHandleId || !fromHandleType || !nodeTypes) return;

      const g = graphRef.current;
      const fromNode = g.nodes.find((n) => n.instance_id === fromNodeId);
      if (!fromNode) return;

      const fromDef = nodeTypes.find((t) => t.type_id === fromNode.type_id);
      if (!fromDef) return;

      const isOutput = fromHandleType === 'source';
      const portList = isOutput ? fromDef.output_ports : fromDef.input_ports;
      const portDef = portList.find((p) => p.id === fromHandleId);
      if (!portDef) return;

      const realType = resolvePortType(portDef, fromNode.config);

      const reactFlowBounds = (
        event.target as HTMLElement
      ).closest('.react-flow')?.getBoundingClientRect();
      if (!reactFlowBounds) return;

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
      const g = graphRef.current;

      onGraphChange({
        ...g,
        nodes: g.nodes.filter((n) => !deletedIds.has(n.instance_id)),
        edges: g.edges.filter(
          (e) => !deletedIds.has(e.from.node) && !deletedIds.has(e.to.node),
        ),
      } as T);
    },
    [onGraphChange],
  );

  // ── 拖到画布：从左侧栏列表项拖入 ────────────────────────
  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'copy';
  }, []);

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const payload = readDragPayload(event.nativeEvent);
      if (!payload) return;

      // 屏幕坐标 → 画布坐标
      const position = rf.screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });

      const g = graphRef.current;
      const node: NodeInstance = {
        instance_id: newUUID(),
        type_id: payload.type_id,
        config: payload.config,
        position,
      };
      onGraphChange({ ...g, nodes: [...g.nodes, node] } as T);
    },
    [rf, onGraphChange],
  );

  // ── 删除连线 ──────────────────────────────────────────
  const onEdgesDelete = useCallback(
    (deleted: RfEdge[]) => {
      const deletedIds = new Set(deleted.map((e) => e.id));
      const g = graphRef.current;

      onGraphChange({
        ...g,
        edges: g.edges.filter((e) => {
          const rfId = `${e.from.node}:${e.from.port}->${e.to.node}:${e.to.port}`;
          return !deletedIds.has(rfId);
        }),
      } as T);
    },
    [onGraphChange],
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
      onDrop={onDrop}
      onDragOver={onDragOver}
      onNodeClick={(_, n) => onSelectNode(n.id)}
      onNodeDoubleClick={(_, n) => {
        const node = graphRef.current.nodes.find(
          (gn) => gn.instance_id === n.id,
        );
        if (node && onNodeDoubleClick) onNodeDoubleClick(node.type_id);
      }}
      onPaneClick={() => onSelectNode(null)}
      onNodeDragStop={() => {
        onGraphChange(mergeNodePositions(graphRef.current, nodes));
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

// ── 辅助：向图中添加节点 ────────────────────────────────────
export function addNodeToGraph<T extends GraphDef>(
  graph: T,
  typeId: string,
  position: { x: number; y: number },
  config: Record<string, unknown> = {},
): { graph: T; nodeId: string } {
  const node: NodeInstance = {
    instance_id: newUUID(),
    type_id: typeId,
    config,
    position,
  };
  return {
    graph: { ...graph, nodes: [...graph.nodes, node] } as T,
    nodeId: node.instance_id,
  };
}

// ── 辅助：添加节点并自动连线 ─────────────────────────────
export function addNodeWithEdge<T extends GraphDef>(
  graph: T,
  typeId: string,
  position: { x: number; y: number },
  pending: PendingConnection,
  targetPortId: string,
  config: Record<string, unknown> = {},
): { graph: T; nodeId: string } {
  const nodeId = newUUID();
  const node: NodeInstance = {
    instance_id: nodeId,
    type_id: typeId,
    config,
    position,
  };

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
    graph: {
      ...graph,
      nodes: [...graph.nodes, node],
      edges: [...graph.edges, edge],
    } as T,
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

// 简单 key：节点/边 id 拼接，用于检测数据变化
function buildKey(graph: GraphDef): string {
  const nodeKeys = graph.nodes.map((n) => n.instance_id).join(',');
  const edgeKeys = graph.edges
    .map((e) => `${e.from.node}:${e.from.port}-${e.to.node}:${e.to.port}`)
    .join(',');
  return `${nodeKeys}|${edgeKeys}`;
}
