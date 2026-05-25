// 解析画布上某节点某端口的类型与方向
// 除 NodeTypeDef 静态端口外，还覆盖 assemble_start / assemble_end 的动态 param_* / return_*

import type { ParamDef } from '@/types/assemble';
import type { NodeInstance } from '@/types/workflow';
import {
  ASSEMBLE_END,
  ASSEMBLE_START,
  type NodeTypeDef,
  type PortType,
  resolvePortType,
} from '@/types/nodeType';
import type { GraphDef } from './canvasMapping';

export interface ResolvedGraphPort {
  id: string;
  label: string;
  portType: PortType;
  direction: 'input' | 'output';
}

function fromParamList(
  portId: string,
  prefix: string,
  items: ParamDef[] | undefined,
  direction: 'input' | 'output',
): ResolvedGraphPort | null {
  if (!portId.startsWith(prefix)) return null;
  const name = portId.slice(prefix.length);
  const item = items?.find((p) => p.name === name);
  if (!item) return null;
  return {
    id: portId,
    label: item.name,
    portType: item.var_type as PortType,
    direction,
  };
}

/** 查找节点端口；未找到返回 null */
export function resolveGraphPort(
  graph: GraphDef,
  node: NodeInstance,
  portId: string,
  nodeTypes: NodeTypeDef[] | undefined,
): ResolvedGraphPort | null {
  const def = nodeTypes?.find((t) => t.type_id === node.type_id);
  if (def) {
    const out = def.output_ports.find((p) => p.id === portId);
    if (out) {
      return {
        id: portId,
        label: out.label || out.id,
        portType: resolvePortType(out, node.config),
        direction: 'output',
      };
    }
    const inp = def.input_ports.find((p) => p.id === portId);
    if (inp) {
      return {
        id: portId,
        label: inp.label || inp.id,
        portType: resolvePortType(inp, node.config),
        direction: 'input',
      };
    }
  }

  if (node.type_id === ASSEMBLE_START) {
    return fromParamList(portId, 'param_', graph.params, 'output');
  }
  if (node.type_id === ASSEMBLE_END) {
    return fromParamList(portId, 'return_', graph.returns, 'input');
  }

  return null;
}
