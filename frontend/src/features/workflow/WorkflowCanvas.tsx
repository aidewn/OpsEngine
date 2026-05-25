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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
import { buildDefaultConfig, resolvePortType } from '@/types/nodeType';
import { newUUID } from '@/lib/uuid';
import { readDragPayload } from './dragNode';
import { effectiveBranchCount } from './cleanupParallel';
import {
  PortContextMenu,
  type PortContextMenuState,
} from './PortContextMenu';
import { promoteToVariable } from './promoteToVariable';

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
  onGraphChange?: (graph: T) => void;
  // 拖线到空白处时触发，通知父组件弹出节点创建框
  onPendingConnection?: (pending: PendingConnection) => void;
  // 节点双击：用于集合调用节点跳转到集合编辑页
  onNodeDoubleClick?: (typeId: string) => void;
  // 选中节点集合变化（用于框选复制粘贴）
  onSelectedNodesChange?: (selected: Set<string>) => void;
  // 只读模式：禁用拖动节点、连线、删除、拖入新节点（执行详情页使用）
  readOnly?: boolean;
}

export function WorkflowCanvas<T extends GraphDef>({
  graph,
  selectedNodeId,
  onSelectNode,
  onGraphChange,
  onPendingConnection,
  onNodeDoubleClick,
  onSelectedNodesChange,
  readOnly = false,
}: CanvasProps<T>) {
  const { data: nodeTypes } = useNodeTypes();
  const rf = useReactFlow();

  // 监听 Shift 键：按下时切换为框选模式（左键拖空白 = 框选）
  const [isShiftHeld, setIsShiftHeld] = useState(false);
  useEffect(() => {
    if (readOnly) return;
    const down = (e: KeyboardEvent) => {
      if (e.key === 'Shift') setIsShiftHeld(true);
    };
    const up = (e: KeyboardEvent) => {
      if (e.key === 'Shift') setIsShiftHeld(false);
    };
    window.addEventListener('keydown', down);
    window.addEventListener('keyup', up);
    return () => {
      window.removeEventListener('keydown', down);
      window.removeEventListener('keyup', up);
    };
  }, [readOnly]);

  const initial = graphToRf(graph);
  const [nodes, setNodes, onNodesChange] = useNodesState<RfNode<RfNodeData>>(
    initial.nodes,
  );
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges);

  // 端口右键菜单状态：null = 不显示
  const [portMenu, setPortMenu] = useState<PortContextMenuState | null>(null);

  // 推送 RF 选中节点集合给父组件（用于复制粘贴）
  // 关键：避免每次 nodes 变化（即使内容相同）都创建新 Set 推送
  // 用排序后的 id 字符串作为 key 比较内容，只在真正变化时通知
  const selectedIdsKey = useMemo(
    () =>
      nodes
        .filter((n) => n.selected)
        .map((n) => n.id)
        .sort()
        .join(','),
    [nodes],
  );
  const lastSelectedKey = useRef<string | null>(null);
  useEffect(() => {
    if (!onSelectedNodesChange) return;
    if (selectedIdsKey === lastSelectedKey.current) return;
    lastSelectedKey.current = selectedIdsKey;
    const ids = selectedIdsKey ? selectedIdsKey.split(',') : [];
    onSelectedNodesChange(new Set(ids));
  }, [selectedIdsKey, onSelectedNodesChange]);

  // 最新 graph 引用，供回调中使用
  const graphRef = useRef(graph);
  graphRef.current = graph;

  // 外部 graph 变化时重置画布（结构性变化：节点新增/删除、连线变化）
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

  // 节点 data（即 NodeInstance，含 config）同步：保留 RF 内部状态（selected / dragging / position）
  // 只在某节点的 NodeInstance 引用变化时更新对应 RF node 的 data
  // 用途：右侧 ConfigForm 改 parallel.branch_count 后，画布端口数量能立刻刷新；
  // 因为只改 config 时 buildKey 不变，单靠上面 key-based reset 是检测不到的
  useEffect(() => {
    setNodes((prev) => {
      let changed = false;
      const next = prev.map((n) => {
        const fresh = graph.nodes.find((gn) => gn.instance_id === n.id);
        if (!fresh || n.data === fresh) return n;
        changed = true;
        return { ...n, data: fresh };
      });
      return changed ? next : prev;
    });
  }, [graph.nodes, setNodes]);

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
      if (srcType !== tgtType) return false;

      // parallel 节点的 exec_out_<i> 限制：超出 branch_count 的端口拒绝连接
      if (sourceNode.type_id === 'parallel') {
        const m = /^exec_out_(\d+)$/.exec(connection.sourceHandle ?? '');
        if (m) {
          const idx = parseInt(m[1] ?? '0', 10);
          if (idx > effectiveBranchCount(sourceNode.config)) return false;
        }
      }

      // exec_out 单出 / input 单入的「冲突自动断开旧线」逻辑放在 onConnect 中处理
      // 这里允许通过（用户拖动时不显示拒绝色），保持静默体验
      return true;
    },
    [nodeTypes],
  );

  // ── 连线完成 ──────────────────────────────────────────
  // 处理冲突边自动断开：
  //   1. 如果是 exec 输出端口（前缀 "exec_"），删除其旧的出边（exec_out 单出）
  //   2. 不管什么类型 input 端口，删除其旧的入边（input 单入）
  // 然后追加新边
  const onConnect = useCallback(
    (connection: Connection) => {
      if (!connection.sourceHandle || !connection.targetHandle) return;

      const newEdge: EdgeConfig = {
        from: { node: connection.source, port: connection.sourceHandle },
        to: { node: connection.target, port: connection.targetHandle },
      };

      const g = graphRef.current;
      const isExecSource = connection.sourceHandle.startsWith('exec_');

      // 1. 删除 source 上的旧 exec 出边（仅 exec_out 单出）
      // 2. 删除 target 上的旧入边（所有 input 单入）
      const cleaned = g.edges.filter((e) => {
        if (
          isExecSource &&
          e.from.node === connection.source &&
          e.from.port === connection.sourceHandle
        ) {
          return false;
        }
        if (
          e.to.node === connection.target &&
          e.to.port === connection.targetHandle
        ) {
          return false;
        }
        return true;
      });

      // 防御性去重（理论上 cleaned 不会包含相同边）
      const exists = cleaned.some(
        (e) =>
          e.from.node === newEdge.from.node &&
          e.from.port === newEdge.from.port &&
          e.to.node === newEdge.to.node &&
          e.to.port === newEdge.to.port,
      );
      if (exists) return;

      const finalEdges = [...cleaned, newEdge];

      // 更新画布：RF edges 整体重置（旧边可能被删）
      setEdges(() => finalEdges.map(toRfEdge));

      // 持久化
      onGraphChange?.({ ...g, edges: finalEdges } as T);
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

      // 取松手时的鼠标/触控屏幕坐标
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

      // 屏幕坐标 → 画布 flow 坐标（受 pan / zoom 影响），与 node.position 语义一致
      const flowPos = rf.screenToFlowPosition({ x: clientX, y: clientY });

      onPendingConnection?.({
        sourcePortType: realType,
        sourceDirection: isOutput ? 'output' : 'input',
        nodeId: fromNodeId,
        portId: fromHandleId,
        position: flowPos,
      });
    },
    [nodeTypes, onPendingConnection, rf],
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

      // 用 ConfigSchema 默认值 + payload.config 兜底未指定字段
      const typeDef = nodeTypes?.find((t) => t.type_id === payload.type_id);
      const config = typeDef
        ? buildDefaultConfig(typeDef, payload.config)
        : payload.config;

      const g = graphRef.current;
      const node: NodeInstance = {
        instance_id: newUUID(),
        type_id: payload.type_id,
        config,
        position,
      };
      onGraphChange?.({ ...g, nodes: [...g.nodes, node] } as T);
    },
    [rf, nodeTypes, onGraphChange],
  );

  // ── 端口右键：识别 handle DOM，定位端口与节点，弹出 PortContextMenu ──
  // 仅对数据 output 有效；exec / 输入 / 未注册类型直接放过
  const onContextMenuCapture = useCallback(
    (event: React.MouseEvent) => {
      if (readOnly || !nodeTypes) return;
      const target = event.target as HTMLElement | null;
      if (!target) return;
      const handleEl = target.closest('.react-flow__handle') as HTMLElement | null;
      if (!handleEl) return;

      const isSource = handleEl.classList.contains('source');
      if (!isSource) return;

      const portId =
        handleEl.getAttribute('data-handleid') ?? handleEl.id ?? '';
      if (!portId) return;

      const nodeEl = handleEl.closest('.react-flow__node') as HTMLElement | null;
      const nodeId = nodeEl?.getAttribute('data-id') ?? '';
      if (!nodeId) return;

      const g = graphRef.current;
      const node = g.nodes.find((n) => n.instance_id === nodeId);
      const def = node && nodeTypes.find((t) => t.type_id === node.type_id);
      const portDef = def?.output_ports.find((p) => p.id === portId);
      if (!node || !def || !portDef) return;

      const realType = resolvePortType(portDef, node.config);
      if (realType === 'Exec') return;

      event.preventDefault();
      setPortMenu({
        x: event.clientX,
        y: event.clientY,
        nodeId,
        portId,
        portLabel: portDef.label || portDef.id,
        portType: realType,
      });
    },
    [readOnly, nodeTypes],
  );

  // 执行端口的 Promote 动作：调用纯函数生成新 graph 并保存
  const onPromote = useCallback(() => {
    if (!portMenu || !onGraphChange) return;
    const g = graphRef.current;
    const sourceNode = g.nodes.find((n) => n.instance_id === portMenu.nodeId);
    if (!sourceNode) return;
    const result = promoteToVariable(
      g,
      portMenu.nodeId,
      portMenu.portId,
      portMenu.portType as never,
      portMenu.portLabel,
      sourceNode.position,
    );
    if (!result) return;
    onGraphChange(result.graph as T);
  }, [portMenu, onGraphChange]);

  // ── 删除节点 / 连线（统一回调，避免 onNodesDelete + onEdgesDelete 竞争同一 graphRef） ──
  // RF v12 删除一个节点时会同时触发节点 + 被波及连线，旧实现分两个 callback 各自读 graphRef.current
  // 第二个回调读到的还是旧 graph → 覆盖第一个的更新 → 用户感受为需要再按一次 Delete
  const onDelete = useCallback(
    ({ nodes: delNodes, edges: delEdges }: { nodes: RfNode[]; edges: RfEdge[] }) => {
      const delNodeIds = new Set(delNodes.map((n) => n.id));
      const delEdgeIds = new Set(delEdges.map((e) => e.id));
      const g = graphRef.current;
      onGraphChange?.({
        ...g,
        nodes: g.nodes.filter((n) => !delNodeIds.has(n.instance_id)),
        edges: g.edges.filter((e) => {
          // 节点被删 → 连带连线全部移除
          if (delNodeIds.has(e.from.node) || delNodeIds.has(e.to.node)) return false;
          // 用户直接选中的连线
          const rfId = `${e.from.node}:${e.from.port}->${e.to.node}:${e.to.port}`;
          return !delEdgeIds.has(rfId);
        }),
      } as T);
    },
    [onGraphChange],
  );

  return (
    <div
      className="relative h-full w-full"
      onContextMenuCapture={readOnly ? undefined : onContextMenuCapture}
    >
      <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={readOnly ? undefined : onNodesChange}
      onEdgesChange={readOnly ? undefined : onEdgesChange}
      onConnect={readOnly ? undefined : onConnect}
      onConnectEnd={readOnly ? undefined : onConnectEnd}
      onDelete={readOnly ? undefined : onDelete}
      isValidConnection={readOnly ? () => false : isValidConnection}
      onDrop={readOnly ? undefined : onDrop}
      onDragOver={readOnly ? undefined : onDragOver}
      onNodeClick={(_, n) => onSelectNode(n.id)}
      onNodeDoubleClick={(_, n) => {
        const node = graphRef.current.nodes.find(
          (gn) => gn.instance_id === n.id,
        );
        if (node && onNodeDoubleClick) onNodeDoubleClick(node.type_id);
      }}
      onPaneClick={() => onSelectNode(null)}
      onNodeDragStop={
        readOnly || !onGraphChange
          ? undefined
          : () => onGraphChange(mergeNodePositions(graphRef.current, nodes))
      }
      nodesDraggable={!readOnly}
      nodesConnectable={!readOnly}
      elementsSelectable
      nodeTypes={nodeTypeMap}
      deleteKeyCode={readOnly ? null : ['Backspace', 'Delete']}
      // Shift 按下时启用框选（左键拖空白 = 框选），其余时间平移
      selectionOnDrag={!readOnly && isShiftHeld}
      panOnDrag={readOnly || !isShiftHeld}
      selectionMode={'partial' as never}
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
      {portMenu && (
        <PortContextMenu
          state={portMenu}
          onClose={() => setPortMenu(null)}
          onPromote={onPromote}
        />
      )}
    </div>
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
