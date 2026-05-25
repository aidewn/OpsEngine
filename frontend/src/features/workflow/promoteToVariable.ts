// 把某个输出数据端口提升为图变量（参考 UE Blueprint）
// 流程：
//   1. 在 graph.variables 里追加一个同类型变量（名字基于端口 label，避免重名）
//   2. 创建一个 var_set 节点，位置在源节点右侧
//   3. 增加一条边：源端口 → var_set.value

import type {
  EdgeConfig,
  NodeInstance,
  VariableDef,
} from '@/types/workflow';
import type { PortType } from '@/types/nodeType';
import type { GraphDef } from './canvasMapping';
import { newUUID } from '@/lib/uuid';

// 提升结果
export interface PromoteResult<T extends GraphDef> {
  graph: T;
  variableName: string;
  newNodeId: string;
}

// 不能直接提升为变量的端口类型（只支持 VarTypeOptions 里的类型）
const NON_PROMOTABLE: PortType[] = ['Exec', 'Dynamic'];

// promoteToVariable 把指定输出端口提升为变量并连接到新建 var_set
//
// graph: 当前图（WorkflowDef 或 AssembleDef）
// sourceNodeId: 源节点 instance_id
// sourcePortId: 源端口 ID
// portType: 已解析过 Dynamic 的真实端口类型
// portLabel: 端口标签，用于生成默认变量名
// sourcePosition: 源节点位置，用于定位新建节点
export function promoteToVariable<T extends GraphDef>(
  graph: T,
  sourceNodeId: string,
  sourcePortId: string,
  portType: PortType,
  portLabel: string,
  sourcePosition: { x: number; y: number },
): PromoteResult<T> | null {
  if (NON_PROMOTABLE.includes(portType)) return null;

  const existingVars = graph.variables ?? [];
  const variableName = uniqueVariableName(
    sanitizeName(portLabel || sourcePortId || 'var'),
    existingVars,
  );

  const newVar: VariableDef = {
    name: variableName,
    var_type: portType,
    default: null,
  };

  const setNodeId = newUUID();
  const setNode: NodeInstance = {
    instance_id: setNodeId,
    type_id: 'var_set',
    config: {
      var_name: variableName,
      var_type: portType,
    },
    position: {
      x: sourcePosition.x + 280,
      y: sourcePosition.y + 40,
    },
  };

  const edge: EdgeConfig = {
    from: { node: sourceNodeId, port: sourcePortId },
    to: { node: setNodeId, port: 'value' },
  };

  return {
    graph: {
      ...graph,
      variables: [...existingVars, newVar],
      nodes: [...graph.nodes, setNode],
      edges: [...graph.edges, edge],
    } as T,
    variableName,
    newNodeId: setNodeId,
  };
}

// sanitizeName 简化端口 label：去空格、保留字母数字下划线
function sanitizeName(raw: string): string {
  const cleaned = raw.replace(/[^A-Za-z0-9_]+/g, '_').replace(/^_+|_+$/g, '');
  return cleaned || 'var';
}

// uniqueVariableName 基于 base 生成不与已有变量重名的名字
// 不重名直接返回 base，否则 base_2 / base_3 …
function uniqueVariableName(base: string, existing: VariableDef[]): string {
  const taken = new Set(existing.map((v) => v.name));
  if (!taken.has(base)) return base;
  for (let i = 2; i < 1000; i++) {
    const candidate = `${base}_${i}`;
    if (!taken.has(candidate)) return candidate;
  }
  return `${base}_${Date.now()}`;
}
