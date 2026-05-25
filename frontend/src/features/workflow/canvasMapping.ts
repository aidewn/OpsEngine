// 在后端 WorkflowDef / AssembleDef 与 React Flow 的 Node/Edge 之间转换
// 单向同步：拖动节点改变 position 后调用 mergeNodePositions 写回

import { type Node as RfNode, type Edge as RfEdge } from '@xyflow/react';
import type { EdgeConfig, NodeInstance, VariableDef } from '@/types/workflow';
import {
  isSystemNodeType,
  isAssembleNodeType,
  isInternalNodeType,
} from '@/types/nodeType';

// 画布数据的通用接口（WorkflowDef 和 AssembleDef 都满足）
// variables 在两类图里都存在；声明为可选便于 GraphDef 适配执行详情等只读视图
export interface GraphDef {
  nodes: NodeInstance[];
  edges: EdgeConfig[];
  variables?: VariableDef[];
}

// React Flow 节点 data 直接用 NodeInstance（已改为 type，满足 Record 约束）
export type RfNodeData = NodeInstance;

// 后端 NodeInstance → RF Node
// 系统节点和集合内部节点走专用组件，其余走 generic
// 内部节点（system_*、assemble_*）标记 deletable=false，React Flow 会拒绝删除
export function toRfNode(node: NodeInstance): RfNode<RfNodeData> {
  let rfType = 'generic';
  if (isSystemNodeType(node.type_id) || isAssembleNodeType(node.type_id)) {
    rfType = node.type_id;
  }
  return {
    id: node.instance_id,
    type: rfType,
    position: node.position,
    data: node,
    deletable: !isInternalNodeType(node.type_id),
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

// 通用转换：任何含 nodes + edges 的结构 → RF 数据
export function graphToRf(graph: GraphDef): {
  nodes: RfNode<RfNodeData>[];
  edges: RfEdge[];
} {
  return {
    nodes: graph.nodes.map(toRfNode),
    edges: graph.edges.map(toRfEdge),
  };
}

// 把 RF 节点的最新 position 合并回原结构
// 只更新 position，其他字段（config 等）保留原值
// 使用泛型保留原始类型（WorkflowDef 或 AssembleDef）
export function mergeNodePositions<T extends GraphDef>(
  graph: T,
  rfNodes: RfNode<RfNodeData>[],
): T {
  const posMap = new Map(rfNodes.map((n) => [n.id, n.position]));
  return {
    ...graph,
    nodes: graph.nodes.map((n) => {
      const pos = posMap.get(n.instance_id);
      return pos ? { ...n, position: pos } : n;
    }),
  };
}
