// 在后端 WorkflowDef 与 React Flow 的 Node/Edge 之间转换
// 单向同步：拖动节点改变 position 后调用 mergeNodePositions 写回 WorkflowDef

import { type Node as RfNode, type Edge as RfEdge } from '@xyflow/react';
import type {
  EdgeConfig,
  NodeInstance,
  WorkflowDef,
} from '@/types/workflow';
import { isSystemNodeType } from '@/types/nodeType';

// 内部 React Flow 节点 data 形状直接用 NodeInstance，便于节点组件取数
export type RfNodeData = NodeInstance;

// 后端 NodeInstance → RF Node
export function toRfNode(node: NodeInstance): RfNode<RfNodeData> {
  return {
    id: node.instance_id,
    type: isSystemNodeType(node.type_id) ? node.type_id : 'generic',
    position: node.position,
    data: node,
  };
}

// 后端 EdgeConfig → RF Edge
// edge id 由 from/to 组合派生，方便去重
export function toRfEdge(edge: EdgeConfig): RfEdge {
  return {
    id: `${edge.from.node}:${edge.from.port}->${edge.to.node}:${edge.to.port}`,
    source: edge.from.node,
    sourceHandle: edge.from.port,
    target: edge.to.node,
    targetHandle: edge.to.port,
  };
}

export function workflowToRf(workflow: WorkflowDef): {
  nodes: RfNode<RfNodeData>[];
  edges: RfEdge[];
} {
  return {
    nodes: workflow.nodes.map(toRfNode),
    edges: workflow.edges.map(toRfEdge),
  };
}

// 把 RF 节点的最新 position 合并回原 WorkflowDef
// 只更新 position，其他字段（config 等）保留原值
export function mergeNodePositions(
  workflow: WorkflowDef,
  rfNodes: RfNode<RfNodeData>[],
): WorkflowDef {
  const posMap = new Map(rfNodes.map((n) => [n.id, n.position]));
  return {
    ...workflow,
    nodes: workflow.nodes.map((n) => {
      const pos = posMap.get(n.instance_id);
      return pos ? { ...n, position: pos } : n;
    }),
  };
}
